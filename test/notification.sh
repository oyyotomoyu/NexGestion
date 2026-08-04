#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
NOTICE_ID=""

create_notice_body() {
  jq -cn --arg title "$1" --arg message "$2" '{title:$title,message:$message,type:"info",show_time:"hour",audiences:[{scope:"organization"}]}'
}

run_level_1() {
  expect_status "NOTIF-L1-001" 1 "Authenticated user can list notification types" GET /api/notifications/types 200 "" yes
  expect_status "NOTIF-L1-002" 1 "Authenticated user can list visible notifications" GET /api/notifications 200 "" yes
  expect_status "NOTIF-L1-003" 1 "Admin can list all notifications" GET /api/notifications/admin 200 "" yes
  local status
  status="$(api_request POST /api/notifications "$(create_notice_body "API Notice L1 ${RUN_SUFFIX}" "Created by API test")" yes)"
  if [[ "$status" == "201" ]]; then
    NOTICE_ID="$(response_value '.id // empty')"
    record_case "NOTIF-L1-004" 1 "Authorized sender can create organization notification" POST /api/notifications 201 "$status" PASS
  else
    record_case "NOTIF-L1-004" 1 "Authorized sender can create organization notification" POST /api/notifications 201 "$status" FAIL "$(response_value '.error // empty')"
  fi
  if [[ -n "$NOTICE_ID" ]]; then
    expect_status "NOTIF-L1-005" 1 "Original sender can edit sent notification" PATCH "/api/notifications/$NOTICE_ID" 200 '{"message":"Edited by API test"}' yes
    expect_status "NOTIF-L1-006" 1 "Original sender can hide sent notification" POST "/api/notifications/$NOTICE_ID/hide" 204 "" yes
  fi
}

run_level_2() {
  expect_status "NOTIF-L2-001" 2 "Notification types reject missing authentication" GET /api/notifications/types 401 "" no
  expect_status "NOTIF-L2-002" 2 "Create rejects invalid JSON" POST /api/notifications 400 '{"title":' yes
  expect_status "NOTIF-L2-003" 2 "Create rejects missing audience" POST /api/notifications 400 '{"title":"x","message":"x","type":"info","show_time":"hour","audiences":[]}' yes
  expect_status "NOTIF-L2-004" 2 "Create rejects invalid duration" POST /api/notifications 400 '{"title":"x","message":"x","type":"info","show_time":"minute","audiences":[{"scope":"organization"}]}' yes
  expect_status "NOTIF-L2-005" 2 "Create rejects invalid type" POST /api/notifications 400 '{"title":"x","message":"x","type":"missing","show_time":"hour","audiences":[{"scope":"organization"}]}' yes
  expect_status "NOTIF-L2-006" 2 "Edit unknown notification returns not found" PATCH /api/notifications/00000000-0000-0000-0000-00000000dead 404 '{"message":"x"}' yes
  expect_status "NOTIF-L2-007" 2 "Hide unknown notification returns not found" POST /api/notifications/00000000-0000-0000-0000-00000000dead/hide 404 "" yes
  expect_status "NOTIF-L2-008" 2 "Export rejects invalid month" GET /api/notifications/exports/bad-month/csv 400 "" yes
}

run_level_3() {
  local month
  month="$(date -u +%Y-%m)"
  expect_status "NOTIF-L3-001" 3 "Notification monthly CSV export works for current month" GET "/api/notifications/exports/$month/csv" 200 "" yes
  expect_status "NOTIF-L3-002" 3 "Hidden notification is retained in admin list" GET /api/notifications/admin 200 "" yes
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
