#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
ROLE_ID=""
USER_ID=""
DELEGATE_ROLE_ID=""
DELEGATE_USER_ID=""

create_role() {
  local title="$1"
  local description="${2:-API test role}"
  local body
  body="$(jq -cn --arg title "$title" --arg description "$description" '{title:$title,description:$description}')"
  api_request POST /api/roles "$body" yes
}

create_user() {
  local email="$1"
  local body
  body="$(jq -cn --arg email "$email" '{display_name:"API Role User",email:$email,password:"TempPass12345",status:"active",must_change_password:false}')"
  api_request POST /api/users "$body" yes
}

cleanup_role() {
  local id="$1"
  [[ -n "$id" ]] && expect_status "ROLE-CLEANUP-${id:0:8}" "$LEVEL" "Cleanup custom role when deletion is allowed" DELETE "/api/roles/$id" 204 "" yes || true
}

cleanup_user() {
  local id="$1"
  [[ -n "$id" ]] && expect_status "ROLE-USER-CLEANUP-${id:0:8}" "$LEVEL" "Cleanup user created by role tests" DELETE "/api/users/$id" 204 "" yes || true
}

permission_id_for() {
  local key="$1"
  expect_status "ROLE-PERM-LOOKUP" "$LEVEL" "Permission catalog can be listed for setup" GET /api/permissions 200 "" yes >/dev/null || return 1
  jq -r --arg key "$key" '.permissions[] | select(.permission_key == $key) | .id' "$RESPONSE_BODY" | head -1
}

run_level_1() {
  expect_status "ROLE-L1-001" 1 "Authenticated admin can list roles" GET /api/roles 200 "" yes

  local status
  status="$(create_role "API Role L1 ${RUN_SUFFIX}")"
  if [[ "$status" == "201" ]]; then
    ROLE_ID="$(response_value '.id // empty')"
    record_case "ROLE-L1-002" 1 "Admin can create a custom role" POST /api/roles 201 "$status" PASS
  else
    record_case "ROLE-L1-002" 1 "Admin can create a custom role" POST /api/roles 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$ROLE_ID" ]]; then
    expect_status "ROLE-L1-003" 1 "Admin can read the created role" GET "/api/roles/$ROLE_ID" 200 "" yes
    expect_status "ROLE-L1-004" 1 "Admin can update a custom role" PATCH "/api/roles/$ROLE_ID" 200 '{"description":"Updated by API test"}' yes
    expect_status "ROLE-L1-005" 1 "Admin can delete a custom role" DELETE "/api/roles/$ROLE_ID" 204 "" yes
    ROLE_ID=""
  fi
}

run_level_2() {
  expect_status "ROLE-L2-001" 2 "Role list rejects missing authentication" GET /api/roles 401 "" no
  expect_status "ROLE-L2-002" 2 "Create rejects invalid JSON" POST /api/roles 400 '{"title":' yes
  expect_status "ROLE-L2-003" 2 "Create rejects empty title" POST /api/roles 400 '{"title":"   "}' yes
  expect_status "ROLE-L2-004" 2 "Unknown role returns not found" GET /api/roles/00000000-0000-0000-0000-00000000dead 404 "" yes
  expect_status "ROLE-L2-005" 2 "Admin system role cannot be updated" PATCH /api/roles/00000000-0000-0000-0000-000000000001 403 '{"title":"Not Admin"}' yes
  expect_status "ROLE-L2-006" 2 "Admin system role cannot be deleted" DELETE /api/roles/00000000-0000-0000-0000-000000000001 403 "" yes

  local title="API Role L2 ${RUN_SUFFIX}"
  local status
  status="$(create_role "$title")"
  if [[ "$status" == "201" ]]; then
    ROLE_ID="$(response_value '.id // empty')"
    record_case "ROLE-L2-007" 2 "Create accepts unique role title" POST /api/roles 201 "$status" PASS
  else
    record_case "ROLE-L2-007" 2 "Create accepts unique role title" POST /api/roles 201 "$status" FAIL "$(response_value '.error // empty')"
  fi
  expect_status "ROLE-L2-008" 2 "Create rejects duplicate role title case-insensitively" POST /api/roles 409 "$(jq -cn --arg title "${title^^}" '{title:$title}')" yes
  cleanup_role "$ROLE_ID"
  ROLE_ID=""
}

run_level_3() {
  local status
  status="$(create_role "API Role L3 ${RUN_SUFFIX}")"
  if [[ "$status" == "201" ]]; then
    ROLE_ID="$(response_value '.id // empty')"
    record_case "ROLE-L3-001" 3 "Create custom role for assignment lifecycle" POST /api/roles 201 "$status" PASS
  else
    record_case "ROLE-L3-001" 3 "Create custom role for assignment lifecycle" POST /api/roles 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  status="$(create_user "api-role-user-${RUN_SUFFIX}@example.test")"
  if [[ "$status" == "201" ]]; then
    USER_ID="$(response_value '.id // empty')"
    record_case "ROLE-L3-002" 3 "Create non-protected user for role assignment" POST /api/users 201 "$status" PASS
  else
    record_case "ROLE-L3-002" 3 "Create non-protected user for role assignment" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$ROLE_ID" && -n "$USER_ID" ]]; then
    expect_status "ROLE-L3-003" 3 "Assign custom role to non-protected user" PUT "/api/roles/$ROLE_ID/users/$USER_ID" 204 "" yes
    expect_status "ROLE-L3-004" 3 "List role users after assignment" GET "/api/roles/$ROLE_ID/users" 200 "" yes
    expect_status "ROLE-L3-005" 3 "Remove custom role from user" DELETE "/api/roles/$ROLE_ID/users/$USER_ID" 204 "" yes
  fi

  local permission_id
  permission_id="$(permission_id_for "users.read")"
  if [[ -n "$ROLE_ID" && -n "$permission_id" ]]; then
    expect_status "ROLE-L3-006" 3 "Initial administrator can grant role permission" PUT "/api/roles/$ROLE_ID/permissions/$permission_id" 204 "" yes
    expect_status "ROLE-L3-007" 3 "Initial administrator can revoke role permission" DELETE "/api/roles/$ROLE_ID/permissions/$permission_id" 204 "" yes
  else
    record_case "ROLE-L3-006" 3 "Initial administrator can grant role permission" PUT "/api/roles/<id>/permissions/<id>" 204 "SKIP" FAIL "missing role or permission id"
    record_case "ROLE-L3-007" 3 "Initial administrator can revoke role permission" DELETE "/api/roles/<id>/permissions/<id>" 204 "SKIP" FAIL "missing role or permission id"
  fi

  expect_status "ROLE-L3-008" 3 "Admin role cannot be assigned to another user" PUT "/api/roles/00000000-0000-0000-0000-000000000001/users/$USER_ID" 403 "" yes

  cleanup_role "$ROLE_ID"; ROLE_ID=""
  cleanup_user "$USER_ID"; USER_ID=""
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
