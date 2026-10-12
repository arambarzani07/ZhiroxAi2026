from pathlib import Path
import re

main = Path('buildsrc/main.go')
owner = Path('buildsrc/owner_actions.go')
pc_os = Path('buildsrc/pc_os_v510.go')

main_text = main.read_text()
owner_text = owner.read_text()
pc_text = pc_os.read_text()

# -----------------------------------------------------------------------------
# v5.1.1 Self-Healing hardening
# - Generic stop/restart of the Zhirox Core service is permanently blocked.
# - A dedicated exact-confirmation action configures bounded Windows SCM failure
#   recovery without cmd.exe/PowerShell, reboot actions, or external commands.
# - Read-only recovery status remains available for verification.
# -----------------------------------------------------------------------------

# Wire the dedicated recovery surface into the already-patched PC OS switch.
old = '''\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "support_bundle":'''
new = '''\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "pc_os_core_recovery_status":
\t\tout, err = pcOSCoreRecoveryStatus()
\tcase "pc_os_core_recovery_set":
\t\tout, err = pcOSSetCoreRecovery(c.Args)
\tcase "support_bundle":'''
if old not in main_text:
    raise SystemExit('v5.1.1 recovery command insertion point not found')
main_text = main_text.replace(old, new, 1)

# The legacy restart_service path is distinct from owner service_control and was
# the path that could stop the parent Core before the child Agent finished the
# restart. Keep start-only recovery through service_control, but refuse generic
# restart_service for the Core service itself.
pat = re.compile(r'(?m)^(\tcase "restart_service":\n)(\t\t[^\n]+\n)')
m = pat.search(main_text)
if not m:
    raise SystemExit('restart_service dispatch block not found')
legacy_dispatch = m.group(2).rstrip('\n')
legacy_dispatch_nested = '\t' + legacy_dispatch
replacement = (
    '\tcase "restart_service":\n'
    '\t\tif pcOSIsCoreServiceTarget(sarg(c.Args, "name")) {\n'
    '\t\t\terr = errors.New("generic Zhirox Core restart is locked; use start-only recovery or dedicated self-healing")\n'
    '\t\t} else {\n'
    + legacy_dispatch_nested + '\n'
    '\t\t}\n'
)
main_text = main_text[:m.start()] + replacement + main_text[m.end():]

# Extend the non-overridable Critical Protection Lock so owner service_control
# can START the Core for recovery but can never stop/restart it, and startup mode
# cannot be changed through the generic action surface.
needle = '"lanmanworkstation": true,'
pos = owner_text.find(needle)
if pos < 0:
    raise SystemExit('critical service-name insertion point not found')
insert_at = pos + len(needle)
owner_text = owner_text[:insert_at] + '''
        "zhiroxaicore": true,
        "zhirox ai core service": true,
        "zhiroxcoreservice": true,'''+ owner_text[insert_at:]

# Runtime and PC OS module version move together. Refuse partial version bumps.
if '5.1.0' not in main_text:
    raise SystemExit('v5.1.0 runtime base version not found')
main_text = main_text.replace('5.1.0', '5.1.1')

old_const = 'const pcOSVersion = "5.1.0"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.0 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.1"', 1)

# Fail closed if the self-healing invariants disappeared during source evolution.
for invariant in (
    'pc_os_core_recovery_status',
    'pc_os_core_recovery_set',
    'pcOSIsCoreServiceTarget',
    'generic Zhirox Core restart is locked',
):
    if invariant not in main_text:
        raise SystemExit(f'missing main self-healing invariant: {invariant}')
for invariant in ('"zhiroxaicore": true', '"zhirox ai core service": true'):
    if invariant not in owner_text:
        raise SystemExit(f'missing critical-lock invariant: {invariant}')

main.write_text(main_text)
owner.write_text(owner_text)
pc_os.write_text(pc_text)
