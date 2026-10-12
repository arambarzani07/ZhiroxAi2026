from pathlib import Path

relay = Path('ops/supabase/zhirox-pc-relay-v15.ts')
text = relay.read_text()

old_read = '''  "pc_os_overview","pc_os_modules","pc_os_guardian","pc_os_backup","pc_os_continuity",
  "pc_os_update_chain"'''
new_read = '''  "pc_os_overview","pc_os_modules","pc_os_guardian","pc_os_backup","pc_os_backup_readiness",
  "pc_os_backup_verify","pc_os_continuity","pc_os_update_chain"'''
if old_read not in text and '"pc_os_backup_readiness"' not in text:
    raise SystemExit('PC_OS_READ_ACTIONS insertion point not found')
if old_read in text:
    text = text.replace(old_read, new_read, 1)

old_mut = '''  "reboot","owner_batch"
]);'''
new_mut = '''  "reboot","owner_batch","pc_os_backup_execute"
]);'''
if old_mut not in text and '"pc_os_backup_execute"' not in text:
    raise SystemExit('MUTATION_ACTIONS insertion point not found')
if old_mut in text:
    text = text.replace(old_mut, new_mut, 1)

relay.write_text(text)

workflow = Path('.github/workflows/build-v510.yml')
w = workflow.read_text()
anchor = '''          grep -q 'binaryReadRefused' ops/supabase/zhirox-pc-relay-v15.ts
'''
addition = '''          grep -q 'binaryReadRefused' ops/supabase/zhirox-pc-relay-v15.ts
          grep -q 'pc_os_backup_readiness' ops/supabase/zhirox-pc-relay-v15.ts
          grep -q 'pc_os_backup_verify' ops/supabase/zhirox-pc-relay-v15.ts
          grep -q 'pc_os_backup_execute' ops/supabase/zhirox-pc-relay-v15.ts
'''
if anchor in w and "grep -q 'pc_os_backup_execute'" not in w:
    w = w.replace(anchor, addition, 1)

py_anchor = '''          assert 'approval_required' in block
          PY
'''
py_new = '''          assert 'approval_required' in block
          read_start=s.index('const PC_OS_READ_ACTIONS=new Set([')
          read_end=s.index(']);', read_start)
          read_block=s[read_start:read_end]
          mut_start=s.index('const MUTATION_ACTIONS=new Set([')
          mut_end=s.index(']);', mut_start)
          mut_block=s[mut_start:mut_end]
          assert '"pc_os_backup_readiness"' in read_block
          assert '"pc_os_backup_verify"' in read_block
          assert '"pc_os_backup_execute"' not in read_block
          assert '"pc_os_backup_execute"' in mut_block
          PY
'''
if py_anchor in w and 'mut_block=s[mut_start:mut_end]' not in w:
    w = w.replace(py_anchor, py_new, 1)

workflow.write_text(w)
