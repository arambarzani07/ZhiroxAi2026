from pathlib import Path

p = Path('buildsrc/pc_os_v510.go')
text = p.read_text()


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
    for i in range(brace, len(src)):
        ch = src[i]
        if in_string:
            if escaped:
                escaped = False
            elif ch == '\\':
                escaped = True
            elif ch == '"':
                in_string = False
            continue
        if in_raw:
            if ch == '`':
                in_raw = False
            continue
        if in_rune:
            if escaped:
                escaped = False
            elif ch == '\\':
                escaped = True
            elif ch == "'":
                in_rune = False
            continue
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
    raise SystemExit(f'{name} closing brace not found')


replacement = r'''func pcOSLoadPublicKey() (ed25519.PublicKey, error) {
	keyHex := strings.TrimSpace(pcOSPinnedUpdatePublicKeyHex)
	if keyHex == "" {
		return nil, errors.New("Ed25519 update public key is not pinned in this build")
	}
	b, err := hex.DecodeString(keyHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid pinned Ed25519 update public key")
	}
	return ed25519.PublicKey(b), nil
}'''

text = replace_go_func(text, 'pcOSLoadPublicKey', replacement)
if 'update-public-key.hex' in text[text.find('func pcOSLoadPublicKey('):text.find('func pcOSVerifyUpdateWithKey(')]:
    raise SystemExit('mutable update public-key file reference remains in pcOSLoadPublicKey')
p.write_text(text)
