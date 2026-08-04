#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in
  1|2|3) ;;
  *)
    echo "usage: $0 <level: 1|2|3>" >&2
    exit 2
    ;;
esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
CREATED_USER_ID=""
SECOND_USER_ID=""

create_test_user() {
  local email="$1"
  local display_name="$2"
  local password="${3:-TempPass12345}"
  local status="${4:-active}"
  local body
  body="$(jq -cn \
    --arg display_name "$display_name" \
    --arg email "$email" \
    --arg password "$password" \
    --arg status "$status" \
    '{display_name:$display_name,email:$email,password:$password,status:$status,locale:"ENG",timezone:"Asia/Taipei",must_change_password:false}')"
  api_request POST /api/users "$body" yes
}

cleanup_user() {
  local user_id="$1"
  if [[ -n "$user_id" ]]; then
    expect_status "USER-CLEANUP-${user_id:0:8}" "$LEVEL" "Cleanup created user when soft deletion is allowed" DELETE "/api/users/$user_id" 204 "" yes || true
  fi
}

run_level_1() {
  expect_status "USER-L1-001" 1 "Health endpoint is publicly readable" GET /api/health 200 "" no
  expect_status "USER-L1-002" 1 "Authenticated admin can read current user" GET /api/auth/me 200 "" yes
  expect_status "USER-L1-003" 1 "Authenticated admin can list users" GET /api/users 200 "" yes

  local email="api-user-l1-${RUN_SUFFIX}@example.test"
  local status
  status="$(create_test_user "$email" "API User L1")"
  if [[ "$status" == "201" ]]; then
    CREATED_USER_ID="$(response_value '.id // empty')"
    record_case "USER-L1-004" 1 "Admin can create a normal active user" POST /api/users 201 "$status" PASS
  else
    record_case "USER-L1-004" 1 "Admin can create a normal active user" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$CREATED_USER_ID" ]]; then
    expect_status "USER-L1-005" 1 "Admin can read the created user" GET "/api/users/$CREATED_USER_ID" 200 "" yes
    expect_status "USER-L1-006" 1 "Admin can edit the created user" PATCH "/api/users/$CREATED_USER_ID" 200 '{"display_name":"API User L1 Edited"}' yes
    expect_status "USER-L1-007" 1 "Admin can soft-delete a non-protected user" DELETE "/api/users/$CREATED_USER_ID" 204 "" yes
    CREATED_USER_ID=""
  else
    record_case "USER-L1-005" 1 "Admin can read the created user" GET "/api/users/<created>" 200 "SKIP" FAIL "create failed, no user id"
    record_case "USER-L1-006" 1 "Admin can edit the created user" PATCH "/api/users/<created>" 200 "SKIP" FAIL "create failed, no user id"
    record_case "USER-L1-007" 1 "Admin can soft-delete a non-protected user" DELETE "/api/users/<created>" 204 "SKIP" FAIL "create failed, no user id"
  fi
}

run_level_2() {
  expect_status "USER-L2-001" 2 "Protected user list rejects missing authentication" GET /api/users 401 "" no
  expect_status "USER-L2-002" 2 "Create rejects invalid JSON" POST /api/users 400 '{"display_name":' yes
  expect_status "USER-L2-003" 2 "Create rejects missing required fields" POST /api/users 400 '{"display_name":""}' yes
  expect_status "USER-L2-004" 2 "Read unknown user returns not found" GET /api/users/00000000-0000-0000-0000-00000000dead 404 "" yes
  expect_status "USER-L2-005" 2 "Delete protected initial administrator is forbidden" DELETE /api/users/00000000-0000-0000-0000-000000000000 403 "" yes

  local email="api-user-l2-${RUN_SUFFIX}@example.test"
  local status
  status="$(create_test_user "$email" "API User L2")"
  if [[ "$status" == "201" ]]; then
    CREATED_USER_ID="$(response_value '.id // empty')"
    record_case "USER-L2-006" 2 "Create accepts a valid active user" POST /api/users 201 "$status" PASS
  else
    record_case "USER-L2-006" 2 "Create accepts a valid active user" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  expect_status "USER-L2-007" 2 "Create rejects duplicate email case-insensitively" POST /api/users 409 "$(jq -cn --arg email "${email^^}" '{display_name:"Duplicate Email",email:$email,password:"TempPass12345",status:"active"}')" yes

  if [[ -n "$CREATED_USER_ID" ]]; then
    expect_status "USER-L2-008" 2 "Update rejects invalid status" PATCH "/api/users/$CREATED_USER_ID" 400 '{"status":"not-a-status"}' yes
    expect_status "USER-L2-009" 2 "Update rejects short password" PATCH "/api/users/$CREATED_USER_ID" 400 '{"password":"short"}' yes
    expect_status "USER-L2-010" 2 "Delete created user succeeds once" DELETE "/api/users/$CREATED_USER_ID" 204 "" yes
    expect_status "USER-L2-011" 2 "Reading a soft-deleted user returns not found" GET "/api/users/$CREATED_USER_ID" 404 "" yes
    CREATED_USER_ID=""
  fi

  expect_status "USER-L2-012" 2 "Login rejects bad credentials with unauthorized" POST /api/auth/login 401 '{"email":"missing@example.test","password":"wrong-password"}' no
}

run_level_3() {
  local first_email="api-user-l3a-${RUN_SUFFIX}@example.test"
  local second_email="api-user-l3b-${RUN_SUFFIX}@example.test"
  local status

  status="$(create_test_user "$first_email" "API User L3 A")"
  if [[ "$status" == "201" ]]; then
    CREATED_USER_ID="$(response_value '.id // empty')"
    record_case "USER-L3-001" 3 "Create first user for multi-user lifecycle" POST /api/users 201 "$status" PASS
  else
    record_case "USER-L3-001" 3 "Create first user for multi-user lifecycle" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  status="$(create_test_user "$second_email" "API User L3 B")"
  if [[ "$status" == "201" ]]; then
    SECOND_USER_ID="$(response_value '.id // empty')"
    record_case "USER-L3-002" 3 "Create second user for isolation checks" POST /api/users 201 "$status" PASS
  else
    record_case "USER-L3-002" 3 "Create second user for isolation checks" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$CREATED_USER_ID" && -n "$SECOND_USER_ID" ]]; then
    expect_status "USER-L3-003" 3 "Updating one user does not block reading another active user" PATCH "/api/users/$CREATED_USER_ID" 200 '{"display_name":"API User L3 A Updated","timezone":"UTC"}' yes
    expect_status "USER-L3-004" 3 "Second user remains readable after first user update" GET "/api/users/$SECOND_USER_ID" 200 "" yes
    expect_status "USER-L3-005" 3 "Deleting one non-protected user does not delete another user" DELETE "/api/users/$CREATED_USER_ID" 204 "" yes
    CREATED_USER_ID=""
    expect_status "USER-L3-006" 3 "Second user remains readable after first user deletion" GET "/api/users/$SECOND_USER_ID" 200 "" yes
  fi

  cleanup_user "$CREATED_USER_ID"
  cleanup_user "$SECOND_USER_ID"
  SECOND_USER_ID=""

  expect_status "USER-L3-007" 3 "Refresh token rotates through cookie-based refresh endpoint" POST /api/auth/refresh 200 "" no
  ACCESS_TOKEN="$(response_value '.access_token // empty')"
  expect_status "USER-L3-008" 3 "Authenticated user can log out current session" POST /api/auth/logout 204 "" yes
  expect_status "USER-L3-009" 3 "Refresh after logout is rejected" POST /api/auth/refresh 401 "" no
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then
  run_level_2
fi
if [[ "$LEVEL" -ge 3 ]]; then
  run_level_3
fi

exit_for_report
