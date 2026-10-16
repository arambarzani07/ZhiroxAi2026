package remoteagent

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AttendedControlRuntime binds a previously approved attended authorization to
// the visible session indicator and the per-message authorization gate. This
// runtime deliberately keeps the abstract dispatcher deactivated; a later,
// separately reviewed activation phase must opt in after the same local
// approval remains valid. No OS input execution happens here.
type AttendedControlRuntime struct {
	dispatcher *ControlDispatcher
	indicator  SessionIndicator
	gate       *AttendedControlMessageGate
	done       chan error
	stopOnce   sync.Once
	stateMu    sync.RWMutex
	active     bool
}

type AttendedControlRuntimeConfig struct {
	Arbiter         *TrustedSessionArbiter
	Permit          TrustedControlPermit
	ApprovalResult  AttendedInputApprovalResult
	Consent         RemoteInputConsent
	ControllerLabel string
	Dispatcher      *ControlDispatcher
	Now             time.Time
}

// PrepareAttendedControlRuntime re-validates the complete attended
// authorization, starts the visible indicator, and creates the per-message
// gate. The dispatcher remains inactive and is explicitly deactivated again on
// every runtime teardown path.
func PrepareAttendedControlRuntime(ctx context.Context, cfg AttendedControlRuntimeConfig) (*AttendedControlRuntime, error) {
	if ctx == nil {
		return nil, errors.New("attended control runtime context is required")
	}
	if cfg.Arbiter == nil {
		return nil, errors.New("trusted session arbiter is required")
	}
	if cfg.Dispatcher == nil {
		return nil, errors.New("control dispatcher is required")
	}
	if cfg.Consent == nil {
		return nil, errors.New("interactive input consent provider is required")
	}
	now := trustedArbiterNow(cfg.Now)
	if err := cfg.ApprovalResult.Authorization.Validate(cfg.Arbiter, cfg.Permit, cfg.ApprovalResult.Approval, now); err != nil {
		return nil, err
	}
	indicator, err := StartAttendedInputIndicator(
		cfg.Arbiter,
		cfg.Permit,
		cfg.Consent,
		cfg.ApprovalResult,
		cfg.ControllerLabel,
		now,
	)
	if err != nil {
		return nil, err
	}
	gate := &AttendedControlMessageGate{
		Arbiter:       cfg.Arbiter,
		Permit:        cfg.Permit,
		Approval:      cfg.ApprovalResult.Approval,
		Authorization: cfg.ApprovalResult.Authorization,
	}
	if err := gate.ValidateControlMessage(now); err != nil {
		_ = indicator.Stop()
		return nil, err
	}

	// Fail closed even if a reused dispatcher arrived in an active state.
	cfg.Dispatcher.Deactivate()
	r := &AttendedControlRuntime{
		dispatcher: cfg.Dispatcher,
		indicator:  indicator,
		gate:       gate,
		done:       make(chan error, 1),
		active:     true,
	}
	go r.watch(ctx)
	return r, nil
}

func (r *AttendedControlRuntime) watch(ctx context.Context) {
	select {
	case <-ctx.Done():
		r.finish(nil, true)
	case err, ok := <-r.indicator.Done():
		if !ok {
			err = nil
		}
		r.finish(err, false)
	}
}

func (r *AttendedControlRuntime) finish(err error, stopIndicator bool) {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.stateMu.Lock()
		r.active = false
		r.stateMu.Unlock()
		if r.dispatcher != nil {
			r.dispatcher.Deactivate()
		}
		if stopIndicator && r.indicator != nil {
			if stopErr := r.indicator.Stop(); err == nil {
				err = stopErr
			}
		}
		r.done <- err
		close(r.done)
	})
}

func (r *AttendedControlRuntime) isActive() bool {
	if r == nil {
		return false
	}
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.active
}

func (r *AttendedControlRuntime) ControlGate() ControlMessageGate {
	if r == nil {
		return nil
	}
	return r.gate
}

func (r *AttendedControlRuntime) Done() <-chan error {
	if r == nil {
		return nil
	}
	return r.done
}

func (r *AttendedControlRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.finish(nil, true)
	return nil
}
