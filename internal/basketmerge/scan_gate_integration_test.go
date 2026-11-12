package basketmerge

import "testing"

func TestScannerToFocusedBasketMergeAndReceiptParity(t *testing.T) {
	g := focusedGate(t, mustGate(t, "T1", "T2"))
	lease1 := mustLease(t, g)
	state, at := feedDigits(t, 100, "670237553985")
	_, barcode, done, err := FeedScannerKey(state, '\r', at+10)
	if err != nil || !done { t.Fatalf("Enter framing: %v", err) }
	x := scan("physical-001", "T1")
	x.Barcode = barcode
	g, status, err := g.ApplyCapturedScan(lease1, x)
	if err != nil || status != Added { t.Fatalf("first scan: %s %v", status, err) }

	lease2 := mustLease(t, g)
	state, at = feedDigits(t, 1000, "670237553985")
	_, barcode, done, err = FlushScannerIdle(state, at+HIDIdleMS)
	if err != nil || !done { t.Fatalf("no-Enter framing: %v", err) }
	y := scan("physical-002", "T1")
	y.Barcode = barcode
	g, status, err = g.ApplyCapturedScan(lease2, y)
	if err != nil || status != Merged { t.Fatalf("second scan: %s %v", status, err) }

	b := g.Snapshot()
	if len(b.Tickets[0].Lines)!=1 || b.Tickets[0].Lines[0].Quantity!=2 ||
		len(b.Tickets[1].Lines)!=0 {
		t.Fatalf("basket isolation/merge wrong: %+v", b)
	}
	r, err := BuildReceiptPreview(b.Tickets[0])
	if err != nil || r.TotalIQD!=1000 || len(r.Lines)!=1 ||
		r.Lines[0].LineTotalIQD!=1000 { t.Fatalf("receipt mismatch: %+v %v", r, err) }
	if err := CheckReceiptParity(b.Tickets[0], r); err != nil { t.Fatal(err) }
}

func TestScannerMidFlightTicketSwitchRejected(t *testing.T) {
	g := focusedGate(t, mustGate(t, "T1", "T2"))
	pending := mustLease(t, g)
	scannerState, at := feedDigits(t, 20, "670237553985")
	var err error
	g, err = g.SwitchTicket("T2")
	if err != nil { t.Fatal(err) }
	g = focusedGate(t, g)
	_, barcode, done, err := FlushScannerIdle(scannerState, at+HIDIdleMS)
	if err != nil || !done { t.Fatal(err) }
	first := scan("e1", "T1")
	first.Barcode = barcode
	if got, _, err := g.ApplyCapturedScan(pending, first); err == nil ||
		len(got.Snapshot().Tickets[0].Lines)!=0 ||
		len(got.Snapshot().Tickets[1].Lines)!=0 {
		t.Fatal("stale scan was charged after a ticket switch")
	}

	// A new scan with a new lease is required after switching tickets.
	fresh := mustLease(t, g)
	second := scan("e2", "T2")
	second.Barcode = barcode
	out, status, err := g.ApplyCapturedScan(fresh, second)
	if err != nil || status != Added || len(out.Snapshot().Tickets[1].Lines)!=1 {
		t.Fatalf("fresh scan rejected: %v %s", err, status)
	}
}

func TestScannerMidFlightFocusLossRejected(t *testing.T) {
	g := focusedGate(t, mustGate(t, "T1"))
	lease := mustLease(t, g)
	state, at := feedDigits(t, 100, "670237553985")
	var err error
	g, err = g.SetScannerFocus(false)
	if err != nil { t.Fatal(err) }
	_, barcode, done, err := FeedScannerKey(state, '\r', at+10)
	if err != nil || !done { t.Fatal(err) }
	x := scan("e1", "T1")
	x.Barcode=barcode
	if next, _, err := g.ApplyCapturedScan(lease, x); err == nil ||
		len(next.Snapshot().Tickets[0].Lines)!=0 {
		t.Fatal("scan accepted after focus loss")
	}
}
