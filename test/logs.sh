#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

run_level_1() {
  expect_status "LOG-L1-001" 1 "Authorized user can query logs with default range" GET /api/logs 200 "" yes
}

run_level_2() {
  expect_status "LOG-L2-001" 2 "Logs reject missing authentication" GET /api/logs 401 "" no
  expect_status "LOG-L2-002" 2 "Logs reject invalid start time" GET /api/logs?start=bad 400 "" yes
  expect_status "LOG-L2-003" 2 "Logs reject invalid end time" GET /api/logs?end=bad 400 "" yes
  expect_status "LOG-L2-004" 2 "Logs reject invalid status filter" GET /api/logs?status=debug 400 "" yes
  expect_status "LOG-L2-005" 2 "Logs reject limit below one" GET /api/logs?limit=0 400 "" yes
  expect_status "LOG-L2-006" 2 "Logs accept status and limit filters" GET "/api/logs?status=info,warning,error&limit=10" 200 "" yes
}

run_level_3() {
  local old_start old_end
  old_start="$(date -u -v-8d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '8 days ago' +%Y-%m-%dT%H:%M:%SZ)"
  old_end="$(date -u -v-7d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ)"
  expect_status "LOG-L3-001" 3 "Logs reject ranges older than seven days" GET "/api/logs?start=$old_start&end=$old_end" 400 "" yes
  expect_status "LOG-L3-002" 3 "Logs reject invalid cursor" GET /api/logs?cursor=invalid-cursor 400 "" yes
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
