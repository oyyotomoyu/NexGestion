#!/usr/bin/env bash

set -u

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@nexgestion.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-password}"
REPORT_DIR="${REPORT_DIR:-report}"

SCRIPT_NAME="$(basename "${BASH_SOURCE[1]}" .sh)"
RUN_LEVEL="${1:-1}"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT_MD="${REPORT_DIR}/${SCRIPT_NAME}-level${RUN_LEVEL}-${RUN_ID}.md"
REPORT_JSONL="${REPORT_DIR}/${SCRIPT_NAME}-level${RUN_LEVEL}-${RUN_ID}.jsonl"
COOKIE_JAR="$(mktemp)"
RESPONSE_BODY="$(mktemp)"
RESPONSE_HEADERS="$(mktemp)"
PASS_COUNT=0
FAIL_COUNT=0
ACCESS_TOKEN=""

mkdir -p "$REPORT_DIR"

finish() {
  rm -f "$COOKIE_JAR" "$RESPONSE_BODY" "$RESPONSE_HEADERS"
}
trap finish EXIT

require_tools() {
  local missing=0
  for tool in curl jq; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      echo "missing required tool: $tool" >&2
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    exit 2
  fi
}

init_report() {
  cat >"$REPORT_MD" <<EOF
# ${SCRIPT_NAME} API Test Report

- Level: ${RUN_LEVEL}
- Base URL: ${BASE_URL}
- Started: ${RUN_ID}

| Case | Level | Result | Expected | Actual | Description | Reason |
| --- | --- | --- | --- | --- | --- | --- |
EOF
  : >"$REPORT_JSONL"
}

json_escape() {
  jq -Rsa .
}

record_case() {
  local case_id="$1"
  local level="$2"
  local description="$3"
  local method="$4"
  local path="$5"
  local expected="$6"
  local actual="$7"
  local result="$8"
  local reason="${9:-}"

  if [[ "$result" == "PASS" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi

  printf '| `%s` | %s | %s | `%s` | `%s` | %s | %s |\n' \
    "$case_id" "$level" "$result" "$expected" "$actual" "$description" "${reason:- }" >>"$REPORT_MD"

  jq -cn \
    --arg case "$case_id" \
    --arg level "$level" \
    --arg description "$description" \
    --arg method "$method" \
    --arg path "$path" \
    --arg expected "$expected" \
    --arg actual "$actual" \
    --arg result "$result" \
    --arg reason "$reason" \
    '{case:$case,level:($level|tonumber),description:$description,method:$method,path:$path,expected:$expected,actual:$actual,result:$result,reason:$reason}' \
    >>"$REPORT_JSONL"
}

api_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local auth="${4:-yes}"

  local args=(-sS -X "$method" "$BASE_URL$path" -D "$RESPONSE_HEADERS" -o "$RESPONSE_BODY" -w "%{http_code}" -b "$COOKIE_JAR" -c "$COOKIE_JAR")
  if [[ "$auth" == "yes" && -n "$ACCESS_TOKEN" ]]; then
    args+=(-H "Authorization: Bearer $ACCESS_TOKEN")
  fi
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  curl "${args[@]}"
}

expect_status() {
  local case_id="$1"
  local level="$2"
  local description="$3"
  local method="$4"
  local path="$5"
  local expected_status="$6"
  local body="${7:-}"
  local auth="${8:-yes}"

  local actual_status
  actual_status="$(api_request "$method" "$path" "$body" "$auth")"
  if [[ "$actual_status" == "$expected_status" ]]; then
    record_case "$case_id" "$level" "$description" "$method" "$path" "$expected_status" "$actual_status" "PASS"
    return 0
  fi

  local reason
  reason="$(jq -r '.error // empty' "$RESPONSE_BODY" 2>/dev/null)"
  if [[ -z "$reason" ]]; then
    reason="$(tr '\n' ' ' <"$RESPONSE_BODY" | cut -c 1-220)"
  fi
  record_case "$case_id" "$level" "$description" "$method" "$path" "$expected_status" "$actual_status" "FAIL" "$reason"
  return 1
}

login_admin() {
  local body
  body="$(jq -cn --arg email "$ADMIN_EMAIL" --arg password "$ADMIN_PASSWORD" '{email:$email,password:$password}')"
  local status
  status="$(api_request POST /api/auth/login "$body" no)"
  if [[ "$status" != "200" ]]; then
    echo "admin login failed with HTTP $status. Set ADMIN_EMAIL and ADMIN_PASSWORD for this server." >&2
    cat "$RESPONSE_BODY" >&2
    exit 1
  fi
  ACCESS_TOKEN="$(jq -r '.access_token // empty' "$RESPONSE_BODY")"
  if [[ -z "$ACCESS_TOKEN" ]]; then
    echo "admin login response did not include access_token" >&2
    exit 1
  fi
}

login_as() {
  local email="$1"
  local password="$2"
  local body
  body="$(jq -cn --arg email "$email" --arg password "$password" '{email:$email,password:$password}')"
  local status
  status="$(api_request POST /api/auth/login "$body" no)"
  if [[ "$status" != "200" ]]; then
    return 1
  fi
  jq -r '.access_token // empty' "$RESPONSE_BODY"
}

with_token() {
  local token="$1"
  shift
  local previous_token="$ACCESS_TOKEN"
  ACCESS_TOKEN="$token"
  "$@"
  local result=$?
  ACCESS_TOKEN="$previous_token"
  return "$result"
}

response_value() {
  jq -r "$1" "$RESPONSE_BODY"
}

append_summary() {
  {
    echo
    echo "## Summary"
    echo
    echo "- Passed: ${PASS_COUNT}"
    echo "- Failed: ${FAIL_COUNT}"
    echo "- JSONL: ${REPORT_JSONL}"
  } >>"$REPORT_MD"
}

exit_for_report() {
  append_summary
  echo "Report: $REPORT_MD"
  echo "Events: $REPORT_JSONL"
  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    exit 1
  fi
}
