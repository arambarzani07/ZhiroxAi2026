package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	pcOSScreenDefaultMaxWidth = 1280
	pcOSScreenMinMaxWidth     = 640
	pcOSScreenMaxMaxWidth     = 1920
	pcOSScreenDefaultQuality  = 45
	pcOSScreenMinQuality      = 25
	pcOSScreenMaxQuality      = 70
	pcOSScreenMaxJPEGBytes    = 900000
)

type pcOSScreenCaptureConfig struct {
	MaxWidth int `json:"max_width"`
	Quality  int `json:"quality"`
}

func pcOSScreenObservabilityPolicy() map[string]interface{} {
	return map[string]interface{}{
		"module":                    "screen_observability_read_only",
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
		"max_jpeg_bytes":            pcOSScreenMaxJPEGBytes,
		"max_width":                 pcOSScreenMaxMaxWidth,
		"max_quality":               pcOSScreenMaxQuality,
		"transient_only":            true,
	}
}

func pcOSScreenCaptureConfigFromArgs(args map[string]interface{}) (pcOSScreenCaptureConfig, error) {
	cfg := pcOSScreenCaptureConfig{MaxWidth: pcOSScreenDefaultMaxWidth, Quality: pcOSScreenDefaultQuality}
	if args == nil {
		return cfg, nil
	}
	var err error
	if v, ok := args["max_width"]; ok {
		cfg.MaxWidth, err = pcOSBoundedScreenInt(v, pcOSScreenMinMaxWidth, pcOSScreenMaxMaxWidth, "max_width")
		if err != nil {
			return pcOSScreenCaptureConfig{}, err
		}
	}
	if v, ok := args["quality"]; ok {
		cfg.Quality, err = pcOSBoundedScreenInt(v, pcOSScreenMinQuality, pcOSScreenMaxQuality, "quality")
		if err != nil {
			return pcOSScreenCaptureConfig{}, err
		}
	}
	return cfg, nil
}

func pcOSBoundedScreenInt(v interface{}, min, max int, name string) (int, error) {
	var n int64
	var err error
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int32:
		n = int64(x)
	case int64:
		n = x
	case float64:
		if x != float64(int64(x)) {
			return 0, fmt.Errorf("%s must be an integer", name)
		}
		n = int64(x)
	case string:
		n, err = strconv.ParseInt(strings.TrimSpace(x), 10, 32)
	default:
		return 0, fmt.Errorf("%s has unsupported type", name)
	}
	if err != nil {
		return 0, fmt.Errorf("%s is invalid: %w", name, err)
	}
	if n < int64(min) || n > int64(max) {
		return 0, fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return int(n), nil
}

func pcOSScreenObservationMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "active_window", "screenshot":
		return mode, nil
	default:
		return "", errors.New("unsupported screen observation mode")
	}
}
