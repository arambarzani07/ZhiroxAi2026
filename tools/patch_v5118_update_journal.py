"""Reconstruct the prior v5.1.17 runtime and add only the bounded journal probe."""
from pathlib import Path
main = Path("buildsrc/main.go")
pc = Path("buildsrc/pc_os_v510.go")
cloud = Path("buildsrc/pc_os_cloud_v510.go")
m = main.read_text(encoding="utf-8-sig")
p = pc.read_text(encoding="utf-8-sig")
c = cloud.read_text(encoding="utf-8-sig")
marker = 'snapshot["cashierynwe_installation"] = pcOSCashierynweRegisteredVersion()'
if m.count("5.1.17") < 1 or p.count('const pcOSVersion = "5.1.17"') != 1 or c.count(marker) != 1:
    raise SystemExit("exact reconstructed v5.1.17 runtime required")
if "pcOSCashierynweUpdateState()" in c:
    raise SystemExit("unexpected existing journal hook")
if not all(Path("buildsrc", name).is_file() for name in ("pc_os_cashierynwe_update_state.go", "pc_os_cashierynwe_update_state_windows.go")):
    raise SystemExit("missing native journal probe")
main.write_text(m.replace("5.1.17", "5.1.18", 1), encoding="utf-8")
pc.write_text(p.replace('const pcOSVersion = "5.1.17"', 'const pcOSVersion = "5.1.18"', 1), encoding="utf-8")
cloud.write_text(c.replace(marker, marker+'\n        snapshot["cashierynwe_update_state"] = pcOSCashierynweUpdateState()',1),encoding="utf-8")
