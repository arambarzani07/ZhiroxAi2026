from pathlib import Path
main=Path("buildsrc/main.go")
pc=Path("buildsrc/pc_os_v510.go")
m=main.read_text(encoding="utf-8")
p=pc.read_text(encoding="utf-8")
if '5.1.9' not in m or 'const pcOSVersion = "5.1.9"' not in p:
 raise SystemExit("v5.1.9 base missing")
needle='\tcase "pc_os_itemname_probe":'
if needle not in m: raise SystemExit("ItemName route missing")
if 'pc_os_sql_compare_v2' in m: raise SystemExit("already installed compare route")
m=m.replace(needle,'\tcase "pc_os_sql_compare_v2":\n\t\tout, err = pcOSSQLCompareV2Action()\n'+needle,1)
m=m.replace('5.1.9','5.1.10',1)
p=p.replace('const pcOSVersion = "5.1.9"','const pcOSVersion = "5.1.10"',1)
if 'pc_os_sql_compare_v2' not in m or 'const pcOSVersion = "5.1.10"' not in p:
 raise SystemExit("v5.1.10 diagnostic invariant missing")
main.write_text(m,encoding="utf-8")
pc.write_text(p,encoding="utf-8")
print("v5.1.10 protected fixed read-only SQL diagnostics registered")
