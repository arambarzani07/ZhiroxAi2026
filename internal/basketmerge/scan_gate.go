package basketmerge

import (
	"errors"
	"fmt"
	"math"
)

// ScanGate is an isolated, in-memory boundary between a focused scan field,
// selected ticket and the deterministic basket. It does not capture input,
// control Windows, authorize checkout, or persist transactions.
//
// All methods return new values; callers MUST serialize access to their
// authoritative ScanGate instance and MUST NOT reuse a stale ScanLease.
type ScanGate struct {
	basket     Session
	ticketID   string
	focused    bool
	focusEpoch uint64
	revision   uint64
}

// ScanLease is an opaque snapshot of scanner focus/ticket at scan start.
// It is intentionally not a permission or device-authentication token.
type ScanLease struct {
	ticketID   string
	focusEpoch uint64
	revision   uint64
}

func cloneBasket(s Session) Session {
	out := Session{Tickets: make([]Ticket, len(s.Tickets))}
	for i, t := range s.Tickets {
		out.Tickets[i] = Ticket{
			ID: t.ID,
			Lines: append([]Line(nil), t.Lines...),
			SeenEventIDs: append([]string(nil), t.SeenEventIDs...),
		}
	}
	return out
}

func validTicket(s Session, id string) bool {
	for _, t := range s.Tickets {
		if t.ID == id {
			return true
		}
	}
	return false
}

// NewScanGate requires 1..5 unique tickets and a selected valid ticket.
// Input sessions are deep-copied; real legacy SQL/Cashier are never opened.
func NewScanGate(s Session, activeTicketID string) (ScanGate, error) {
	if len(s.Tickets) == 0 || len(s.Tickets) > MaxTickets {
		return ScanGate{}, errors.New("invalid number of held tickets")
	}
	ids := make([]string, 0, len(s.Tickets))
	globalEvents := make(map[string]bool)
	for _, t := range s.Tickets {
		if _, err := TotalIQD(t); err != nil {
			return ScanGate{}, fmt.Errorf("invalid ticket: %w", err)
		}
		if len(t.SeenEventIDs) > MaxScansPerTicket {
			return ScanGate{}, errors.New("ticket scan history limit exceeded")
		}
		for _, event := range t.SeenEventIDs {
			if event == "" || globalEvents[event] {
				return ScanGate{}, errors.New("duplicate or empty event ID in session history")
			}
			globalEvents[event] = true
		}
		ids = append(ids, t.ID)
	}
	if _, err := NewSession(ids...); err != nil {
		return ScanGate{}, err
	}
	if !validTicket(s, activeTicketID) {
		return ScanGate{}, errors.New("active ticket not found")
	}
	return ScanGate{basket: cloneBasket(s), ticketID: activeTicketID}, nil
}

// Snapshot returns a copy; the caller cannot mutate the gate's internal state.
func (g ScanGate) Snapshot() Session { return cloneBasket(g.basket) }

func (g ScanGate) Revision() uint64 { return g.revision }
func (g ScanGate) ActiveTicket() string { return g.ticketID }

// SwitchTicket invalidates all pending scans, even when selecting the same
// ticket. Scanner focus must be re-established by the host application.
func (g ScanGate) SwitchTicket(ticketID string) (ScanGate, error) {
	if !validTicket(g.basket, ticketID) {
		return g, errors.New("target ticket not found")
	}
	if g.focusEpoch == math.MaxUint64 {
		return g, errors.New("focus epoch overflow")
	}
	g.focusEpoch++
	g.ticketID = ticketID
	g.focused = false
	return g, nil
}

// SetScannerFocus invalidates pending leases on every focus transition.
// Application-owned input focus is necessary but does NOT attest a physical
// scanner or separate it from rapid manual typing.
func (g ScanGate) SetScannerFocus(focused bool) (ScanGate, error) {
	if g.focusEpoch == math.MaxUint64 {
		return g, errors.New("focus epoch overflow")
	}
	g.focusEpoch++
	g.focused = focused
	return g, nil
}

func (g ScanGate) BeginScan() (ScanLease, error) {
	if !g.focused || !validTicket(g.basket, g.ticketID) {
		return ScanLease{}, errors.New("scanner field is not focused on a valid ticket")
	}
	return ScanLease{ticketID: g.ticketID, focusEpoch: g.focusEpoch, revision: g.revision}, nil
}

// ApplyCapturedScan refuses to apply scans after focus switch, ticket switch,
// intervening basket mutations, or a mismatched ticket ID. This is a
// single-instance, fail-closed development prototype, NOT a multi-PC lock.
func (g ScanGate) ApplyCapturedScan(lease ScanLease, scan Scan) (ScanGate, Outcome, error) {
	if !g.focused || lease.ticketID == "" ||
		lease.ticketID != g.ticketID ||
		lease.focusEpoch != g.focusEpoch ||
		lease.revision != g.revision ||
		scan.TicketID != lease.ticketID {
		return g, "", errors.New("scan context changed; discard pending scan")
	}
	if g.revision == math.MaxUint64 {
		return g, "", errors.New("basket revision overflow")
	}
	next, outcome, err := ApplyScan(g.basket, scan)
	if err != nil {
		return g, "", err
	}
	if outcome == AlreadyProcessed {
		return g, outcome, nil
	}
	g.basket = next
	g.revision++
	return g, outcome, nil
}
