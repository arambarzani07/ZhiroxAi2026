package remoteagent

import (
	"errors"
	"sync"
)

type ControlEventSink interface {
	ApplyControlEvents(events []ControlEvent) error
}

type ControlDispatcher struct {
	mu           sync.Mutex
	sink         ControlEventSink
	active       bool
	lastSequence uint64
}

func NewControlDispatcher(sink ControlEventSink) (*ControlDispatcher, error) {
	if sink == nil {
		return nil, errors.New("control event sink is required")
	}
	return &ControlDispatcher{sink: sink}, nil
}

// Activate must be called only after the attended session has completed local
// capability disclosure and explicit approval. A new dispatcher starts closed.
func (d *ControlDispatcher) Activate() {
	if d == nil { return }
	d.mu.Lock()
	d.active = true
	d.lastSequence = 0
	d.mu.Unlock()
}

func (d *ControlDispatcher) Deactivate() {
	if d == nil { return }
	d.mu.Lock()
	d.active = false
	d.lastSequence = 0
	d.mu.Unlock()
}

func (d *ControlDispatcher) Dispatch(data []byte) error {
	if d == nil {
		return errors.New("control dispatcher is nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return errors.New("control dispatcher is inactive pending local approval")
	}
	msg, next, err := ValidateControlMessage(data, d.lastSequence)
	if err != nil {
		return err
	}
	// Advance sequence only after the sink accepts the whole batch. This keeps
	// failed batches retry-safe while replay protection remains strict.
	if err := d.sink.ApplyControlEvents(msg.Events); err != nil {
		return err
	}
	d.lastSequence = next
	return nil
}

func (d *ControlDispatcher) LastSequence() uint64 {
	if d == nil { return 0 }
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSequence
}
