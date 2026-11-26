"""Overlay only the fixed version probe into a future verified candidate build.
No version bump, signer, installer, generic command or remote access change.
"""
from pathlib import Path
import sys
target = Path(sys.argv[1]) if len(sys.argv)==2 else Path("buildsrc/pc_os_cloud_v510.go")
s = target.read_text(encoding="utf-8-sig")
marker = "snapshot := pcOSBackgroundSnapshot()"
if s.count(marker)!=1 or "pcOSCashierynweRegisteredVersion()" in s:
    raise SystemExit("unexpected cloud snapshot source")
s = s.replace(marker, marker+'\n        snapshot["cashierynwe_installation"] = pcOSCashierynweRegisteredVersion()',1)
target.write_text(s,encoding="utf-8")
