from pathlib import Path

files = [
    Path('remote/encoder-mf-v1/encoder.cpp'),
    Path('remote/encoder-mf-v1/diagnose.cpp'),
]

for path in files:
    text = path.read_text()

    if path.name == 'encoder.cpp':
        old = '''        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0))) return hr;\n        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0))) return hr;\n        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0))) return hr;\n'''
        new = '''        // Do not flush a freshly configured MFT. Microsoft documents\n        // MFT_MESSAGE_COMMAND_FLUSH as a streaming-time command that should be\n        // issued only after streaming has started. A fresh encoder has no\n        // buffered samples to discard.\n        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0))) return hr;\n        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0))) return hr;\n'''
    else:
        old = '''        hr = transform->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0);\n        print_hr("  COMMAND_FLUSH", hr);\n        if (FAILED(hr)) { last = hr; continue; }\n        hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);\n'''
        new = '''        // A fresh MFT must enter streaming before COMMAND_FLUSH is valid.\n        // There is nothing to flush before the first input sample.\n        hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);\n'''

    if old not in text:
        raise SystemExit(f'expected Media Foundation startup sequence not found in {path}')
    text = text.replace(old, new, 1)

    if 'ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0)' in text:
        raise SystemExit(f'initial COMMAND_FLUSH remains in {path}')
    if 'MFT_MESSAGE_NOTIFY_BEGIN_STREAMING' not in text or 'MFT_MESSAGE_NOTIFY_START_OF_STREAM' not in text:
        raise SystemExit(f'stream start notifications missing in {path}')

    path.write_text(text)
