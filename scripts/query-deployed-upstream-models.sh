#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
# shellcheck source=/dev/null
. ./.env
set +a
: "${DOMAIN:?DOMAIN is not set in .env}"
curl -sS -f "https://${DOMAIN}/v1/diagnostics/upstream-models" | python3 -m json.tool
