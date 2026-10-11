from pathlib import Path

owner = Path('buildsrc/owner_actions.go')
main = Path('buildsrc/main.go')
owner_text = owner.read_text()
main_text = main.read_text()


def must_replace(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f'{label}: insertion point not found')
    return text.replace(old, new, 1)


# -----------------------------------------------------------------------------
# v5.0.7 Critical Protection Lock
# Non-overridable mutation protection for Windows, business Admin/Cashier,
# business SQL Server and RLS100 scale software. Read-only discovery is not
# affected. Existing Hikvision protection remains separate and unchanged.
# -----------------------------------------------------------------------------
critical_helpers = r'''

// criticalProtectedText identifies Windows/business/scale assets that must never
// be modified, deleted, moved, quarantined, terminated or disabled through the
// generic owner-action surface. This lock intentionally has NO override token.
func criticalProtectedText(s string) bool {
    raw := strings.TrimSpace(s)
    if raw == "" {
        return false
    }

    v := strings.ToLower(strings.ReplaceAll(raw, "/", `\`))
    v = strings.TrimSpace(v)
    clean := strings.TrimRight(v, `\`)

    // Exact system files outside the Windows directory.
    exactFiles := map[string]bool{
        `c:\pagefile.sys`: true,
        `c:\hiberfil.sys`: true,
        `c:\swapfile.sys`: true,
    }
    if exactFiles[clean] {
        return true
    }

    // Hard path boundaries. A match requires either the exact root or a child
    // path, so similarly named neighboring folders are not captured.
    roots := []string{
        `c:\windows`,
        `c:\program files\windowsapps`,
        `c:\programdata\microsoft`,
        `c:\program files\windows defender`,
        `c:\program files\windows defender advanced threat protection`,
        `c:\program files (x86)\windows defender`,

        `d:\database`,
        `d:\program files\microsoft sql server`,
        `d:\mssql17.mssqlserver01`,
        `c:\program files\microsoft sql server`,
        `c:\program files (x86)\microsoft sql server`,

        `c:\program files (x86)\densف ltd\rls1000arv125`,
        `c:\program files\seagull\printer drivers`,

        `c:\users\administrator\appdata\local\programs\supercashier license admin`,
        `c:\users\administrator\appdata\roaming\supercashier-license-admin`,
        `c:\users\administrator\appdata\local\supercashier-license-admin-updater`,

        `c:\users\administrator\appdata\local\programs\zhirox smart cashier`,
        `c:\users\administrator\appdata\roaming\zhirox-smart-cashier-windows`,
        `c:\users\administrator\appdata\local\zhirox-smart-cashier-windows-updater`,
        `c:\users\administrator\appdata\local\zhiroxcashier`,

        `c:\users\administrator\appdata\local\programs\supercashier offline`,
        `c:\users\administrator\appdata\roaming\zhirox-smart-cashier-offline-windows`,
        `c:\users\administrator\appdata\local\zhirox-smart-cashier-offline-windows-updater`,
    }
    for _, root := range roots {
        root = strings.TrimRight(strings.ToLower(root), `\`)
        if clean == root || strings.HasPrefix(clean, root+`\`) {
            return true
        }
    }

    // Registry roots are classified here for future structured guards. Generic
    // file mutation does not use registry paths.
    registryRoots := []string{
        `hklm:\system`,
        `hkey_local_machine\system`,
        `hklm:\software\microsoft\microsoft sql server`,
        `hkey_local_machine\software\microsoft\microsoft sql server`,
    }
    for _, root := range registryRoots {
        if clean == root || strings.HasPrefix(clean, root+`\`) {
            return true
        }
    }

    // Exact process/service names. Executable suffix is normalized away.
    base := strings.TrimSuffix(strings.ToLower(filepath.Base(clean)), ".exe")
    names := map[string]bool{
        // Business apps / scale.
        "cashier": true,
        "admin": true,
        "supercashier license admin": true,
        "zhirox smart cashier": true,
        "supercashier offline": true,
        "rls1000": true,
        "rtlabel": true,
        "rtplu": true,
        "seagull_driverstartup": true,

        // Business database / printing.
        "sqlservr": true,
        "sqlagent": true,
        "mssqlserver": true,
        "sqlserveragent": true,
        "sqlbrowser": true,
        "sqlwriter": true,
        "spooler": true,
        "spoolsv": true,

        // Windows core processes/services.
        "system": true,
        "smss": true,
        "csrss": true,
        "wininit": true,
        "services": true,
        "lsass": true,
        "winlogon": true,
        "svchost": true,
        "dwm": true,
        "explorer": true,
        "rpcss": true,
        "dcomlaunch": true,
        "eventlog": true,
        "plugplay": true,
        "power": true,
        "schedule": true,
        "winmgmt": true,
        "cryptsvc": true,
        "profsvc": true,
        "dhcp": true,
        "dnscache": true,
        "lanmanworkstation": true,
    }
    if names[base] || names[clean] {
        return true
    }

    return false
}

func guardCritical(values ...string) error {
    for _, v := range values {
        if criticalProtectedText(v) {
            return errors.New("critical Windows/business/scale protection lock: mutation refused")
        }
    }
    return nil
}
'''

marker = 'func allowProtectedOverride(args map[string]interface{}) bool {'
idx = owner_text.find(marker)
if idx < 0:
    raise SystemExit('critical helper insertion point not found')
owner_text = owner_text[:idx] + critical_helpers + '\n' + owner_text[idx:]

# ownerPath: hard-lock only mutation destinations/targets (selfCheck=true).
old = '''\tif err := guardProtected(args, abs); err != nil {\n\t\treturn "", err\n\t}\n\tif selfCheck {\n\t\tif err := guardSelfModify(args, abs); err != nil {'''
new = '''\tif selfCheck {\n\t\tif err := guardCritical(abs); err != nil {\n\t\t\treturn "", err\n\t\t}\n\t}\n\tif err := guardProtected(args, abs); err != nil {\n\t\treturn "", err\n\t}\n\tif selfCheck {\n\t\tif err := guardSelfModify(args, abs); err != nil {'''
owner_text = must_replace(owner_text, old, new, 'ownerPath critical lock')

# serviceControl: starting a critical service is allowed for recovery, while
# stop/restart is permanently blocked.
old = '''\tif name == "" {\n\t\treturn nil, errors.New("service name is required")\n\t}\n\tif err := guardProtected(args, name); err != nil {'''
new = '''\tif name == "" {\n\t\treturn nil, errors.New("service name is required")\n\t}\n\tif operation != "start" {\n\t\tif err := guardCritical(name); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}\n\tif err := guardProtected(args, name); err != nil {'''
owner_text = must_replace(owner_text, old, new, 'serviceControl critical lock')

# serviceStartTypeSet: startup configuration of critical services is immutable.
old = '''\tif name == "" {\n\t\treturn nil, errors.New("service name is required")\n\t}\n\tif err := guardProtected(args, name); err != nil {'''
new = '''\tif name == "" {\n\t\treturn nil, errors.New("service name is required")\n\t}\n\tif err := guardCritical(name); err != nil {\n\t\treturn nil, err\n\t}\n\tif err := guardProtected(args, name); err != nil {'''
owner_text = must_replace(owner_text, old, new, 'serviceStartTypeSet critical lock')

# processTerminate: close PID-only bypass, then guard process name/path.
old = '''\tif pid <= 0 && name == "" && path == "" {\n\t\treturn nil, errors.New("pid, name, or path is required")\n\t}\n\tif err := guardProtected(args, name, path); err != nil {'''
new = '''\tif pid <= 0 && name == "" && path == "" {\n\t\treturn nil, errors.New("pid, name, or path is required")\n\t}\n\tif pid > 0 {\n\t\treturn nil, errors.New("PID-only termination disabled by critical protection; use an explicit non-critical process name or path")\n\t}\n\tif err := guardCritical(name, path); err != nil {\n\t\treturn nil, err\n\t}\n\tif err := guardProtected(args, name, path); err != nil {'''
owner_text = must_replace(owner_text, old, new, 'processTerminate PID/critical lock')

owner.write_text(owner_text)

# restartApp directly terminates an executable and does not pass through
# ownerPath, so hard-lock critical app paths before termination.
old = '''\tabs, err := filepath.Abs(exe)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tif _, err := os.Stat(abs); err != nil {'''
new = '''\tabs, err := filepath.Abs(exe)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tif err := guardCritical(abs); err != nil {\n\t\treturn nil, err\n\t}\n\tif _, err := os.Stat(abs); err != nil {'''
main_text = must_replace(main_text, old, new, 'restartApp critical lock')

# Bump runtime version only after all protection patches succeeded.
main_text = main_text.replace('5.0.6', '5.0.7')
main.write_text(main_text)
