package system

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func newSalaryTestServices(t *testing.T) (*SalaryService, *UserService, string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	user, err := users.Create(context.Background(), adminUserID, CreateUserInput{
		DisplayName: "Salary Employee",
		Email:       "salary-employee@example.com",
		Password:    "a-secure-user-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewSalaryService(directory, users), users, user.ID
}

func TestCreateCompensationRecordRejectsUnknownUser(t *testing.T) {
	salary, _, _ := newSalaryTestServices(t)
	ctx := context.Background()

	_, err := salary.CreateCompensationRecord(ctx, adminUserID, "missing-user", CreateCompensationRecordInput{
		CompensationBasis:  CompensationBasisMonthly,
		RateAmount:         "60000",
		Currency:           "twd",
		JurisdictionID:     "tw",
		EffectiveStartDate: "2026-01-01",
	})
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestCreateCompensationRecordValidatesInput(t *testing.T) {
	salary, _, userID := newSalaryTestServices(t)
	ctx := context.Background()

	cases := []CreateCompensationRecordInput{
		{CompensationBasis: "weird", RateAmount: "100", Currency: "TWD", JurisdictionID: "tw", EffectiveStartDate: "2026-01-01"},
		{CompensationBasis: CompensationBasisHourly, RateAmount: "-5", Currency: "TWD", JurisdictionID: "tw", EffectiveStartDate: "2026-01-01"},
		{CompensationBasis: CompensationBasisHourly, RateAmount: "0", Currency: "TWD", JurisdictionID: "tw", EffectiveStartDate: "2026-01-01"},
		{CompensationBasis: CompensationBasisHourly, RateAmount: "200", Currency: "TW", JurisdictionID: "tw", EffectiveStartDate: "2026-01-01"},
		{CompensationBasis: CompensationBasisHourly, RateAmount: "200", Currency: "TWD", JurisdictionID: "", EffectiveStartDate: "2026-01-01"},
		{CompensationBasis: CompensationBasisHourly, RateAmount: "200", Currency: "TWD", JurisdictionID: "tw", EffectiveStartDate: "01/01/2026"},
	}
	for i, input := range cases {
		if _, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, input); !errors.Is(err, ErrSalaryInvalidInput) {
			t.Fatalf("case %d: expected ErrSalaryInvalidInput, got %v", i, err)
		}
	}
}

func TestCreateCompensationRecordClosesPreviousActiveRecord(t *testing.T) {
	salary, _, userID := newSalaryTestServices(t)
	ctx := context.Background()

	first, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, CreateCompensationRecordInput{
		CompensationBasis:  CompensationBasisHourly,
		RateAmount:         "200",
		Currency:           "twd",
		JurisdictionID:     "tw",
		EffectiveStartDate: "2026-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.EffectiveEndDate != nil || first.Currency != "TWD" {
		t.Fatalf("unexpected first record: %+v", first)
	}

	second, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, CreateCompensationRecordInput{
		CompensationBasis:  CompensationBasisMonthly,
		RateAmount:         "60000",
		Currency:           "TWD",
		JurisdictionID:     "tw",
		EffectiveStartDate: "2026-03-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.EffectiveEndDate != nil {
		t.Fatalf("expected new record to be open-ended, got %+v", second)
	}

	closedFirst, err := salary.getCompensationRecord(ctx, mustOpenSalaryDB(t, salary), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closedFirst.EffectiveEndDate == nil || *closedFirst.EffectiveEndDate != "2026-02-28" {
		t.Fatalf("expected first record closed on 2026-02-28, got %+v", closedFirst.EffectiveEndDate)
	}

	current, err := salary.CurrentCompensationRecord(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != second.ID {
		t.Fatalf("expected current record to be %s, got %s", second.ID, current.ID)
	}
}

func TestCreateCompensationRecordRejectsNonAdvancingStartDate(t *testing.T) {
	salary, _, userID := newSalaryTestServices(t)
	ctx := context.Background()

	if _, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, CreateCompensationRecordInput{
		CompensationBasis:  CompensationBasisHourly,
		RateAmount:         "200",
		Currency:           "TWD",
		JurisdictionID:     "tw",
		EffectiveStartDate: "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, CreateCompensationRecordInput{
		CompensationBasis:  CompensationBasisHourly,
		RateAmount:         "220",
		Currency:           "TWD",
		JurisdictionID:     "tw",
		EffectiveStartDate: "2026-01-01",
	})
	if !errors.Is(err, ErrSalaryOverlap) {
		t.Fatalf("expected ErrSalaryOverlap, got %v", err)
	}
}

func TestCurrentCompensationRecordNotFoundBeforeAnyAssignment(t *testing.T) {
	salary, _, userID := newSalaryTestServices(t)

	_, err := salary.CurrentCompensationRecord(context.Background(), userID)
	if !errors.Is(err, ErrSalaryNotFound) {
		t.Fatalf("expected ErrSalaryNotFound, got %v", err)
	}
}

func TestListCompensationRecordsFiltersAndPaginates(t *testing.T) {
	salary, _, userID := newSalaryTestServices(t)
	ctx := context.Background()
	starts := []string{"2026-01-01", "2026-02-01", "2026-03-01"}
	for _, start := range starts {
		if _, err := salary.CreateCompensationRecord(ctx, adminUserID, userID, CreateCompensationRecordInput{
			CompensationBasis:  CompensationBasisHourly,
			RateAmount:         "200",
			Currency:           "TWD",
			JurisdictionID:     "tw",
			EffectiveStartDate: start,
		}); err != nil {
			t.Fatal(err)
		}
	}

	all, err := salary.ListCompensationRecords(ctx, userID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Pagination.Total != 3 || all.Items[0].EffectiveStartDate != "2026-03-01" {
		t.Fatalf("unexpected list result: %+v", all)
	}

	active, err := salary.ListCompensationRecords(ctx, userID, ListQuery{Filters: map[string]string{"active": "true"}})
	if err != nil {
		t.Fatal(err)
	}
	if active.Pagination.Total != 1 || active.Items[0].EffectiveStartDate != "2026-03-01" {
		t.Fatalf("unexpected active filter result: %+v", active)
	}
}

// mustOpenSalaryDB opens the salary service's underlying database for
// assertions that need to read a record by id directly.
func mustOpenSalaryDB(t *testing.T, salary *SalaryService) *sql.DB {
	t.Helper()
	db, err := salary.open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
