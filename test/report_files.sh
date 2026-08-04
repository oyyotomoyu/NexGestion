#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

FIRST_FILE=""
GENERATED_FILE=""

previous_month() {
  date -u -v-1m +%Y-%m 2>/dev/null || date -u -d '1 month ago' +%Y-%m
}

run_level_1() {
  expect_status "REPORT-L1-001" 1 "Authorized user can list report files" GET /api/reports/files 200 "" yes
  FIRST_FILE="$(jq -r '.files[0].path // empty' "$RESPONSE_BODY")"
}

run_level_2() {
  expect_status "REPORT-L2-001" 2 "Report file list rejects missing authentication" GET /api/reports/files 401 "" no
  expect_status "REPORT-L2-002" 2 "Missing report file returns not found" GET /api/reports/files/missing-file-${RUN_ID}.csv 404 "" yes
  expect_status "REPORT-L2-003" 2 "Path traversal download is rejected" GET /api/reports/files/..%2Fsecret.txt 400 "" yes
  expect_status "REPORT-L2-004" 2 "Path traversal delete is rejected" DELETE /api/reports/files/..%2Fsecret.txt 400 "" yes
}

run_level_3() {
  local month status
  month="$(previous_month)"
  status="$(api_request POST "/api/attendance/reports/$month/generate" "" yes)"
  if [[ "$status" == "200" ]]; then
    GENERATED_FILE="$(response_value '.relative_path // empty')"
    record_case "REPORT-L3-001" 3 "Generate an attendance CSV to use for report-file lifecycle" POST "/api/attendance/reports/$month/generate" 200 "$status" PASS
  else
    record_case "REPORT-L3-001" 3 "Generate an attendance CSV to use for report-file lifecycle" POST "/api/attendance/reports/$month/generate" 200 "$status" FAIL "$(response_value '.error // empty')"
  fi

  if [[ -n "$GENERATED_FILE" ]]; then
    local encoded_generated
    encoded_generated="$(jq -rn --arg v "$GENERATED_FILE" '$v|@uri')"
    expect_status "REPORT-L3-002" 3 "Generated report file can be downloaded" GET "/api/reports/files/$encoded_generated" 200 "" yes
    expect_status "REPORT-L3-003" 3 "Generated report file can be deleted through Report File API" DELETE "/api/reports/files/$encoded_generated" 204 "" yes
    expect_status "REPORT-L3-004" 3 "Deleted report file is no longer downloadable" GET "/api/reports/files/$encoded_generated" 404 "" yes
  elif [[ -n "$FIRST_FILE" ]]; then
    local encoded
    encoded="$(jq -rn --arg v "$FIRST_FILE" '$v|@uri')"
    expect_status "REPORT-L3-002" 3 "Existing report file can be downloaded" GET "/api/reports/files/$encoded" 200 "" yes
  else
    record_case "REPORT-L3-002" 3 "Existing report file can be downloaded" GET "/api/reports/files/<first>" 200 "SKIP" FAIL "no report files exist to download"
  fi
  expect_status "REPORT-L3-005" 3 "Report API stays inside report root for nested traversal" GET /api/reports/files/attendance%2F..%2F..%2Fsecret.txt 400 "" yes
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
