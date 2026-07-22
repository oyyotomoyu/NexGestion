package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	logService, err := applogs.NewService(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logService.Close)
	InitRouter(router, users, system.NewAuthService(users), logService)
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

	response := serveAuthorized(router, http.MethodPatch, "/api/roles/"+adminRoleID, []byte(`{"title":"Owner"}`), accessToken)
	if response.Code != http.StatusForbidden {
		t.Fatalf("update Admin role: expected %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
	}
	response = serveAuthorized(router, http.MethodDelete, "/api/roles/"+adminRoleID, nil, accessToken)
	if response.Code != http.StatusForbidden {
		t.Fatalf("delete Admin role: expected %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
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

	parentResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"Head Office","type":"branch"}`), accessToken)
	if parentResponse.Code != http.StatusCreated {
		t.Fatalf("create parent group: expected %d, got %d: %s", http.StatusCreated, parentResponse.Code, parentResponse.Body.String())
	}
	var parent system.Group
	if err := json.NewDecoder(parentResponse.Body).Decode(&parent); err != nil {
		t.Fatal(err)
	}

	childResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"Finance","type":"department","parent_group_id":"`+parent.ID+`"}`), accessToken)
	if childResponse.Code != http.StatusCreated {
		t.Fatalf("create child group: expected %d, got %d: %s", http.StatusCreated, childResponse.Code, childResponse.Body.String())
	}
	var child system.Group
	if err := json.NewDecoder(childResponse.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	if child.ParentGroupID == nil || *child.ParentGroupID != parent.ID || child.Status != "active" || child.Permissions == nil {
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
	grantResponse := serveAuthorized(router, http.MethodPut, "/api/groups/"+child.ID+"/permissions/groups.read", nil, accessToken)
	if grantResponse.Code != http.StatusNoContent {
		t.Fatalf("grant group permission: %d %s", grantResponse.Code, grantResponse.Body.String())
	}
	renameResponse := serveAuthorized(router, http.MethodPatch, "/api/groups/"+child.ID, []byte(`{"name":"Accounting"}`), accessToken)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename group: %d %s", renameResponse.Code, renameResponse.Body.String())
	}
	var renamed system.Group
	if err := json.NewDecoder(renameResponse.Body).Decode(&renamed); err != nil {
		t.Fatal(err)
	}
	if len(renamed.Permissions) != 1 || renamed.Permissions[0].PermissionKey != "groups.read" {
		t.Fatalf("group permission not returned: %+v", renamed.Permissions)
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

func TestPermissionGrantsDelegateRoleManagement(t *testing.T) {
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
	for _, permissionID := range []string{"roles.manage", "roles.assign", "permissions.assign"} {
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
	// The operator cannot grant a permission they do not possess.
	denied := serveAuthorized(router, http.MethodPut, "/api/roles/"+delegated.ID+"/permissions/users.manage", nil, login.AccessToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("privilege escalation: expected %d, got %d", http.StatusForbidden, denied.Code)
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

func TestPermissionCatalogManagement(t *testing.T) {
	router := testRouter(t)
	token, _ := loginForTest(t, router)
	created := serveAuthorized(router, http.MethodPost, "/api/permissions", []byte(`{"permission_key":"inventory.write","module":"inventory","description":"Write inventory"}`), token)
	if created.Code != http.StatusCreated {
		t.Fatalf("create permission: %d %s", created.Code, created.Body.String())
	}
	var permission system.Permission
	if err := json.NewDecoder(created.Body).Decode(&permission); err != nil {
		t.Fatal(err)
	}
	updated := serveAuthorized(router, http.MethodPatch, "/api/permissions/"+permission.ID, []byte(`{"description":"Manage inventory"}`), token)
	if updated.Code != http.StatusOK {
		t.Fatalf("update permission: %d %s", updated.Code, updated.Body.String())
	}
	listed := serveAuthorized(router, http.MethodGet, "/api/permissions", nil, token)
	if listed.Code != http.StatusOK {
		t.Fatalf("list permissions: %d", listed.Code)
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

	groupResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"North Branch","type":"branch"}`), adminToken)
	var group system.Group
	if err := json.NewDecoder(groupResponse.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	if group.ManagerRoleID == "" || group.MemberRoleID == "" {
		t.Fatalf("expected generated group roles: %+v", group)
	}
	otherResponse := serveAuthorized(router, http.MethodPost, "/api/groups", []byte(`{"name":"South Branch","type":"branch"}`), adminToken)
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
	if addMember.Code != http.StatusOK {
		t.Fatalf("manager add member: expected %d, got %d: %s", http.StatusOK, addMember.Code, addMember.Body.String())
	}
	denied := serveAuthorized(router, http.MethodPut, "/api/groups/"+other.ID+"/members/"+member.ID, []byte(`{"role":"member"}`), login.AccessToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("manage other group: expected %d, got %d: %s", http.StatusForbidden, denied.Code, denied.Body.String())
	}
	list := serveAuthorized(router, http.MethodGet, "/api/groups/"+group.ID+"/members", nil, login.AccessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list own members: %s", list.Body.String())
	}
	var members struct {
		Members []system.GroupMember `json:"members"`
	}
	if err := json.NewDecoder(list.Body).Decode(&members); err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 2 {
		t.Fatalf("expected manager and member, got %d", len(members.Members))
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
