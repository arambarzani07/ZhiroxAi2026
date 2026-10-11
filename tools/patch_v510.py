from pathlib import Path

main = Path('buildsrc/main.go')
text = main.read_text()

# Wire the enterprise PC OS surface into the existing structured command switch.
# All remote pc_os_* commands in v5.1.0 remain read-only. Local module writes are
# limited to atomic PC OS status files under the active ZhiroxAI root.
old = '''\tcase "binary_phrase_probe":
\t\tout, err = binaryPhraseProbe(sarg(c.Args, "path"), sarg(c.Args, "needle"))
\tcase "support_bundle":'''
new = '''\tcase "binary_phrase_probe":
\t\tout, err = binaryPhraseProbe(sarg(c.Args, "path"), sarg(c.Args, "needle"))
\tcase "pc_os_overview":
\t\tout, err = pcOSWindowsOverview()
\tcase "pc_os_modules":
\t\tout, err = pcOSModules()
\tcase "pc_os_guardian":
\t\tout, err = pcOSWindowsGuardian()
\tcase "pc_os_backup":
\t\tout, err = pcOSBackupVault()
\tcase "pc_os_continuity":
\t\tout, err = pcOSWindowsContinuity()
\tcase "pc_os_update_chain":
\t\tout, err = pcOSUpdateChain()
\tcase "pc_os_verify_update":
\t\tout, err = pcOSVerifyUpdate(sarg(c.Args, "manifest_path"), sarg(c.Args, "artifact_path"))
\tcase "pc_os_disk":
\t\tout, err = pcOSWindowsDiskIntelligence()
\tcase "pc_os_network":
\t\tout, err = pcOSNetwork()
\tcase "pc_os_security":
\t\tout, err = pcOSWindowsSecurity()
\tcase "pc_os_files":
\t\tout, err = pcOSFileManager(sarg(c.Args, "path"))
\tcase "pc_os_inventory":
\t\tout, err = pcOSWindowsInventory()
\tcase "pc_os_ai":
\t\tout, err = pcOSWindowsAI()
\tcase "pc_os_zero_ui":
\t\tout, err = pcOSZeroUI()
\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "support_bundle":'''
if old not in text:
    raise SystemExit('v5.1.0 command switch insertion point not found')
text = text.replace(old, new, 1)

# Start shellless local and authenticated cloud telemetry beside the existing
# cleanup worker. Neither loop can restart/stop/delete business assets or touch DVR.
needle = '\tgo cleanupLoop()'
if needle not in text:
    raise SystemExit('cleanupLoop startup insertion point not found')
text = text.replace(needle, needle + '\n\tgo pcOSBackgroundLoop()\n\tgo pcOSCloudLoop()', 1)

if '5.0.9' not in text:
    raise SystemExit('expected v5.0.9 base version not found')
text = text.replace('5.0.9', '5.1.0')
main.write_text(text)
