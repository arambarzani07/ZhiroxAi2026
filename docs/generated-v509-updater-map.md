# Generated updater integration map

> Static source only; secrets/config values are not included.

## `checkForUpdate`

```go
func checkForUpdate() error {
	if !cfg.AutoUpdate {
		return nil
	}
	manifestURL := strings.TrimSpace(cfg.UpdateManifestURL)
	if manifestURL == "" {
		return nil
	}
	if !updateURLAllowed(manifestURL) {
		return errors.New("update manifest URL must use HTTPS (HTTP allowed only for localhost)")
	}

	b, err := downloadWithAgentAuth(manifestURL)
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}

	var m UpdateManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("manifest JSON: %w", err)
	}
	if strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.URL) == "" || strings.TrimSpace(m.SHA256) == "" {
		return errors.New("manifest missing version/url/sha256")
	}
	if !newerVersion(m.Version, version) {
		return nil
	}

	fileURL, err := absoluteUpdateURL(manifestURL, m.URL)
	if err != nil {
		return err
	}
	payload, err := downloadWithAgentAuth(fileURL)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}

	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	expected := strings.ToLower(strings.TrimSpace(m.SHA256))
	if actual != expected {
		return fmt.Errorf("update SHA256 mismatch: expected %s got %s", expected, actual)
	}

	newPath := filepath.Join(baseDir, "ZhiroxAISupervisor.exe.new")
	if err := os.WriteFile(newPath, payload, 0700); err != nil {
		return fmt.Errorf("stage update: %w", err)
	}

	updater := filepath.Join(baseDir, "ZhiroxUpdater.exe")
	if _, err := os.Stat(updater); err != nil {
		_ = os.Remove(newPath)
		return errors.New("ZhiroxUpdater.exe is missing")
	}

	currentExe, err := os.Executable()
	if err != nil {
		_ = os.Remove(newPath)
		return err
	}

	log.Printf("auto-update: v%s -> v%s verified; installing silently", version, m.Version)
	cmd := hiddenCommand(
		updater,
		"--pid", strconv.Itoa(os.Getpid()),
		"--current", currentExe,
		"--new", newPath,
		"--version", strings.TrimSpace(m.Version),
		"--sha256", expected,
		"--health-marker", filepath.Join(baseDir, "update-health.json"),
	)
	cmd.Dir = baseDir
	if err := cmd.Start(); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("start updater: %w", err)
	}

	os.Exit(0)
	return nil
}
```

## `autoUpdateLoop`

```go
func autoUpdateLoop() {
	if !cfg.AutoUpdate {
		log.Printf("auto-update disabled")
		return
	}
	time.Sleep(20 * time.Second)
	for {
		if err := checkForUpdate(); err != nil {
			log.Printf("auto-update: %v", err)
		}
		time.Sleep(time.Duration(cfg.UpdateIntervalMinutes) * time.Minute)
	}
}
```

## `downloadWithAgentAuth`

```go
func downloadWithAgentAuth(rawURL string) ([]byte, error) {
	if !updateURLAllowed(rawURL) {
		return nil, errors.New("update URL must use HTTPS (HTTP allowed only for localhost)")
	}
	req, _ := http.NewRequest("GET", rawURL, nil)
	relayHeaders(req)
	if req.URL == nil || req.URL.Hostname() != "vllwundrtrgdqmkqbduy.supabase.co" {
		req.Header.Del("Authorization")
	}
	resp, err := zhiroxDoWithRetry(httpClient, req, 3)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
}
```

## `absoluteUpdateURL`

```go
func absoluteUpdateURL(manifestURL, fileURL string) (string, error) {
	u, err := url.Parse(fileURL)
	if err != nil {
		return "", err
	}
	if u.IsAbs() {
		return u.String(), nil
	}
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(u).String(), nil
}
```

## `updateURLAllowed`

```go
func updateURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" {
		h := strings.ToLower(u.Hostname())
		return h == "127.0.0.1" || h == "localhost" || h == "::1"
	}
	return false
}
```

## `newerVersion`

```go
func newerVersion(remote, current string) bool {
	a := versionParts(remote)
	b := versionParts(current)
	for i := 0; i < 3; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}
```

## `versionParts`

```go
func versionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
	main := strings.SplitN(v, "-", 2)[0]
	raw := strings.Split(main, ".")
	out := make([]int, 3)
	for i := 0; i < len(out) && i < len(raw); i++ {
		n, _ := strconv.Atoi(raw[i])
		out[i] = n
	}
	return out
}
```

## `writeHealthMarker`

```go
func writeHealthMarker() {
	marker := map[string]interface{}{
		"version":    version,
		"agent_id":   cfg.AgentID,
		"pid":        os.Getpid(),
		"started_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, _ := json.Marshal(marker)
	_ = os.WriteFile(filepath.Join(baseDir, "update-health.json"), b, 0600)
}
```

## `cleanupOldVersionArtifacts`

```go
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

    activeRoot := filepath.Clean(filepath.Dir(exePath))
    legacyRoot := filepath.Clean(`C:\Users\Administrator\OneDrive\Desktop\zhiroxai`)
    roots := []string{activeRoot, legacyRoot}

    activeProtected := map[string]bool{
        "zhiroxaisupervisor.exe": true,
        "zhiroxupdater.exe":     true,
        "zhiroxcoreservice.exe": true,
        "update-health.json":    true,
        "config.json":           true,
    }

    updaterArtifacts := map[string]bool{
        "zhiroxaisupervisor.exe.bak":      true,
        "zhiroxaisupervisor.exe.old":      true,
        "zhiroxaisupervisor.exe.new":      true,
        "zhiroxaisupervisor.exe.tmp":      true,
        "zhiroxaisupervisor.exe.part":     true,
        "zhiroxaisupervisor.exe.download": true,
    }

    isAllowed := func(name string, root string) bool {
        lower := strings.ToLower(name)
        isLegacyRoot := strings.EqualFold(root, legacyRoot)

        // The OneDrive/Desktop folder is a verified legacy-only location.
        // Its stale Supervisor/Updater copies are eligible; the live copies are in ProgramData.
        if isLegacyRoot && (lower == "zhiroxaisupervisor.exe" || lower == "zhiroxupdater.exe") {
            return true
        }

        if activeProtected[lower] {
            return false
        }
        if updaterArtifacts[lower] {
            return true
        }
        if strings.HasPrefix(lower, "zhiroxaisupervisor.exe.pre-v") && strings.HasSuffix(lower, ".bak") {
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
            if entry.IsDir() || !isAllowed(entry.Name(), root) {
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
```

## `UpdateManifest`

```go
type UpdateManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes,omitempty"`
}
```
