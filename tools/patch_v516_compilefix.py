from pathlib import Path

path = Path('buildsrc/pc_os_screen_observability_windows.go')
text = path.read_text()
repls = [
    ('\tvar bits uintptr\n', '\tvar bits unsafe.Pointer\n'),
    ('\tif bitmap == 0 || bits == 0 {\n', '\tif bitmap == 0 || bits == nil {\n'),
    ('\traw := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)\n', '\traw := unsafe.Slice((*byte)(bits), w*h*4)\n'),
]
for old, new in repls:
    if old not in text:
        raise SystemExit(f'compile-fix anchor not found: {old.strip()}')
    text = text.replace(old, new, 1)
if 'var bits unsafe.Pointer' not in text or 'unsafe.Slice((*byte)(bits), w*h*4)' not in text:
    raise SystemExit('vet-safe DIB pointer invariants missing')
path.write_text(text)
