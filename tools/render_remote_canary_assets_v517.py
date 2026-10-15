from pathlib import Path
import hashlib

root = Path('buildsrc/remote-assets-v517')
out = Path('buildsrc/pc_os_remote_canary_assets_v517_windows.go')
assets = [
    ('ZhiroxRemoteScreenAgent.exe', 'pcOSRemoteCanaryAgentAsset', 'pcOSRemoteCanaryAgentSHA256'),
    ('ZhiroxRemoteConsent.exe', 'pcOSRemoteCanaryConsentAsset', 'pcOSRemoteCanaryConsentSHA256'),
    ('ZhiroxMFH264Encoder.dll', 'pcOSRemoteCanaryEncoderAsset', 'pcOSRemoteCanaryEncoderSHA256'),
]

rows = [
    '//go:build windows',
    '',
    'package main',
    '',
    'import _ "embed"',
    '',
]
for filename, var_name, const_name in assets:
    path = root / filename
    if not path.is_file():
        raise SystemExit(f'missing canary asset: {path}')
    data = path.read_bytes()
    if not data:
        raise SystemExit(f'empty canary asset: {path}')
    digest = hashlib.sha256(data).hexdigest()
    rows += [
        f'//go:embed remote-assets-v517/{filename}',
        f'var {var_name} []byte',
        f'const {const_name} = "{digest}"',
        '',
    ]
out.write_text('\n'.join(rows), encoding='utf-8')
