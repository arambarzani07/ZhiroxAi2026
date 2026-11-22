"""Patch a single reviewed signed-installer capability into v5.1.15 source.

NO arbitrary process runner, no caller-chosen executable, path, URL, flags or
PowerShell. Signed app requests require the authenticated one-time owner gate.
The installed NT AUTHORITY\\SYSTEM service fails closed for per-user NSIS
packages pending a separately reviewed attended user-session broker.
"""
from pathlib import Path

main = Path("buildsrc/main.go")
pc = Path("buildsrc/pc_os_v510.go")
src = main.read_text(encoding="utf-8")
pcsrc = pc.read_text(encoding="utf-8")

marker = '\tcase "cashierynwe_sign_status":\n\t\tout, err = pcOSCashierynweSignStatus()\n'
if src.count(marker) != 1:
    raise SystemExit("fatal: v5.1.15 typed signer route anchor changed")
if 'case "signed_app_install":' in src or 'case "signed_app_install_status":' in src:
    raise SystemExit("fatal: signed installer route already exists")

routes = (
    '\tcase "signed_app_install":\n'
    '\t\tout, err = pcOSSignedAppInstall(c.Args)\n'
    '\tcase "signed_app_install_status":\n'
    '\t\tout, err = pcOSSignedAppInstallStatus()\n'
)
src = src.replace(marker, marker + routes, 1)
if src.count("5.1.15") < 1:
    raise SystemExit("fatal: v5.1.15 runtime source version missing")
src = src.replace("5.1.15", "5.1.16", 1)
if pcsrc.count('const pcOSVersion = "5.1.15"') != 1:
    raise SystemExit("fatal: exact v5.1.15 pcOSVersion anchor missing")
pcsrc = pcsrc.replace('const pcOSVersion = "5.1.15"', 'const pcOSVersion = "5.1.16"', 1)

for x in ('case "signed_app_install":', 'case "signed_app_install_status":'):
    if src.count(x) != 1:
        raise SystemExit("fatal: signed installer route count failed")
for x in ('owner_exec_enabled": true', 'arbitrary_shell": true'):
    if x in src:
        raise SystemExit("fatal: unsafe global remote-execution policy detected")

main.write_text(src, encoding="utf-8")
pc.write_text(pcsrc, encoding="utf-8")
print("v5.1.16 signed-app source patched; unsigned candidate, NEVER auto-promote")
