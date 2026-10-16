from pathlib import Path
import sys

source = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("ops/supabase/zhirox-pc-relay-v15.ts")
target = Path(sys.argv[2]) if len(sys.argv) > 2 else source
text = source.read_text()

needle = '  "reboot","owner_batch","pc_os_backup_execute"\n]);'
replacement = '  "reboot","owner_batch","pc_os_backup_execute","pc_os_export_prepare_latest"\n]);'

if needle not in text:
    raise SystemExit("relay mutation allowlist insertion point not found")
if '"pc_os_export_prepare_latest"' in text:
    raise SystemExit("relay already contains pc_os_export_prepare_latest; refusing ambiguous patch")

text = text.replace(needle, replacement, 1)
target.write_text(text)
