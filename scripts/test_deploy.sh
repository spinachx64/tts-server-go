#!/usr/bin/env bash
# Linux-only deployment integration test. All service and HTTP calls are mocked.
set -euo pipefail
repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
export TEST_BASE="$test_dir/server"
mkdir -p "$TEST_BASE/incoming" "$TEST_BASE/releases" "$test_dir/mocks" "$test_dir/payload"
# Run the actual helper against an isolated path; never touch /opt or systemd.
sed "s|^base=/opt/tts-server-go$|base=$TEST_BASE|" "$repo_dir/deploy/deploy-release.sh" > "$test_dir/deploy.sh"
cat > "$test_dir/mocks/sudo" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TEST_BASE/service-calls"
exit 0
MOCK
cat > "$test_dir/mocks/curl" <<'MOCK'
#!/usr/bin/env bash
[[ $(readlink "$TEST_BASE/current") != "releases/${FAIL_RELEASE:-none}" ]]
MOCK
# Avoid real waits during the fifteen failed health checks.
printf '#!/usr/bin/env bash\nexit 0\n' > "$test_dir/mocks/sleep"
chmod +x "$test_dir/mocks/"*
export PATH="$test_dir/mocks:$PATH"
stage() {
  printf '#!/bin/sh\necho test-release\n' > "$test_dir/payload/tts-server-go"
  (cd "$test_dir/payload" && sha256sum tts-server-go > SHA256SUMS)
  if [[ ${2:-valid} == corrupt ]]; then echo corrupt >> "$test_dir/payload/tts-server-go"; fi
  tar -czf "$TEST_BASE/incoming/$1.tar.gz" -C "$test_dir/payload" tts-server-go SHA256SUMS
}
first=1111111111111111111111111111111111111111-1-1
corrupt=2222222222222222222222222222222222222222-2-1
failed=3333333333333333333333333333333333333333-3-1
stage "$first"
bash "$test_dir/deploy.sh" "$first"
[[ $(readlink "$TEST_BASE/current") == "releases/$first" ]]
[[ -x "$TEST_BASE/current/tts-server-go" ]]
[[ ! -e "$TEST_BASE/incoming/$first.tar.gz" ]]
stage "$corrupt" corrupt
if bash "$test_dir/deploy.sh" "$corrupt"; then echo 'Accepted corrupt archive' >&2; exit 1; fi
[[ $(readlink "$TEST_BASE/current") == "releases/$first" ]]
stage "$failed"
export FAIL_RELEASE="$failed"
if bash "$test_dir/deploy.sh" "$failed"; then echo 'Accepted unhealthy release' >&2; exit 1; fi
[[ $(readlink "$TEST_BASE/current") == "releases/$first" ]]
if bash "$test_dir/deploy.sh" '../invalid'; then echo 'Accepted invalid release ID' >&2; exit 1; fi
printf '%s\n' 'Deployment checks passed: activation, checksum rejection, health failure rollback, input validation.'
