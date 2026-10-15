package remoteagent

import (
    "encoding/json"
    "testing"
)

func TestValidateControlMessageAcceptsOrderedPointerAndKeyEvents(t *testing.T) {
    raw, err := json.Marshal(ControlMessage{
        Version: ControlProtocolVersion,
        Type: "input",
        Events: []ControlEvent{
            {Sequence: 11, Kind: "pointer_move", X: 0.25, Y: 0.75},
            {Sequence: 12, Kind: "pointer_button", Button: "left", Down: true},
            {Sequence: 13, Kind: "pointer_button", Button: "left", Down: false},
            {Sequence: 14, Kind: "key", KeyCode: 65, Down: true},
            {Sequence: 15, Kind: "key", KeyCode: 65, Down: false},
        },
    })
    if err != nil { t.Fatal(err) }
    _, next, err := ValidateControlMessage(raw, 10)
    if err != nil { t.Fatalf("unexpected validation error: %v", err) }
    if next != 15 { t.Fatalf("next sequence = %d, want 15", next) }
}

func TestValidateControlMessageRejectsReplay(t *testing.T) {
    raw := []byte(`{"v":2,"type":"input","events":[{"sequence":7,"kind":"key","key_code":65,"down":true}]}`)
    if _, _, err := ValidateControlMessage(raw, 7); err == nil {
        t.Fatal("expected replayed sequence to be rejected")
    }
}

func TestValidateControlMessageRejectsOutOfRangePointer(t *testing.T) {
    raw := []byte(`{"v":2,"type":"input","events":[{"sequence":1,"kind":"pointer_move","x":1.1,"y":0.5}]}`)
    if _, _, err := ValidateControlMessage(raw, 0); err == nil {
        t.Fatal("expected out-of-range pointer coordinates to be rejected")
    }
}

func TestValidateControlMessageRejectsOversizedBatch(t *testing.T) {
    events := make([]ControlEvent, MaxControlEvents+1)
    for i := range events {
        events[i] = ControlEvent{Sequence:uint64(i+1), Kind:"key", KeyCode:65, Down:true}
    }
    raw, err := json.Marshal(ControlMessage{Version:ControlProtocolVersion, Type:"input", Events:events})
    if err != nil { t.Fatal(err) }
    if _, _, err := ValidateControlMessage(raw, 0); err == nil {
        t.Fatal("expected oversized batch to be rejected")
    }
}
