from pathlib import Path
import re

project_host = 'vllwundrtrgdqmkqbduy.supabase.co'
patched = False

for p in sorted(Path('buildsrc').glob('*.go')):
    text = p.read_text()
    marker = 'func downloadWithAgentAuth('
    start = text.find(marker)
    if start < 0:
        continue

    brace = text.find('{', start)
    if brace < 0:
        raise SystemExit(f'Cannot find function body in {p}')

    depth = 0
    end = None
    in_string = False
    escape = False
    for i in range(brace, len(text)):
        ch = text[i]
        if in_string:
            if escape:
                escape = False
            elif ch == '\\':
                escape = True
            elif ch == '"':
                in_string = False
            continue
        if ch == '"':
            in_string = True
            continue
        if ch == '{':
            depth += 1
        elif ch == '}':
            depth -= 1
            if depth == 0:
                end = i + 1
                break
    if end is None:
        raise SystemExit(f'Cannot determine function end in {p}')

    fn = text[start:end]
    req_match = re.search(r'(?m)^\s*([A-Za-z_]\w*)\s*,\s*err\s*:=\s*http\.NewRequest(?:WithContext)?\s*\(', fn)
    if not req_match:
        req_match = re.search(r'(?m)^\s*([A-Za-z_]\w*)\s*,\s*[^:=\n]+\s*:=\s*http\.NewRequest(?:WithContext)?\s*\(', fn)
    if not req_match:
        raise SystemExit(f'Cannot identify request variable in {p}')
    reqvar = req_match.group(1)

    do_match = re.search(r'(?m)^(\s*)resp\s*,\s*err\s*:=\s*([A-Za-z_]\w*)\.Do\(\s*'+re.escape(reqvar)+r'\s*\)', fn)
    if not do_match:
        raise SystemExit(f'Cannot identify HTTP client call in {p}')
    indent, clientvar = do_match.group(1), do_match.group(2)

    auth_guard = (
        f'{indent}if {reqvar}.URL == nil || {reqvar}.URL.Hostname() != "{project_host}" {{\n'
        f'{indent}\t{reqvar}.Header.Del("Authorization")\n'
        f'{indent}}}\n'
    )
    if f'{reqvar}.Header.Del("Authorization")' not in fn:
        fn = fn[:do_match.start()] + auth_guard + fn[do_match.start():]

    fn = re.sub(r'25\s*\*\s*time\.Second', '120 * time.Second', fn)
    fn = re.sub(
        r'(?m)^(\s*)resp\s*,\s*err\s*:=\s*'+re.escape(clientvar)+r'\.Do\(\s*'+re.escape(reqvar)+r'\s*\)',
        r'\1resp, err := zhiroxDoWithRetry('+clientvar+', '+reqvar+', 3)',
        fn,
        count=1,
    )

    text = text[:start] + fn + text[end:]
    if 'func zhiroxDoWithRetry(' not in text:
        text += '''\n\nfunc zhiroxDoWithRetry(client *http.Client, req *http.Request, attempts int) (*http.Response, error) {\n\tif attempts < 1 { attempts = 1 }\n\tvar resp *http.Response\n\tvar err error\n\tfor i := 0; i < attempts; i++ {\n\t\trequest := req\n\t\tif i > 0 { request = req.Clone(req.Context()) }\n\t\tresp, err = client.Do(request)\n\t\tif err == nil { return resp, nil }\n\t\tif i+1 < attempts { time.Sleep(time.Duration(i+1) * 2 * time.Second) }\n\t}\n\treturn resp, err\n}\n'''

    p.write_text(text)
    patched = True
    break

if not patched:
    raise SystemExit('downloadWithAgentAuth not found')
