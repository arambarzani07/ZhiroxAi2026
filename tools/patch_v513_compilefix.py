from pathlib import Path

p = Path('buildsrc/pc_os_watchdog_v512_windows.go')
text = p.read_text()
old = '''func pcOSZeroTouchServiceAccessProbe() map[string]interface{} {
	out := map[string]interface{}{"available": false, "destructive_action_executed": false}
	access := uintptr(pcOSZeroTouchServiceQueryStatus | pcOSZeroTouchServiceStart | pcOSZeroTouchServiceStop)
	svc, cleanup, err := pcOSOpenCoreService(access)
'''
new = '''func pcOSZeroTouchServiceAccessProbe() map[string]interface{} {
	out := map[string]interface{}{"available": false, "destructive_action_executed": false}
	access := uintptr(pcOSZeroTouchServiceQueryStatus | pcOSZeroTouchServiceStart | pcOSZeroTouchServiceStop)
	_, cleanup, err := pcOSOpenCoreService(access)
'''
if old not in text:
    raise SystemExit('v5.1.3 service access probe compile-fix target not found')
text = text.replace(old, new, 1)
p.write_text(text)
