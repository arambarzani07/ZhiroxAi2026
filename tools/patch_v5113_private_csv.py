from pathlib import Path
main=Path("buildsrc/main.go")
pc=Path("buildsrc/pc_os_v510.go")
watch=Path("buildsrc/pc_os_itemname_watch_v518_windows.go")
cloud=Path("buildsrc/pc_os_csv_cloud_v5113_windows.go")
m=main.read_text(encoding="utf-8")
p=pc.read_text(encoding="utf-8")
w=watch.read_text(encoding="utf-8")
if "5.1.12" not in m or 'const pcOSVersion = "5.1.12"' not in p:
    raise SystemExit("requires previous signed 5.1.12 exact source; refuse mismatch")
if not cloud.exists() or "pcOSItemNameCloudUpload(path,sha,len(rows))" not in w:
    raise SystemExit("missing protected cloud CSV component")
m=m.replace("5.1.12","5.1.13",1)
p=p.replace('const pcOSVersion = "5.1.12"','const pcOSVersion = "5.1.13"',1)
main.write_text(m,encoding="utf-8")
pc.write_text(p,encoding="utf-8")
print("isolated v5.1.13 private CSV cloud sync candidate; legacy POS remains read-only")
