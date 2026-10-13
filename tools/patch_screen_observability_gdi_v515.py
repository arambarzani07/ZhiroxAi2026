from pathlib import Path

path = Path('buildsrc/pc_os_screen_observability_windows.go')
text = path.read_text()

old = '''\told, _, _ := pcOSSelectObject.Call(memDC, bitmap)
\tif old == 0 {
\t\treturn nil, errors.New("SelectObject failed")
\t}
\tdefer pcOSSelectObject.Call(memDC, old)
\tret, _, callErr := pcOSBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, pcOSSRCCopy)
\tif ret == 0 {
\t\treturn nil, fmt.Errorf("BitBlt failed: %v", callErr)
\t}
\tpixels := make([]byte, w*h*4)
'''

new = '''\told, _, _ := pcOSSelectObject.Call(memDC, bitmap)
\tif old == 0 {
\t\treturn nil, errors.New("SelectObject failed")
\t}
\tbitmapSelected := true
\tdefer func() {
\t\tif bitmapSelected {
\t\t\tpcOSSelectObject.Call(memDC, old)
\t\t}
\t}()
\tret, _, callErr := pcOSBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, pcOSSRCCopy)
\tif ret == 0 {
\t\treturn nil, fmt.Errorf("BitBlt failed: %v", callErr)
\t}
\t// GetDIBits requires the bitmap not to be selected into any DC.
\trestored, _, _ := pcOSSelectObject.Call(memDC, old)
\tif restored == 0 {
\t\treturn nil, errors.New("SelectObject restore failed before GetDIBits")
\t}
\tbitmapSelected = false
\tpixels := make([]byte, w*h*4)
'''

if old not in text:
    raise SystemExit('GDI capture patch insertion point not found')
text = text.replace(old, new, 1)

old_call = '''\tret, _, callErr = pcOSGetDIBits.Call(memDC, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bmi)), pcOSDIBRGBColors)
'''
new_call = '''\tret, _, callErr = pcOSGetDIBits.Call(screenDC, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bmi)), pcOSDIBRGBColors)
'''
if old_call not in text:
    raise SystemExit('GetDIBits call site not found')
text = text.replace(old_call, new_call, 1)

for invariant in (
    'bitmapSelected := true',
    'SelectObject restore failed before GetDIBits',
    'pcOSGetDIBits.Call(screenDC, bitmap',
):
    if invariant not in text:
        raise SystemExit(f'missing GDI hotfix invariant: {invariant}')

path.write_text(text)
