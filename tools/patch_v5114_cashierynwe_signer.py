from pathlib import Path

main = Path('buildsrc/main.go')
pc = Path('buildsrc/pc_os_v510.go')
m = main.read_text(encoding='utf-8')
p = pc.read_text(encoding='utf-8')

# NEVER relabel an older v5.1.6/5.1.12 source as v5.1.14. Only the complete
# pinned v5.1.13 baseline from GitHub historical source is acceptable.
if m.count('5.1.13') < 1 or 'const pcOSVersion = "5.1.13"' not in p:
    raise SystemExit('fatal: exact complete v5.1.13 baseline is required')
for invariant in (
    'pcOSZeroTouchWatchdogEnsureLoop',
    'pc_os_screenshot',
    'pc_os_sql_compare_v2',
    'pc_os_itemname_probe',
    'pc_os_remote_canary_start',
    'pcOSItemNameWatchLoop',
    'case "cashierynwe_sign_release":',
    'case "cashierynwe_sign_status":',
):
    if invariant not in m:
        raise SystemExit('fatal: missing inherited or new capability: ' + invariant)
for filename in (
    'pc_os_csv_cloud_v5113_windows.go',
    'pc_os_itemname_csv_v5112.go',
    'pc_os_sql_compare_v5110_windows.go',
    'pc_os_cashierynwe_sign.go',
    'pc_os_cashierynwe_sign_policy.go',
    'pc_os_remote_canary_assets_v517_windows.go',
):
    if not Path('buildsrc', filename).is_file():
        raise SystemExit('fatal: missing full inherited binary source: ' + filename)

signer = Path('buildsrc/pc_os_cashierynwe_sign.go').read_text(encoding='utf-8')
for pin in ('-ExpectedCommit', '-ExpectedTag', '-RelayApprovalId', '-UseWindowsCertificateStore'):
    if pin not in signer:
        raise SystemExit('fatal: missing pinned signer invocation parameter: ' + pin)

m = m.replace('5.1.13', '5.1.14', 1)
p = p.replace('const pcOSVersion = "5.1.13"', 'const pcOSVersion = "5.1.14"', 1)
if 'const pcOSVersion = "5.1.14"' not in p:
    raise SystemExit('fatal: version transition failed')

main.write_text(m, encoding='utf-8')
pc.write_text(p, encoding='utf-8')
print('v5.1.14 unsigned candidate assembled from exact v5.1.13 source and bounded signer; no deployment or publishing')
