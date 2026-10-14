package main

import "testing"

func TestRemoteInputRuntimeDefaultsDisabled(t *testing.T) {
	if pcOSRemoteInputRuntimeEnabled {
		t.Fatal("remote input runtime must default disabled in phase 1")
	}
	if _, err := pcOSRemoteValidateInputBatch([]pcOSRemoteInputEvent{{Sequence: 1, Type: "mouse_move", X: 0.5, Y: 0.5}}, 0); err == nil {
		t.Fatal("input batch must fail closed while runtime is disabled")
	}
}

func TestRemoteInputEventValidation(t *testing.T) {
	valid := []pcOSRemoteInputEvent{
		{Sequence: 1, Type: "mouse_move", X: 0, Y: 1},
		{Sequence: 2, Type: "mouse_button", Button: "left", Down: true},
		{Sequence: 3, Type: "mouse_wheel", Wheel: 120},
		{Sequence: 4, Type: "key", KeyCode: 65, Down: true},
	}
	for _, event := range valid {
		if err := pcOSRemoteValidateInputEvent(event); err != nil {
			t.Fatalf("valid event rejected: %+v: %v", event, err)
		}
	}

	invalid := []pcOSRemoteInputEvent{
		{Type: "mouse_move", X: -0.01, Y: 0.5},
		{Type: "mouse_button", Button: "x1"},
		{Type: "mouse_wheel", Wheel: pcOSRemoteInputMaxWheelDelta + 1},
		{Type: "key", KeyCode: 0},
		{Type: "shell"},
	}
	for _, event := range invalid {
		if err := pcOSRemoteValidateInputEvent(event); err == nil {
			t.Fatalf("invalid event accepted: %+v", event)
		}
	}
}
