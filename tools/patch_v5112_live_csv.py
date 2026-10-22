from pathlib import Path

main = Path("buildsrc/main.go")
pc = Path("buildsrc/pc_os_v510.go")
watch = Path("buildsrc/pc_os_itemname_watch_v518_windows.go")
csv = Path("buildsrc/pc_os_itemname_csv_v5112.go")
m = main.read_text(encoding="utf-8")
p = pc.read_text(encoding="utf-8")
w = watch.read_text(encoding="utf-8")
if '5.1.11' not in m or 'const pcOSVersion = "5.1.11"' not in p:
    raise SystemExit("v5.1.11 signed-candidate source is required; refusing mismatch")
if not csv.exists() or "pcOSItemNameRefreshCSV(now)" not in w:
    raise SystemExit("CSV source and hook missing")
m = m.replace("5.1.11", "5.1.12", 1)
p = p.replace('const pcOSVersion = "5.1.11"', 'const pcOSVersion = "5.1.12"', 1)
main.write_text(m, encoding="utf-8")
pc.write_text(p, encoding="utf-8")
print("v5.1.12 safe CSV candidate build source prepared; no protected PC mutation")
