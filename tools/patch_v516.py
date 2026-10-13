from pathlib import Path

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')
main_text = main.read_text()
pc_text = pc_os.read_text()

if '5.1.5' not in main_text:
    raise SystemExit('v5.1.5 runtime base version not found')
main_text = main_text.replace('5.1.5', '5.1.6', 1)
old_const = 'const pcOSVersion = "5.1.5"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.5 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.6"', 1)

for invariant in ('pc_os_active_window','pc_os_screenshot','pcOSScreenObservationHelperRequested(os.Args[1:])','pcOSZeroTouchWatchdogEnsureLoop()'):
    if invariant not in main_text:
        raise SystemExit(f'missing v5.1.6 runtime invariant: {invariant}')
for invariant in ('"zero_touch_recovery"','"screen_observability_read_only"','const pcOSVersion = "5.1.6"'):
    if invariant not in pc_text:
        raise SystemExit(f'missing v5.1.6 pc-os invariant: {invariant}')

main.write_text(main_text)
pc_os.write_text(pc_text)
