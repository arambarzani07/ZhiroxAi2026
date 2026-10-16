from pathlib import Path

main = Path("buildsrc/main.go")
text = main.read_text()

old = '''\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "support_bundle":'''
new = '''\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "pc_os_export_prepare_latest":
\t\tout, err = pcOSPrepareLatestSuperMarketBackupExport(sarg(c.Args, "confirm") == "STAGE READ-ONLY BACKUP EXPORT")
\tcase "support_bundle":'''

if old not in text:
    raise SystemExit("read-only export command insertion point not found")

text = text.replace(old, new, 1)
main.write_text(text)
