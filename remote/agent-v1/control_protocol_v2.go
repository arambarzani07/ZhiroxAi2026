package remoteagent

import (
    "encoding/json"
    "errors"
    "fmt"
    "math"
    "strings"
)

const (
    ControlProtocolVersion = 2
    MaxControlEvents = 32
    MaxControlMessageBytes = 32 * 1024
)

type ControlEvent struct {
    Sequence uint64 `json:"sequence"`
    Kind string `json:"kind"`
    X float64 `json:"x,omitempty"`
    Y float64 `json:"y,omitempty"`
    Button string `json:"button,omitempty"`
    Down bool `json:"down,omitempty"`
    KeyCode int `json:"key_code,omitempty"`
    Wheel int `json:"wheel,omitempty"`
}

type ControlMessage struct {
    Version int `json:"v"`
    Type string `json:"type"`
    Events []ControlEvent `json:"events"`
}

func blockedSystemKey(code int) bool {
    switch code {
    case 0x5B, 0x5C, 0x5D: // left/right Windows key and Apps/Menu key
        return true
    default:
        return false
    }
}

func ValidateControlMessage(data []byte, lastSequence uint64) (ControlMessage, uint64, error) {
    if len(data) == 0 || len(data) > MaxControlMessageBytes {
        return ControlMessage{}, lastSequence, errors.New("control message size outside approved bounds")
    }
    var msg ControlMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        return ControlMessage{}, lastSequence, errors.New("invalid control message JSON")
    }
    if msg.Version != ControlProtocolVersion || !strings.EqualFold(strings.TrimSpace(msg.Type), "input") {
        return ControlMessage{}, lastSequence, errors.New("unsupported control protocol")
    }
    if len(msg.Events) == 0 || len(msg.Events) > MaxControlEvents {
        return ControlMessage{}, lastSequence, fmt.Errorf("control message must contain 1..%d events", MaxControlEvents)
    }
    seq := lastSequence
    for i, event := range msg.Events {
        if event.Sequence <= seq {
            return ControlMessage{}, lastSequence, fmt.Errorf("event %d replayed or out of order", i)
        }
        switch strings.ToLower(strings.TrimSpace(event.Kind)) {
        case "pointer_move":
            if math.IsNaN(event.X) || math.IsNaN(event.Y) || math.IsInf(event.X, 0) || math.IsInf(event.Y, 0) || event.X < 0 || event.X > 1 || event.Y < 0 || event.Y > 1 {
                return ControlMessage{}, lastSequence, fmt.Errorf("event %d has invalid pointer coordinates", i)
            }
        case "pointer_button":
            b := strings.ToLower(strings.TrimSpace(event.Button))
            if b != "left" && b != "right" && b != "middle" {
                return ControlMessage{}, lastSequence, fmt.Errorf("event %d has unsupported button", i)
            }
        case "pointer_wheel":
            if event.Wheel < -1200 || event.Wheel > 1200 {
                return ControlMessage{}, lastSequence, fmt.Errorf("event %d wheel delta outside approved bounds", i)
            }
        case "key":
            if event.KeyCode < 1 || event.KeyCode > 255 || blockedSystemKey(event.KeyCode) {
                return ControlMessage{}, lastSequence, fmt.Errorf("event %d key code outside approved range", i)
            }
        default:
            return ControlMessage{}, lastSequence, fmt.Errorf("event %d has unsupported kind", i)
        }
        seq = event.Sequence
    }
    return msg, seq, nil
}
