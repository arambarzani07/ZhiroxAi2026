from pathlib import Path

p = Path('buildsrc/main.go')
text = p.read_text()

text = text.replace('5.0.1', '5.0.6').replace('5.0.2', '5.0.6').replace('5.0.3', '5.0.6').replace('5.0.4', '5.0.6').replace('5.0.5', '5.0.6')

needle = 'deleted := []string{}'
insert = r'''
    // Owner-approved Clean Runtime policy: after the existing health gate,
    // remove only these audited obsolete installer/backup files from the active ZhiroxAI root.
    auditedObsolete := []string{
        "ZhiroxCoreService.exe.pre-v4.8.bak",
        "ZhiroxUpdater.exe.pre-v4.8.bak",
        "config.json.pre-v4.8.bak",
        "install-success-v4.5.txt",
        "install-v4.8.log",
        "install-v4.9.log",
        "install-v5.0.log",
    }
    for _, name := range auditedObsolete {
        candidate := filepath.Clean(filepath.Join(activeRoot, name))
        if strings.EqualFold(filepath.Dir(candidate), activeRoot) {
            if err := os.Remove(candidate); err == nil {
                deleted = append(deleted, candidate)
            } else if !os.IsNotExist(err) {
                log.Printf("clean-runtime could not remove %s: %v", candidate, err)
            }
        }
    }

    // Remove only the two audited legacy backup directories, and only when empty.
    // os.Remove is intentionally non-recursive: unknown contents are never deleted.
    auditedEmptyDirs := []string{
        filepath.Join(activeRoot, "backup", "pre-v4.5-20261010-172144"),
        filepath.Join(activeRoot, "backup"),
    }
    for _, candidate := range auditedEmptyDirs {
        candidate = filepath.Clean(candidate)
        if !strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(activeRoot)+string(os.PathSeparator)) {
            continue
        }
        if err := os.Remove(candidate); err == nil {
            deleted = append(deleted, candidate)
        } else if !os.IsNotExist(err) {
            log.Printf("clean-runtime kept non-empty/locked directory %s: %v", candidate, err)
        }
    }
'''
if needle not in text:
    raise SystemExit('cleanup insertion point not found')
text = text.replace(needle, needle + '\n' + insert, 1)
text = text.replace('old-version cleanup removed after health verification', 'clean-runtime cleanup removed after health verification')
p.write_text(text)
