package main

import "testing"

func TestScreenObservabilityPolicyIsReadOnly(t *testing.T) {
	p := pcOSScreenObservabilityPolicy()
	checks := map[string]interface{}{
		"active_window_read_only":   true,
		"screenshot_on_demand_only": true,
		"continuous_capture":        false,
		"input_control":             false,
		"keyboard_capture":          false,
		"mouse_capture":             false,
		"clipboard_capture":         false,
		"camera_capture":            false,
		"microphone_capture":        false,
		"primary_display_only":      true,
		"transient_only":            true,
	}
	for k, want := range checks {
		if got := p[k]; got != want {
			t.Fatalf("policy %s: got %#v want %#v", k, got, want)
		}
	}
}

func TestScreenCaptureConfigBounds(t *testing.T) {
	cfg, err := pcOSScreenCaptureConfigFromArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxWidth != pcOSScreenDefaultMaxWidth || cfg.Quality != pcOSScreenDefaultQuality {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}

	cfg, err = pcOSScreenCaptureConfigFromArgs(map[string]interface{}{"max_width": float64(1600), "quality": float64(60)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxWidth != 1600 || cfg.Quality != 60 {
		t.Fatalf("unexpected parsed config: %#v", cfg)
	}

	bad := []map[string]interface{}{
		{"max_width": 639},
		{"max_width": 1921},
		{"quality": 24},
		{"quality": 71},
		{"quality": 40.5},
	}
	for _, args := range bad {
		if _, err := pcOSScreenCaptureConfigFromArgs(args); err == nil {
			t.Fatalf("expected bounds/type rejection for %#v", args)
		}
	}
}

func TestScreenObservationModeAllowlist(t *testing.T) {
	for _, mode := range []string{"active_window", "screenshot", " ACTIVE_WINDOW "} {
		if _, err := pcOSScreenObservationMode(mode); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	for _, mode := range []string{"continuous", "camera", "clipboard", "../../cmd"} {
		if _, err := pcOSScreenObservationMode(mode); err == nil {
			t.Fatalf("mode %q must be rejected", mode)
		}
	}
}
