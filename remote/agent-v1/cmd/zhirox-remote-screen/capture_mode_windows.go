//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	remoteagent "zhirox/remoteagent"
)

type captureModeArgs struct {
	PipeHandle    uintptr
	EncoderPath   string
	EncoderSHA256 string
	FPS           int
	Bitrate       uint32
}

func captureHelperRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--zhirox-capture-helper" {
			return true
		}
	}
	return false
}

func parseCaptureModeArgs(args []string) (captureModeArgs, error) {
	out := captureModeArgs{FPS: remoteagent.DefaultScreenFPS, Bitrate: 4_000_000}
	seen := map[string]bool{}
	modeSeen := false
	for _, arg := range args {
		if arg == "--zhirox-capture-helper" {
			if modeSeen { return captureModeArgs{}, errors.New("duplicate capture-helper mode flag") }
			modeSeen = true
			continue
		}
		name, value, ok := strings.Cut(arg, "=")
		if !ok || value == "" {
			return captureModeArgs{}, errors.New("capture-helper arguments must use --name=value form")
		}
		if seen[name] { return captureModeArgs{}, fmt.Errorf("duplicate capture-helper argument %s", name) }
		seen[name] = true
		switch name {
		case "--pipe-handle":
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil || v == 0 { return captureModeArgs{}, errors.New("capture pipe handle is invalid") }
			out.PipeHandle = uintptr(v)
		case "--encoder-path-b64":
			decoded, err := base64.RawURLEncoding.DecodeString(value)
			if err != nil || len(decoded) == 0 || len(decoded) > 8192 { return captureModeArgs{}, errors.New("capture encoder path argument is invalid") }
			out.EncoderPath = filepath.Clean(string(decoded))
		case "--encoder-sha256":
			v := strings.ToLower(strings.TrimSpace(value))
			if len(v) != 64 { return captureModeArgs{}, errors.New("capture encoder SHA-256 pin is invalid") }
			for _, r := range v {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) { return captureModeArgs{}, errors.New("capture encoder SHA-256 pin is invalid") }
			}
			out.EncoderSHA256 = v
		case "--fps":
			v, err := strconv.Atoi(value)
			if err != nil || v < 1 || v > remoteagent.MaxScreenFPS { return captureModeArgs{}, errors.New("capture FPS is outside approved bounds") }
			out.FPS = v
		case "--bitrate":
			v, err := strconv.ParseUint(value, 10, 32)
			if err != nil || v < 250_000 || v > 20_000_000 { return captureModeArgs{}, errors.New("capture bitrate is outside approved bounds") }
			out.Bitrate = uint32(v)
		default:
			return captureModeArgs{}, fmt.Errorf("unsupported capture-helper argument %s", name)
		}
	}
	if !modeSeen || out.PipeHandle == 0 || out.EncoderPath == "" || out.EncoderSHA256 == "" {
		return captureModeArgs{}, errors.New("capture-helper required arguments are missing")
	}
	if !filepath.IsAbs(out.EncoderPath) {
		return captureModeArgs{}, errors.New("capture encoder path must be absolute")
	}
	return out, nil
}

func runCaptureHelper(args []string) error {
	cfg, err := parseCaptureModeArgs(args)
	if err != nil { return err }
	pipe := os.NewFile(cfg.PipeHandle, "zhirox-remote-h264-write")
	if pipe == nil { return errors.New("capture helper could not wrap inherited output pipe") }
	defer pipe.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return remoteagent.RunWindowsCaptureHelper(ctx, pipe, remoteagent.WindowsCaptureHelperConfig{
		EncoderDLLPath: cfg.EncoderPath,
		EncoderDLLSHA256: cfg.EncoderSHA256,
		FPS: cfg.FPS,
		Bitrate: cfg.Bitrate,
	})
}
