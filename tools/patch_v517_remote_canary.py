from pathlib import Path

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')
main_text = main.read_text(encoding='utf-8')
pc_text = pc_os.read_text(encoding='utf-8')

if '5.1.6' not in main_text:
    raise SystemExit('v5.1.6 runtime base version not found')
main_text = main_text.replace('5.1.6', '5.1.7', 1)
old_const = 'const pcOSVersion = "5.1.6"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.6 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.7"', 1)

old = '''\tcase "pc_os_screenshot":
\t\tout, err = pcOSCaptureScreenshot(c.Args)
\tcase "support_bundle":'''
new = '''\tcase "pc_os_screenshot":
\t\tout, err = pcOSCaptureScreenshot(c.Args)
\tcase "pc_os_remote_canary_start":
\t\tout, err = pcOSRemoteCanaryStart()
\tcase "pc_os_remote_canary_status":
\t\tout, err = pcOSRemoteCanaryStatus()
\tcase "support_bundle":'''
if old not in main_text:
    raise SystemExit('v5.1.7 canary command insertion point not found')
main_text = main_text.replace(old, new, 1)

module_anchor = '"screen_observability_read_only",'
if module_anchor not in pc_text:
    raise SystemExit('screen observability module anchor not found')
pc_text = pc_text.replace(module_anchor, module_anchor + '\n            "remote_screen_canary_attended",', 1)

for invariant in (
    'pc_os_active_window',
    'pc_os_screenshot',
    'pc_os_remote_canary_start',
    'pc_os_remote_canary_status',
    'pcOSZeroTouchWatchdogEnsureLoop()',
):
    if invariant not in main_text:
        raise SystemExit(f'missing v5.1.7 runtime invariant: {invariant}')
for invariant in (
    'const pcOSVersion = "5.1.7"',
    '"screen_observability_read_only"',
    '"remote_screen_canary_attended"',
):
    if invariant not in pc_text:
        raise SystemExit(f'missing v5.1.7 pc-os invariant: {invariant}')

main.write_text(main_text, encoding='utf-8')
pc_os.write_text(pc_text, encoding='utf-8')
