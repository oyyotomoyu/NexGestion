#!/usr/bin/env bash

# API-level test script for the Security module (server/apis/security.go,
# server/system/security.go). Follows the same contract as test/README.md:
# level 1 = smoke/happy path, level 2 = every documented correct and
# incorrect input (auth, validation, permission boundaries), level 3 =
# multi-step workflows.
#
# IMPORTANT - shared singleton state: unlike every other module tested under
# test/, security_settings (deployment_posture/retention_days) and each row
# in security_rules are singletons, not creatable-per-run resources. Level 3
# captures the pre-test values and restores them before exiting so reruns
# never see leftover state from a prior run. Do not run two instances of
# this script against the same server at the same time - they would race on
# the same singleton rows.

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

source "$(dirname "$0")/../test/lib/api_test.sh" "$LEVEL"

require_tools
init_report
login_admin

RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"

READER_ROLE_ID=""
READER_USER_ID=""
READER_TOKEN=""
RULES_ROLE_ID=""
RULES_USER_ID=""
RULES_TOKEN=""
ALERTS_ROLE_ID=""
ALERTS_USER_ID=""
ALERTS_TOKEN=""

ORIG_POSTURE=""
ORIG_RETENTION=""
ORIG_NETWORK_ENABLED=""
ORIG_NETWORK_CONFIG=""

urlencode() {
  jq -rn --arg v "$1" '$v|@uri'
}

permission_id_for() {
  local key="$1"
  # The catalog has 95+ entries and GET /api/permissions defaults to
  # page_size=20 - an unfiltered first-page fetch silently misses any
  # permission past the first page (role.sh and attendance.sh have the same
  # latent bug; it only works there by luck of alphabetical position).
  # Filtering by keyword sidesteps pagination entirely.
  expect_status "SEC-PERM-LOOKUP-${key//./-}" "$LEVEL" "Permission catalog can be searched for setup" GET "/api/permissions?keyword=${key}" 200 "" yes >/dev/null || return 1
  jq -r --arg key "$key" '.permissions[] | select(.permission_key == $key) | .id' "$RESPONSE_BODY" | head -1
}

# grant_permission always uses the initial administrator's own password,
# since security.rules.manage and security.alerts.manage are both
# high_risk + requires_password in config/permission.json - the one gate in
# this module no other test script in test/ exercises yet.
grant_permission() {
  local role_id="$1" key="$2" password="$3" expected="$4" case_id="$5" description="$6"
  local permission_id
  permission_id="$(permission_id_for "$key")"
  if [[ -z "$permission_id" ]]; then
    record_case "$case_id" "$LEVEL" "$description" PUT "/api/roles/$role_id/permissions/<$key>" "$expected" "SKIP" FAIL "permission key $key not found in catalog"
    return 1
  fi
  local body
  if [[ -n "$password" ]]; then
    body="$(jq -cn --arg p "$password" '{current_password:$p}')"
  else
    body=""
  fi
  expect_status "$case_id" "$LEVEL" "$description" PUT "/api/roles/$role_id/permissions/$permission_id" "$expected" "$body" yes
}

setup_permission_fixture() {
  local email_prefix="$1" permission_key="$2"
  local email="sec-${email_prefix}-${RUN_SUFFIX}@example.test"
  local role_status user_status

  role_status="$(api_request POST /api/roles "$(jq -cn --arg title "API Security ${email_prefix} ${RUN_SUFFIX}" '{title:$title}')" yes)"
  local role_id=""
  if [[ "$role_status" == "201" ]]; then
    role_id="$(response_value '.id // empty')"
    record_case "SEC-SETUP-ROLE-${email_prefix}" "$LEVEL" "Create single-permission role for $permission_key boundary test" POST /api/roles 201 "$role_status" PASS
  else
    record_case "SEC-SETUP-ROLE-${email_prefix}" "$LEVEL" "Create single-permission role for $permission_key boundary test" POST /api/roles 201 "$role_status" FAIL "$(response_value '.error // empty')"
    return 1
  fi

  user_status="$(api_request POST /api/users "$(jq -cn --arg email "$email" '{display_name:"API Security Test User",email:$email,password:"TempPass12345",status:"active",must_change_password:false}')" yes)"
  local user_id=""
  if [[ "$user_status" == "201" ]]; then
    user_id="$(response_value '.id // empty')"
    record_case "SEC-SETUP-USER-${email_prefix}" "$LEVEL" "Create user for $permission_key boundary test" POST /api/users 201 "$user_status" PASS
  else
    record_case "SEC-SETUP-USER-${email_prefix}" "$LEVEL" "Create user for $permission_key boundary test" POST /api/users 201 "$user_status" FAIL "$(response_value '.error // empty')"
    expect_status "SEC-SETUP-ROLE-${email_prefix}-CLEANUP" "$LEVEL" "Cleanup role after user creation failed" DELETE "/api/roles/$role_id" 204 "" yes || true
    return 1
  fi

  grant_permission "$role_id" "$permission_key" "$ADMIN_PASSWORD" 204 "SEC-SETUP-GRANT-${email_prefix}" "Grant $permission_key to boundary-test role"
  expect_status "SEC-SETUP-ASSIGN-${email_prefix}" "$LEVEL" "Assign boundary-test role to user" PUT "/api/roles/$role_id/users/$user_id" 204 "" yes

  local token
  token="$(login_as "$email" "TempPass12345")"

  printf '%s\n%s\n%s\n' "$role_id" "$user_id" "$token"
}

cleanup_fixture() {
  local role_id="$1" user_id="$2" label="$3"
  [[ -n "$role_id" && -n "$user_id" ]] && expect_status "SEC-CLEANUP-UNASSIGN-${label}" "$LEVEL" "Unassign $label role before deletion" DELETE "/api/roles/$role_id/users/$user_id" 204 "" yes || true
  [[ -n "$user_id" ]] && expect_status "SEC-CLEANUP-USER-${label}" "$LEVEL" "Delete $label boundary-test user" DELETE "/api/users/$user_id" 204 "" yes || true
  [[ -n "$role_id" ]] && expect_status "SEC-CLEANUP-ROLE-${label}" "$LEVEL" "Delete $label boundary-test role" DELETE "/api/roles/$role_id" 204 "" yes || true
}

run_level_1() {
  expect_status "SEC-L1-001" 1 "Admin can read security settings" GET /api/security/settings 200 "" yes
  expect_status "SEC-L1-002" 1 "Admin can list security rules" GET /api/security/rules 200 "" yes
  if jq -e '.rules[] | select(.rule_key == "brute_force_login")' "$RESPONSE_BODY" >/dev/null 2>&1; then
    record_case "SEC-L1-003" 1 "Rule catalog includes brute_force_login" WORKFLOW security.rules.catalog "brute_force_login present" "present" PASS
  else
    record_case "SEC-L1-003" 1 "Rule catalog includes brute_force_login" WORKFLOW security.rules.catalog "brute_force_login present" "missing" FAIL "brute_force_login not found in /api/security/rules"
  fi
  expect_status "SEC-L1-004" 1 "Admin can list security events" GET /api/security/events 200 "" yes
  expect_status "SEC-L1-005" 1 "Admin can list security alert recipients" GET /api/security/recipients 200 "" yes

  local status recipient_id=""
  status="$(api_request POST /api/security/recipients "$(jq -cn --arg email "sec-l1-${RUN_SUFFIX}@example.test" '{email:$email,scope_type:"severity",scope_value:"critical",channel:"email"}')" yes)"
  if [[ "$status" == "201" ]]; then
    recipient_id="$(response_value '.id // empty')"
    record_case "SEC-L1-006" 1 "Admin can add an email alert recipient" POST /api/security/recipients 201 "$status" PASS
  else
    record_case "SEC-L1-006" 1 "Admin can add an email alert recipient" POST /api/security/recipients 201 "$status" FAIL "$(response_value '.error // empty')"
  fi
  if [[ -n "$recipient_id" ]]; then
    expect_status "SEC-L1-007" 1 "Admin can remove the alert recipient" DELETE "/api/security/recipients/$recipient_id" 204 "" yes
  fi

  status="$(api_request POST /api/security/test-alert "" yes)"
  if [[ "$status" == "200" ]]; then
    local event_type
    event_type="$(response_value '.event_type // empty')"
    if [[ "$event_type" == "test_alert" ]]; then
      record_case "SEC-L1-008" 1 "Admin can send a security test alert" POST /api/security/test-alert 200 "$status" PASS
    else
      record_case "SEC-L1-008" 1 "Admin can send a security test alert" POST /api/security/test-alert 200 "$status" FAIL "event_type was '$event_type', expected test_alert"
    fi
  else
    record_case "SEC-L1-008" 1 "Admin can send a security test alert" POST /api/security/test-alert 200 "$status" FAIL "$(response_value '.error // empty')"
  fi
}

run_level_2() {
  # Missing authentication on every endpoint.
  expect_status "SEC-L2-001" 2 "Events list rejects missing authentication" GET /api/security/events 401 "" no
  expect_status "SEC-L2-002" 2 "Rules list rejects missing authentication" GET /api/security/rules 401 "" no
  expect_status "SEC-L2-003" 2 "Settings read rejects missing authentication" GET /api/security/settings 401 "" no
  expect_status "SEC-L2-004" 2 "Recipients list rejects missing authentication" GET /api/security/recipients 401 "" no
  expect_status "SEC-L2-005" 2 "Rule update rejects missing authentication" PATCH /api/security/rules/brute_force_login 401 '{"enabled":true}' no
  expect_status "SEC-L2-006" 2 "Settings update rejects missing authentication" PATCH /api/security/settings 401 '{"retention_days":30}' no
  expect_status "SEC-L2-007" 2 "Recipient create rejects missing authentication" POST /api/security/recipients 401 '{"email":"x@example.test","scope_type":"severity","scope_value":"critical","channel":"email"}' no
  expect_status "SEC-L2-008" 2 "Recipient delete rejects missing authentication" DELETE /api/security/recipients/00000000-0000-0000-0000-00000000dead 401 "" no
  expect_status "SEC-L2-009" 2 "Test alert rejects missing authentication" POST /api/security/test-alert 401 "" no

  # Malformed / invalid bodies (authenticated as admin, so only input shape is under test).
  expect_status "SEC-L2-010" 2 "Rule update rejects invalid JSON" PATCH /api/security/rules/brute_force_login 400 '{"enabled":' yes
  expect_status "SEC-L2-011" 2 "Rule update rejects unknown fields" PATCH /api/security/rules/brute_force_login 400 '{"enabled":true,"threshold":9}' yes
  expect_status "SEC-L2-012" 2 "Rule update rejects non-JSON config_json payload" PATCH /api/security/rules/brute_force_login 400 '{"config_json":"not-json"}' yes
  expect_status "SEC-L2-013" 2 "Rule update on unknown rule key returns not found" PATCH "/api/security/rules/does-not-exist-${RUN_SUFFIX}" 404 '{"enabled":true}' yes
  expect_status "SEC-L2-014" 2 "Settings update rejects unknown deployment posture" PATCH /api/security/settings 400 '{"deployment_posture":"not-a-posture"}' yes
  expect_status "SEC-L2-015" 2 "Settings update rejects zero retention days" PATCH /api/security/settings 400 '{"retention_days":0}' yes
  expect_status "SEC-L2-016" 2 "Settings update rejects negative retention days" PATCH /api/security/settings 400 '{"retention_days":-5}' yes
  expect_status "SEC-L2-017" 2 "Settings update rejects invalid JSON" PATCH /api/security/settings 400 '{"retention_days":' yes
  expect_status "SEC-L2-018" 2 "Recipient create rejects neither user_id nor email" POST /api/security/recipients 400 '{"scope_type":"severity","scope_value":"critical","channel":"email"}' yes
  expect_status "SEC-L2-019" 2 "Recipient create rejects both user_id and email" POST /api/security/recipients 400 "$(jq -cn '{user_id:"00000000-0000-0000-0000-000000000000",email:"x@example.test",scope_type:"severity",scope_value:"critical",channel:"email"}')" yes
  expect_status "SEC-L2-020" 2 "Recipient create rejects unknown scope_type" POST /api/security/recipients 400 '{"email":"x@example.test","scope_type":"module","scope_value":"security","channel":"email"}' yes
  expect_status "SEC-L2-021" 2 "Recipient create rejects empty scope_value" POST /api/security/recipients 400 '{"email":"x@example.test","scope_type":"severity","scope_value":"","channel":"email"}' yes
  expect_status "SEC-L2-022" 2 "Recipient create rejects unknown channel" POST /api/security/recipients 400 '{"email":"x@example.test","scope_type":"severity","scope_value":"critical","channel":"carrier-pigeon"}' yes
  expect_status "SEC-L2-023" 2 "Recipient create rejects invalid JSON" POST /api/security/recipients 400 '{"email":' yes
  expect_status "SEC-L2-024" 2 "Deleting unknown recipient returns not found" DELETE /api/security/recipients/00000000-0000-0000-0000-00000000dead 404 "" yes
  expect_status "SEC-L2-025" 2 "Events list rejects non-numeric page" GET /api/security/events?page=abc 400 "" yes
  expect_status "SEC-L2-026" 2 "Events list rejects non-numeric page_size" GET /api/security/events?page_size=abc 400 "" yes
  expect_status "SEC-L2-027" 2 "Events list rejects negative page_size" GET /api/security/events?page_size=-1 400 "" yes
  expect_status "SEC-L2-028" 2 "Events list rejects page_size over the maximum" GET /api/security/events?page_size=99999 400 "" yes
  expect_status "SEC-L2-029" 2 "Events list rejects unknown sort field" GET /api/security/events?sort=not-a-field 400 "" yes
  expect_status "SEC-L2-030" 2 "Events list rejects unknown sort order" GET /api/security/events?order=sideways 400 "" yes

  # Granting a high-risk, requires_password permission with no/wrong password
  # must fail with 403 before it ever succeeds - nothing else in test/
  # exercises this gate, and it is the one every fixture below depends on.
  local probe_role_status probe_role_id=""
  probe_role_status="$(api_request POST /api/roles "$(jq -cn --arg title "API Security Probe Role ${RUN_SUFFIX}" '{title:$title}')" yes)"
  if [[ "$probe_role_status" == "201" ]]; then
    probe_role_id="$(response_value '.id // empty')"
    record_case "SEC-L2-031" 2 "Create probe role for password-gated grant check" POST /api/roles 201 "$probe_role_status" PASS
    grant_permission "$probe_role_id" "security.rules.manage" "" 403 "SEC-L2-032" "Granting security.rules.manage without current_password is rejected"
    grant_permission "$probe_role_id" "security.rules.manage" "definitely-the-wrong-password" 403 "SEC-L2-033" "Granting security.rules.manage with wrong current_password is rejected"
    expect_status "SEC-L2-034" 2 "Cleanup probe role" DELETE "/api/roles/$probe_role_id" 204 "" yes
  else
    record_case "SEC-L2-031" 2 "Create probe role for password-gated grant check" POST /api/roles 201 "$probe_role_status" FAIL "$(response_value '.error // empty')"
  fi

  # Permission-boundary fixtures: one user per security permission, each
  # holding exactly that permission and nothing else in the module.
  local fixture
  fixture="$(setup_permission_fixture "reader" "security.events.read")"
  READER_ROLE_ID="$(sed -n '1p' <<<"$fixture")"
  READER_USER_ID="$(sed -n '2p' <<<"$fixture")"
  READER_TOKEN="$(sed -n '3p' <<<"$fixture")"

  fixture="$(setup_permission_fixture "rules" "security.rules.manage")"
  RULES_ROLE_ID="$(sed -n '1p' <<<"$fixture")"
  RULES_USER_ID="$(sed -n '2p' <<<"$fixture")"
  RULES_TOKEN="$(sed -n '3p' <<<"$fixture")"

  fixture="$(setup_permission_fixture "alerts" "security.alerts.manage")"
  ALERTS_ROLE_ID="$(sed -n '1p' <<<"$fixture")"
  ALERTS_USER_ID="$(sed -n '2p' <<<"$fixture")"
  ALERTS_TOKEN="$(sed -n '3p' <<<"$fixture")"

  if [[ -n "$READER_TOKEN" ]]; then
    with_token "$READER_TOKEN" expect_status "SEC-L2-040" 2 "events.read holder can read events" GET /api/security/events 200 "" yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-041" 2 "events.read holder can read rules" GET /api/security/rules 200 "" yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-042" 2 "events.read holder can read settings" GET /api/security/settings 200 "" yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-043" 2 "events.read holder cannot update rules" PATCH /api/security/rules/brute_force_login 403 '{"enabled":true}' yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-044" 2 "events.read holder cannot update settings" PATCH /api/security/settings 403 '{"retention_days":30}' yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-045" 2 "events.read holder cannot read recipients" GET /api/security/recipients 403 "" yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-046" 2 "events.read holder cannot add a recipient" POST /api/security/recipients 403 '{"email":"x@example.test","scope_type":"severity","scope_value":"critical","channel":"email"}' yes
    with_token "$READER_TOKEN" expect_status "SEC-L2-047" 2 "events.read holder cannot send a test alert" POST /api/security/test-alert 403 "" yes
  else
    record_case "SEC-L2-040" 2 "events.read boundary checks" WORKFLOW security.boundary.reader "token" "SKIP" FAIL "reader fixture login failed"
  fi

  if [[ -n "$RULES_TOKEN" ]]; then
    with_token "$RULES_TOKEN" expect_status "SEC-L2-050" 2 "rules.manage holder cannot read events (lacks events.read)" GET /api/security/events 403 "" yes
    with_token "$RULES_TOKEN" expect_status "SEC-L2-051" 2 "rules.manage holder cannot read rules (lacks events.read)" GET /api/security/rules 403 "" yes
    with_token "$RULES_TOKEN" expect_status "SEC-L2-052" 2 "rules.manage holder cannot read settings (lacks events.read)" GET /api/security/settings 403 "" yes
    with_token "$RULES_TOKEN" expect_status "SEC-L2-053" 2 "rules.manage holder cannot manage recipients (lacks alerts.manage)" GET /api/security/recipients 403 "" yes
    with_token "$RULES_TOKEN" expect_status "SEC-L2-054" 2 "rules.manage holder cannot send a test alert (lacks alerts.manage)" POST /api/security/test-alert 403 "" yes
  else
    record_case "SEC-L2-050" 2 "rules.manage boundary checks" WORKFLOW security.boundary.rules "token" "SKIP" FAIL "rules fixture login failed"
  fi

  if [[ -n "$ALERTS_TOKEN" ]]; then
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-060" 2 "alerts.manage holder can read recipients" GET /api/security/recipients 200 "" yes
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-061" 2 "alerts.manage holder can send a test alert" POST /api/security/test-alert 200 "" yes
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-062" 2 "alerts.manage holder cannot read events (lacks events.read)" GET /api/security/events 403 "" yes
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-063" 2 "alerts.manage holder cannot read rules (lacks events.read)" GET /api/security/rules 403 "" yes
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-064" 2 "alerts.manage holder cannot update rules (lacks rules.manage)" PATCH /api/security/rules/brute_force_login 403 '{"enabled":true}' yes
    with_token "$ALERTS_TOKEN" expect_status "SEC-L2-065" 2 "alerts.manage holder cannot update settings (lacks rules.manage)" PATCH /api/security/settings 403 '{"retention_days":30}' yes
  else
    record_case "SEC-L2-060" 2 "alerts.manage boundary checks" WORKFLOW security.boundary.alerts "token" "SKIP" FAIL "alerts fixture login failed"
  fi

  cleanup_fixture "$READER_ROLE_ID" "$READER_USER_ID" "reader"
  cleanup_fixture "$RULES_ROLE_ID" "$RULES_USER_ID" "rules"
  cleanup_fixture "$ALERTS_ROLE_ID" "$ALERTS_USER_ID" "alerts"
  READER_ROLE_ID=""; READER_USER_ID=""; READER_TOKEN=""
  RULES_ROLE_ID=""; RULES_USER_ID=""; RULES_TOKEN=""
  ALERTS_ROLE_ID=""; ALERTS_USER_ID=""; ALERTS_TOKEN=""
}

run_level_3() {
  # Capture shared singleton state so it can be restored at the end,
  # regardless of which assertions below pass or fail.
  expect_status "SEC-L3-001" 3 "Capture settings before mutation" GET /api/security/settings 200 "" yes
  ORIG_POSTURE="$(response_value '.deployment_posture // empty')"
  ORIG_RETENTION="$(response_value '.retention_days // empty')"
  expect_status "SEC-L3-002" 3 "Capture network_change rule before mutation" GET /api/security/rules 200 "" yes
  ORIG_NETWORK_ENABLED="$(jq -r '.rules[] | select(.rule_key == "network_change") | .enabled' "$RESPONSE_BODY")"
  ORIG_NETWORK_CONFIG="$(jq -r '.rules[] | select(.rule_key == "network_change") | .config_json' "$RESPONSE_BODY")"

  # Deployment posture toggle: each direction records its own security event
  # at a different severity (SIEM.md 7.2), and this is the only place that
  # behavior is exercised end-to-end over HTTP rather than in Go.
  expect_status "SEC-L3-003" 3 "Settings update switches posture to internet_exposed" PATCH /api/security/settings 200 '{"deployment_posture":"internet_exposed"}' yes
  expect_status "SEC-L3-004" 3 "Posture change to internet_exposed is logged at critical severity" GET "/api/security/events?event_type=deployment_posture_changed&page_size=1" 200 "" yes
  if [[ "$(response_value '.events[0].severity // empty')" == "critical" && "$(response_value '.events[0].record_ref // empty')" == "internet_exposed" ]]; then
    record_case "SEC-L3-005" 3 "Posture-change event matches internet_exposed/critical" WORKFLOW security.posture.event "critical/internet_exposed" "$(response_value '.events[0].severity // empty')/$(response_value '.events[0].record_ref // empty')" PASS
  else
    record_case "SEC-L3-005" 3 "Posture-change event matches internet_exposed/critical" WORKFLOW security.posture.event "critical/internet_exposed" "$(response_value '.events[0].severity // empty')/$(response_value '.events[0].record_ref // empty')" FAIL "unexpected severity or record_ref on the most recent deployment_posture_changed event"
  fi

  expect_status "SEC-L3-006" 3 "Settings update switches posture back to lan_only" PATCH /api/security/settings 200 '{"deployment_posture":"lan_only"}' yes
  expect_status "SEC-L3-007" 3 "Posture change back to lan_only is logged at warning severity" GET "/api/security/events?event_type=deployment_posture_changed&page_size=1" 200 "" yes
  if [[ "$(response_value '.events[0].severity // empty')" == "warning" && "$(response_value '.events[0].record_ref // empty')" == "lan_only" ]]; then
    record_case "SEC-L3-008" 3 "Posture-change event matches lan_only/warning" WORKFLOW security.posture.event "warning/lan_only" "$(response_value '.events[0].severity // empty')/$(response_value '.events[0].record_ref // empty')" PASS
  else
    record_case "SEC-L3-008" 3 "Posture-change event matches lan_only/warning" WORKFLOW security.posture.event "warning/lan_only" "$(response_value '.events[0].severity // empty')/$(response_value '.events[0].record_ref // empty')" FAIL "unexpected severity or record_ref on the most recent deployment_posture_changed event"
  fi

  # Rule enable/disable + config_json round trip on an inert rule
  # (network_change ships disabled with no event source yet, so toggling it
  # cannot affect any other concurrently-evaluated rule).
  local toggled
  if [[ "$ORIG_NETWORK_ENABLED" == "true" ]]; then toggled="false"; else toggled="true"; fi
  expect_status "SEC-L3-009" 3 "Rule update flips network_change enabled flag" PATCH /api/security/rules/network_change 200 "$(jq -cn --argjson v "$toggled" '{enabled:$v}')" yes
  if [[ "$(response_value '.enabled')" == "$toggled" ]]; then
    record_case "SEC-L3-010" 3 "network_change enabled flag reflects the update" WORKFLOW security.rule.toggle "$toggled" "$(response_value '.enabled')" PASS
  else
    record_case "SEC-L3-010" 3 "network_change enabled flag reflects the update" WORKFLOW security.rule.toggle "$toggled" "$(response_value '.enabled')" FAIL "PATCH response did not reflect the new enabled value"
  fi
  expect_status "SEC-L3-011" 3 "Rule update accepts a valid config_json payload" PATCH /api/security/rules/network_change 200 '{"config_json":"{\"threshold\":7,\"window_minutes\":5}"}' yes

  # Login-failure visibility: use a victim email unique to this run so the
  # keyword search below cannot match events left behind by any other run
  # sharing this server, even though security_events itself is never
  # per-run isolated data.
  local victim_email="sec-victim-${RUN_SUFFIX}@example.test"
  expect_status "SEC-L3-012" 3 "Failed login against unique victim email is rejected" POST /api/auth/login 401 "$(jq -cn --arg email "$victim_email" '{email:$email,password:"wrong-password-1"}')" no
  expect_status "SEC-L3-013" 3 "Second failed login against the same victim email is rejected" POST /api/auth/login 401 "$(jq -cn --arg email "$victim_email" '{email:$email,password:"wrong-password-2"}')" no
  expect_status "SEC-L3-014" 3 "Third failed login against the same victim email is rejected" POST /api/auth/login 401 "$(jq -cn --arg email "$victim_email" '{email:$email,password:"wrong-password-3"}')" no
  local encoded_victim
  encoded_victim="$(urlencode "$victim_email")"
  expect_status "SEC-L3-015" 3 "Failed logins for the victim email are recorded as security events" GET "/api/security/events?keyword=${encoded_victim}&page_size=10" 200 "" yes
  local matched
  matched="$(jq --arg email "$victim_email" '[.events[] | select(.event_type == "login_failed" and .record_ref == $email)] | length' "$RESPONSE_BODY")"
  if [[ "$matched" -ge 3 ]]; then
    record_case "SEC-L3-016" 3 "All three failed logins appear as login_failed events" WORKFLOW security.events.keyword ">=3" "$matched" PASS
  else
    record_case "SEC-L3-016" 3 "All three failed logins appear as login_failed events" WORKFLOW security.events.keyword ">=3" "$matched" FAIL "expected at least 3 login_failed events for $victim_email, found $matched"
  fi

  # Alert recipient lifecycle, including a second delete to prove deletion
  # is not idempotent (ErrSecurityRecipientNotFound -> 404 on the repeat).
  local recipient_status recipient_id=""
  recipient_status="$(api_request POST /api/security/recipients "$(jq -cn --arg email "sec-l3-${RUN_SUFFIX}@example.test" '{email:$email,scope_type:"rule",scope_value:"network_change",channel:"both"}')" yes)"
  if [[ "$recipient_status" == "201" ]]; then
    recipient_id="$(response_value '.id // empty')"
    record_case "SEC-L3-017" 3 "Create email recipient scoped to a rule" POST /api/security/recipients 201 "$recipient_status" PASS
  else
    record_case "SEC-L3-017" 3 "Create email recipient scoped to a rule" POST /api/security/recipients 201 "$recipient_status" FAIL "$(response_value '.error // empty')"
  fi
  if [[ -n "$recipient_id" ]]; then
    expect_status "SEC-L3-018" 3 "Recipients list includes the newly created recipient" GET /api/security/recipients 200 "" yes
    if jq -e --arg id "$recipient_id" '.recipients[] | select(.id == $id)' "$RESPONSE_BODY" >/dev/null 2>&1; then
      record_case "SEC-L3-019" 3 "Created recipient is present in the list" WORKFLOW security.recipients.list "present" "present" PASS
    else
      record_case "SEC-L3-019" 3 "Created recipient is present in the list" WORKFLOW security.recipients.list "present" "missing" FAIL "recipient $recipient_id not found in /api/security/recipients"
    fi
    expect_status "SEC-L3-020" 3 "Delete the created recipient" DELETE "/api/security/recipients/$recipient_id" 204 "" yes
    expect_status "SEC-L3-021" 3 "Deleting the same recipient again returns not found" DELETE "/api/security/recipients/$recipient_id" 404 "" yes
  fi

  # Restore shared singleton state unconditionally.
  if [[ -n "$ORIG_RETENTION" ]]; then
    expect_status "SEC-L3-022" 3 "Restore original retention_days" PATCH /api/security/settings 200 "$(jq -cn --argjson d "$ORIG_RETENTION" '{retention_days:$d}')" yes
  fi
  if [[ -n "$ORIG_POSTURE" ]]; then
    expect_status "SEC-L3-023" 3 "Restore original deployment_posture" PATCH /api/security/settings 200 "$(jq -cn --arg p "$ORIG_POSTURE" '{deployment_posture:$p}')" yes
  fi
  if [[ -n "$ORIG_NETWORK_ENABLED" ]]; then
    expect_status "SEC-L3-024" 3 "Restore original network_change enabled flag" PATCH /api/security/rules/network_change 200 "$(jq -cn --argjson v "$([[ "$ORIG_NETWORK_ENABLED" == "true" ]] && echo true || echo false)" '{enabled:$v}')" yes
  fi
  if [[ -n "$ORIG_NETWORK_CONFIG" ]]; then
    expect_status "SEC-L3-025" 3 "Restore original network_change config_json" PATCH /api/security/rules/network_change 200 "$(jq -cn --arg c "$ORIG_NETWORK_CONFIG" '{config_json:$c}')" yes
  fi
}

run_level_1
if [[ "$LEVEL" -ge 2 ]]; then run_level_2; fi
if [[ "$LEVEL" -ge 3 ]]; then run_level_3; fi

exit_for_report
