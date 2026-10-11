from pathlib import Path

main = Path('buildsrc/main.go')
text = main.read_text()

# v5.0.8 — one-time, owner-approved SSIS Scale Out log retention cleanup.
# This does NOT weaken the v5.0.7 critical path lock. Cleanup is internal,
# restricted to two exact flat log directories, two exact filename prefixes,
# .txt extension only, and files older than 14 days only.
ssis_cleanup = r'''

func cleanupSSISScaleOutLogsOnce() (int, int64, error) {
    exePath, err := os.Executable()
    if err != nil {
        return 0, 0, fmt.Errorf("resolve executable for SSIS cleanup marker: %w", err)
    }
    exePath, err = filepath.Abs(exePath)
    if err != nil {
        return 0, 0, fmt.Errorf("resolve executable absolute path for SSIS cleanup marker: %w", err)
    }
    activeRoot := filepath.Clean(filepath.Dir(exePath))
    if !strings.EqualFold(filepath.Base(activeRoot), "ZhiroxAI") {
        return 0, 0, fmt.Errorf("SSIS cleanup refused unexpected active root: %s", activeRoot)
    }

    marker := filepath.Join(activeRoot, "ssis-scaleout-log-cleanup-14d.done")
    if _, err := os.Stat(marker); err == nil {
        return 0, 0, nil
    } else if !os.IsNotExist(err) {
        return 0, 0, fmt.Errorf("check SSIS cleanup marker: %w", err)
    }

    cutoff := time.Now().Add(-14 * 24 * time.Hour)
    targets := []struct {
        dir    string
        prefix string
    }{
        {
            dir:    `C:\Windows\ServiceProfiles\SSISScaleOutMaster170\AppData\Local\SSIS\ScaleOut\17\Master`,
            prefix: "MastertService.log_",
        },
        {
            dir:    `C:\Windows\ServiceProfiles\SSISScaleOutWorker170\AppData\Local\SSIS\ScaleOut\17\Agent`,
            prefix: "WorkerAgentService.log_",
        },
    }

    deletedFiles := 0
    var deletedBytes int64
    failures := 0
    var firstErr error

    for _, target := range targets {
        root := filepath.Clean(target.dir)
        entries, err := os.ReadDir(root)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            failures++
            if firstErr == nil {
                firstErr = fmt.Errorf("read SSIS log root %s: %w", root, err)
            }
            continue
        }

        for _, entry := range entries {
            // Flat files only. Never recurse and never follow links/reparse-like entries.
            if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
                continue
            }
            name := entry.Name()
            if !strings.HasPrefix(name, target.prefix) || !strings.EqualFold(filepath.Ext(name), ".txt") {
                continue
            }

            info, err := entry.Info()
            if err != nil {
                failures++
                if firstErr == nil {
                    firstErr = fmt.Errorf("stat SSIS log %s: %w", name, err)
                }
                continue
            }
            if !info.ModTime().Before(cutoff) {
                continue
            }

            candidate := filepath.Clean(filepath.Join(root, name))
            if !strings.EqualFold(filepath.Dir(candidate), root) {
                continue
            }

            size := info.Size()
            if err := os.Remove(candidate); err != nil {
                if os.IsNotExist(err) {
                    continue
                }
                failures++
                if firstErr == nil {
                    firstErr = fmt.Errorf("remove SSIS log %s: %w", candidate, err)
                }
                continue
            }
            deletedFiles++
            deletedBytes += size
        }
    }

    if failures > 0 {
        return deletedFiles, deletedBytes, fmt.Errorf("SSIS 14-day cleanup had %d failure(s); first: %w", failures, firstErr)
    }

    summary := fmt.Sprintf(
        "completed=%s\r\nretention_days=14\r\ndeleted_files=%d\r\ndeleted_bytes=%d\r\n",
        time.Now().Format(time.RFC3339), deletedFiles, deletedBytes,
    )
    if err := os.WriteFile(marker, []byte(summary), 0600); err != nil {
        return deletedFiles, deletedBytes, fmt.Errorf("write SSIS cleanup marker: %w", err)
    }

    return deletedFiles, deletedBytes, nil
}
'''

marker = 'func cleanupLoop() {'
idx = text.find(marker)
if idx < 0:
    raise SystemExit('cleanupLoop insertion point not found')
text = text[:idx] + ssis_cleanup + '\n' + text[idx:]

old = '''        if len(deleted) > 0 {
            log.Printf("clean-runtime cleanup removed after health verification: %s", strings.Join(deleted, ", "))
        }
        return
'''
new = '''        if len(deleted) > 0 {
            log.Printf("clean-runtime cleanup removed after health verification: %s", strings.Join(deleted, ", "))
        }

        ssisDeleted, ssisBytes, ssisErr := cleanupSSISScaleOutLogsOnce()
        if ssisErr != nil {
            log.Printf("SSIS Scale Out 14-day cleanup: %v", ssisErr)
            time.Sleep(5 * time.Minute)
            continue
        }
        if ssisDeleted > 0 {
            log.Printf("SSIS Scale Out 14-day cleanup removed %d old log files (%d bytes)", ssisDeleted, ssisBytes)
        }
        return
'''
if old not in text:
    raise SystemExit('cleanupLoop call insertion point not found')
text = text.replace(old, new, 1)

text = text.replace('5.0.7', '5.0.8')
main.write_text(text)
