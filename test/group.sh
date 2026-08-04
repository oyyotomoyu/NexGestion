#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
GROUP_ID=""
PARENT_ID=""
CHILD_ID=""
USER_ID=""
MANAGER_ROLE_ID=""
MEMBER_ROLE_ID=""

create_org_group() {
  local name="$1"
  local level="$2"
  local parent="${3:-}"
  local body
  if [[ -n "$parent" ]]; then
    body="$(jq -cn --arg name "$name" --arg parent "$parent" --argjson level "$level" '{name:$name,type:"organization",organization_level:$level,parent_group_id:$parent,status:"active"}')"
  else
    body="$(jq -cn --arg name "$name" --argjson level "$level" '{name:$name,type:"organization",organization_level:$level,status:"active"}')"
  fi
  api_request POST /api/groups "$body" yes
}

create_user() {
  local email="$1"
  local body
  body="$(jq -cn --arg email "$email" '{display_name:"API Group User",email:$email,password:"TempPass12345",status:"active",must_change_password:false}')"
  api_request POST /api/users "$body" yes
}

cleanup_group() {
  local id="$1"
  [[ -n "$id" ]] && expect_status "GROUP-CLEANUP-${id:0:8}" "$LEVEL" "Cleanup group when leaf deletion is allowed" DELETE "/api/groups/$id" 204 "" yes || true
}

cleanup_user() {
  local id="$1"
  [[ -n "$id" ]] && expect_status "GROUP-USER-CLEANUP-${id:0:8}" "$LEVEL" "Cleanup user created by group tests" DELETE "/api/users/$id" 204 "" yes || true
}

run_level_1() {
  expect_status "GROUP-L1-001" 1 "Authenticated admin can list groups" GET /api/groups 200 "" yes

  local status
  status="$(create_org_group "API Group L1 ${RUN_SUFFIX}" 1)"
  if [[ "$status" == "201" ]]; then
    GROUP_ID="$(response_value '.id // empty')"
    record_case "GROUP-L1-002" 1 "Admin can create a root organization group" POST /api/groups 201 "$status" PASS
  else
    record_case "GROUP-L1-002" 1 "Admin can create a root organization group" POST /api/groups 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$GROUP_ID" ]]; then
    expect_status "GROUP-L1-003" 1 "Admin can read created group" GET "/api/groups/$GROUP_ID" 200 "" yes
    expect_status "GROUP-L1-004" 1 "Admin can update group metadata" PATCH "/api/groups/$GROUP_ID" 200 '{"status":"inactive"}' yes
    expect_status "GROUP-L1-005" 1 "Admin can delete a leaf group" DELETE "/api/groups/$GROUP_ID" 204 "" yes
    GROUP_ID=""
  fi
}

run_level_2() {
  expect_status "GROUP-L2-001" 2 "Group list rejects missing authentication" GET /api/groups 401 "" no
  expect_status "GROUP-L2-002" 2 "Create rejects invalid JSON" POST /api/groups 400 '{"name":' yes
  expect_status "GROUP-L2-003" 2 "Create rejects missing name" POST /api/groups 400 '{"type":"organization","organization_level":1}' yes
  expect_status "GROUP-L2-004" 2 "Create rejects invalid group type" POST /api/groups 400 '{"name":"Bad","type":"bad","organization_level":1}' yes
  expect_status "GROUP-L2-005" 2 "Create rejects organization without required parent below level one" POST /api/groups 400 '{"name":"Bad Child","type":"organization","organization_level":2}' yes
  expect_status "GROUP-L2-006" 2 "Unknown group returns not found" GET /api/groups/00000000-0000-0000-0000-00000000dead 404 "" yes

  local status
  status="$(create_org_group "API Group L2 ${RUN_SUFFIX}" 1)"
  if [[ "$status" == "201" ]]; then
    GROUP_ID="$(response_value '.id // empty')"
    record_case "GROUP-L2-007" 2 "Create accepts unique group name" POST /api/groups 201 "$status" PASS
  else
    record_case "GROUP-L2-007" 2 "Create accepts unique group name" POST /api/groups 201 "$status" FAIL "$(response_value '.error // empty')"
  fi
  expect_status "GROUP-L2-008" 2 "Create rejects duplicate group name" POST /api/groups 409 "$(jq -cn --arg name "API Group L2 ${RUN_SUFFIX}" '{name:$name,type:"organization",organization_level:1,status:"active"}')" yes

  if [[ -n "$GROUP_ID" ]]; then
    status="$(create_org_group "API Group L2 Child ${RUN_SUFFIX}" 2 "$GROUP_ID")"
    if [[ "$status" == "201" ]]; then
      CHILD_ID="$(response_value '.id // empty')"
      record_case "GROUP-L2-009" 2 "Create child group with valid parent" POST /api/groups 201 "$status" PASS
    else
      record_case "GROUP-L2-009" 2 "Create child group with valid parent" POST /api/groups 201 "$status" FAIL "$(response_value '.error // empty')"
    fi
    expect_status "GROUP-L2-010" 2 "Parent group with child cannot be deleted" DELETE "/api/groups/$GROUP_ID" 409 "" yes
  fi

  cleanup_group "$CHILD_ID"; CHILD_ID=""
  cleanup_group "$GROUP_ID"; GROUP_ID=""
}

run_level_3() {
  local status
  status="$(create_org_group "API Group L3 ${RUN_SUFFIX}" 1)"
  if [[ "$status" == "201" ]]; then
    GROUP_ID="$(response_value '.id // empty')"
    MANAGER_ROLE_ID="$(response_value '.manager_role_id // empty')"
    MEMBER_ROLE_ID="$(response_value '.member_role_id // empty')"
    record_case "GROUP-L3-001" 3 "Create group with generated manager/member roles" POST /api/groups 201 "$status" PASS
  else
    record_case "GROUP-L3-001" 3 "Create group with generated manager/member roles" POST /api/groups 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  status="$(create_user "api-group-user-${RUN_SUFFIX}@example.test")"
  if [[ "$status" == "201" ]]; then
    USER_ID="$(response_value '.id // empty')"
    record_case "GROUP-L3-002" 3 "Create user for membership lifecycle" POST /api/users 201 "$status" PASS
  else
    record_case "GROUP-L3-002" 3 "Create user for membership lifecycle" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$GROUP_ID" && -n "$USER_ID" ]]; then
    expect_status "GROUP-L3-003" 3 "Add user as group member" PUT "/api/groups/$GROUP_ID/members/$USER_ID" 200 '{"role":"member","title":"Staff"}' yes
    expect_status "GROUP-L3-004" 3 "List group members after add" GET "/api/groups/$GROUP_ID/members" 200 "" yes
    expect_status "GROUP-L3-005" 3 "Update membership to manager" PUT "/api/groups/$GROUP_ID/members/$USER_ID" 200 '{"role":"manager","title":"Manager"}' yes
    expect_status "GROUP-L3-006" 3 "Reject invalid membership role" PUT "/api/groups/$GROUP_ID/members/$USER_ID" 400 '{"role":"owner"}' yes
  fi

  if [[ -n "$MANAGER_ROLE_ID" ]]; then
    expect_status "GROUP-L3-007" 3 "Generated group manager role cannot be deleted through Role API" DELETE "/api/roles/$MANAGER_ROLE_ID" 403 "" yes
  fi
  if [[ -n "$MEMBER_ROLE_ID" ]]; then
    expect_status "GROUP-L3-008" 3 "Generated group member role cannot be updated through Role API" PATCH "/api/roles/$MEMBER_ROLE_ID" 403 '{"title":"Bad"}' yes
  fi

  if [[ -n "$GROUP_ID" && -n "$USER_ID" ]]; then
    expect_status "GROUP-L3-009" 3 "Remove group member independently" DELETE "/api/groups/$GROUP_ID/members/$USER_ID" 204 "" yes
    expect_status "GROUP-L3-010" 3 "Removing missing group member returns not found" DELETE "/api/groups/$GROUP_ID/members/$USER_ID" 404 "" yes
  fi

  cleanup_group "$GROUP_ID"; GROUP_ID=""
  if [[ -n "$MANAGER_ROLE_ID" ]]; then
    expect_status "GROUP-L3-011" 3 "Deleting group owner-deletes generated manager role" GET "/api/roles/$MANAGER_ROLE_ID" 404 "" yes
  fi
  if [[ -n "$MEMBER_ROLE_ID" ]]; then
    expect_status "GROUP-L3-012" 3 "Deleting group owner-deletes generated member role" GET "/api/roles/$MEMBER_ROLE_ID" 404 "" yes
  fi
  cleanup_user "$USER_ID"; USER_ID=""
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
