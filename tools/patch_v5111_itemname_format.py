from pathlib import Path
main=Path("buildsrc/main.go")
pc=Path("buildsrc/pc_os_v510.go")
m=main.read_text(encoding="utf-8")
p=pc.read_text(encoding="utf-8")
if '5.1.10' not in m or 'const pcOSVersion = "5.1.10"' not in p:
 raise SystemExit("required fixed v5.1.10 baseline missing")
m=m.replace('5.1.10','5.1.11',1)
p=p.replace('const pcOSVersion = "5.1.10"','const pcOSVersion = "5.1.11"',1)
if 'pc_os_sql_compare_v2' not in m or 'pc_os_itemname_probe' not in m:raise SystemExit("fixed diagnostic routes missing")
if 'const pcOSVersion = "5.1.11"' not in p:raise SystemExit("version bump failed")
main.write_text(m,encoding="utf-8")
pc.write_text(p,encoding="utf-8")
print("v5.1.11 bounded read-only sqlcmd format fix")
