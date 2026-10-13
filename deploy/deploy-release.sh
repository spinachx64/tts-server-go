#!/usr/bin/env bash
# Root-owned helper executed as ttsdeploy, not through sudo.
set -euo pipefail
release_id=${1:?Usage: tts-server-go-deploy SHA-RUN-ATTEMPT [PORT]}
port=${2:-1233}
[[ $release_id =~ ^[0-9a-f]{40}-[0-9]+-[0-9]+$ ]] || { echo 'Invalid release ID' >&2; exit 1; }
[[ $port =~ ^[0-9]+$ ]] && ((port >= 1 && port <= 65535)) || { echo 'Invalid port' >&2; exit 1; }
base=/opt/tts-server-go
archive="$base/incoming/$release_id.tar.gz"
release="$base/releases/$release_id"
# Serialize both Actions deployments and manual invocations.
exec 9>"$base/.deploy.lock"
flock -w 120 9
[[ -f $archive && ! -e $release ]] || { echo 'Missing upload or release already exists' >&2; exit 1; }
# Extract only the expected files, never arbitrary paths from an archive.
mkdir -m 755 "$release"
tar -xzf "$archive" -C "$release" --no-same-owner --no-same-permissions tts-server-go SHA256SUMS
(cd "$release" && sha256sum --check --strict SHA256SUMS)
chmod 755 "$release/tts-server-go"
previous=$(readlink "$base/current" || true)
if [[ -n $previous && ! $previous =~ ^releases/[0-9a-f]{40}-[0-9]+-[0-9]+$ ]]; then echo 'Unexpected current symlink' >&2; exit 1; fi
activate() {
  ln -s "$1" "$base/.current-$release_id"
  mv -Tf "$base/.current-$release_id" "$base/current"
}
healthy() {
  for attempt in {1..15}; do
    if sudo -n systemctl is-active --quiet tts-server-go.service && curl --fail --silent --max-time 2 "http://127.0.0.1:$port/" >/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}
activate "releases/$release_id"
if sudo -n systemctl restart tts-server-go.service && healthy; then
  rm -f "$archive"
  echo "Deployed $release_id"
  exit 0
fi
echo 'Deployment failed; rolling back.' >&2
if [[ -n $previous ]]; then
  activate "$previous"
  sudo -n systemctl restart tts-server-go.service
  healthy || { echo 'Rollback health check failed; inspect the systemd journal.' >&2; exit 1; }
  echo "Restored $previous" >&2
else
  rm -f "$base/current"
  echo 'First deployment failed; no previous release exists.' >&2
fi
exit 1
