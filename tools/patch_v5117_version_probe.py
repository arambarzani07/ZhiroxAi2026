from pathlib import Path

main = Path("buildsrc/main.go")
pc = Path("buildsrc/pc_os_v510.go")
m = main.read_text(encoding="utf-8-sig")
p = pc.read_text(encoding="utf-8-sig")
if m.count("5.1.15") < 1 or p.count('const pcOSVersion = "5.1.15"') != 1:
    raise SystemExit("exact reconstructed v5.1.15 source required")
if not all(Path("buildsrc", name).is_file() for name in (
    "pc_os_cashierynwe_version_policy.go", "pc_os_cashierynwe_version_windows.go")):
    raise SystemExit("missing native probe")
main.write_text(m.replace("5.1.15", "5.1.17", 1), encoding="utf-8")
pc.write_text(p.replace('const pcOSVersion = "5.1.15"', 'const pcOSVersion = "5.1.17"', 1), encoding="utf-8")
