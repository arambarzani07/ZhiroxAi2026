from pathlib import Path
main=Path("buildsrc/main.go")
pc=Path("buildsrc/pc_os_v510.go")
m=main.read_text(encoding="utf-8")
p=pc.read_text(encoding="utf-8")
if '5.1.8' not in m:
    raise SystemExit("prior v5.1.8 main.go version missing")
if 'const pcOSVersion = "5.1.8"' not in p:
    raise SystemExit("prior v5.1.8 pcOSVersion missing")
m=m.replace('5.1.8','5.1.9',1)
p=p.replace('const pcOSVersion = "5.1.8"', 'const pcOSVersion = "5.1.9"',1)
if 'pcOSItemNameWatchLoop()' not in m or 'pc_os_itemname_probe' not in m:
    raise SystemExit("read-only ItemName diagnostics routes missing")
if 'const pcOSVersion = "5.1.9"' not in p:
    raise SystemExit("new version assignment missing")
main.write_text(m, encoding="utf-8")
pc.write_text(p, encoding="utf-8")
print("v5.1.9 safe SQL diagnostics update applied")
