#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
USER_ID=""
ROLE_ID=""
USER_TOKEN=""
TODAY_ID=""
LEAVE_ID=""

permission_id_for() {
  local key="$1"
  expect_status "ATT-PERM-LOOKUP-${key//./-}" "$LEVEL" "Permission catalog can be listed for setup" GET /api/permissions 200 "" yes >/dev/null || return 1
  jq -r --arg key "$key" '.permissions[] | select(.permission_key == $key) | .id' "$RESPONSE_BODY" | head -1
}

grant_permission() {
  local role_id="$1"
  local key="$2"
  local permission_id
  permission_id="$(permission_id_for "$key")"
  [[ -n "$permission_id" ]] && expect_status "ATT-GRANT-${key//./-}" "$LEVEL" "Grant setup permission $key" PUT "/api/roles/$role_id/permissions/$permission_id" 204 "" yes >/dev/null
}

setup_attendance_user() {
  local email="api-attendance-${RUN_SUFFIX}@example.test"
  local status
  status="$(api_request POST /api/users "$(jq -cn --arg email "$email" '{display_name:"API Attendance User",email:$email,password:"TempPass12345",status:"active",timezone:"Asia/Taipei",must_change_password:false}')" yes)"
  if [[ "$status" == "201" ]]; then
    USER_ID="$(response_value '.id // empty')"
    record_case "ATT-SETUP-USER" "$LEVEL" "Create attendance test user" POST /api/users 201 "$status" PASS
  else
    record_case "ATT-SETUP-USER" "$LEVEL" "Create attendance test user" POST /api/users 201 "$status" FAIL "$(response_value '.error // empty')"
    return 1
  fi
  status="$(api_request POST /api/roles "$(jq -cn --arg title "API Attendance Role ${RUN_SUFFIX}" '{title:$title}')" yes)"
  if [[ "$status" == "201" ]]; then
    ROLE_ID="$(response_value '.id // empty')"
    record_case "ATT-SETUP-ROLE" "$LEVEL" "Create attendance permission role" POST /api/roles 201 "$status" PASS
  else
    record_case "ATT-SETUP-ROLE" "$LEVEL" "Create attendance permission role" POST /api/roles 201 "$status" FAIL "$(response_value '.error // empty')"
    return 1
  fi
  grant_permission "$ROLE_ID" "attendance.read.self"
  grant_permission "$ROLE_ID" "attendance.clock.self"
  expect_status "ATT-SETUP-ASSIGN" "$LEVEL" "Assign attendance role to test user" PUT "/api/roles/$ROLE_ID/users/$USER_ID" 204 "" yes >/dev/null
  USER_TOKEN="$(login_as "$email" "TempPass12345")"
}

cleanup() {
  ACCESS_TOKEN="${ACCESS_TOKEN}"
  [[ -n "$ROLE_ID" ]] && expect_status "ATT-CLEANUP-ROLE" "$LEVEL" "Cleanup attendance role" DELETE "/api/roles/$ROLE_ID" 204 "" yes || true
  [[ -n "$USER_ID" ]] && expect_status "ATT-CLEANUP-USER" "$LEVEL" "Cleanup attendance user" DELETE "/api/users/$USER_ID" 204 "" yes || true
}

run_level_1() {
  setup_attendance_user || return
  with_token "$USER_TOKEN" expect_status "ATT-L1-001" 1 "User can read today's attendance state" GET /api/attendance/today 200 "" yes
  TODAY_ID="$(response_value '.id // empty')"
  with_token "$USER_TOKEN" expect_status "ATT-L1-002" 1 "User can list own attendance days" GET /api/attendance/days 200 "" yes
  with_token "$USER_TOKEN" expect_status "ATT-L1-003" 1 "User can list leave types" GET /api/attendance/leave-types 200 "" yes
  with_token "$USER_TOKEN" expect_status "ATT-L1-004" 1 "User can sign in with no client timestamp" POST /api/attendance/today/sign-in 200 '{}' yes
  with_token "$USER_TOKEN" expect_status "ATT-L1-005" 1 "User can sign out after sign-in" POST /api/attendance/today/sign-out 200 '{}' yes
}

run_level_2() {
  with_token "$USER_TOKEN" expect_status "ATT-L2-001" 2 "Attendance today rejects missing authentication" GET /api/attendance/today 401 "" no
  with_token "$USER_TOKEN" expect_status "ATT-L2-002" 2 "Repeated sign-out conflicts when not working" POST /api/attendance/today/sign-out 409 '{}' yes
  with_token "$USER_TOKEN" expect_status "ATT-L2-003" 2 "Invalid month query is rejected" GET /api/attendance/days?month=bad-month 400 "" yes
  with_token "$USER_TOKEN" expect_status "ATT-L2-004" 2 "Leave request rejects unknown leave type" POST /api/attendance/leave-requests 400 '{"leave_type":"unknown","leave_date":"2026-12-10","duration_type":"full_day","reason":"test"}' yes
  with_token "$USER_TOKEN" expect_status "ATT-L2-005" 2 "Hourly leave requires at least one hour" POST /api/attendance/leave-requests 400 '{"leave_type":"sick_leave","leave_date":"2026-12-11","duration_type":"hourly","start_time":"09:00","end_time":"09:30","reason":"test"}' yes
  with_token "$USER_TOKEN" expect_status "ATT-L2-006" 2 "Self monthly report returns not found before generation" GET /api/attendance/monthly/2099-01 404 "" yes
  expect_status "ATT-L2-007" 2 "Organization report rejects invalid month" GET /api/attendance/reports/bad-month 400 "" yes
}

run_level_3() {
  local leave_date="2026-12-12"
  with_token "$USER_TOKEN" expect_status "ATT-L3-001" 3 "User can submit valid full-day leave request" POST /api/attendance/leave-requests 201 "$(jq -cn --arg date "$leave_date" '{leave_type:"sick_leave",leave_date:$date,duration_type:"full_day",reason:"API test leave"}')" yes
  LEAVE_ID="$(response_value '.id // empty')"
  with_token "$USER_TOKEN" expect_status "ATT-L3-002" 3 "User can list own leave requests" GET /api/attendance/leave-requests 200 "" yes
  with_token "$USER_TOKEN" expect_status "ATT-L3-003" 3 "Overlapping leave request is rejected" POST /api/attendance/leave-requests 400 "$(jq -cn --arg date "$leave_date" '{leave_type:"personal_leave",leave_date:$date,duration_type:"full_day",reason:"overlap"}')" yes
  expect_status "ATT-L3-004" 3 "Administrator can list leave approvals assigned to admin fallback" GET /api/attendance/leave-approvals 200 "" yes
  if [[ -n "$LEAVE_ID" ]]; then
    expect_status "ATT-L3-005" 3 "Assigned administrator can approve pending leave request" PATCH "/api/attendance/leave-approvals/$LEAVE_ID" 200 '{"decision":"approved","note":"API test"}' yes
    expect_status "ATT-L3-006" 3 "Approved leave request cannot be decided twice" PATCH "/api/attendance/leave-approvals/$LEAVE_ID" 409 '{"decision":"rejected","note":"second decision"}' yes
  fi
  if [[ -n "$TODAY_ID" ]]; then
    expect_status "ATT-L3-007" 3 "Attendance correction rejects missing reason/sessions" PATCH "/api/attendance/days/$TODAY_ID" 400 '{"reason":"","sessions":[]}' yes
  fi
  local current_month
  current_month="$(date -u +%Y-%m)"
  expect_status "ATT-L3-008" 3 "Open current month cannot generate final attendance CSV" POST "/api/attendance/reports/$current_month/generate" 409 "" yes
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi
cleanup

exit_for_report
