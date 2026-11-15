"""Wire only two typed Cashierynwe actions into the existing PC Relay switch.

Never add owner_exec, cmd.exe, generic PowerShell, or arbitrary script arguments.
This patch is source-stage ONLY; do not auto-promote an unsigned agent.
"""
from pathlib import Path

main = Path("buildsrc/main.go")
text = main.read_text(encoding="utf-8")
marker = '\tcase "support_bundle":'
if text.count(marker) != 1:
    raise SystemExit("PC Relay command switch anchor changed")
if 'case "cashierynwe_sign_release":' in text or 'case "cashierynwe_sign_status":' in text:
    raise SystemExit("Typed Cashierynwe action is already registered")
insert = '''\tcase "cashierynwe_sign_release":
\t\tout, err = pcOSCashierynweSignRelease(c.Args)
\tcase "cashierynwe_sign_status":
\t\tout, err = pcOSCashierynweSignStatus()
'''
text = text.replace(marker, insert + marker, 1)
if 'owner_exec_enabled": true' in text or 'arbitrary_shell": true' in text:
    raise SystemExit("Unsafe Relay mode detected")
main.write_text(text, encoding="utf-8")
