package system

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"
)

func testTemplateService(t *testing.T) (*TemplateService, *UserService, string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	templates := NewTemplateService(directory, t.TempDir(), users)
	templates.now = func() time.Time {
		return time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	}
	return templates, users, directory
}

// setSystemSetting overwrites a system_settings row directly, the same table
// an administrator would tune template size limits through.
func setSystemSetting(t *testing.T, directory, key, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", directory+"/system.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO system_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
		t.Fatal(err)
	}
}

// createTestUserWithPermission creates a non-admin user, a custom role
// granting the given permission, assigns both, and returns the user id.
func createTestUserWithPermission(t *testing.T, users *UserService, email, permissionKey string) string {
	t.Helper()
	ctx := context.Background()
	user, err := users.Create(ctx, adminUserID, CreateUserInput{
		DisplayName: email, Email: email, Password: "a-secure-user-password", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := users.CreateRole(ctx, adminUserID, CreateRoleInput{Title: email + "-role"})
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := users.ListPermissions(ctx, ListQuery{PageSize: MaxPageSize})
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	var permissionID string
	for _, permission := range permissions.Items {
		if permission.PermissionKey == permissionKey {
			permissionID = permission.ID
			break
		}
	}
	if permissionID == "" {
		t.Fatalf("permission %q not found in catalog", permissionKey)
	}
	if err := users.SetRolePermission(ctx, adminUserID, role.ID, permissionID, true); err != nil {
		t.Fatal(err)
	}
	if err := users.SetRoleUser(ctx, adminUserID, role.ID, user.ID, true); err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func TestTemplateUploadRequiresAudience(t *testing.T) {
	service, _, _ := testTemplateService(t)
	_, err := service.Upload(context.Background(), adminUserID, UploadTemplateInput{}, "form.docx", "", bytes.NewReader([]byte("content")))
	if !errorsIs(err, ErrTemplateInvalid) {
		t.Fatalf("expected invalid template error, got %v", err)
	}
}

func TestTemplateUploadEnforcesFileSizeLimit(t *testing.T) {
	service, _, directory := testTemplateService(t)
	setSystemSetting(t, directory, templateFileMaxBytesKey, "4")
	_, err := service.Upload(context.Background(), adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "form.docx", "", bytes.NewReader([]byte("too large")))
	if !errorsIs(err, ErrTemplateFileTooLarge) {
		t.Fatalf("expected file too large error, got %v", err)
	}
}

func TestTemplateUploadEnforcesStorageLimit(t *testing.T) {
	service, _, directory := testTemplateService(t)
	setSystemSetting(t, directory, templateStorageMaxBytesKey, "10")
	ctx := context.Background()
	if _, err := service.Upload(ctx, adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "first.docx", "", bytes.NewReader([]byte("12345"))); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	_, err := service.Upload(ctx, adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "second.docx", "", bytes.NewReader([]byte("123456")))
	if !errorsIs(err, ErrTemplateStorageFull) {
		t.Fatalf("expected storage limit error, got %v", err)
	}
}

func TestTemplateVisibilityByAudienceScope(t *testing.T) {
	service, users, _ := testTemplateService(t)
	ctx := context.Background()

	memberID := createTestUserWithPermission(t, users, "member@example.com", "templates.read")
	outsiderID := createTestUserWithPermission(t, users, "outsider@example.com", "templates.read")

	level := 1
	group, err := users.CreateGroup(ctx, adminUserID, CreateGroupInput{Name: "HR", Type: "organization", OrganizationLevel: &level})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.SetGroupMember(ctx, adminUserID, group.ID, memberID, SetGroupMemberInput{Role: "member"}); err != nil {
		t.Fatal(err)
	}

	orgFile, err := service.Upload(ctx, adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "organization-wide.docx", "", bytes.NewReader([]byte("org")))
	if err != nil {
		t.Fatalf("upload org file: %v", err)
	}
	groupFile, err := service.Upload(ctx, adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "group", TargetGroupID: group.ID}},
	}, "group-only.docx", "", bytes.NewReader([]byte("group")))
	if err != nil {
		t.Fatalf("upload group file: %v", err)
	}

	memberList, err := service.ListForUser(ctx, memberID, ListQuery{})
	if err != nil {
		t.Fatalf("list for member: %v", err)
	}
	if len(memberList.Items) != 2 {
		t.Fatalf("expected member to see both files, got %+v", memberList.Items)
	}

	outsiderList, err := service.ListForUser(ctx, outsiderID, ListQuery{})
	if err != nil {
		t.Fatalf("list for outsider: %v", err)
	}
	if len(outsiderList.Items) != 1 || outsiderList.Items[0].ID != orgFile.ID {
		t.Fatalf("expected outsider to see only the org-wide file, got %+v", outsiderList.Items)
	}

	if _, _, err := service.DownloadPath(ctx, outsiderID, groupFile.ID); !errorsIs(err, ErrTemplatePermission) {
		t.Fatalf("expected outsider download to be denied, got %v", err)
	}
	if _, _, err := service.DownloadPath(ctx, memberID, groupFile.ID); err != nil {
		t.Fatalf("expected member download to succeed, got %v", err)
	}
}

func TestTemplateListVisibleGrantsManagerEverything(t *testing.T) {
	service, users, _ := testTemplateService(t)
	ctx := context.Background()

	managerID := createTestUserWithPermission(t, users, "manager@example.com", "templates.manage")
	uploaderID := createTestUserWithPermission(t, users, "uploader@example.com", "templates.upload")

	if _, err := service.Upload(ctx, uploaderID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "user", TargetUserID: managerID}},
	}, "private.docx", "", bytes.NewReader([]byte("private"))); err != nil {
		t.Fatalf("upload: %v", err)
	}

	visible, err := service.ListVisible(ctx, managerID, ListQuery{})
	if err != nil {
		t.Fatalf("list visible for manager: %v", err)
	}
	if len(visible.Items) != 1 {
		t.Fatalf("expected manager to see the file via templates.manage, got %+v", visible.Items)
	}
}

func TestTemplateDeleteRequiresOwnershipOrManagePermission(t *testing.T) {
	service, users, _ := testTemplateService(t)
	ctx := context.Background()

	uploaderID := createTestUserWithPermission(t, users, "owner@example.com", "templates.upload")
	strangerID := createTestUserWithPermission(t, users, "stranger@example.com", "templates.read")
	managerID := createTestUserWithPermission(t, users, "delete-manager@example.com", "templates.manage")

	item, err := service.Upload(ctx, uploaderID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "owned.docx", "", bytes.NewReader([]byte("owned")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := service.Delete(ctx, strangerID, item.ID); !errorsIs(err, ErrTemplatePermission) {
		t.Fatalf("expected stranger delete to be denied, got %v", err)
	}
	if err := service.Delete(ctx, managerID, item.ID); err != nil {
		t.Fatalf("expected manager delete to succeed, got %v", err)
	}
	if _, err := service.Get(ctx, item.ID); !errorsIs(err, ErrTemplateNotFound) {
		t.Fatalf("expected file to be gone after delete, got %v", err)
	}
}

func TestTemplateOwnerCanDeleteOwnUpload(t *testing.T) {
	service, users, _ := testTemplateService(t)
	ctx := context.Background()
	uploaderID := createTestUserWithPermission(t, users, "self-owner@example.com", "templates.upload")

	item, err := service.Upload(ctx, uploaderID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "mine.docx", "", bytes.NewReader([]byte("mine")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if err := service.Delete(ctx, uploaderID, item.ID); err != nil {
		t.Fatalf("expected owner delete to succeed, got %v", err)
	}
}

func TestTemplateStorageUsageReportsTotals(t *testing.T) {
	service, _, _ := testTemplateService(t)
	ctx := context.Background()
	if _, err := service.Upload(ctx, adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "organization"}},
	}, "one.docx", "", bytes.NewReader([]byte("12345"))); err != nil {
		t.Fatalf("upload: %v", err)
	}
	usage, err := service.StorageUsage(ctx)
	if err != nil {
		t.Fatalf("storage usage: %v", err)
	}
	if usage.FileCount != 1 || usage.UsedBytes != 5 || usage.MaxBytes != defaultTemplateStorageMaxBytes || usage.MaxFileBytes != defaultTemplateFileMaxBytes {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestTemplateUploadRejectsInvalidAudienceScope(t *testing.T) {
	service, _, _ := testTemplateService(t)
	_, err := service.Upload(context.Background(), adminUserID, UploadTemplateInput{
		Audiences: []TemplateAudienceInput{{Scope: "group"}},
	}, "form.docx", "", bytes.NewReader([]byte("content")))
	if !errorsIs(err, ErrTemplateInvalid) {
		t.Fatalf("expected invalid template error for missing group id, got %v", err)
	}
}
