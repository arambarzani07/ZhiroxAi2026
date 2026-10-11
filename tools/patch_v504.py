from pathlib import Path

p = Path('buildsrc/main.go')
text = p.read_text()


def function_bounds(src: str, name: str):
    marker = f'func {name}('
    start = src.find(marker)
    if start < 0:
        raise SystemExit(f'{name} not found')
    brace = src.find('{', start)
    if brace < 0:
        raise SystemExit(f'{name}: opening brace not found')
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
    // Owner policy: permanent automatic cleanup is always enabled, but only for
    // explicitly allowlisted Zhirox legacy artifacts and only after health gates.
    cfg.AutoCleanupOldVersions = true

    delay := cfg.CleanupDelaySeconds
    if delay < 120 {
        delay = 120
    }
    time.Sleep(time.Duration(delay) * time.Second)

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

cleanup_artifacts = r'''
func cleanupOldVersionArtifacts() ([]string, error) {
    exePath, err := os.Executable()
    if err != nil {
        return nil, fmt.Errorf("resolve current executable: %w", err)
    }
    exePath, err = filepath.Abs(exePath)
    if err != nil {
        return nil, fmt.Errorf("resolve current executable absolute path: %w", err)
    }
    exePath = filepath.Clean(exePath)

    // Only these two Zhirox roots are eligible. The active installation root is
    // derived from the running executable. The second root is the verified legacy
    // OneDrive/Desktop Zhirox folder from the previous installations.
    roots := []string{
        filepath.Dir(exePath),
        `C:\Users\Administrator\OneDrive\Desktop\zhiroxai`,
    }

    protected := map[string]bool{
        "zhiroxaisupervisor.exe": true,
        "zhiroxupdater.exe":     true,
        "zhiroxcoreservice.exe": true,
        "update-health.json":    true,
    }

    updaterArtifacts := map[string]bool{
        "zhiroxaisupervisor.exe.bak":      true,
        "zhiroxaisupervisor.exe.old":      true,
        "zhiroxaisupervisor.exe.new":      true,
        "zhiroxaisupervisor.exe.tmp":      true,
        "zhiroxaisupervisor.exe.part":     true,
        "zhiroxaisupervisor.exe.download": true,
        "zhiroxupdater.exe.bak":           true,
        "zhiroxupdater.exe.old":           true,
        "zhiroxupdater.exe.new":           true,
        "zhiroxupdater.exe.tmp":           true,
        "zhiroxupdater.exe.part":          true,
        "zhiroxupdater.exe.download":      true,
    }

    isAllowedLegacy := func(name string) bool {
        lower := strings.ToLower(name)
        if protected[lower] {
            return false
        }
        if updaterArtifacts[lower] {
            return true
        }
        if strings.HasPrefix(lower, "update-") &&
            (strings.HasSuffix(lower, ".tmp") || strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".download")) {
            return true
        }
        if !strings.HasSuffix(lower, ".exe") {
            return false
        }
        return strings.HasPrefix(lower, "zhiroxai-universalownercontrol-v") ||
            strings.HasPrefix(lower, "zhiroxai-zeroui-absolute-v") ||
            strings.HasPrefix(lower, "zhiroxai-final-background-service-") ||
            strings.HasPrefix(lower, "zhiroxai-background-service-")
    }

    seen := map[string]bool{}
    deleted := []string{}
    var firstErr error
    failures := 0

    for _, rawRoot := range roots {
        root := filepath.Clean(rawRoot)
        rootKey := strings.ToLower(root)
        if seen[rootKey] {
            continue
        }
        seen[rootKey] = true

        // Hard path boundary: cleanup can never run outside a directory named ZhiroxAI.
        if !strings.EqualFold(filepath.Base(root), "zhiroxai") {
            log.Printf("old-version cleanup refused non-Zhirox root: %s", root)
            continue
        }

        entries, err := os.ReadDir(root)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            failures++
            if firstErr == nil {
                firstErr = fmt.Errorf("read managed root %s: %w", root, err)
            }
            continue
        }

        for _, entry := range entries {
            if entry.IsDir() || !isAllowedLegacy(entry.Name()) {
                continue
            }

            candidate := filepath.Clean(filepath.Join(root, entry.Name()))
            if !strings.EqualFold(filepath.Dir(candidate), root) {
                continue
            }
            if strings.EqualFold(candidate, exePath) {
                continue
            }

            if err := os.Remove(candidate); err != nil {
                if os.IsNotExist(err) {
                    continue
                }
                failures++
                if firstErr == nil {
                    firstErr = fmt.Errorf("remove managed legacy artifact %s: %w", candidate, err)
                }
                continue
            }
            deleted = append(deleted, candidate)
        }
    }

    if failures > 0 {
        return deleted, fmt.Errorf("managed cleanup had %d failure(s); first: %w", failures, firstErr)
    }
    return deleted, nil
}
'''

text = replace_func(text, 'cleanupLoop', cleanup_loop)
text = replace_func(text, 'cleanupOldVersionArtifacts', cleanup_artifacts)
text = text.replace('5.0.1', '5.0.4').replace('5.0.2', '5.0.4').replace('5.0.3', '5.0.4')
p.write_text(text)
