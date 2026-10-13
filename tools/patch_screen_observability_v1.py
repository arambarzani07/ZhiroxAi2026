from pathlib import Path
import re

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')

main_text = main.read_text()
pc_text = pc_os.read_text()

# The helper is the same signed GUI binary launched into the active interactive
# Windows session. It exits before service startup and accepts only a fixed
# observation-mode allowlist implemented in pc_os_screen_observability_windows.go.
main_match = re.search(r'func main\(\)\s*\{\n', main_text)
if not main_match:
    raise SystemExit('main() insertion point not found')
needle = main_match.group(0)
helper = needle + '''\tif pcOSScreenObservationHelperRequested(os.Args[1:]) {\n\t\tos.Exit(pcOSRunScreenObservationHelper(os.Args[1:]))\n\t}\n'''
main_text = main_text[:main_match.start()] + helper + main_text[main_match.end():]

# Add read-only structured actions next to the existing zero-touch status action.
old = '''\tcase "pc_os_zero_touch_status":
\t\tout, err = pcOSZeroTouchStatus()
\tcase "support_bundle":'''
new = '''\tcase "pc_os_zero_touch_status":
\t\tout, err = pcOSZeroTouchStatus()
\tcase "pc_os_active_window":
\t\tout, err = pcOSActiveWindow()
\tcase "pc_os_screenshot":
\t\tout, err = pcOSCaptureScreenshot(c.Args)
\tcase "support_bundle":'''
if old not in main_text:
    raise SystemExit('screen observability command insertion point not found')
main_text = main_text.replace(old, new, 1)

module_anchor = '"zero_touch_recovery",'
if module_anchor not in pc_text:
    raise SystemExit('zero_touch_recovery module anchor not found')
pc_text = pc_text.replace(module_anchor, module_anchor + '\n            "screen_observability_read_only",', 1)

for invariant in (
    'pcOSScreenObservationHelperRequested(os.Args[1:])',
    'pc_os_active_window',
    'pc_os_screenshot',
):
    if invariant not in main_text:
        raise SystemExit(f'missing screen observability invariant: {invariant}')
if '"screen_observability_read_only"' not in pc_text:
    raise SystemExit('missing screen observability module invariant')

main.write_text(main_text)
pc_os.write_text(pc_text)
