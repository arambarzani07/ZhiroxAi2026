from pathlib import Path

path = Path('buildsrc/pc_os_screen_observability_windows.go')
text = path.read_text()

# Replace the DDB/GetDIBits path with a top-down 32-bit DIB section. The DIB
# exposes its pixel memory directly, so screenshot capture no longer depends on
# GetDIBits behavior in the interactive user session.
old_proc = '\tpcOSCreateCompatibleBitmap       = pcOSGDI32.NewProc("CreateCompatibleBitmap")\n'
if old_proc not in text:
    raise SystemExit('CreateCompatibleBitmap proc anchor not found')
text = text.replace(old_proc, old_proc + '\tpcOSCreateDIBSection              = pcOSGDI32.NewProc("CreateDIBSection")\n\tpcOSGDIFlush                      = pcOSGDI32.NewProc("GdiFlush")\n', 1)
text = text.replace('\tpcOSGetDIBits                     = pcOSGDI32.NewProc("GetDIBits")\n', '', 1)


def replace_go_func(src: str, name: str, replacement: str) -> str:
    marker = 'func ' + name + '('
    start = src.find(marker)
    if start < 0:
        raise SystemExit(f'{name} not found')
    brace = src.find('{', start)
    if brace < 0:
        raise SystemExit(f'{name} opening brace not found')
    depth = 0
    in_string = False
    in_raw = False
    in_rune = False
    escaped = False
    i = brace
    while i < len(src):
        ch = src[i]
        if in_string:
            if escaped:
                escaped = False
            elif ch == '\\':
                escaped = True
            elif ch == '"':
                in_string = False
        elif in_raw:
            if ch == '`':
                in_raw = False
        elif in_rune:
            if escaped:
                escaped = False
            elif ch == '\\':
                escaped = True
            elif ch == "'":
                in_rune = False
        else:
            if ch == '"':
                in_string = True
            elif ch == '`':
                in_raw = True
            elif ch == "'":
                in_rune = True
            elif ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
                if depth == 0:
                    return src[:start] + replacement.rstrip() + src[i + 1:]
        i += 1
    raise SystemExit(f'{name} closing brace not found')

replacement = r'''func pcOSCapturePrimaryDisplayRGBA() (*image.RGBA, error) {
	wRaw, _, _ := pcOSGetSystemMetrics.Call(pcOSSMCXScreen)
	hRaw, _, _ := pcOSGetSystemMetrics.Call(pcOSSMCYScreen)
	w, h := int(int32(wRaw)), int(int32(hRaw))
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, fmt.Errorf("invalid primary display geometry %dx%d", w, h)
	}

	screenDC, _, callErr := pcOSGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed: %v", callErr)
	}
	defer pcOSReleaseDC.Call(0, screenDC)

	memDC, _, callErr := pcOSCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed: %v", callErr)
	}
	defer pcOSDeleteDC.Call(memDC)

	bmi := pcOSBitmapInfo{Header: pcOSBitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(pcOSBitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h),
		Planes:      1,
		BitCount:    32,
		Compression: pcOSBI_RGB,
		SizeImage:   uint32(w * h * 4),
	}}
	var bits uintptr
	bitmap, _, callErr := pcOSCreateDIBSection.Call(
		screenDC,
		uintptr(unsafe.Pointer(&bmi)),
		pcOSDIBRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if bitmap == 0 || bits == 0 {
		return nil, fmt.Errorf("CreateDIBSection failed: %v", callErr)
	}
	defer pcOSDeleteObject.Call(bitmap)

	old, _, _ := pcOSSelectObject.Call(memDC, bitmap)
	if old == 0 {
		return nil, errors.New("SelectObject DIB section failed")
	}
	defer pcOSSelectObject.Call(memDC, old)

	ret, _, callErr := pcOSBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, pcOSSRCCopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed: %v", callErr)
	}
	flushRet, _, flushErr := pcOSGDIFlush.Call()
	if flushRet == 0 {
		return nil, fmt.Errorf("GdiFlush failed: %v", flushErr)
	}

	raw := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, j := 0, 0; i+3 < len(raw) && j+3 < len(img.Pix); i, j = i+4, j+4 {
		img.Pix[j+0] = raw[i+2]
		img.Pix[j+1] = raw[i+1]
		img.Pix[j+2] = raw[i+0]
		img.Pix[j+3] = 0xFF
	}
	return img, nil
}'''.replace('\\t', '\t')

text = replace_go_func(text, 'pcOSCapturePrimaryDisplayRGBA', replacement)

for invariant in (
    'pcOSCreateDIBSection',
    'pcOSGDIFlush',
    'unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)',
    'Height:      -int32(h)',
):
    if invariant not in text:
        raise SystemExit(f'missing DIB hotfix invariant: {invariant}')
if 'pcOSGetDIBits.Call' in text:
    raise SystemExit('GetDIBits call remains after DIB hotfix')

path.write_text(text)
