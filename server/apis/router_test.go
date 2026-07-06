package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	InitRouter(router, users, system.NewAuthService(users))
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
