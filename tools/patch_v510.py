from pathlib import Path

main = Path('buildsrc/main.go')
text = main.read_text()

# Wire the enterprise PC OS surface into the existing structured command switch.
# All remote pc_os_* commands in v5.1.0 remain read-only. Local module writes are
# limited to atomic PC OS status files under the active ZhiroxAI root.
old = '''\tcase "binary_phrase_probe":
\t\tout, err = binaryPhraseProbe(sarg(c.Args, "path"), sarg(c.Args, "needle"))
\tcase "support_bundle":'''
new = '''\tcase "binary_phrase_probe":
\t\tout, err = binaryPhraseProbe(sarg(c.Args, "path"), sarg(c.Args, "needle"))
\tcase "pc_os_overview":
\t\tout, err = pcOSWindowsOverview()
\tcase "pc_os_modules":
\t\tout, err = pcOSModules()
\tcase "pc_os_guardian":
\t\tout, err = pcOSWindowsGuardian()
\tcase "pc_os_backup":
\t\tout, err = pcOSBackupVault()
\tcase "pc_os_continuity":
\t\tout, err = pcOSWindowsContinuity()
\tcase "pc_os_update_chain":
\t\tout, err = pcOSUpdateChain()
\tcase "pc_os_verify_update":
\t\tout, err = pcOSVerifyUpdate(sarg(c.Args, "manifest_path"), sarg(c.Args, "artifact_path"))
\tcase "pc_os_disk":
\t\tout, err = pcOSWindowsDiskIntelligence()
\tcase "pc_os_network":
\t\tout, err = pcOSNetwork()
\tcase "pc_os_security":
\t\tout, err = pcOSWindowsSecurity()
\tcase "pc_os_files":
\t\tout, err = pcOSFileManager(sarg(c.Args, "path"))
\tcase "pc_os_inventory":
\t\tout, err = pcOSWindowsInventory()
\tcase "pc_os_ai":
\t\tout, err = pcOSWindowsAI()
\tcase "pc_os_zero_ui":
\t\tout, err = pcOSZeroUI()
\tcase "pc_os_disaster_recovery":
\t\tout, err = pcOSWindowsDisasterRecovery()
\tcase "support_bundle":'''
if old not in text:
    raise SystemExit('v5.1.0 command switch insertion point not found')
text = text.replace(old, new, 1)

# Start shellless local and authenticated cloud telemetry beside the existing
# cleanup worker. Neither loop can restart/stop/delete business assets or touch DVR.
needle = '\tgo cleanupLoop()'
if needle not in text:
    raise SystemExit('cleanupLoop startup insertion point not found')
text = text.replace(needle, needle + '\n\tgo pcOSBackgroundLoop()\n\tgo pcOSCloudLoop()', 1)

# Extend the legacy manifest shape without breaking old field names. v5.1+ requires
# Size + Signature for every NEWER version before the payload is staged or executed.
manifest_old = '''type UpdateManifest struct {
\tVersion string `json:"version"`
\tURL     string `json:"url"`
\tSHA256  string `json:"sha256"`
\tNotes   string `json:"notes,omitempty"`
}'''
manifest_new = '''type UpdateManifest struct {
\tVersion   string `json:"version"`
\tURL       string `json:"url"`
\tSHA256    string `json:"sha256"`
\tSize      int64  `json:"size"`
\tSignature string `json:"signature"`
\tAlgorithm string `json:"algorithm,omitempty"`
\tNotes     string `json:"notes,omitempty"`
}'''
if manifest_old not in text:
    raise SystemExit('legacy UpdateManifest shape not found')
text = text.replace(manifest_old, manifest_new, 1)


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


signed_check = r'''func checkForUpdate() error {
\tif !cfg.AutoUpdate {
\t\treturn nil
\t}
\tmanifestURL := strings.TrimSpace(cfg.UpdateManifestURL)
\tif manifestURL == "" {
\t\treturn nil
\t}
\tif !updateURLAllowed(manifestURL) {
\t\treturn errors.New("update manifest URL must use HTTPS (HTTP allowed only for localhost)")
\t}

\tb, err := downloadWithAgentAuth(manifestURL)
\tif err != nil {
\t\treturn fmt.Errorf("manifest: %w", err)
\t}

\tvar m UpdateManifest
\tif err := json.Unmarshal(b, &m); err != nil {
\t\treturn fmt.Errorf("manifest JSON: %w", err)
\t}
\tif strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.URL) == "" || strings.TrimSpace(m.SHA256) == "" {
\t\treturn errors.New("manifest missing version/url/sha256")
\t}
\tif !newerVersion(m.Version, version) {
\t\treturn nil
\t}

\t// v5.1+ fail-closed trust gate. A newer build without size/signature is never staged.
\tif m.Size <= 0 || strings.TrimSpace(m.Signature) == "" {
\t\treturn errors.New("newer update rejected: signed manifest requires size and Ed25519 signature")
\t}
\tif strings.TrimSpace(m.Algorithm) != "" && !strings.EqualFold(strings.TrimSpace(m.Algorithm), "Ed25519") {
\t\treturn errors.New("newer update rejected: unsupported signature algorithm")
\t}

\tfileURL, err := absoluteUpdateURL(manifestURL, m.URL)
\tif err != nil {
\t\treturn err
\t}
\tpayload, err := downloadWithAgentAuth(fileURL)
\tif err != nil {
\t\treturn fmt.Errorf("download update: %w", err)
\t}

\texpected, err := pcOSVerifySignedUpdatePayload(m.Version, m.SHA256, m.Size, m.Signature, payload)
\tif err != nil {
\t\treturn fmt.Errorf("signed update verification: %w", err)
\t}

\t// Only a fully verified payload reaches disk.
\tnewPath := filepath.Join(baseDir, "ZhiroxAISupervisor.exe.new")
\tif err := os.WriteFile(newPath, payload, 0700); err != nil {
\t\treturn fmt.Errorf("stage verified update: %w", err)
\t}

\tupdater := filepath.Join(baseDir, "ZhiroxUpdater.exe")
\tif _, err := os.Stat(updater); err != nil {
\t\t_ = os.Remove(newPath)
\t\treturn errors.New("ZhiroxUpdater.exe is missing")
\t}

\tcurrentExe, err := os.Executable()
\tif err != nil {
\t\t_ = os.Remove(newPath)
\t\treturn err
\t}

\tlog.Printf("auto-update: v%s -> v%s SHA-256 + Ed25519 verified; installing silently", version, m.Version)
\tcmd := hiddenCommand(
\t\tupdater,
\t\t"--pid", strconv.Itoa(os.Getpid()),
\t\t"--current", currentExe,
\t\t"--new", newPath,
\t\t"--version", strings.TrimSpace(m.Version),
\t\t"--sha256", expected,
\t\t"--health-marker", filepath.Join(baseDir, "update-health.json"),
\t)
\tcmd.Dir = baseDir
\tif err := cmd.Start(); err != nil {
\t\t_ = os.Remove(newPath)
\t\treturn fmt.Errorf("start updater: %w", err)
\t}

\tos.Exit(0)
\treturn nil
}'''
# The raw literal above uses escaped tabs/newlines only for readability inside Python;
# turn literal backslash-t into real tabs before inserting Go source.
signed_check = signed_check.replace('\\t', '\t')
text = replace_go_func(text, 'checkForUpdate', signed_check)

if '5.0.9' not in text:
    raise SystemExit('expected v5.0.9 base version not found')
text = text.replace('5.0.9', '5.1.0')
main.write_text(text)
