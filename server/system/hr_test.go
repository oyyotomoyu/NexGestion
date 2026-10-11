package system

import (
	"context"
	"errors"
	"testing"
)

type hrTestEnv struct {
	hr    *HRService
	users *UserService
	admin HRActor
}

func newHRTestEnv(t *testing.T) *hrTestEnv {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	keys, err := users.EffectivePermissionKeys(context.Background(), adminUserID)
	if err != nil {
		t.Fatal(err)
	}
	return &hrTestEnv{
		hr:    NewHRService(directory, users, NewNotificationService(directory, users)),
		users: users,
		admin: NewHRActor(adminUserID, keys),
	}
}

func (e *hrTestEnv) createUser(t *testing.T, email string, permissionKeys ...string) string {
	t.Helper()
	ctx := context.Background()
	user, err := e.users.Create(ctx, adminUserID, CreateUserInput{
		DisplayName: email, Email: email, Password: "a-secure-user-password", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(permissionKeys) == 0 {
		return user.ID
	}
	role, err := e.users.CreateRole(ctx, adminUserID, CreateRoleInput{Title: "role for " + email})
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := e.users.ListPermissions(ctx, ListQuery{PageSize: MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range permissionKeys {
		found := false
		for _, permission := range permissions.Items {
			if permission.PermissionKey == key {
				found = true
				if err := e.users.SetRolePermission(ctx, adminUserID, role.ID, permission.ID, true, "a-secure-test-password"); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !found {
			t.Fatalf("permission %q not in catalog", key)
		}
	}
	if err := e.users.SetRoleUser(ctx, adminUserID, role.ID, user.ID, true); err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func (e *hrTestEnv) actor(t *testing.T, userID string) HRActor {
	t.Helper()
	keys, err := e.users.EffectivePermissionKeys(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return NewHRActor(userID, keys)
}

func TestEmploymentLifecycle(t *testing.T) {
	env := newHRTestEnv(t)
	ctx := context.Background()
	employeeID := env.createUser(t, "employee@example.com")
	level := 1
	group, err := env.users.CreateGroup(ctx, adminUserID, CreateGroupInput{Name: "Company", Type: "organization", OrganizationLevel: &level})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.users.SetGroupMember(ctx, adminUserID, group.ID, employeeID, SetGroupMemberInput{Role: "member", IsPrimaryOrganization: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventOnboarded, EffectiveDate: "2026-01-05",
	}); err == nil {
		t.Fatal("onboarding without employee_number must fail when no profile exists")
	}
	onboarded, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventOnboarded, EffectiveDate: "2026-01-05", EmployeeNumber: "E-001",
		Tasks: []CreateHRTaskInput{{Title: "Issue laptop", AssignedToUserID: &env.admin.UserID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if onboarded.NewEmploymentStatus == nil || *onboarded.NewEmploymentStatus != EmploymentStatusProbation ||
		onboarded.ProbationEndDate == nil || *onboarded.ProbationEndDate != "2026-04-05" {
		t.Fatalf("expected probation with default 90-day end date, got %+v", onboarded)
	}
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventOnboarded, EffectiveDate: "2026-01-06",
	}); !errors.Is(err, ErrHRInvalidTransition) {
		t.Fatalf("second onboarding: expected ErrHRInvalidTransition, got %v", err)
	}
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventReinstated, EffectiveDate: "2026-02-01",
	}); !errors.Is(err, ErrHRInvalidTransition) {
		t.Fatalf("reinstate from probation: expected ErrHRInvalidTransition, got %v", err)
	}
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventProbationPassed, EffectiveDate: "2026-04-05",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventTerminated, EffectiveDate: "2026-06-30",
	}); !errors.Is(err, ErrHRInvalidInput) {
		t.Fatalf("termination without reason: expected ErrHRInvalidInput, got %v", err)
	}
	reason := "resignation"
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventTerminated, EffectiveDate: "2026-06-30", Reason: &reason,
		Tasks: []CreateHRTaskInput{{Title: "Return laptop"}},
	}); err != nil {
		t.Fatal(err)
	}

	user, err := env.users.Get(ctx, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	if user.Status != "disabled" || user.EmployeeProfile.EmploymentStatus != EmploymentStatusTerminated ||
		user.EmployeeProfile.TerminationDate == nil || *user.EmployeeProfile.TerminationDate != "2026-06-30" {
		t.Fatalf("offboarding did not update the account: %+v %+v", user, user.EmployeeProfile)
	}
	if len(user.Groups) != 1 || user.Groups[0].LeftAt == nil {
		t.Fatalf("expected group membership ended via left_at, got %+v", user.Groups)
	}

	record, err := env.hr.EmploymentRecord(ctx, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Events) != 3 || record.Events[0].EventType != EmploymentEventTerminated {
		t.Fatalf("unexpected history: %+v", record.Events)
	}
	tasks, err := env.hr.ListTasks(ctx, employeeID, "", ListQuery{})
	if err != nil || tasks.Pagination.Total != 2 {
		t.Fatalf("expected two checklist tasks, got %+v err=%v", tasks, err)
	}

	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, employeeID, RecordEmploymentEventInput{
		EventType: EmploymentEventRehired, EffectiveDate: "2027-01-04", SkipProbation: true,
	}); err != nil {
		t.Fatal(err)
	}
	user, _ = env.users.Get(ctx, employeeID)
	if user.Status != "active" || user.EmployeeProfile.EmploymentStatus != EmploymentStatusActive || user.EmployeeProfile.TerminationDate != nil {
		t.Fatalf("rehire did not restore employment: %+v %+v", user, user.EmployeeProfile)
	}

	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, adminUserID, RecordEmploymentEventInput{
		EventType: EmploymentEventOnboarded, EffectiveDate: "2026-01-01", EmployeeNumber: "ADMIN",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, adminUserID, RecordEmploymentEventInput{
		EventType: EmploymentEventTerminated, EffectiveDate: "2026-02-01", Reason: &reason,
	}); err == nil {
		t.Fatal("protected administrator must not be terminated")
	}
}

func TestHRTaskAssigneeMayOnlyChangeStatus(t *testing.T) {
	env := newHRTestEnv(t)
	ctx := context.Background()
	employeeID := env.createUser(t, "employee@example.com")
	assigneeID := env.createUser(t, "it@example.com")
	task, err := env.hr.CreateTask(ctx, env.admin, employeeID, CreateHRTaskInput{
		Kind: HRTaskKindOnboarding, Title: "Create accounts", AssignedToUserID: &assigneeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assignee := env.actor(t, assigneeID)
	title := "changed"
	if _, err := env.hr.UpdateTask(ctx, assignee, task.ID, UpdateHRTaskInput{Title: &title}, true); !errors.Is(err, ErrHRInvalidInput) {
		t.Fatalf("assignee title change: expected ErrHRInvalidInput, got %v", err)
	}
	other := env.actor(t, employeeID)
	done := HRTaskDone
	if _, err := env.hr.UpdateTask(ctx, other, task.ID, UpdateHRTaskInput{Status: &done}, true); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("non-assignee: expected ErrHRNotFound, got %v", err)
	}
	updated, err := env.hr.UpdateTask(ctx, assignee, task.ID, UpdateHRTaskInput{Status: &done}, true)
	if err != nil || updated.Status != HRTaskDone || updated.CompletedAt == nil {
		t.Fatalf("assignee completion: %+v err=%v", updated, err)
	}
}

func TestPerformanceReviewWorkflow(t *testing.T) {
	env := newHRTestEnv(t)
	ctx := context.Background()
	employeeID := env.createUser(t, "employee@example.com", PermissionHRPerformanceReadSelf)
	managerID := env.createUser(t, "manager@example.com", PermissionHRPerformanceRead, PermissionHRPerformanceReview)
	outsiderID := env.createUser(t, "outsider@example.com", PermissionHRPerformanceRead, PermissionHRPerformanceReview)
	level := 1
	group, err := env.users.CreateGroup(ctx, adminUserID, CreateGroupInput{Name: "Sales", Type: "organization", OrganizationLevel: &level})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.users.SetGroupMember(ctx, adminUserID, group.ID, managerID, SetGroupMemberInput{Role: "manager"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.users.SetGroupMember(ctx, adminUserID, group.ID, employeeID, SetGroupMemberInput{Role: "member", IsPrimaryOrganization: true}); err != nil {
		t.Fatal(err)
	}
	employee, manager, outsider := env.actor(t, employeeID), env.actor(t, managerID), env.actor(t, outsiderID)

	cycle, err := env.hr.CreatePerformanceCycle(ctx, env.admin, CreatePerformanceCycleInput{Name: "2026 H1", StartDate: "2026-01-01", EndDate: "2026-06-30"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.SubmitSelfReview(ctx, employee, cycle.ID, SubmitSelfReviewInput{SelfComments: "done"}); !errors.Is(err, ErrHRInvalidTransition) {
		t.Fatalf("self review in draft cycle: expected ErrHRInvalidTransition, got %v", err)
	}
	open := PerformanceCycleOpen
	if _, err := env.hr.UpdatePerformanceCycle(ctx, env.admin, cycle.ID, UpdatePerformanceCycleInput{Status: &open}); err != nil {
		t.Fatal(err)
	}

	if _, err := env.hr.CreatePerformanceGoal(ctx, employee, employeeID, CreatePerformanceGoalInput{CycleID: cycle.ID, Title: "Close 10 deals"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.CreatePerformanceGoal(ctx, manager, employeeID, CreatePerformanceGoalInput{CycleID: cycle.ID, Title: "Mentor a junior"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.CreatePerformanceGoal(ctx, outsider, employeeID, CreatePerformanceGoalInput{CycleID: cycle.ID, Title: "x"}); !errors.Is(err, ErrHRForbidden) {
		t.Fatalf("outsider goal: expected ErrHRForbidden, got %v", err)
	}
	if _, err := env.hr.ListPerformanceGoals(ctx, outsider, employeeID, ListQuery{}); !errors.Is(err, ErrHRForbidden) {
		t.Fatalf("outsider reading goals: expected ErrHRForbidden, got %v", err)
	}
	goals, err := env.hr.ListPerformanceGoals(ctx, manager, employeeID, ListQuery{})
	if err != nil || goals.Pagination.Total != 2 {
		t.Fatalf("manager goals: %+v err=%v", goals, err)
	}

	if _, err := env.hr.SubmitManagerReview(ctx, manager, employeeID, cycle.ID, SubmitManagerReviewInput{Rating: "4"}); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("manager review before self review: expected ErrHRNotFound, got %v", err)
	}
	if _, err := env.hr.SubmitSelfReview(ctx, employee, cycle.ID, SubmitSelfReviewInput{SelfComments: "I did well"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.hr.SubmitManagerReview(ctx, manager, employeeID, cycle.ID, SubmitManagerReviewInput{Rating: "9"}); !errors.Is(err, ErrHRInvalidInput) {
		t.Fatalf("rating outside scale: expected ErrHRInvalidInput, got %v", err)
	}
	if _, err := env.hr.SubmitManagerReview(ctx, outsider, employeeID, cycle.ID, SubmitManagerReviewInput{Rating: "4"}); !errors.Is(err, ErrHRForbidden) {
		t.Fatalf("outsider review: expected ErrHRForbidden, got %v", err)
	}
	if _, err := env.hr.SubmitManagerReview(ctx, manager, employeeID, cycle.ID, SubmitManagerReviewInput{Rating: "4", ManagerComments: "Strong"}); err != nil {
		t.Fatal(err)
	}
	own, err := env.hr.ListPerformanceReviews(ctx, employee, employeeID)
	if err != nil || len(own) != 1 || own[0].Rating != nil || own[0].ManagerComments != nil {
		t.Fatalf("employee must not see unfinalized manager assessment: %+v err=%v", own, err)
	}
	review, err := env.hr.FinalizePerformanceReview(ctx, manager, employeeID, cycle.ID)
	if err != nil || review.Status != PerformanceReviewFinalized {
		t.Fatalf("finalize: %+v err=%v", review, err)
	}
	if _, err := env.hr.SubmitManagerReview(ctx, manager, employeeID, cycle.ID, SubmitManagerReviewInput{Rating: "5"}); !errors.Is(err, ErrHRInvalidTransition) {
		t.Fatalf("overwrite finalized: expected ErrHRInvalidTransition, got %v", err)
	}
	five := "5"
	corrected, err := env.hr.CorrectPerformanceReview(ctx, manager, review.ID, CorrectPerformanceReviewInput{Rating: &five, Reason: "calibration"})
	if err != nil {
		t.Fatal(err)
	}
	if *corrected.Rating != "4" || *corrected.EffectiveRating != "5" || len(corrected.Corrections) != 1 {
		t.Fatalf("correction must not overwrite finalized rating: %+v", corrected)
	}
	own, _ = env.hr.ListPerformanceReviews(ctx, employee, employeeID)
	if own[0].EffectiveRating == nil || *own[0].EffectiveRating != "5" {
		t.Fatalf("employee should see finalized corrected rating: %+v", own[0])
	}
}

func TestEmployeeRelationsConfidentiality(t *testing.T) {
	env := newHRTestEnv(t)
	ctx := context.Background()
	handlerID := env.createUser(t, "handler@example.com", PermissionHREmployeeRelationsManage)
	seniorID := env.createUser(t, "senior@example.com", PermissionHREmployeeRelationsManage, PermissionHREmployeeRelationsRead)
	readerID := env.createUser(t, "reader@example.com", PermissionHREmployeeRelationsRead)
	// The subject holds the broad read permission but must still never see
	// the case about themself.
	subjectID := env.createUser(t, "subject@example.com", PermissionHREmployeeRelationsRead, PermissionHREmploymentManage)
	reporterID := env.createUser(t, "reporter@example.com", PermissionHREmployeeRelationsReadSelf)
	handler, senior, reader := env.actor(t, handlerID), env.actor(t, seniorID), env.actor(t, readerID)
	subject, reporter := env.actor(t, subjectID), env.actor(t, reporterID)

	if _, err := env.hr.CreateEmployeeRelationsCase(ctx, handler, CreateEmployeeRelationsCaseInput{
		CaseType: CaseTypeDisciplinary, SubjectUserID: subjectID, IsAnonymous: true,
	}); !errors.Is(err, ErrHRInvalidInput) {
		t.Fatalf("anonymous disciplinary: expected ErrHRInvalidInput, got %v", err)
	}
	if _, err := env.hr.CreateEmployeeRelationsCase(ctx, handler, CreateEmployeeRelationsCaseInput{
		CaseType: CaseTypeGrievance, SubjectUserID: subjectID, AssignedToUserID: &readerID,
	}); !errors.Is(err, ErrHRInvalidInput) {
		t.Fatalf("handler without manage: expected ErrHRInvalidInput, got %v", err)
	}
	standard, err := env.hr.CreateEmployeeRelationsCase(ctx, handler, CreateEmployeeRelationsCaseInput{
		CaseType: CaseTypeDisciplinary, SubjectUserID: subjectID, ReportedByUserID: &reporterID, Summary: ptr("late repeatedly"),
	})
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := env.hr.CreateEmployeeRelationsCase(ctx, handler, CreateEmployeeRelationsCaseInput{
		CaseType: CaseTypeGrievance, SubjectUserID: subjectID, ConfidentialityLevel: CaseConfidentialityRestricted,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := env.hr.GetEmployeeRelationsCase(ctx, reader, standard.ID); err != nil {
		t.Fatalf("reader should see standard case: %v", err)
	}
	if _, err := env.hr.GetEmployeeRelationsCase(ctx, reader, restricted.ID); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("reader must not see restricted case: %v", err)
	}
	if _, err := env.hr.GetEmployeeRelationsCase(ctx, subject, standard.ID); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("subject must not see full case: %v", err)
	}
	list, err := env.hr.ListEmployeeRelationsCases(ctx, subject, ListQuery{})
	if err != nil || list.Pagination.Total != 0 {
		t.Fatalf("subject case list must be empty: %+v err=%v", list, err)
	}
	mine, err := env.hr.ListMyEmployeeRelationsCases(ctx, reporter)
	if err != nil || len(mine) != 1 || mine[0].Relation != "reporter" {
		t.Fatalf("reporter self view: %+v err=%v", mine, err)
	}

	if _, err := env.hr.TransitionEmployeeRelationsCase(ctx, handler, restricted.ID, TransitionEmployeeRelationsCaseInput{Action: CaseActionResolve}); !errors.Is(err, ErrHRInvalidTransition) {
		t.Fatalf("resolve from open: expected ErrHRInvalidTransition, got %v", err)
	}
	if _, err := env.hr.TransitionEmployeeRelationsCase(ctx, senior, restricted.ID, TransitionEmployeeRelationsCaseInput{Action: CaseActionInvestigate}); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("non-handler transition: expected ErrHRNotFound, got %v", err)
	}
	if _, err := env.hr.GetEmployeeRelationsCase(ctx, senior, restricted.ID); !errors.Is(err, ErrHRNotFound) {
		t.Fatalf("senior must not see restricted case before escalation: %v", err)
	}
	if _, err := env.hr.TransitionEmployeeRelationsCase(ctx, handler, restricted.ID, TransitionEmployeeRelationsCaseInput{
		Action: CaseActionEscalate, AssignedToUserID: &seniorID,
	}); err != nil {
		t.Fatal(err)
	}
	escalated, err := env.hr.GetEmployeeRelationsCase(ctx, senior, restricted.ID)
	if err != nil || escalated.Status != CaseStatusEscalated || len(escalated.History) != 2 {
		t.Fatalf("senior after escalation: %+v err=%v", escalated, err)
	}
	if _, err := env.hr.GetEmployeeRelationsCase(ctx, handler, restricted.ID); err != nil {
		t.Fatalf("former handler keeps visibility: %v", err)
	}
	for _, step := range []TransitionEmployeeRelationsCaseInput{
		{Action: CaseActionInvestigate},
		{Action: CaseActionResolve, ResolutionNotes: ptr("mediated")},
		{Action: CaseActionClose},
	} {
		if _, err := env.hr.TransitionEmployeeRelationsCase(ctx, senior, restricted.ID, step); err != nil {
			t.Fatalf("%s: %v", step.Action, err)
		}
	}

	// Section 5.3: a disciplinary case leading to suspension links both ways.
	if _, err := env.hr.RecordEmploymentEvent(ctx, env.admin, subjectID, RecordEmploymentEventInput{
		EventType: EmploymentEventOnboarded, EffectiveDate: "2026-01-01", EmployeeNumber: "E-9", SkipProbation: true,
	}); err != nil {
		t.Fatal(err)
	}
	event, err := env.hr.RecordEmploymentEvent(ctx, env.admin, subjectID, RecordEmploymentEventInput{
		EventType: EmploymentEventSuspended, EffectiveDate: "2026-03-01", RelatedCaseID: &standard.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := env.hr.GetEmployeeRelationsCase(ctx, handler, standard.ID)
	if err != nil || linked.RelatedEmploymentEventID == nil || *linked.RelatedEmploymentEventID != event.ID {
		t.Fatalf("case not linked to suspension event: %+v err=%v", linked, err)
	}

	summary, err := env.hr.FileGrievance(ctx, reporter, FileGrievanceInput{Summary: "unsafe equipment", IsAnonymous: true})
	if err != nil {
		t.Fatal(err)
	}
	filed, err := env.hr.GetEmployeeRelationsCase(ctx, env.admin, summary.ID)
	if err != nil {
		// The grievance was routed to some handler other than admin; the
		// anonymity guarantee is what matters here.
		filed, err = env.hr.GetEmployeeRelationsCase(ctx, senior, summary.ID)
	}
	if err != nil || filed.ReportedByUserID != nil || !filed.IsAnonymous {
		t.Fatalf("anonymous grievance must not store reporter: %+v err=%v", filed, err)
	}
}

func ptr(value string) *string { return &value }
