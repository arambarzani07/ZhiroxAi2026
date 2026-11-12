package basketmerge

import (
	"reflect"
	"testing"
)

func mustGate(t *testing.T, ids ...string) ScanGate {
	t.Helper()
	s, err := NewSession(ids...)
	if err != nil { t.Fatal(err) }
	g, err := NewScanGate(s, ids[0])
	if err != nil { t.Fatal(err) }
	return g
}

func focusedGate(t *testing.T, g ScanGate) ScanGate {
	t.Helper()
	var err error
	g, err = g.SetScannerFocus(true)
	if err != nil { t.Fatal(err) }
	return g
}

func mustLease(t *testing.T, g ScanGate) ScanLease {
	t.Helper()
	l, err := g.BeginScan()
	if err != nil { t.Fatal(err) }
	return l
}

func TestScanGateRequiresExplicitFocusAndTicket(t *testing.T) {
	g := mustGate(t, "1", "2")
	if _, err := g.BeginScan(); err == nil { t.Fatal("unfocused scanner accepted") }
	g = focusedGate(t, g)
	l := mustLease(t, g)
	var err error
	g, err = g.SwitchTicket("2")
	if err != nil { t.Fatal(err) }
	if _, err := g.BeginScan(); err == nil { t.Fatal("switch kept scanner focus") }
	g = focusedGate(t, g)
	if got, _, err := g.ApplyCapturedScan(l, scan("e1", "1")); err == nil || got.Revision() != 0 {
		t.Fatal("stale scan applied to switched ticket")
	}
	current := mustLease(t, g)
	g, status, err := g.ApplyCapturedScan(current, scan("e2", "2"))
	if err != nil || status != Added { t.Fatalf("valid focused scan failed: %v %s", err, status) }
	s := g.Snapshot()
	if len(s.Tickets[0].Lines) != 0 || len(s.Tickets[1].Lines) != 1 { t.Fatal("wrong ticket charged") }
}

func TestScanGateFocusLossAndSameTicketSwitchInvalidateLease(t *testing.T) {
	g := focusedGate(t, mustGate(t, "1"))
	l := mustLease(t, g)
	var err error
	g, err = g.SetScannerFocus(false)
	if err != nil { t.Fatal(err) }
	if _, _, err := g.ApplyCapturedScan(l, scan("e1", "1")); err == nil { t.Fatal("lost focus did not block scan") }
	g = focusedGate(t, g)
	if _, _, err := g.ApplyCapturedScan(l, scan("e1", "1")); err == nil { t.Fatal("stale lease survived focus regain") }
	fresh := mustLease(t, g)
	g, err = g.SwitchTicket("1")
	if err != nil { t.Fatal(err) }
	g = focusedGate(t, g)
	if _, _, err := g.ApplyCapturedScan(fresh, scan("e1", "1")); err == nil {
		t.Fatal("same ticket switch did not invalidate pending scan")
	}
	if len(g.Snapshot().Tickets[0].Lines) != 0 { t.Fatal("unexpected basket mutation") }
}

func TestScanGateRejectsStaleRevisionAndDuplicateRetry(t *testing.T) {
	g := focusedGate(t, mustGate(t, "1"))
	old := mustLease(t, g)
	next, status, err := g.ApplyCapturedScan(old, scan("event1", "1"))
	if err != nil || status != Added || next.Revision() != 1 || g.Revision() != 0 {
		t.Fatalf("revision/copy-on-write invalid: %s %v", status, err)
	}
	if got, _, err := next.ApplyCapturedScan(old, scan("event2", "1")); err == nil || got.Revision() != 1 {
		t.Fatal("stale basket revision used")
	}
	retry := mustLease(t, next)
	again, status, err := next.ApplyCapturedScan(retry, scan("event1", "1"))
	if err != nil || status != AlreadyProcessed || again.Revision() != 1 {
		t.Fatalf("duplicate event double-charged: %s %v", status, err)
	}
	fresh := mustLease(t, next)
	merged, status, err := next.ApplyCapturedScan(fresh, scan("event2", "1"))
	if err != nil || status != Merged || merged.Revision() != 2 {
		t.Fatalf("new physical scan failed: %s %v", status, err)
	}
	ticket := merged.Snapshot().Tickets[0]
	total, err := TotalIQD(ticket)
	if err != nil || total != 1000 || ticket.Lines[0].Quantity != 2 {
		t.Fatalf("merged basket inconsistent: %+v %v", ticket, err)
	}
}

func TestScanGateCopyAndExternalSnapshotIsolated(t *testing.T) {
	s, err := NewSession("1")
	if err != nil { t.Fatal(err) }
	g, err := NewScanGate(s, "1")
	if err != nil { t.Fatal(err) }
	s.Tickets[0].ID = "tampered"
	original := g.Snapshot()
	if original.Tickets[0].ID != "1" { t.Fatal("source aliases gate internal ticket") }
	g = focusedGate(t, g)
	l := mustLease(t, g)
	g, _, err = g.ApplyCapturedScan(l, scan("e1", "1"))
	if err != nil { t.Fatal(err) }
	snapshot := g.Snapshot()
	snapshot.Tickets[0].Lines[0].Quantity = 9999
	snapshot.Tickets[0].SeenEventIDs[0] = "tampered"
	if got := g.Snapshot(); got.Tickets[0].Lines[0].Quantity != 1 || got.Tickets[0].SeenEventIDs[0] != "e1" {
		t.Fatal("external caller mutated authoritative state")
	}
}

func TestScanGateFailClosedWithoutMutation(t *testing.T) {
	g := focusedGate(t, mustGate(t, "1", "2"))
	l := mustLease(t, g)
	before := g.Snapshot()
	wrong := scan("e1", "2")
	if got, _, err := g.ApplyCapturedScan(l, wrong); err == nil || !reflect.DeepEqual(before, got.Snapshot()) {
		t.Fatal("wrong ticket accepted or changed state")
	}
	invalid := scan("e1", "1")
	invalid.Unit = "kg"
	if got, _, err := g.ApplyCapturedScan(l, invalid); err == nil || !reflect.DeepEqual(before, got.Snapshot()) {
		t.Fatal("unsupported weight sale changed state")
	}
	if _, err := g.SwitchTicket("nonexistent"); err == nil { t.Fatal("invalid ticket switch accepted") }
}

func TestScanGateRejectsCorruptHistory(t *testing.T) {
	s, err := NewSession("1", "2")
	if err != nil { t.Fatal(err) }
	s.Tickets[0].SeenEventIDs = []string{"scan1"}
	s.Tickets[1].SeenEventIDs = []string{"scan1"}
	if _, err := NewScanGate(s, "1"); err == nil { t.Fatal("cross-ticket duplicate IDs accepted") }
	s.Tickets[1].SeenEventIDs = []string{""}
	if _, err := NewScanGate(s, "1"); err == nil { t.Fatal("blank event accepted") }
	s.Tickets[1].SeenEventIDs = nil
	if _, err := NewScanGate(s, "missing"); err == nil { t.Fatal("unknown active ticket accepted") }
}
