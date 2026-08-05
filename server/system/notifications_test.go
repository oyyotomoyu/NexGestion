package system

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testNotificationService(t *testing.T) (*NotificationService, *UserService) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	notifications := NewNotificationService(directory, users)
	notifications.now = func() time.Time {
		return time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	}
	return notifications, users
}

func TestNotificationCreateListUpdateHideAndExport(t *testing.T) {
	service, _ := testNotificationService(t)
	ctx := context.Background()

	created, err := service.Create(ctx, adminUserID, CreateNotificationInput{
		Title:        "Maintenance",
		Message:      "System maintenance tonight.",
		TypeCode:     "important",
		DurationCode: "day",
		Audiences: []NotificationAudienceInput{
			{Scope: "organization"},
		},
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if created.Status != "active" || created.SenderUserID != adminUserID || created.ShowUntil == nil || created.RetainUntil == nil {
		t.Fatalf("unexpected created notification: %+v", created)
	}

	inbox, err := service.ListForUser(ctx, adminUserID, ListQuery{})
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(inbox.Items) != 1 || inbox.Items[0].ID != created.ID {
		t.Fatalf("expected created notification in inbox, got %+v", inbox)
	}

	title := "Updated maintenance"
	updated, err := service.Update(ctx, adminUserID, created.ID, UpdateNotificationInput{Title: &title})
	if err != nil {
		t.Fatalf("update notification: %v", err)
	}
	if updated.Status != "edited" || updated.EditedAt == nil || updated.Title != title {
		t.Fatalf("unexpected updated notification: %+v", updated)
	}

	var csv bytes.Buffer
	if err := service.ExportCSV(ctx, "2026-08", &csv); err != nil {
		t.Fatalf("export csv: %v", err)
	}
	if !strings.Contains(csv.String(), "Updated maintenance") {
		t.Fatalf("export missing notification: %s", csv.String())
	}

	if err := service.Hide(ctx, adminUserID, created.ID); err != nil {
		t.Fatalf("hide notification: %v", err)
	}
	hiddenInbox, err := service.ListForUser(ctx, adminUserID, ListQuery{})
	if err != nil {
		t.Fatalf("list after hide: %v", err)
	}
	if len(hiddenInbox.Items) != 0 {
		t.Fatalf("hidden notification should not show, got %+v", hiddenInbox)
	}
}

func TestNotificationMaintenanceExpiresAndCleans(t *testing.T) {
	service, _ := testNotificationService(t)
	ctx := context.Background()
	created, err := service.Create(ctx, adminUserID, CreateNotificationInput{
		Title:        "Short notice",
		Message:      "Expires soon.",
		TypeCode:     "info",
		DurationCode: "hour",
		Audiences:    []NotificationAudienceInput{{Scope: "organization"}},
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}

	service.now = func() time.Time {
		return time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	}
	if err := service.ExpireAndCleanup(ctx); err != nil {
		t.Fatalf("expire notification: %v", err)
	}
	expired, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get expired notification: %v", err)
	}
	if expired.Status != "expired" || expired.ExpiredAt == nil {
		t.Fatalf("expected expired status, got %+v", expired)
	}

	service.now = func() time.Time {
		return time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	}
	if err := service.ExpireAndCleanup(ctx); err != nil {
		t.Fatalf("cleanup notification: %v", err)
	}
	_, err = service.Get(ctx, created.ID)
	if !errorsIs(err, ErrNotificationNotFound) {
		t.Fatalf("expected notification cleanup, got %v", err)
	}
}

func TestNotificationOwnGroupRequiresMembership(t *testing.T) {
	service, _ := testNotificationService(t)
	ctx := context.Background()
	groupID := "00000000-0000-0000-0000-000000000201"

	_, err := service.Create(ctx, adminUserID, CreateNotificationInput{
		Title:        "Team notice",
		Message:      "Hello team.",
		TypeCode:     "info",
		DurationCode: "day",
		Audiences:    []NotificationAudienceInput{{Scope: "own_group", TargetGroupID: groupID}},
	})
	if !errorsIs(err, ErrNotificationPermission) {
		t.Fatalf("expected own group permission failure for non-member, got %v", err)
	}

	userDB, err := service.users.open()
	if err != nil {
		t.Fatal(err)
	}
	defer userDB.Close()
	insertTestGroupMembership(t, userDB, groupID)

	created, err := service.Create(ctx, adminUserID, CreateNotificationInput{
		Title:        "Team notice",
		Message:      "Hello team.",
		TypeCode:     "info",
		DurationCode: "day",
		Audiences:    []NotificationAudienceInput{{Scope: "own_group", TargetGroupID: groupID}},
	})
	if err != nil {
		t.Fatalf("create own group notification: %v", err)
	}
	inbox, err := service.ListForUser(ctx, adminUserID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Items) != 1 || inbox.Items[0].ID != created.ID {
		t.Fatalf("expected own group notification in inbox, got %+v", inbox)
	}
}

func insertTestGroupMembership(t *testing.T, db *sql.DB, groupID string) {
	t.Helper()
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO groups
		(id,name,type,organization_level,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?)`, groupID, "Operations", "organization", 1, "active", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_groups
		(user_id,group_id,joined_at,is_primary_organization,created_at)
		VALUES(?,?,?,?,?)`, adminUserID, groupID, now, 1, now); err != nil {
		t.Fatal(err)
	}
}

func errorsIs(err, target error) bool {
	return err != nil && (err == target || strings.Contains(err.Error(), target.Error()))
}
