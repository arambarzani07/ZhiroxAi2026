//go:build windows

package main

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

func validCaptureArgs(t *testing.T) []string {
	t.Helper()
	path := filepath.Clean(`C:\ProgramData\ZhiroxAI\remote\ZhiroxMFH264Encoder.dll`)
	return []string{
		"--zhirox-capture-helper",
		"--pipe-handle=1234",
		"--encoder-path-b64=" + base64.RawURLEncoding.EncodeToString([]byte(path)),
		"--encoder-sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"--fps=15",
		"--bitrate=4000000",
	}
}

func TestParseCaptureModeArgsAcceptsStrictMediaOnlyContract(t *testing.T) {
	got, err := parseCaptureModeArgs(validCaptureArgs(t))
	if err != nil {
		t.Fatalf("valid capture helper args rejected: %v", err)
	}
	if got.PipeHandle != 1234 || got.FPS != 15 || got.Bitrate != 4_000_000 {
		t.Fatalf("unexpected parsed capture config: %+v", got)
	}
	if !filepath.IsAbs(got.EncoderPath) {
		t.Fatal("encoder path must remain absolute")
	}
}

func TestParseCaptureModeArgsRejectsUnknownOrSecretArguments(t *testing.T) {
	for _, extra := range []string{
		"--agent-secret=do-not-accept",
		"--controller-token=do-not-accept",
		"--signaling-url=https://example.test",
		"--turn-credential=do-not-accept",
	} {
		args := append(validCaptureArgs(t), extra)
		if _, err := parseCaptureModeArgs(args); err == nil {
			t.Fatalf("capture helper accepted forbidden argument %q", extra)
		}
	}
}

func TestParseCaptureModeArgsRejectsDuplicateArgument(t *testing.T) {
	args := append(validCaptureArgs(t), "--fps=20")
	if _, err := parseCaptureModeArgs(args); err == nil {
		t.Fatal("duplicate capture helper argument must be rejected")
	}
}

func TestParseCaptureModeArgsRejectsRelativeEncoderPath(t *testing.T) {
	args := validCaptureArgs(t)
	relative := base64.RawURLEncoding.EncodeToString([]byte(`ZhiroxMFH264Encoder.dll`))
	for i := range args {
		if len(args[i]) >= len("--encoder-path-b64=") && args[i][:len("--encoder-path-b64=")] == "--encoder-path-b64=" {
			args[i] = "--encoder-path-b64=" + relative
		}
	}
	if _, err := parseCaptureModeArgs(args); err == nil {
		t.Fatal("relative encoder path must be rejected")
	}
}

func TestCaptureHelperRequestedOnlyForExplicitFlag(t *testing.T) {
	if !captureHelperRequested([]string{"--zhirox-capture-helper"}) {
		t.Fatal("explicit capture helper flag was not detected")
	}
	if captureHelperRequested([]string{"--mode=capture", "--zhirox-capture-helper=true"}) {
		t.Fatal("lookalike helper flags must not activate media-only mode")
	}
}
