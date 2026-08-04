#!/usr/bin/env bash

set -u

LEVEL="${1:-1}"
case "$LEVEL" in 1|2|3) ;; *) echo "usage: $0 <level: 1|2|3>" >&2; exit 2 ;; esac

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS=(
  user.sh
  role.sh
  permissions.sh
  group.sh
  attendance.sh
  notification.sh
  logs.sh
  report_files.sh
)

failed=0
for script in "${SCRIPTS[@]}"; do
  echo "==> test/$script level $LEVEL"
  if ! "$ROOT_DIR/test/$script" "$LEVEL"; then
    failed=1
  fi
done

exit "$failed"
