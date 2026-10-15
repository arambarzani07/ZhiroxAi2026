package remoteagent

import (
	"encoding/json"
	"errors"
	"testing"
)

type fakeControlSink struct {
	batches int
	fail    bool
}

func (f *fakeControlSink) ApplyControlEvents(events []ControlEvent) error {
	if f.fail { return errors.New("sink failed") }
	if len(events) == 0 { return errors.New("empty batch") }
	f.batches++
	return nil
}

func controlPayload(t *testing.T, seq uint64) []byte {
	t.Helper()
	raw, err := json.Marshal(ControlMessage{
		Version: ControlProtocolVersion,
		Type: "input",
		Events: []ControlEvent{{Sequence: seq, Kind: "pointer_move", X: 0.5, Y: 0.5}},
	})
	if err != nil { t.Fatal(err) }
	return raw
}

func TestControlDispatcherFailClosedUntilActivated(t *testing.T) {
	sink := &fakeControlSink{}
	d, err := NewControlDispatcher(sink)
	if err != nil { t.Fatal(err) }
	if err := d.Dispatch(controlPayload(t, 1)); err == nil { t.Fatal("inactive dispatcher must refuse control") }
	if sink.batches != 0 { t.Fatal("inactive dispatcher reached sink") }
}

func TestControlDispatcherAppliesOnlyValidatedOrderedBatches(t *testing.T) {
	sink := &fakeControlSink{}
	d, err := NewControlDispatcher(sink)
	if err != nil { t.Fatal(err) }
	d.Activate()
	if err := d.Dispatch(controlPayload(t, 1)); err != nil { t.Fatalf("dispatch 1: %v", err) }
	if d.LastSequence() != 1 || sink.batches != 1 { t.Fatal("first batch not committed") }
	if err := d.Dispatch(controlPayload(t, 1)); err == nil { t.Fatal("replay should fail") }
	if sink.batches != 1 { t.Fatal("replayed batch reached sink") }
	if err := d.Dispatch(controlPayload(t, 2)); err != nil { t.Fatalf("dispatch 2: %v", err) }
	if d.LastSequence() != 2 || sink.batches != 2 { t.Fatal("second batch not committed") }
}

func TestControlDispatcherDoesNotAdvanceOnSinkFailure(t *testing.T) {
	sink := &fakeControlSink{fail: true}
	d, err := NewControlDispatcher(sink)
	if err != nil { t.Fatal(err) }
	d.Activate()
	if err := d.Dispatch(controlPayload(t, 1)); err == nil { t.Fatal("sink failure should propagate") }
	if d.LastSequence() != 0 { t.Fatal("sequence advanced after sink failure") }
	sink.fail = false
	if err := d.Dispatch(controlPayload(t, 1)); err != nil { t.Fatalf("retry should succeed: %v", err) }
	if d.LastSequence() != 1 { t.Fatal("retry did not commit sequence") }
}

func TestControlDispatcherDeactivateResetsAndClosesGate(t *testing.T) {
	sink := &fakeControlSink{}
	d, _ := NewControlDispatcher(sink)
	d.Activate()
	if err := d.Dispatch(controlPayload(t, 1)); err != nil { t.Fatal(err) }
	d.Deactivate()
	if d.LastSequence() != 0 { t.Fatal("deactivate should reset sequence") }
	if err := d.Dispatch(controlPayload(t, 2)); err == nil { t.Fatal("deactivated dispatcher must refuse control") }
}
