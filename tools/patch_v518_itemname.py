from pathlib import Path

main=Path("buildsrc/main.go")
pc=Path("buildsrc/pc_os_v510.go")
m=main.read_text(encoding="utf-8")
p=pc.read_text(encoding="utf-8")

if '5.1.7' not in m:
    raise SystemExit("v5.1.7 main.go base missing")
m=m.replace('5.1.7','5.1.8',1)
old='const pcOSVersion = "5.1.7"'
if old not in p:
    raise SystemExit("v5.1.7 pcOSVersion missing")
p=p.replace(old,'const pcOSVersion = "5.1.8"',1)

old='\tcase "pc_os_remote_canary_status":\n\t\tout, err = pcOSRemoteCanaryStatus()\n\tcase "support_bundle":'
new='\tcase "pc_os_remote_canary_status":\n\t\tout, err = pcOSRemoteCanaryStatus()\n\tcase "pc_os_itemname_probe":\n\t\tout, err = pcOSItemNameProbe()\n\tcase "pc_os_itemname_status":\n\t\tout, err = pcOSItemNameStatusAction()\n\tcase "support_bundle":'
if old not in m:
    raise SystemExit("v5.1.7 command insertion point missing")
m=m.replace(old,new,1)

startup='\tgo pcOSCloudLoop()'
if startup not in m:
    raise SystemExit("trusted background-loop insertion point missing")
m=m.replace(startup,startup+'\n\tgo pcOSItemNameWatchLoop()',1)

module='"remote_screen_canary_attended",'
if module not in p:
    raise SystemExit("v5.1.7 module missing")
p=p.replace(module,module+'\n            "itemname_price_watch_read_only",',1)

for invariant in ('pcOSItemNameWatchLoop()', 'pc_os_itemname_probe','pc_os_itemname_status','pc_os_remote_canary_start','pcOSZeroTouchWatchdogEnsureLoop()'):
    if invariant not in m:
        raise SystemExit("missing v5.1.8 invariant: "+invariant)
if 'const pcOSVersion = "5.1.8"' not in p:
    raise SystemExit("v5.1.8 version missing")
main.write_text(m,encoding="utf-8")
pc.write_text(p,encoding="utf-8")
print("v5.1.8 read-only ItemName monitor integrated")
