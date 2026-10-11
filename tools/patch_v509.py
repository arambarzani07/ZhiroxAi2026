from pathlib import Path

p = Path('buildsrc/main.go')
text = p.read_text()

probe = r'''

func binaryPhraseProbe(path string, needle string) (interface{}, error) {
    target, err := allowedPath(path)
    if err != nil {
        return nil, err
    }
    expected := filepath.Clean(`D:\DataBase\Cashier.exe`)
    if !strings.EqualFold(filepath.Clean(target), expected) {
        return nil, errors.New("binary_phrase_probe is locked to D:\\DataBase\\Cashier.exe")
    }

    info, err := os.Stat(target)
    if err != nil {
        return nil, err
    }
    if info.IsDir() {
        return nil, errors.New("target is a directory")
    }
    if info.Size() > 8*1024*1024 {
        return nil, errors.New("target too large for guarded binary phrase probe")
    }

    data, err := os.ReadFile(target)
    if err != nil {
        return nil, err
    }
    if strings.TrimSpace(needle) == "" {
        needle = "سوپەرمارکێتی کانی چنار"
    }

    encodeUTF16LE := func(s string) []byte {
        out := make([]byte, 0, len([]rune(s))*2)
        for _, r := range []rune(s) {
            if r > 0xFFFF {
                continue
            }
            out = append(out, byte(r&0xff), byte((r>>8)&0xff))
        }
        return out
    }
    findBytes := func(hay []byte, n []byte) int {
        if len(n) == 0 || len(hay) < len(n) {
            return -1
        }
        for i := 0; i <= len(hay)-len(n); i++ {
            ok := true
            for j := range n {
                if hay[i+j] != n[j] {
                    ok = false
                    break
                }
            }
            if ok {
                return i
            }
        }
        return -1
    }
    toHex := func(b []byte) string {
        var sb strings.Builder
        for _, v := range b {
            fmt.Fprintf(&sb, "%02x", v)
        }
        return sb.String()
    }

    utf8Needle := []byte(needle)
    utf16Needle := encodeUTF16LE(needle)
    utf8Offset := findBytes(data, utf8Needle)
    utf16Offset := findBytes(data, utf16Needle)

    partialNames := []string{"سوپەرمارکێتی", "کانی", "چنار"}
    partials := []map[string]interface{}{}
    for _, s := range partialNames {
        partials = append(partials, map[string]interface{}{
            "text": s,
            "utf8_offset": findBytes(data, []byte(s)),
            "utf16le_offset": findBytes(data, encodeUTF16LE(s)),
        })
    }

    candidates := []map[string]interface{}{}
    runStart := -1
    high := 0
    flush := func(end int) {
        if runStart < 0 {
            return
        }
        ln := end - runStart
        if ln >= 8 && ln <= 120 && high >= 3 && len(candidates) < 120 {
            raw := data[runStart:end]
            candidates = append(candidates, map[string]interface{}{
                "offset": runStart,
                "length": ln,
                "hex": toHex(raw),
            })
        }
        runStart = -1
        high = 0
    }
    for i, b := range data {
        printable := b >= 0x20 && b != 0x7f
        if printable {
            if runStart < 0 {
                runStart = i
                high = 0
            }
            if b >= 0x80 {
                high++
            }
            if i-runStart+1 > 120 {
                flush(i + 1)
            }
        } else {
            flush(i)
        }
    }
    flush(len(data))

    return map[string]interface{}{
        "path": target,
        "read_only": true,
        "zero_ui": true,
        "size": len(data),
        "needle": needle,
        "utf8_offset": utf8Offset,
        "utf16le_offset": utf16Offset,
        "partials": partials,
        "byte_candidates": candidates,
    }, nil
}
'''

marker = 'func containsCI(xs []string, s string) bool {'
idx = text.find(marker)
if idx < 0:
    raise SystemExit('containsCI insertion point not found')
text = text[:idx] + probe + '\n' + text[idx:]

old = '''\tcase "read_text":
\t\tout, err = readText(sarg(c.Args, "path"))
\tcase "support_bundle":'''
new = '''\tcase "read_text":
\t\tout, err = readText(sarg(c.Args, "path"))
\tcase "binary_phrase_probe":
\t\tout, err = binaryPhraseProbe(sarg(c.Args, "path"), sarg(c.Args, "needle"))
\tcase "support_bundle":'''
if old not in text:
    raise SystemExit('command switch insertion point not found')
text = text.replace(old, new, 1)

text = text.replace('5.0.8', '5.0.9')
p.write_text(text)
