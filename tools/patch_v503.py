from pathlib import Path

p = Path('buildsrc/main.go')
text = p.read_text()


def function_bounds(src: str, name: str):
    marker = f'func {name}('
    start = src.find(marker)
    if start < 0:
        raise SystemExit(f'{name} not found')
    brace = src.find('{', start)
    depth = 0
    in_string = False
    escape = False
    for i in range(brace, len(src)):
        ch = src[i]
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
                return start, i + 1
    raise SystemExit(f'{name}: function end not found')


def replace_func(src: str, name: str, replacement: str):
    start, end = function_bounds(src, name)
    return src[:start] + replacement.strip() + src[end:]


cleanup_loop = r'''
func cleanupLoop() {
    if !cfg.AutoCleanupOldVersions {
        log.Printf("old-version cleanup disabled")
        return
    }

    delay := cfg.CleanupDelaySeconds
    if delay < 120 {
        delay = 120
    }
    time.Sleep(time.Duration(delay) * time.Second)

    // A previous version is removed only after the new process can prove
    // stable cloud connectivity with three consecutive successful heartbeats.
    for {
        healthy := true
        for i := 0; i < 3; i++ {
            if err := sendHeartbeat(); err != nil {
                log.Printf("old-version cleanup health check %d/3 failed: %v", i+1, err)
                healthy = false
                break
            }
            if i < 2 {
                time.Sleep(15 * time.Second)
            }
        }
        if !healthy {
            time.Sleep(5 * time.Minute)
            continue
        }

        deleted, err := cleanupOldVersionArtifacts()
        if err != nil {
            log.Printf("old-version cleanup: %v", err)
            time.Sleep(5 * time.Minute)
            continue
        }
        if len(deleted) > 0 {
            log.Printf("old-version cleanup removed after health verification: %s", strings.Join(deleted, ", "))
        }
        return
    }
}
'''

text = replace_func(text, 'cleanupLoop', cleanup_loop)
text = text.replace('5.0.1', '5.0.3').replace('5.0.2', '5.0.3')
p.write_text(text)
