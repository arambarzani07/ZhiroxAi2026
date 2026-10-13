package main

import (
	"errors"
	"fmt"
	"strings"
)

const (
	pcOSRemoteInputRuntimeEnabled = false
	pcOSRemoteInputMaxBatch       = 64
	pcOSRemoteInputMaxKeyCode     = 255
	pcOSRemoteInputMaxWheelDelta  = 1200
)

type pcOSRemoteInputEvent struct {
	Sequence uint64  `json:"sequence"`
	Type     string  `json:"type"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	Button   string  `json:"button,omitempty"`
	Down     bool    `json:"down,omitempty"`
	KeyCode  int     `json:"key_code,omitempty"`
	Wheel    int     `json:"wheel,omitempty"`
}

func pcOSRemoteValidateInputBatch(events []pcOSRemoteInputEvent, lastSequence uint64) (uint64, error) {
	if !pcOSRemoteInputRuntimeEnabled {
		return lastSequence, errors.New("remote input runtime is disabled in phase 1")
	}
	if len(events) == 0 || len(events) > pcOSRemoteInputMaxBatch {
		return lastSequence, fmt.Errorf("remote input batch must contain 1..%d events", pcOSRemoteInputMaxBatch)
	}
	seq := lastSequence
	for i, event := range events {
		if event.Sequence <= seq {
			return lastSequence, fmt.Errorf("remote input event %d is replayed or out of order", i)
		}
		if err := pcOSRemoteValidateInputEvent(event); err != nil {
			return lastSequence, fmt.Errorf("remote input event %d: %w", i, err)
		}
		seq = event.Sequence
	}
	return seq, nil
}

func pcOSRemoteValidateInputEvent(event pcOSRemoteInputEvent) error {
	t := strings.ToLower(strings.TrimSpace(event.Type))
	switch t {
	case "mouse_move":
		if event.X < 0 || event.X > 1 || event.Y < 0 || event.Y > 1 {
			return errors.New("mouse coordinates must be normalized to 0..1")
		}
	case "mouse_button":
		button := strings.ToLower(strings.TrimSpace(event.Button))
		if button != "left" && button != "right" && button != "middle" {
			return errors.New("mouse button is not allowed")
		}
	case "mouse_wheel":
		if event.Wheel < -pcOSRemoteInputMaxWheelDelta || event.Wheel > pcOSRemoteInputMaxWheelDelta {
			return errors.New("mouse wheel delta exceeds safety bound")
		}
	case "key":
		if event.KeyCode < 1 || event.KeyCode > pcOSRemoteInputMaxKeyCode {
			return errors.New("key code outside approved range")
		}
	default:
		return errors.New("unsupported remote input event type")
	}
	return nil
}
