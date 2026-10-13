from pathlib import Path
import re

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')

main_text = main.read_text()
pc_text = pc_os.read_text()

# v5.1.2 zero-touch recovery runs as a detached companion instance of the same
# signed binary. The watchdog mode must return before normal Supervisor/service
# startup so it can remain independent of a stuck parent process.
main_match = re.search(r'func main\(\)\s*\{\n', main_text)
if not main_match:
    raise SystemExit('main() insertion point not found')
main_prefix = main_match.group(0)
main_replacement = main_prefix + '''\tif pcOSZeroTouchWatchdogRequested(os.Args[1:]) {\n\t\tpcOSRunZeroTouchWatchdog()\n\t\treturn\n\t}\n'''
main_text = main_text[:main_match.start()] + main_replacement + main_text[main_match.end():]

# Keep a watchdog companion alive whenever the normal Supervisor is healthy.
# The companion itself is singleton-protected and uses a freshness marker, so
# this loop does not create a restart/process storm.
startup = '\tgo cleanupLoop()'
if startup not in main_text:
    raise SystemExit('cleanupLoop startup insertion point not found')
main_text = main_text.replace(startup, startup + '\n\tgo pcOSZeroTouchWatchdogEnsureLoop()', 1)

# Add a read-only verification command. It reports watchdog and SCM recovery
# state but cannot trigger a restart or modify business applications/database.
old = '''\tcase "pc_os_core_recovery_set":
\t\tout, err = pcOSSetCoreRecovery(c.Args)
\tcase "support_bundle":'''
new = '''\tcase "pc_os_core_recovery_set":
\t\tout, err = pcOSSetCoreRecovery(c.Args)
\tcase "pc_os_zero_touch_status":
\t\tout, err = pcOSZeroTouchStatus()
\tcase "support_bundle":'''
if old not in main_text:
    raise SystemExit('zero-touch command insertion point not found')
main_text = main_text.replace(old, new, 1)

# Runtime and PC OS module version move together.
if '5.1.1' not in main_text:
    raise SystemExit('v5.1.1 runtime base version not found')
main_text = main_text.replace('5.1.1', '5.1.2')

old_const = 'const pcOSVersion = "5.1.1"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.1 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.2"', 1)

# Surface the new module in the overview without altering protected business
# continuity semantics.
module_anchor = '"disaster_recovery",'
if module_anchor not in pc_text:
    raise SystemExit('pcOS module list anchor not found')
pc_text = pc_text.replace(module_anchor, module_anchor + '\n            "zero_touch_recovery",', 1)

for invariant in (
    'pcOSZeroTouchWatchdogRequested(os.Args[1:])',
    'go pcOSZeroTouchWatchdogEnsureLoop()',
    'pc_os_zero_touch_status',
    '5.1.2',
):
    if invariant not in main_text:
        raise SystemExit(f'missing v5.1.2 main invariant: {invariant}')
if '"zero_touch_recovery"' not in pc_text:
    raise SystemExit('missing zero_touch_recovery module invariant')

main.write_text(main_text)
pc_os.write_text(pc_text)
