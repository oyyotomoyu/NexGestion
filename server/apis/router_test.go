package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

func testRouter(t *testing.T) *http.ServeMux {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := system.EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	router := http.NewServeMux()
	users := system.NewUserService(directory)
	attendance := system.NewAttendanceService(directory, t.TempDir(), users)
	notifications := system.NewNotificationService(directory, users)
	logService, err := applogs.NewService(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logService.Close)
	InitRouter(router, users, attendance, notifications, system.NewAuthService(users), logService)
	return router
}

func TestRouterDirectsHealthAPI(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected JSON response, got %q", contentType)
	}
}

func TestRouterRejectsUnknownAPI(t *testing.T) {
	router := testRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestUserCRUDAPI(t *testing.T) {
	router := testRouter(t)
	accessToken, _ := loginForTest(t, router)

	createBody := []byte(`{
		"display_name":"Test User",
		"email":"Test.User@example.com",
		"password":"a-secure-user-password",
		"locale":"CHT",
		"timezone":"Asia/Taipei"
	}`)
	createResponse := serveAuthorized(router, http.MethodPost, "/api/users", createBody, accessToken)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create: expected %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created system.User
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Email != "test.user@example.com" {
		t.Fatalf("unexpected created user: %+v", created)
	}

	userLogin := serve(router, http.MethodPost, "/api/auth/login", []byte(`{"email":"test.user@example.com","password":"a-secure-user-password"}`))
	if userLogin.Code != http.StatusOK {
		t.Fatalf("user login: expected %d, got %d: %s", http.StatusOK, userLogin.Code, userLogin.Body.String())
	}
	var userTokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(userLogin.Body).Decode(&userTokens); err != nil {
		t.Fatal(err)
	}
	missingPassword := serveAuthorized(router, http.MethodPatch, "/api/users/"+created.ID, []byte(`{"display_name":"Self Edit"}`), userTokens.AccessToken)
	if missingPassword.Code != http.StatusForbidden {
		t.Fatalf("self update without current password: expected %d, got %d", http.StatusForbidden, missingPassword.Code)
	}
	wrongPassword := serveAuthorized(router, http.MethodPatch, "/api/users/"+created.ID, []byte(`{"display_name":"Self Edit","current_password":"wrong-password"}`), userTokens.AccessToken)
	if wrongPassword.Code != http.StatusForbidden {
		t.Fatalf("self update with wrong current password: expected %d, got %d", http.StatusForbidden, wrongPassword.Code)
	}
	selfUpdate := serveAuthorized(router, http.MethodPatch, "/api/users/"+created.ID, []byte(`{"display_name":"Self Edit","current_password":"a-secure-user-password"}`), userTokens.AccessToken)
	if selfUpdate.Code != http.StatusForbidden {
		t.Fatalf("self update without users.manage: expected %d, got %d: %s", http.StatusForbidden, selfUpdate.Code, selfUpdate.Body.String())
	}

	listResponse := serveAuthorized(router, http.MethodGet, "/api/users", nil, accessToken)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list: expected %d, got %d", http.StatusOK, listResponse.Code)
	}
	var list struct {
		Users []system.User `json:"users"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Users) != 2 {
		t.Fatalf("expected administrator and created user, got %d users", len(list.Users))
	}

	getResponse := serveAuthorized(router, http.MethodGet, "/api/users/"+created.ID, nil, accessToken)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get: expected %d, got %d", http.StatusOK, getResponse.Code)
	}

	updateResponse := serveAuthorized(router, http.MethodPatch, "/api/users/"+created.ID, []byte(`{"display_name":"Updated User","status":"disabled"}`), accessToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update: expected %d, got %d: %s", http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	}
	var updated system.User
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Updated User" || updated.Status != "disabled" {
		t.Fatalf("unexpected updated user: %+v", updated)
	}

	deleteResponse := serveAuthorized(router, http.MethodDelete, "/api/users/"+created.ID, nil, accessToken)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete: expected %d, got %d: %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body.String())
	}
	missingResponse := serveAuthorized(router, http.MethodGet, "/api/users/"+created.ID, nil, accessToken)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("get deleted: expected %d, got %d", http.StatusNotFound, missingResponse.Code)
	}
}

func TestUserAPIRequiresAuthentication(t *testing.T) {
	response := serve(testRouter(t), http.MethodGet, "/api/users", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestRoleCRUDAPI(t *testing.T) {
	router := testRouter(t)
	accessToken, _ := loginForTest(t, router)

	createResponse := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{
		"title":"Store Manager",
		"description":"Manages daily store operations"
	}`), accessToken)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create role: expected %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created system.Role
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Title != "Store Manager" || created.IsSystem || created.GrantsAllPermissions {
		t.Fatalf("unexpected role: %+v", created)
	}
	if created.Permissions == nil {
		t.Fatal("expected an empty permissions array")
	}

	listResponse := serveAuthorized(router, http.MethodGet, "/api/roles", nil, accessToken)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list roles: expected %d, got %d: %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var list struct {
		Roles []system.Role `json:"roles"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Roles) != 2 {
		t.Fatalf("expected Admin and custom roles, got %d", len(list.Roles))
	}

	getResponse := serveAuthorized(router, http.MethodGet, "/api/roles/"+created.ID, nil, accessToken)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get role: expected %d, got %d: %s", http.StatusOK, getResponse.Code, getResponse.Body.String())
	}

	updateResponse := serveAuthorized(router, http.MethodPatch, "/api/roles/"+created.ID, []byte(`{"title":"Branch Manager"}`), accessToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update role: expected %d, got %d: %s", http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	}
	var updated system.Role
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Branch Manager" {
		t.Fatalf("unexpected updated role: %+v", updated)
	}

	deleteResponse := serveAuthorized(router, http.MethodDelete, "/api/roles/"+created.ID, nil, accessToken)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete role: expected %d, got %d: %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body.String())
	}
	missingResponse := serveAuthorized(router, http.MethodGet, "/api/roles/"+created.ID, nil, accessToken)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("get deleted role: expected %d, got %d", http.StatusNotFound, missingResponse.Code)
	}
}

func TestAdminRoleIsProtectedAPI(t *testing.T) {
	router := testRouter(t)
	accessToken, _ := loginForTest(t, router)
	const adminRoleID = "00000000-0000-0000-0000-000000000001"

	getResponse := serveAuthorized(router, http.MethodGet, "/api/roles/"+adminRoleID, nil, accessToken)
	var adminRole system.Role
	if err := json.NewDecoder(getResponse.Body).Decode(&adminRole); err != nil {
		t.Fatal(err)
	}
	catalog, err := system.LoadPermissionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if !adminRole.GrantsAllPermissions || len(adminRole.Permissions) != len(catalog.Permissions) {
		t.Fatalf("Admin must expose every catalog permission: %+v", adminRole)
	}

	response := serveAuthorized(router, http.MethodPatch, "/api/roles/"+adminRoleID, []byte(`{"title":"Owner"}`), accessToken)
	if response.Code != http.StatusForbidden {
		t.Fatalf("update Admin role: expected %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
	}
	response = serveAuthorized(router, http.MethodDelete, "/api/roles/"+adminRoleID, nil, accessToken)
	if response.Code != http.StatusForbidden {
		t.Fatalf("delete Admin role: expected %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
	}
	response = serveAuthorized(router, http.MethodDelete, "/api/roles/"+adminRoleID+"/permissions/users.read", nil, accessToken)
	if response.Code != http.StatusForbidden {
		t.Fatalf("edit Admin permission: expected %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
	}
}

func TestRoleAPIRequiresAuthentication(t *testing.T) {
	response := serve(testRouter(t), http.MethodGet, "/api/roles", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestGroupCRUDAPI(t *testing.T) {
	router := testRouter(t)
	accessToken, _ := loginForTest(t, router)

	parentResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"Head Office","type":"organization","organization_level":1}`), accessToken)
	if parentResponse.Code != http.StatusCreated {
		t.Fatalf("create parent group: expected %d, got %d: %s", http.StatusCreated, parentResponse.Code, parentResponse.Body.String())
	}
	var parent system.Group
	if err := json.NewDecoder(parentResponse.Body).Decode(&parent); err != nil {
		t.Fatal(err)
	}

	childResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"Finance","type":"organization","organization_level":2,"parent_group_id":"`+parent.ID+`"}`), accessToken)
	if childResponse.Code != http.StatusCreated {
		t.Fatalf("create child group: expected %d, got %d: %s", http.StatusCreated, childResponse.Code, childResponse.Body.String())
	}
	var child system.Group
	if err := json.NewDecoder(childResponse.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	if child.ParentGroupID == nil || *child.ParentGroupID != parent.ID || child.Status != "active" {
		t.Fatalf("unexpected child group: %+v", child)
	}

	listResponse := serveAuthorized(router, http.MethodGet, "/api/groups", nil, accessToken)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list groups: %s", listResponse.Body.String())
	}
	var list struct {
		Groups []system.Group `json:"groups"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(list.Groups))
	}
	renameResponse := serveAuthorized(router, http.MethodPatch, "/api/groups/"+child.ID, []byte(`{"name":"Accounting"}`), accessToken)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename group: %d %s", renameResponse.Code, renameResponse.Body.String())
	}
	var renamed system.Group
	if err := json.NewDecoder(renameResponse.Body).Decode(&renamed); err != nil {
		t.Fatal(err)
	}
	for roleID, want := range map[string]string{renamed.ManagerRoleID: "Accounting Manager", renamed.MemberRoleID: "Accounting Member"} {
		response := serveAuthorized(router, http.MethodGet, "/api/roles/"+roleID, nil, accessToken)
		var generated system.Role
		if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
			t.Fatal(err)
		}
		if generated.Title != want {
			t.Fatalf("generated role title: got %q want %q", generated.Title, want)
		}
	}

	cycleResponse := serveAuthorized(router, http.MethodPatch, "/api/groups/"+parent.ID, []byte(`{"parent_group_id":"`+child.ID+`"}`), accessToken)
	if cycleResponse.Code != http.StatusBadRequest {
		t.Fatalf("cycle: expected %d, got %d: %s", http.StatusBadRequest, cycleResponse.Code, cycleResponse.Body.String())
	}

	inUseResponse := serveAuthorized(router, http.MethodDelete, "/api/groups/"+parent.ID, nil, accessToken)
	if inUseResponse.Code != http.StatusConflict {
		t.Fatalf("delete parent: expected %d, got %d: %s", http.StatusConflict, inUseResponse.Code, inUseResponse.Body.String())
	}
	if response := serveAuthorized(router, http.MethodDelete, "/api/groups/"+child.ID, nil, accessToken); response.Code != http.StatusNoContent {
		t.Fatalf("delete child: expected %d, got %d: %s", http.StatusNoContent, response.Code, response.Body.String())
	}
	if response := serveAuthorized(router, http.MethodDelete, "/api/groups/"+parent.ID, nil, accessToken); response.Code != http.StatusNoContent {
		t.Fatalf("delete parent: expected %d, got %d: %s", http.StatusNoContent, response.Code, response.Body.String())
	}
}

func TestGroupAPIRequiresAuthentication(t *testing.T) {
	response := serve(testRouter(t), http.MethodGet, "/api/groups", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestGroupOrganizationLevelsAndProjectHierarchyValidation(t *testing.T) {
	router := testRouter(t)
	token, _ := loginForTest(t, router)

	rootResponse := serveAuthorized(router, http.MethodPost, "/api/groups",
		[]byte(`{"name":"Company","type":"organization","organization_level":1}`), token)
	if rootResponse.Code != http.StatusCreated {
		t.Fatalf("create level 1: %d %s", rootResponse.Code, rootResponse.Body.String())
	}
	var root system.Group
	if err := json.NewDecoder(rootResponse.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	if root.OrganizationLevel == nil || *root.OrganizationLevel != 1 {
		t.Fatalf("level 1 response: %+v", root)
	}

	skipped := serveAuthorized(router, http.MethodPost, "/api/groups",
		[]byte(`{"name":"Skipped Team","type":"organization","organization_level":3,"parent_group_id":"`+root.ID+`"}`), token)
	if skipped.Code != http.StatusBadRequest {
		t.Fatalf("skipped level: expected 400, got %d: %s", skipped.Code, skipped.Body.String())
	}
	projectParent := serveAuthorized(router, http.MethodPost, "/api/groups",
		[]byte(`{"name":"Project Alpha","type":"project","parent_group_id":"`+root.ID+`"}`), token)
	if projectParent.Code != http.StatusBadRequest {
		t.Fatalf("project parent: expected 400, got %d: %s", projectParent.Code, projectParent.Body.String())
	}
	levelSix := serveAuthorized(router, http.MethodPost, "/api/groups",
		[]byte(`{"name":"Too Deep","type":"organization","organization_level":6}`), token)
	if levelSix.Code != http.StatusBadRequest {
		t.Fatalf("level 6: expected 400, got %d: %s", levelSix.Code, levelSix.Body.String())
	}
}

func TestDelegatedRoleManagerCannotEditRolePermissions(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)
	userResponse := serveAuthorized(router, http.MethodPost, "/api/users", []byte(`{"display_name":"Role Operator","email":"operator@example.com","password":"a-secure-user-password"}`), adminToken)
	var operator system.User
	if err := json.NewDecoder(userResponse.Body).Decode(&operator); err != nil {
		t.Fatal(err)
	}
	roleResponse := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{"title":"Role Operator"}`), adminToken)
	var role system.Role
	if err := json.NewDecoder(roleResponse.Body).Decode(&role); err != nil {
		t.Fatal(err)
	}
	for _, permissionID := range []string{"roles.read", "roles.manage", "roles.assign", "permissions.assign"} {
		response := serveAuthorized(router, http.MethodPut, "/api/roles/"+role.ID+"/permissions/"+permissionID, nil, adminToken)
		if response.Code != http.StatusNoContent {
			t.Fatalf("grant %s: %d %s", permissionID, response.Code, response.Body.String())
		}
	}
	assign := serveAuthorized(router, http.MethodPut, "/api/roles/"+role.ID+"/users/"+operator.ID, nil, adminToken)
	if assign.Code != http.StatusNoContent {
		t.Fatalf("assign delegated role: %d %s", assign.Code, assign.Body.String())
	}
	loginResponse := serve(router, http.MethodPost, "/api/auth/login", []byte(`{"email":"operator@example.com","password":"a-secure-user-password"}`))
	var login struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(loginResponse.Body).Decode(&login)
	created := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{"title":"Delegated Role"}`), login.AccessToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("delegated role create: %d %s", created.Code, created.Body.String())
	}
	var delegated system.Role
	json.NewDecoder(created.Body).Decode(&delegated)
	// permissions.assign does not allow a delegated role manager to change any
	// role permission, including one the operator already possesses.
	for _, permissionID := range []string{"roles.manage", "users.manage"} {
		denied := serveAuthorized(router, http.MethodPut, "/api/roles/"+delegated.ID+"/permissions/"+permissionID, nil, login.AccessToken)
		if denied.Code != http.StatusForbidden {
			t.Fatalf("grant %s: expected %d, got %d", permissionID, http.StatusForbidden, denied.Code)
		}
	}
	deniedRevoke := serveAuthorized(router, http.MethodDelete, "/api/roles/"+role.ID+"/permissions/roles.manage", nil, login.AccessToken)
	if deniedRevoke.Code != http.StatusForbidden {
		t.Fatalf("revoke role permission: expected %d, got %d", http.StatusForbidden, deniedRevoke.Code)
	}
	readRole := serveAuthorized(router, http.MethodGet, "/api/roles/"+role.ID, nil, login.AccessToken)
	if readRole.Code != http.StatusOK {
		t.Fatalf("read assigned permissions: expected %d, got %d", http.StatusOK, readRole.Code)
	}
	var readable system.Role
	if err := json.NewDecoder(readRole.Body).Decode(&readable); err != nil {
		t.Fatal(err)
	}
	if len(readable.Permissions) != 4 {
		t.Fatalf("expected delegated user to read 4 assigned permissions, got %+v", readable.Permissions)
	}
	deleted := serveAuthorized(router, http.MethodDelete, "/api/roles/"+role.ID, nil, adminToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete assigned role: %d %s", deleted.Code, deleted.Body.String())
	}
	operatorResponse := serveAuthorized(router, http.MethodGet, "/api/users/"+operator.ID, nil, adminToken)
	var remaining system.User
	if err := json.NewDecoder(operatorResponse.Body).Decode(&remaining); err != nil {
		t.Fatal(err)
	}
	for _, assigned := range remaining.Roles {
		if assigned.ID == role.ID {
			t.Fatal("deleted custom role remained assigned")
		}
	}
}

func TestRequestPermissionUsesUnionOfUserRoles(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)
	userResponse := serveAuthorized(router, http.MethodPost, "/api/users", []byte(`{"display_name":"Multi Role","email":"multi-role@example.com","password":"a-secure-user-password"}`), adminToken)
	var user system.User
	if err := json.NewDecoder(userResponse.Body).Decode(&user); err != nil {
		t.Fatal(err)
	}
	createRole := func(title string) system.Role {
		response := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{"title":"`+title+`"}`), adminToken)
		var role system.Role
		if err := json.NewDecoder(response.Body).Decode(&role); err != nil {
			t.Fatal(err)
		}
		return role
	}
	emptyRole := createRole("Empty Role")
	readerRole := createRole("Role Reader")
	if response := serveAuthorized(router, http.MethodPut, "/api/roles/"+readerRole.ID+"/permissions/roles.read", nil, adminToken); response.Code != http.StatusNoContent {
		t.Fatalf("grant roles.read: %d %s", response.Code, response.Body.String())
	}
	for _, role := range []system.Role{emptyRole, readerRole} {
		if response := serveAuthorized(router, http.MethodPut, "/api/roles/"+role.ID+"/users/"+user.ID, nil, adminToken); response.Code != http.StatusNoContent {
			t.Fatalf("assign %s: %d %s", role.Title, response.Code, response.Body.String())
		}
	}
	loginResponse := serve(router, http.MethodPost, "/api/auth/login", []byte(`{"email":"multi-role@example.com","password":"a-secure-user-password"}`))
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	if response := serveAuthorized(router, http.MethodGet, "/api/roles", nil, login.AccessToken); response.Code != http.StatusOK {
		t.Fatalf("permission from second role was not accepted: %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{"title":"Forbidden"}`), login.AccessToken); response.Code != http.StatusForbidden {
		t.Fatalf("missing roles.manage: expected %d, got %d", http.StatusForbidden, response.Code)
	}
	logResponse := serveAuthorized(router, http.MethodGet, "/api/logs?status=warning&limit=100", nil, adminToken)
	var logResult applogs.QueryResult
	if err := json.NewDecoder(logResponse.Body).Decode(&logResult); err != nil {
		t.Fatal(err)
	}
	foundDenial := false
	for _, record := range logResult.Logs {
		if record.UserID == user.ID && record.Content == "permission denied: POST /api/roles requires roles.manage" {
			foundDenial = true
			break
		}
	}
	if !foundDenial {
		t.Fatalf("permission denial was not attributed to user %s: %+v", user.ID, logResult.Logs)
	}
}

func TestPermissionCatalogIsConfigManaged(t *testing.T) {
	router := testRouter(t)
	token, _ := loginForTest(t, router)
	listed := serveAuthorized(router, http.MethodGet, "/api/permissions", nil, token)
	if listed.Code != http.StatusOK {
		t.Fatalf("list permissions: %d", listed.Code)
	}
	var response struct {
		Permissions []system.Permission `json:"permissions"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	catalog, err := system.LoadPermissionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Permissions) != len(catalog.Permissions) {
		t.Fatalf("database catalog has %d permissions, config has %d", len(response.Permissions), len(catalog.Permissions))
	}
	if created := serveAuthorized(router, http.MethodPost, "/api/permissions", []byte(`{}`), token); created.Code != http.StatusNotFound {
		t.Fatalf("permission definitions must be config-managed: got %d", created.Code)
	}
}

func TestGroupCreatesRolesAndManagerCanManageOwnMembers(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)
	createUser := func(name, email string) system.User {
		response := serveAuthorized(router, http.MethodPost, "/api/users", []byte(`{"display_name":"`+name+`","email":"`+email+`","password":"a-secure-user-password"}`), adminToken)
		if response.Code != http.StatusCreated {
			t.Fatalf("create user: %s", response.Body.String())
		}
		var user system.User
		if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
			t.Fatal(err)
		}
		return user
	}
	manager := createUser("Group Manager", "manager@example.com")
	member := createUser("Group Member", "member@example.com")

	groupResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"North Branch","type":"project"}`), adminToken)
	var group system.Group
	if err := json.NewDecoder(groupResponse.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	if group.ManagerRoleID == "" || group.MemberRoleID == "" {
		t.Fatalf("expected generated group roles: %+v", group)
	}
	otherResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"South Branch","type":"project"}`), adminToken)
	var other system.Group
	if err := json.NewDecoder(otherResponse.Body).Decode(&other); err != nil {
		t.Fatal(err)
	}

	assign := serveAuthorized(router, http.MethodPut, "/api/groups/"+group.ID+"/members/"+manager.ID, []byte(`{"role":"manager","title":"Branch Manager"}`), adminToken)
	if assign.Code != http.StatusOK {
		t.Fatalf("assign manager: expected %d, got %d: %s", http.StatusOK, assign.Code, assign.Body.String())
	}

	loginResponse := serve(router, http.MethodPost, "/api/auth/login", []byte(`{"email":"manager@example.com","password":"a-secure-user-password"}`))
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	addMember := serveAuthorized(router, http.MethodPut, "/api/groups/"+group.ID+"/members/"+member.ID, []byte(`{"role":"member"}`), login.AccessToken)
	if addMember.Code != http.StatusForbidden {
		t.Fatalf("generated role without groups.assign: expected %d, got %d: %s", http.StatusForbidden, addMember.Code, addMember.Body.String())
	}
	denied := serveAuthorized(router, http.MethodPut, "/api/groups/"+other.ID+"/members/"+member.ID, []byte(`{"role":"member"}`), login.AccessToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("manage other group: expected %d, got %d: %s", http.StatusForbidden, denied.Code, denied.Body.String())
	}
	list := serveAuthorized(router, http.MethodGet, "/api/groups/"+group.ID+"/members", nil, adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list own members: %s", list.Body.String())
	}
	var members struct {
		Members []system.GroupMember `json:"members"`
	}
	if err := json.NewDecoder(list.Body).Decode(&members); err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 1 {
		t.Fatalf("expected only manager, got %d", len(members.Members))
	}
	deleted := serveAuthorized(router, http.MethodDelete, "/api/groups/"+group.ID, nil, adminToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete group with members: expected %d, got %d: %s", http.StatusNoContent, deleted.Code, deleted.Body.String())
	}
	userResponse := serveAuthorized(router, http.MethodGet, "/api/users/"+manager.ID, nil, adminToken)
	var remaining system.User
	if err := json.NewDecoder(userResponse.Body).Decode(&remaining); err != nil {
		t.Fatal(err)
	}
	for _, assignedRole := range remaining.Roles {
		if assignedRole.ID == group.ManagerRoleID || assignedRole.ID == group.MemberRoleID {
			t.Fatal("generated group role remained assigned after group deletion")
		}
	}
	for _, assignedGroup := range remaining.Groups {
		if assignedGroup.ID == group.ID {
			t.Fatal("group membership remained after group deletion")
		}
	}
}

func TestLoginRefreshMeAndLogout(t *testing.T) {
	router := testRouter(t)
	accessToken, refreshCookie := loginForTest(t, router)

	meResponse := serveAuthorized(router, http.MethodGet, "/api/auth/me", nil, accessToken)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("me: expected %d, got %d: %s", http.StatusOK, meResponse.Code, meResponse.Body.String())
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	refreshRequest.AddCookie(refreshCookie)
	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh: expected %d, got %d: %s", http.StatusOK, refreshResponse.Code, refreshResponse.Body.String())
	}
	var refreshed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(refreshResponse.Body).Decode(&refreshed); err != nil {
		t.Fatal(err)
	}
	refreshCookies := refreshResponse.Result().Cookies()
	if len(refreshCookies) == 0 || refreshed.AccessToken == "" {
		t.Fatal("refresh did not rotate tokens")
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer "+refreshed.AccessToken)
	logoutRequest.AddCookie(refreshCookies[0])
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout: expected %d, got %d", http.StatusNoContent, logoutResponse.Code)
	}
}

func TestReadLogsAPI(t *testing.T) {
	router := testRouter(t)
	accessToken, _ := loginForTest(t, router)
	response := serveAuthorized(router, http.MethodGet, "/api/logs?status=info&limit=10", nil, accessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("logs: expected %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	var result applogs.QueryResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected login log")
	}
	if result.Logs[0].UserID == "" || result.Logs[0].Content != "login succeeded" {
		t.Fatalf("unexpected log: %+v", result.Logs[0])
	}
}

func TestAttendanceAPIUsesRolePermissionsAndAllowsRepeatedSessions(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)
	userResponse := serveAuthorized(router, http.MethodPost, "/api/users", []byte(`{
		"display_name":"Attendance Operator",
		"email":"attendance-operator@example.com",
		"password":"a-secure-user-password",
		"timezone":"Asia/Taipei"
	}`), adminToken)
	var user system.User
	if err := json.NewDecoder(userResponse.Body).Decode(&user); err != nil {
		t.Fatal(err)
	}
	loginResponse := serve(router, http.MethodPost, "/api/auth/login", []byte(`{
		"email":"attendance-operator@example.com",
		"password":"a-secure-user-password"
	}`))
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	if response := serveAuthorized(router, http.MethodGet, "/api/attendance/today", nil, login.AccessToken); response.Code != http.StatusForbidden {
		t.Fatalf("attendance without role permission: expected %d, got %d", http.StatusForbidden, response.Code)
	}
	roleResponse := serveAuthorized(router, http.MethodPost, "/api/roles", []byte(`{"title":"Attendance User"}`), adminToken)
	var role system.Role
	if err := json.NewDecoder(roleResponse.Body).Decode(&role); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"attendance.read.self", "attendance.clock.self"} {
		response := serveAuthorized(router, http.MethodPut, "/api/roles/"+role.ID+"/permissions/"+permission, nil, adminToken)
		if response.Code != http.StatusNoContent {
			t.Fatalf("grant %s: %d %s", permission, response.Code, response.Body.String())
		}
	}
	if response := serveAuthorized(router, http.MethodPut, "/api/roles/"+role.ID+"/users/"+user.ID, nil, adminToken); response.Code != http.StatusNoContent {
		t.Fatalf("assign attendance role: %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		"/api/attendance/today/sign-in",
		"/api/attendance/today/sign-out",
		"/api/attendance/today/sign-in",
		"/api/attendance/today/sign-out",
	} {
		response := serveAuthorized(router, http.MethodPost, path, []byte(`{}`), login.AccessToken)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
	}
	today := serveAuthorized(router, http.MethodGet, "/api/attendance/today", nil, login.AccessToken)
	var day system.AttendanceDay
	if err := json.NewDecoder(today.Body).Decode(&day); err != nil {
		t.Fatal(err)
	}
	if day.Status != system.AttendanceNonWorking || len(day.Sessions) != 2 {
		t.Fatalf("attendance day: %+v", day)
	}
}

func TestAttendanceCSVAPI(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)
	generated := serveAuthorized(router, http.MethodPost, "/api/attendance/reports/2020-01/generate", []byte(`{}`), adminToken)
	if generated.Code != http.StatusOK {
		t.Fatalf("generate CSV: %d %s", generated.Code, generated.Body.String())
	}
	download := serveAuthorized(router, http.MethodGet, "/api/attendance/reports/2020-01/csv", nil, adminToken)
	if download.Code != http.StatusOK {
		t.Fatalf("download CSV: %d %s", download.Code, download.Body.String())
	}
	if contentType := download.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("CSV content type: %q", contentType)
	}
	if !bytes.HasPrefix(download.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("downloaded CSV does not have UTF-8 BOM")
	}
}

func TestNotificationAPI(t *testing.T) {
	router := testRouter(t)
	adminToken, _ := loginForTest(t, router)

	typesResponse := serveAuthorized(router, http.MethodGet, "/api/notifications/types", nil, adminToken)
	if typesResponse.Code != http.StatusOK {
		t.Fatalf("list notification types: %d %s", typesResponse.Code, typesResponse.Body.String())
	}
	var typesBody struct {
		Types []system.NotificationType `json:"notification_types"`
	}
	if err := json.NewDecoder(typesResponse.Body).Decode(&typesBody); err != nil {
		t.Fatal(err)
	}
	if len(typesBody.Types) < 5 {
		t.Fatalf("expected default notification types, got %+v", typesBody.Types)
	}

	createResponse := serveAuthorized(router, http.MethodPost, "/api/notifications", []byte(`{
		"title":"Maintenance",
		"message":"System maintenance tonight.",
		"type":"important",
		"show_time":"day",
		"audiences":[{"scope":"organization"}]
	}`), adminToken)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create notification: %d %s", createResponse.Code, createResponse.Body.String())
	}
	var created system.Notification
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Status != "active" || created.Type.Code != "important" {
		t.Fatalf("unexpected notification: %+v", created)
	}

	listResponse := serveAuthorized(router, http.MethodGet, "/api/notifications", nil, adminToken)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list notifications: %d %s", listResponse.Code, listResponse.Body.String())
	}
	var listBody struct {
		Notifications []system.Notification `json:"notifications"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Notifications) != 1 || listBody.Notifications[0].ID != created.ID {
		t.Fatalf("expected created notification in inbox, got %+v", listBody.Notifications)
	}

	updateResponse := serveAuthorized(router, http.MethodPatch, "/api/notifications/"+created.ID, []byte(`{"title":"Updated maintenance"}`), adminToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update notification: %d %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated system.Notification
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status != "edited" || updated.Title != "Updated maintenance" {
		t.Fatalf("unexpected updated notification: %+v", updated)
	}

	csvResponse := serveAuthorized(router, http.MethodGet, "/api/notifications/exports/"+updated.CreatedAt[:7]+"/csv", nil, adminToken)
	if csvResponse.Code != http.StatusOK {
		t.Fatalf("export notifications csv: %d %s", csvResponse.Code, csvResponse.Body.String())
	}
	if contentType := csvResponse.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("CSV content type: %q", contentType)
	}
	if !strings.Contains(csvResponse.Body.String(), "Updated maintenance") {
		t.Fatalf("CSV missing updated notification: %s", csvResponse.Body.String())
	}

	hideResponse := serveAuthorized(router, http.MethodPost, "/api/notifications/"+created.ID+"/hide", nil, adminToken)
	if hideResponse.Code != http.StatusNoContent {
		t.Fatalf("hide notification: %d %s", hideResponse.Code, hideResponse.Body.String())
	}
	hiddenList := serveAuthorized(router, http.MethodGet, "/api/notifications", nil, adminToken)
	if hiddenList.Code != http.StatusOK {
		t.Fatalf("list hidden notifications: %d %s", hiddenList.Code, hiddenList.Body.String())
	}
	var hiddenBody struct {
		Notifications []system.Notification `json:"notifications"`
	}
	if err := json.NewDecoder(hiddenList.Body).Decode(&hiddenBody); err != nil {
		t.Fatal(err)
	}
	if len(hiddenBody.Notifications) != 0 {
		t.Fatalf("hidden notification should not appear, got %+v", hiddenBody.Notifications)
	}

	adminList := serveAuthorized(router, http.MethodGet, "/api/notifications/admin", nil, adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list notifications: %d %s", adminList.Code, adminList.Body.String())
	}
	var adminBody struct {
		Notifications []system.Notification `json:"notifications"`
	}
	if err := json.NewDecoder(adminList.Body).Decode(&adminBody); err != nil {
		t.Fatal(err)
	}
	if len(adminBody.Notifications) != 1 || adminBody.Notifications[0].Status != "hidden" {
		t.Fatalf("admin list should include hidden notification, got %+v", adminBody.Notifications)
	}
}

func loginForTest(t *testing.T, router http.Handler) (string, *http.Cookie) {
	t.Helper()
	response := serve(router, http.MethodPost, "/api/auth/login", []byte(`{"email":"admin@nexgestion.local","password":"a-secure-test-password"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("login: expected %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	cookies := response.Result().Cookies()
	if result.AccessToken == "" || len(cookies) == 0 {
		t.Fatal("login did not return access and refresh tokens")
	}
	return result.AccessToken, cookies[0]
}

func serveAuthorized(router http.Handler, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func serve(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
