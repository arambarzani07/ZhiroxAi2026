#!/usr/bin/env bash
set -euo pipefail

: "${ZHIROX_UPDATE_ED25519_SEED_B64:?Missing protected signing seed}"

python3 - <<'PY'
import base64, os
raw=base64.b64decode(os.environ['ZHIROX_UPDATE_ED25519_SEED_B64'], validate=True)
if len(raw) != 32:
    raise SystemExit('Signing seed must decode to exactly 32 bytes')
PY

# Standalone production builds must preserve the same Ed25519 trust anchor that
# is already pinned in deployed v5.1.0. Feature/ephemeral CI paths do not set this.
export ZHIROX_ENFORCE_PRODUCTION_TRUST_CONTINUITY=1

rm -rf buildsrc channel release-out
mkdir -p buildsrc channel release-out
cat source-v5.0.1.part001 source-v5.0.1.part002 source-v5.0.1.part003 | base64 -d > /tmp/zhirox-source.tar.gz
tar -xzf /tmp/zhirox-source.tar.gz -C buildsrc

python3 tools/patch_downloader_v503.py
python3 tools/patch_v505.py
python3 tools/patch_v506.py
python3 tools/patch_v507.py
python3 tools/patch_v508.py
python3 tools/patch_v509.py

cp src/pc_os_v510.go buildsrc/pc_os_v510.go
cp src/pc_os_cloud_v510.go buildsrc/pc_os_cloud_v510.go
cp src/pc_os_windows_v510.go buildsrc/pc_os_windows_v510.go
cp src/pc_os_backup_policy_v510.go buildsrc/pc_os_backup_policy_v510.go
cp src/pc_os_backup_v510_windows.go buildsrc/pc_os_backup_v510_windows.go
cp src/pc_os_update_enforce_v510.go buildsrc/pc_os_update_enforce_v510.go
cp src/pc_os_self_updater_v510_windows.go buildsrc/pc_os_self_updater_v510_windows.go
cp src/update_trust_v510.go buildsrc/update_trust_v510.go
python3 tools/patch_v510.py
python3 tools/patch_trust_v510.py
python3 tools/patch_self_updater_v510.py
gofmt -w buildsrc/*.go

go run tools/prepare_trust_v510.go
gofmt -w buildsrc/update_trust_v510.go

test -s channel/update-public-key.hex
grep -Eq 'pcOSPinnedUpdatePublicKeyHex = "[0-9a-f]{64}"' buildsrc/update_trust_v510.go

grep -q '5.1.0' buildsrc/main.go
grep -q 'health_gate_seconds' buildsrc/pc_os_self_updater_v510_windows.go
grep -q 'automatic_delete' buildsrc/pc_os_self_updater_v510_windows.go
grep -q 'Ed25519 update public key is not pinned in this build' buildsrc/pc_os_v510.go

(
  cd buildsrc
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-H=windowsgui -s -w' -o ../channel/ZhiroxAISupervisor.exe main.go admin_actions_v48.go owner_actions.go hash_v48.go pc_os_v510.go pc_os_cloud_v510.go pc_os_windows_v510.go pc_os_backup_policy_v510.go pc_os_backup_v510_windows.go pc_os_update_enforce_v510.go pc_os_self_updater_v510_windows.go update_trust_v510.go
)

go run tools/sign_metadata_v510.go
go run tools/verify_metadata_v510.go

grep -q '"algorithm":"Ed25519"' channel/metadata.json
sha256sum channel/ZhiroxAISupervisor.exe > channel/ZhiroxAISupervisor.exe.sha256

cp channel/ZhiroxAISupervisor.exe release-out/ZhiroxAISupervisor.exe
cp channel/ZhiroxAISupervisor.exe.sha256 release-out/ZhiroxAISupervisor.exe.sha256
cp channel/metadata.json release-out/metadata.json
cp channel/update-public-key.hex release-out/update-public-key.hex
printf '%s\n' '{"service":"Zhirox protected v5.1 relay artifact","status":"staged","production_cutover":false}' > release-out/index.json

# Never emit the signing seed. Remove release-only trust controls from this
# process environment before any later shell step.
unset ZHIROX_UPDATE_ED25519_SEED_B64
unset ZHIROX_ENFORCE_PRODUCTION_TRUST_CONTINUITY
