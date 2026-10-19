#!/usr/bin/env bash
# Run once as root from a checkout: sudo bash deploy/install-server.sh
set -euo pipefail
if [[ $EUID -ne 0 ]]; then echo 'Run this script as root.' >&2; exit 1; fi
for tool in systemctl curl tar sha256sum sudo flock; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 1; }
done
config_dir=/opt/project-config/tts-server-go
if [[ -e /etc/tts-server-go.env && ! -e $config_dir/service.env ]]; then
  echo 'Legacy paths found. Migrate configuration, database and logs to /opt before reinstalling (docs/DEPLOY.md).' >&2
  exit 1
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
id tts >/dev/null 2>&1 || useradd --system --user-group --home-dir /nonexistent --shell /usr/sbin/nologin tts
id ttsdeploy >/dev/null 2>&1 || useradd --create-home --user-group --shell /bin/bash ttsdeploy
install -d -o ttsdeploy -g ttsdeploy -m 755 /opt/tts-server-go /opt/tts-server-go/releases
install -d -o ttsdeploy -g ttsdeploy -m 700 /opt/tts-server-go/incoming
install -d -o tts -g tts -m 750 /opt/tts-server-go/data /opt/tts-server-go/logs
install -d -o root -g root -m 700 /opt/project-config "$config_dir"
install -d -o root -g root -m 755 /usr/local/libexec
install -o root -g root -m 755 "$script_dir/deploy-release.sh" /usr/local/libexec/tts-server-go-deploy
install -o root -g root -m 644 "$script_dir/tts-server-go.service" /etc/systemd/system/tts-server-go.service
# Do not overwrite the existing token, port, or proxy configuration.
if [[ ! -e $config_dir/service.env ]]; then
  umask 077
  printf 'TTS_PORT=1233\nTTS_TOKEN=%s\n' "$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')" > "$config_dir/service.env"
fi
chown root:root "$config_dir/service.env"
chmod 600 "$config_dir/service.env"
# Only these service operations may be run through passwordless sudo.
systemctl_path=$(command -v systemctl)
sudoers_tmp=$(mktemp)
trap 'rm -f "$sudoers_tmp"' EXIT
printf 'ttsdeploy ALL=(root) NOPASSWD: %s restart tts-server-go.service, %s is-active --quiet tts-server-go.service\n' "$systemctl_path" "$systemctl_path" > "$sudoers_tmp"
visudo -cf "$sudoers_tmp"
install -o root -g root -m 440 "$sudoers_tmp" /etc/sudoers.d/tts-server-go-deploy
systemctl daemon-reload
systemctl enable tts-server-go.service
printf '%s\n' 'Server prepared. Add the deploy public key to ~ttsdeploy/.ssh/authorized_keys.' 'Configure the GitHub Secrets before the first deployment.' 'The service starts on the first successful deployment.' 'API token and port: /opt/project-config/tts-server-go/service.env (root-readable only).'
