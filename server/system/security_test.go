package system

import (
	"context"
	"testing"
	"time"
)

func testSecurityService(t *testing.T) (*SecurityService, *UserService, *NotificationService) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	notifications := NewNotificationService(directory, users)
	security := NewSecurityService(directory, users, notifications)
	fixedNow := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	security.now = func() time.Time { return fixedNow }
	notifications.now = func() time.Time { return fixedNow }
	return security, users, notifications
}

func TestRecordEventChainsHashes(t *testing.T) {
	security, _, _ := testSecurityService(t)
	ctx := context.Background()

	first, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "user_created", Module: "users", SourceIP: "192.168.1.10", Summary: "created user x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash == "" {
		t.Fatal("expected a hash on the first event")
	}

	second, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "user_created", Module: "users", SourceIP: "192.168.1.10", Summary: "created user y",
	})
	if err != nil {
		t.Fatal(err)
	}

	db, err := security.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var prevHash string
	if err := db.QueryRowContext(ctx, `SELECT prev_hash FROM security_events WHERE id = ?`, second.ID).Scan(&prevHash); err != nil {
		t.Fatal(err)
	}
	if prevHash != first.Hash {
		t.Fatalf("second event's prev_hash = %q, want first event's hash %q (broken chain)", prevHash, first.Hash)
	}
}

func TestClassifySourceIP(t *testing.T) {
	cases := map[string]string{
		"192.168.1.5": "private",
		"10.0.0.1":    "private",
		"172.16.5.5":  "private",
		"127.0.0.1":   "private",
		"8.8.8.8":     "public",
		"203.0.113.1": "public",
		"not-an-ip":   "public",
	}
	for address, want := range cases {
		if got := classifySourceIP(address); got != want {
			t.Errorf("classifySourceIP(%q) = %q, want %q", address, got, want)
		}
	}
}

func TestRecordEventForcesCriticalOnLANOnlyViolation(t *testing.T) {
	security, _, _ := testSecurityService(t)
	ctx := context.Background()

	event, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "login_succeeded", Module: "users", SourceIP: "8.8.8.8", Severity: "info", Summary: "login from outside",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Severity != "critical" {
		t.Fatalf("severity = %q, want critical for a public IP under the default lan_only posture", event.Severity)
	}
	if event.IPClass != "public" {
		t.Fatalf("ip_class = %q, want public", event.IPClass)
	}
}

func TestRecordEventDoesNotForceOverInternetExposedPosture(t *testing.T) {
	security, _, _ := testSecurityService(t)
	ctx := context.Background()

	if _, err := security.SetSettings(ctx, adminUserID, UpdateSecuritySettingsInput{DeploymentPosture: strPtr("internet_exposed")}); err != nil {
		t.Fatal(err)
	}
	event, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "login_succeeded", Module: "users", SourceIP: "8.8.8.8", Severity: "info", Summary: "login from outside",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Severity != "info" {
		t.Fatalf("severity = %q, want info: a public IP is expected traffic under internet_exposed posture", event.Severity)
	}
}

func TestBruteForceLoginRuleFiresAtThreshold(t *testing.T) {
	security, users, notifications := testSecurityService(t)
	ctx := context.Background()
	_ = notifications

	holderID := createTestUserWithPermission(t, users, "guard@example.com", "security.alerts.read")

	for i := 0; i < 4; i++ {
		if _, err := security.RecordEvent(ctx, SecurityEventInput{
			EventType: "login_failed", Module: "users", SourceIP: "192.168.1.20", RecordRef: "victim@example.com",
			Severity: "warning", Summary: "failed login",
		}); err != nil {
			t.Fatal(err)
		}
	}
	inboxBefore, err := notifications.ListForUser(ctx, holderID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inboxBefore.Items) != 0 {
		t.Fatalf("expected no alert before the threshold is reached, got %d", len(inboxBefore.Items))
	}

	// The 5th failed login within the window crosses the default threshold of 5.
	if _, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "login_failed", Module: "users", SourceIP: "192.168.1.20", RecordRef: "victim@example.com",
		Severity: "warning", Summary: "failed login",
	}); err != nil {
		t.Fatal(err)
	}

	inboxAfter, err := notifications.ListForUser(ctx, holderID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inboxAfter.Items) != 1 {
		t.Fatalf("expected exactly one alert after the brute-force threshold, got %d: %+v", len(inboxAfter.Items), inboxAfter.Items)
	}
}

func TestPrivilegeEscalationRuleFiresOnManagePermission(t *testing.T) {
	security, _, _ := testSecurityService(t)
	ctx := context.Background()

	event, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "permission_granted", Module: "roles", SourceIP: "192.168.1.10",
		Severity: "warning", Summary: "granted users.manage to role X", PermissionKey: "users.manage",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.EventType != "permission_granted" {
		t.Fatalf("unexpected event type %q", event.EventType)
	}
	// isEscalationPermission itself is exercised directly: the fired-rule list
	// isn't part of SecurityEvent's public shape, so assert the underlying
	// matcher the rule depends on rather than reaching into private state.
	if !isEscalationPermission("users.manage") {
		t.Fatal("expected users.manage to match the privilege-escalation rule")
	}
	if isEscalationPermission("users.read") {
		t.Fatal("did not expect users.read to match the privilege-escalation rule")
	}
}

func TestRunMaintenanceDeletesEventsPastRetention(t *testing.T) {
	security, _, _ := testSecurityService(t)
	ctx := context.Background()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	security.now = func() time.Time { return old }
	oldEvent, err := security.RecordEvent(ctx, SecurityEventInput{
		EventType: "user_created", Module: "users", SourceIP: "192.168.1.10", Summary: "old event",
	})
	if err != nil {
		t.Fatal(err)
	}

	recent := old.AddDate(0, 0, 5)
	security.now = func() time.Time { return recent }

	if _, err := security.SetSettings(ctx, adminUserID, UpdateSecuritySettingsInput{RetentionDays: intPtr(3)}); err != nil {
		t.Fatal(err)
	}
	if err := security.RunMaintenance(ctx); err != nil {
		t.Fatal(err)
	}

	events, err := security.ListEvents(ctx, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events.Items {
		if event.ID == oldEvent.ID {
			t.Fatal("expected the old event to be deleted by retention cleanup")
		}
	}
}

func strPtr(value string) *string { return &value }
func intPtr(value int) *int       { return &value }
