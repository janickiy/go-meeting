#!/usr/bin/env bash
# Запускает браузер только в отдельном Linux-контейнере, без доступа к secrets проекта.
set -uo pipefail
smoke_browser="${MEET_SMOKE_BROWSER:-firefox}"
[[ "$smoke_browser" == firefox || "$smoke_browser" == chromium ]] || exit 1
browser_workspace="$(mktemp -d /tmp/meetrix-browser.XXXXXX)"
cd "$browser_workspace" || exit 1
npm install --ignore-scripts --no-audit --no-fund @playwright/test@1.63.0 || exit 1
mkdir e2e
cp /source/frontend-live-smoke.spec.ts e2e/
cp /source/playwright.remote.config.ts ./
export MEET_REMOTE_SMOKE=true
./node_modules/.bin/playwright test --config playwright.remote.config.ts --project "$smoke_browser"
test_status=$?
if [[ -d test-results/remote ]]; then cp -R test-results/remote "/evidence/$smoke_browser"; fi
exit "$test_status"
