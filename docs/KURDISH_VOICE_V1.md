# Zhirox Kurdish Voice v1

## Scope

Kurdish Voice v1 is a Zero-UI, event-only Sorani Kurdish audio module for Zhirox PC OS. It is intentionally isolated from the production updater and from microphone/voice-command features.

## Safety boundary

- Locale: `ckb-IQ`
- Disabled by default until an explicit production enable action is approved.
- No microphone access.
- No continuous listening.
- No speech recognition.
- No network or cloud dependency in v1.
- No shell or subprocess execution.
- No popup/window requirement.
- Playback is limited to an allowlist of fixed WAV filenames under `C:\ProgramData\ZhiroxAI\voice\ku`.
- User-controlled file paths are not accepted.
- Business executables, SQL Server data, `D:\DataBase`, and Hikvision/DVR are outside this module.

## Initial event phrases

| Event | Sorani Kurdish phrase | Asset |
|---|---|---|
| `system_ready` | سیستەمەکە ئامادەیە. | `system-ready.wav` |
| `update_success` | نوێکردنەوە بە سەرکەوتوویی تەواو بوو. | `update-success.wav` |
| `internet_problem` | کێشەیەک لە پەیوەندی ئینتەرنێت هەیە. | `internet-problem.wav` |
| `disk_low` | شوێنی بەردەست لە دیسک کەمە. | `disk-low.wav` |
| `service_recovered` | خزمەتگوزاری زیڕۆکس دووبارە چالاک کرایەوە. | `service-recovered.wav` |
| `backup_success` | پاشەکەوتکردن بە سەرکەوتوویی تەواو بوو. | `backup-success.wav` |
| `security_warning` | ئاگادارییەکی ئاسایشی هەیە. | `security-warning.wav` |

## Phased roadmap

1. **v1 — local event voice**: fixed Kurdish phrases, local WAV playback, mic off, offline, Zero-UI.
2. **v2 — natural free-form TTS**: pluggable Kurdish TTS provider with explicit network/security policy and local caching. This remains off by default until separately approved.
3. **v3 — voice assistant**: optional push-to-talk or wake interaction. Microphone access must be a separate explicit opt-in and must never be silently enabled by an update.

## Production rule

Kurdish Voice development must not block, weaken, or bypass v5.1.1 update signing, trust rotation, rollback, Critical Protection, or Zero-UI guarantees. Production enablement requires its own explicit authorization after v5.1.1 is stable.
