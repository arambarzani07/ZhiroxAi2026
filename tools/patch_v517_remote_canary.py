from pathlib import Path

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')
launcher = Path('buildsrc/pc_os_remote_agent_launcher_v1_windows.go')

main_text = main.read_text(encoding='utf-8')
pc_text = pc_os.read_text(encoding='utf-8')
launcher_text = launcher.read_text(encoding='utf-8')

# Version bump is intentionally last in the reconstructed v5.1.6 source chain.
if '5.1.6' not in main_text:
    raise SystemExit('v5.1.6 runtime base version not found')
main_text = main_text.replace('5.1.6', '5.1.7', 1)
old_const = 'const pcOSVersion = "5.1.6"'
if old_const not in pc_text:
    raise SystemExit('pcOSVersion v5.1.6 constant not found')
pc_text = pc_text.replace(old_const, 'const pcOSVersion = "5.1.7"', 1)

# Route only fixed no-argument start/stop actions. The relay separately classifies
# these as approval-gated mutations before they may ever be delivered.
route_old = '''\tcase "pc_os_screenshot":
\t\tout, err = pcOSCaptureScreenshot(c.Args)
\tcase "support_bundle":'''
route_new = '''\tcase "pc_os_screenshot":
\t\tout, err = pcOSCaptureScreenshot(c.Args)
\tcase "pc_os_remote_canary_start":
\t\tout, err = pcOSRemoteCanaryStart()
\tcase "pc_os_remote_canary_stop":
\t\tout, err = pcOSRemoteCanaryStop()
\tcase "support_bundle":'''
if route_old not in main_text:
    raise SystemExit('v5.1.7 remote canary command insertion point not found')
main_text = main_text.replace(route_old, route_new, 1)

module_anchor = '"screen_observability_read_only",'
if module_anchor not in pc_text:
    raise SystemExit('screen observability module anchor not found')
pc_text = pc_text.replace(module_anchor, module_anchor + '\n            "remote_control_canary_screen_only",', 1)

# Extend the secure bootstrap schema. Production fields stay unchanged; canary
# mode adds only mode + display label and deliberately leaves agent credentials empty.
bootstrap_old = '''type pcOSRemoteBridgeBootstrap struct {
\tSignalURL   string                      `json:"signal_url"`
\tAgentID     string                      `json:"agent_id"`
\tAgentSecret string                      `json:"agent_secret"`
\tICEServers  []pcOSRemoteBridgeICEServer `json:"ice_servers,omitempty"`
\tRelayOnly   bool                        `json:"relay_only,omitempty"`
}'''
bootstrap_new = '''type pcOSRemoteBridgeBootstrap struct {
\tMode        string                      `json:"mode,omitempty"`
\tSignalURL   string                      `json:"signal_url"`
\tAgentID     string                      `json:"agent_id,omitempty"`
\tAgentSecret string                      `json:"agent_secret,omitempty"`
\tAgentLabel  string                      `json:"agent_label,omitempty"`
\tICEServers  []pcOSRemoteBridgeICEServer `json:"ice_servers,omitempty"`
\tRelayOnly   bool                        `json:"relay_only,omitempty"`
}'''
if bootstrap_old not in launcher_text:
    raise SystemExit('remote bootstrap struct anchor not found')
launcher_text = launcher_text.replace(bootstrap_old, bootstrap_new, 1)

# Factor process creation so both production-auth and credential-free canary
# payloads traverse the exact same bounded anonymous-pipe + handle-allowlist path.
start_prefix_old = '''func pcOSStartRemoteScreenAgent(spec pcOSRemoteAgentLaunchSpec, creds pcOSRelayCredentials) (*pcOSRemoteAgentProcess, error) {
\tnormalized, err := pcOSRemoteNormalizeLaunchSpec(spec)
\tif err != nil {
\t\treturn nil, err
\t}
\tpayload, err := pcOSRemoteBuildBootstrapPayload(normalized, creds)
\tif err != nil {
\t\treturn nil, err
\t}
\tdefer pcOSRemoteZeroBytes(payload)

'''
start_prefix_new = '''func pcOSStartRemoteScreenAgent(spec pcOSRemoteAgentLaunchSpec, creds pcOSRelayCredentials) (*pcOSRemoteAgentProcess, error) {
\tnormalized, err := pcOSRemoteNormalizeLaunchSpec(spec)
\tif err != nil {
\t\treturn nil, err
\t}
\tpayload, err := pcOSRemoteBuildBootstrapPayload(normalized, creds)
\tif err != nil {
\t\treturn nil, err
\t}
\tdefer pcOSRemoteZeroBytes(payload)
\treturn pcOSStartRemoteScreenAgentWithPayload(normalized, payload)
}

func pcOSStartRemoteScreenAgentWithPayload(normalized pcOSRemoteAgentLaunchSpec, payload []byte) (*pcOSRemoteAgentProcess, error) {
\tif len(payload) == 0 || len(payload) > pcOSRemoteBridgeMaxBootstrapBytes {
\t\treturn nil, errors.New("secure remote bootstrap payload exceeds safety bounds")
\t}

'''
if start_prefix_old not in launcher_text:
    raise SystemExit('remote secure launcher function prefix not found')
launcher_text = launcher_text.replace(start_prefix_old, start_prefix_new, 1)

canary_anchor = 'func pcOSRemoteVerifyLaunchArtifacts(spec pcOSRemoteAgentLaunchSpec) error {'
if canary_anchor not in launcher_text:
    raise SystemExit('remote artifact verification anchor not found')
canary_helpers = r'''const pcOSRemoteCanarySignalURL = "https://zhirox-remote-control-canary-aviglm.v2.appdeploy.ai"

func pcOSStartRemoteCanaryAgent(spec pcOSRemoteAgentLaunchSpec, agentLabel string) (*pcOSRemoteAgentProcess, error) {
	normalized, err := pcOSRemoteNormalizeCanaryLaunchSpec(spec)
	if err != nil {
		return nil, err
	}
	payload, err := pcOSRemoteBuildCanaryBootstrapPayload(normalized, agentLabel)
	if err != nil {
		return nil, err
	}
	defer pcOSRemoteZeroBytes(payload)
	return pcOSStartRemoteScreenAgentWithPayload(normalized, payload)
}

func pcOSRemoteNormalizeCanaryLaunchSpec(spec pcOSRemoteAgentLaunchSpec) (pcOSRemoteAgentLaunchSpec, error) {
	requested := strings.TrimRight(strings.TrimSpace(spec.SignalURL), "/")
	if requested != pcOSRemoteCanarySignalURL {
		return spec, errors.New("remote canary signaling URL is not approved")
	}
	// Reuse every existing path/hash/FPS/bitrate/ICE validator without widening
	// the production signaling allowlist. The placeholder URL is never used for I/O.
	spec.SignalURL = "https://canary.invalid/functions/v1/zhirox-pc-remote-signaling-v1"
	normalized, err := pcOSRemoteNormalizeLaunchSpec(spec)
	if err != nil {
		return spec, err
	}
	normalized.SignalURL = pcOSRemoteCanarySignalURL
	return normalized, nil
}

func pcOSRemoteBuildCanaryBootstrapPayload(spec pcOSRemoteAgentLaunchSpec, agentLabel string) ([]byte, error) {
	label := strings.TrimSpace(agentLabel)
	if label == "" || len(label) > 120 || strings.ContainsAny(label, "\x00\r\n") {
		return nil, errors.New("remote canary agent label is invalid")
	}
	payload, err := json.Marshal(pcOSRemoteBridgeBootstrap{
		Mode:       "canary",
		SignalURL:  spec.SignalURL,
		AgentLabel: label,
		ICEServers: append([]pcOSRemoteBridgeICEServer(nil), spec.ICEServers...),
		RelayOnly:  spec.RelayOnly,
	})
	if err != nil {
		return nil, errors.New("encode secure remote canary bootstrap")
	}
	if len(payload) == 0 || len(payload) > pcOSRemoteBridgeMaxBootstrapBytes {
		pcOSRemoteZeroBytes(payload)
		return nil, errors.New("secure remote canary bootstrap exceeds safety bounds")
	}
	return payload, nil
}

'''
launcher_text = launcher_text.replace(canary_anchor, canary_helpers + canary_anchor, 1)

for invariant in (
    'const pcOSVersion = "5.1.7"',
    '"remote_control_canary_screen_only"',
):
    if invariant not in pc_text:
        raise SystemExit(f'missing v5.1.7 pc-os invariant: {invariant}')
for invariant in (
    'pc_os_remote_canary_start',
    'pc_os_remote_canary_stop',
    'pcOSRemoteCanaryStart()',
    'pcOSRemoteCanaryStop()',
):
    if invariant not in main_text:
        raise SystemExit(f'missing v5.1.7 command invariant: {invariant}')
for invariant in (
    'pcOSStartRemoteScreenAgentWithPayload',
    'Mode:       "canary"',
    'pcOSRemoteCanarySignalURL',
    'https://zhirox-remote-control-canary-aviglm.v2.appdeploy.ai',
):
    if invariant not in launcher_text:
        raise SystemExit(f'missing v5.1.7 secure canary launcher invariant: {invariant}')

# Fail closed if the canary wiring accidentally learns production credentials.
canary_segment = launcher_text[launcher_text.index('func pcOSStartRemoteCanaryAgent'):launcher_text.index(canary_anchor)]
if 'pcOSLoadRelayCredentials' in canary_segment or 'AgentSecret:' in canary_segment or 'AgentID:' in canary_segment:
    raise SystemExit('canary launcher must remain credential-free')

main.write_text(main_text, encoding='utf-8')
pc_os.write_text(pc_text, encoding='utf-8')
launcher.write_text(launcher_text, encoding='utf-8')
