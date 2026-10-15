package remoteagent

import "testing"

func exactCaps(screen, mouse, keyboard bool) map[string]any {
	return map[string]any{
		"screen_stream": screen,
		"mouse": mouse,
		"keyboard": keyboard,
		"clipboard": false,
		"file_transfer": false,
		"audio": false,
	}
}

func TestParseRemoteCapabilitiesV2ScreenOnly(t *testing.T) {
	caps, err := ParseRemoteCapabilitiesV2(exactCaps(true, false, false))
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if caps.RequiresInputApproval() { t.Fatal("screen-only must not require input approval") }
	if got := caps.ConsentSummary(); got != "Screen viewing only" { t.Fatalf("unexpected summary: %q", got) }
}

func TestParseRemoteCapabilitiesV2MouseKeyboard(t *testing.T) {
	caps, err := ParseRemoteCapabilitiesV2(exactCaps(true, true, true))
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if !caps.RequiresInputApproval() { t.Fatal("mouse/keyboard must require explicit input approval") }
	if got := caps.ConsentSummary(); got != "Screen viewing + mouse and keyboard control" { t.Fatalf("unexpected summary: %q", got) }
}

func TestParseRemoteCapabilitiesV2RejectsDisabledBoundaries(t *testing.T) {
	cases := []map[string]any{
		{"screen_stream": true, "mouse": false, "keyboard": false, "clipboard": true, "file_transfer": false, "audio": false},
		{"screen_stream": true, "mouse": false, "keyboard": false, "clipboard": false, "file_transfer": true, "audio": false},
		{"screen_stream": true, "mouse": false, "keyboard": false, "clipboard": false, "file_transfer": false, "audio": true},
	}
	for i, raw := range cases {
		if _, err := ParseRemoteCapabilitiesV2(raw); err == nil { t.Fatalf("case %d should fail", i) }
	}
}

func TestParseRemoteCapabilitiesV2RejectsUnknownOrMissing(t *testing.T) {
	unknown := exactCaps(true, false, false)
	unknown["shell"] = false
	if _, err := ParseRemoteCapabilitiesV2(unknown); err == nil { t.Fatal("unknown capability should fail") }

	missing := exactCaps(true, false, false)
	delete(missing, "keyboard")
	if _, err := ParseRemoteCapabilitiesV2(missing); err == nil { t.Fatal("missing capability should fail") }
}

func TestParseRemoteCapabilitiesV2RequiresScreen(t *testing.T) {
	if _, err := ParseRemoteCapabilitiesV2(exactCaps(false, true, true)); err == nil {
		t.Fatal("input without screen stream should fail")
	}
}
