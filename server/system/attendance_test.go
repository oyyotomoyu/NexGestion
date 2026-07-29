package system

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newAttendanceTestService(t *testing.T, timezone string) (*AttendanceService, string, *time.Time) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	user, err := users.Create(context.Background(), adminUserID, CreateUserInput{
		DisplayName: "Attendance User",
		Email:       "attendance@example.com",
		Password:    "a-secure-user-password",
		Timezone:    &timezone,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	service := NewAttendanceService(directory, t.TempDir(), users)
	service.now = func() time.Time { return now }
	return service, user.ID, &now
}

func TestAttendanceSupportsMultipleSessionsPerDay(t *testing.T) {
	service, userID, now := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()

	*now = time.Date(2026, 7, 28, 0, 5, 49, 0, time.UTC)
	first, err := service.SignIn(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != AttendanceWorking || first.Sessions[0].SignInAt != "2026-07-28T00:05:00Z" {
		t.Fatalf("first sign in: %+v", first)
	}
	*now = time.Date(2026, 7, 28, 4, 10, 37, 0, time.UTC)
	if _, err := service.SignOut(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	if _, err := service.SignIn(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 7, 28, 9, 30, 0, 0, time.UTC)
	finished, err := service.SignOut(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != AttendanceNonWorking || len(finished.Sessions) != 2 {
		t.Fatalf("finished day: %+v", finished)
	}
	if finished.WorkedMinutes != 515 || finished.WorkedHours != 8.58 {
		t.Fatalf("worked time: got %d / %.2f", finished.WorkedMinutes, finished.WorkedHours)
	}
}

func TestAttendanceSplitsOpenSessionAtLocalMidnight(t *testing.T) {
	service, userID, now := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()

	*now = time.Date(2026, 7, 28, 15, 30, 0, 0, time.UTC) // 23:30 local
	if _, err := service.SignIn(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 7, 28, 17, 15, 0, 0, time.UTC) // 01:15 local next day
	current, err := service.Today(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AttendanceDate != "2026-07-29" || current.Status != AttendanceWorking || len(current.Sessions) != 1 {
		t.Fatalf("continued day: %+v", current)
	}
	if current.Sessions[0].SignInAt != "2026-07-28T16:00:00Z" || current.Sessions[0].ContinuedFromSessionID == nil {
		t.Fatalf("continuation session: %+v", current.Sessions[0])
	}
	days, err := service.ListDays(ctx, userID, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[1].WorkedMinutes != 30 {
		t.Fatalf("split days: %+v", days)
	}
	finished, err := service.SignOut(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.WorkedMinutes != 75 {
		t.Fatalf("new day minutes = %d, want 75", finished.WorkedMinutes)
	}
}

func TestAttendanceUsesEachUsersTimezoneForMidnight(t *testing.T) {
	service, userID, now := newAttendanceTestService(t, "America/New_York")
	ctx := context.Background()
	*now = time.Date(2026, 1, 6, 4, 45, 0, 0, time.UTC) // Jan 5, 23:45 EST
	if _, err := service.SignIn(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 1, 6, 5, 30, 0, 0, time.UTC) // Jan 6, 00:30 EST
	current, err := service.Today(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AttendanceDate != "2026-01-06" || current.Timezone != "America/New_York" ||
		current.Sessions[0].SignInAt != "2026-01-06T05:00:00Z" {
		t.Fatalf("New York rollover: %+v", current)
	}
	days, err := service.ListDays(ctx, userID, "2026-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[1].WorkedMinutes != 15 {
		t.Fatalf("New York split days: %+v", days)
	}
}

func TestAttendanceGeneratesUTF8MonthlyCSV(t *testing.T) {
	service, userID, now := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()

	*now = time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC)
	if _, err := service.SignIn(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 1, 5, 9, 31, 0, 0, time.UTC)
	if _, err := service.SignOut(ctx, userID); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	export, err := service.GenerateMonthlyCSV(ctx, "2026-01")
	if err != nil {
		t.Fatal(err)
	}
	path, err := service.CSVPath(ctx, "2026-01")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV does not have a UTF-8 BOM")
	}
	for _, expected := range [][]byte{[]byte("worked_hours_minutes"), []byte("8:31"), []byte("511")} {
		if !bytes.Contains(data, expected) {
			t.Fatalf("CSV %s missing %q", filepath.Base(path), expected)
		}
	}
	if export.RowCount != 2 || export.SHA256 == "" {
		t.Fatalf("export metadata: %+v", export)
	}
}

func TestAttendanceCorrectionRecalculatesDayAndAuditsReason(t *testing.T) {
	service, userID, now := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()
	*now = time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC)
	day, err := service.SignIn(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
	if _, err := service.SignOut(ctx, userID); err != nil {
		t.Fatal(err)
	}
	corrected, err := service.CorrectDay(ctx, adminUserID, day.ID, CorrectAttendanceDayInput{
		Reason: "Approved missing afternoon session",
		Sessions: []CorrectAttendanceSessionInput{
			{SignInAt: "2026-07-28T01:00:00Z", SignOutAt: stringPointer("2026-07-28T05:00:00Z")},
			{SignInAt: "2026-07-28T06:00:00Z", SignOutAt: stringPointer("2026-07-28T10:30:00Z")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.WorkedMinutes != 510 || corrected.WorkedHours != 8.5 || len(corrected.Sessions) != 2 {
		t.Fatalf("corrected day: %+v", corrected)
	}
	db, err := service.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM attendance_events
		WHERE attendance_day_id=? AND event_type='correction' AND reason=?`,
		day.ID, "Approved missing afternoon session").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("correction audit count = %d", count)
	}
}

func TestLeaveRequestsSupportFullDayAndHourlyDurations(t *testing.T) {
	service, userID, _ := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()

	fullDay, err := service.ApplyLeave(ctx, userID, ApplyLeaveInput{
		LeaveType: "annual_leave", LeaveDate: "2026-08-03",
		DurationType: LeaveDurationFullDay, Reason: "Vacation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fullDay.RequestedMinutes != 480 || fullDay.StartTime != nil || fullDay.EndTime != nil {
		t.Fatalf("full-day request: %+v", fullDay)
	}

	hourly, err := service.ApplyLeave(ctx, userID, ApplyLeaveInput{
		LeaveType: "sick_leave", LeaveDate: "2026-08-04",
		DurationType: LeaveDurationHourly, StartTime: stringPointer("09:30"), EndTime: stringPointer("11:00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hourly.RequestedMinutes != 90 || hourly.Status != "pending" {
		t.Fatalf("hourly request: %+v", hourly)
	}
	requests, err := service.ListLeaveRequests(ctx, userID)
	if err != nil || len(requests) != 2 {
		t.Fatalf("leave requests: count=%d error=%v", len(requests), err)
	}
}

func TestLeaveRequestRejectsLessThanOneHourAndUnknownType(t *testing.T) {
	service, userID, _ := newAttendanceTestService(t, "Asia/Taipei")
	_, err := service.ApplyLeave(context.Background(), userID, ApplyLeaveInput{
		LeaveType: "sick_leave", LeaveDate: "2026-08-03",
		DurationType: LeaveDurationHourly, StartTime: stringPointer("09:00"), EndTime: stringPointer("09:30"),
	})
	if !errors.Is(err, ErrInvalidLeaveRequest) {
		t.Fatalf("short leave error = %v", err)
	}
	_, err = service.ApplyLeave(context.Background(), userID, ApplyLeaveInput{
		LeaveType: "not_configured", LeaveDate: "2026-08-03", DurationType: LeaveDurationFullDay,
	})
	if !errors.Is(err, ErrInvalidLeaveRequest) {
		t.Fatalf("unknown type error = %v", err)
	}
}

func TestLeaveRequestRoutesToOrganizationManagerAndCanBeApproved(t *testing.T) {
	service, requesterID, _ := newAttendanceTestService(t, "Asia/Taipei")
	ctx := context.Background()
	manager, err := service.users.Create(ctx, adminUserID, CreateUserInput{
		DisplayName: "Team Manager", Email: "leave-manager@example.com",
		Password: "a-secure-user-password", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	level := 1
	group, err := service.users.CreateGroup(ctx, adminUserID, CreateGroupInput{
		Name: "Company", Type: "organization", OrganizationLevel: &level,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.users.SetGroupMember(ctx, adminUserID, group.ID, manager.ID, SetGroupMemberInput{Role: "manager"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.users.SetGroupMember(ctx, adminUserID, group.ID, requesterID, SetGroupMemberInput{
		Role: "member", IsPrimaryOrganization: true,
	}); err != nil {
		t.Fatal(err)
	}
	request, err := service.ApplyLeave(ctx, requesterID, ApplyLeaveInput{
		LeaveType: "annual_leave", LeaveDate: "2026-08-10", DurationType: LeaveDurationFullDay,
	})
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := service.ListLeaveApprovals(ctx, manager.ID)
	if err != nil || len(inbox) != 1 || inbox[0].ID != request.ID || inbox[0].RequesterName == "" {
		t.Fatalf("manager inbox: %+v error=%v", inbox, err)
	}
	decided, err := service.DecideLeave(ctx, manager.ID, request.ID, DecideLeaveInput{
		Decision: "approved", Note: "Coverage confirmed",
	})
	if err != nil || decided.Status != "approved" {
		t.Fatalf("decision: %+v error=%v", decided, err)
	}
	if _, err := service.DecideLeave(ctx, manager.ID, request.ID, DecideLeaveInput{Decision: "rejected"}); !errors.Is(err, ErrLeaveDecision) {
		t.Fatalf("second decision error = %v", err)
	}
}

func stringPointer(value string) *string { return &value }
