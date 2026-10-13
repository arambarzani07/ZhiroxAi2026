from pathlib import Path

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')

main_text = main.read_text()
pc_text = pc_os.read_text()

# v5.1.4 is the production candidate that carries read-only screen observability
# on top of the already promoted v5.1.2 trust/recovery baseline. v5.1.3 remains
# reserved by its separate unreleased fallback branch and is not reused here.
if '5.1.2' not in main_text:
    raise SystemExit('v5.1.2 runtime base version not found')
main_text = main_text.replace('5.1.2', '5.1.4', 1)

old_const = 'const pcOSVersion = "5.1.2"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.2 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.4"', 1)

for invariant in (
    'pc_os_active_window',
    'pc_os_screenshot',
    'pcOSScreenObservationHelperRequested(os.Args[1:])',
    'pcOSZeroTouchWatchdogEnsureLoop()',
):
    if invariant not in main_text:
        raise SystemExit(f'missing v5.1.4 runtime invariant: {invariant}')

for invariant in (
    '"zero_touch_recovery"',
    '"screen_observability_read_only"',
    'const pcOSVersion = "5.1.4"',
):
    if invariant not in pc_text:
        raise SystemExit(f'missing v5.1.4 pc-os invariant: {invariant}')

main.write_text(main_text)
pc_os.write_text(pc_text)
