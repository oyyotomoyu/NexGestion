#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

run_level_1() {
  expect_status "PERM-L1-001" 1 "Authenticated admin can list permission catalog" GET /api/permissions 200 "" yes
}

run_level_2() {
  expect_status "PERM-L2-001" 2 "Permission catalog rejects missing authentication" GET /api/permissions 401 "" no
  expect_status "PERM-L2-002" 2 "Permission catalog includes users.read from system document" GET /api/permissions 200 "" yes
  if jq -e '.permissions[] | select(.permission_key == "users.read")' "$RESPONSE_BODY" >/dev/null; then
    record_case "PERM-L2-003" 2 "Permission catalog exposes stable permission keys" WORKFLOW permissions.catalog "users.read present" "users.read present" PASS
  else
    record_case "PERM-L2-003" 2 "Permission catalog exposes stable permission keys" WORKFLOW permissions.catalog "users.read present" "missing" FAIL "users.read was not found in /api/permissions"
  fi
}

run_level_3() {
  expect_status "PERM-L3-001" 3 "Permission assignment endpoint rejects unknown role" PUT /api/roles/00000000-0000-0000-0000-00000000dead/permissions/00000000-0000-0000-0000-00000000dead 404 "" yes
  expect_status "PERM-L3-002" 3 "Permission revoke endpoint rejects unknown role" DELETE /api/roles/00000000-0000-0000-0000-00000000dead/permissions/00000000-0000-0000-0000-00000000dead 404 "" yes
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
