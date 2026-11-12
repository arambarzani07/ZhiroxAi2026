package basketmerge

import (
	"math"
	"testing"
)

func TestScannerToBasketToReceiptEndToEnd(t *testing.T) {
	s := session(t, "T1", "T2")
	state, at := feedDigits(t, 100, "670237553985")
	_, code1, done, err := FeedScannerKey(state, '\r', at+10)
	if err != nil || !done || !BarcodeCanBeAutoMerged(code1) {
		t.Fatalf("Enter barcode not ready: %q %v", code1, err)
	}
	first := scan("physical-scan-1", "T1")
	first.Barcode = code1
	s = apply(t, s, first, Added)

	state, at = feedDigits(t, 1000, "670237553985")
	_, code2, done, err := FlushScannerIdle(state, at+HIDIdleMS)
	if err != nil || !done || !BarcodeCanBeAutoMerged(code2) {
		t.Fatalf("no-Enter barcode not ready: %q %v", code2, err)
	}
	second := scan("physical-scan-2", "T1")
	second.Barcode = code2
	s = apply(t, s, second, Merged)

	receipt, err := BuildReceiptPreview(s.Tickets[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Lines) != 1 || receipt.Lines[0].Quantity != 2 ||
		receipt.Lines[0].UnitPriceIQD != 500 || receipt.Lines[0].LineTotalIQD != 1000 ||
		receipt.TotalIQD != 1000 {
		t.Fatalf("receipt not equal to merged basket: %+v", receipt)
	}
	if err := CheckReceiptParity(s.Tickets[0], receipt); err != nil {
		t.Fatal(err)
	}
	other, err := BuildReceiptPreview(s.Tickets[1])
	if err != nil || other.TotalIQD != 0 || len(other.Lines) != 0 {
		t.Fatalf("second ticket was contaminated: %+v %v", other, err)
	}
	// A retried delivery of an earlier scan must not change the receipt.
	s = apply(t, s, first, AlreadyProcessed)
	retained, err := BuildReceiptPreview(s.Tickets[0])
	if err != nil || retained.TotalIQD != 1000 {
		t.Fatalf("duplicate delivery changed total: %+v %v", retained, err)
	}
}

func TestReceiptParityRejectsAnyTampering(t *testing.T) {
	s := session(t, "T1")
	s = apply(t, s, scan("physical-1", "T1"), Added)
	s = apply(t, s, scan("physical-2", "T1"), Merged)
	original, err := BuildReceiptPreview(s.Tickets[0])
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		change func(*ReceiptPreview)
	}{
		{"grand total", func(p *ReceiptPreview) { p.TotalIQD++ }},
		{"ticket", func(p *ReceiptPreview) { p.TicketID = "T2" }},
		{"quantity", func(p *ReceiptPreview) { p.Lines[0].Quantity++ }},
		{"unit price", func(p *ReceiptPreview) { p.Lines[0].UnitPriceIQD++ }},
		{"line total", func(p *ReceiptPreview) { p.Lines[0].LineTotalIQD++ }},
		{"barcode", func(p *ReceiptPreview) { p.Lines[0].Barcode = "00000000" }},
		{"missing line", func(p *ReceiptPreview) { p.Lines = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tampered := original
			tampered.Lines = append([]ReceiptLine(nil), original.Lines...)
			tc.change(&tampered)
			if err := CheckReceiptParity(s.Tickets[0], tampered); err == nil {
				t.Fatal("tampered receipt accepted")
			}
		})
	}
	if err := CheckReceiptParity(s.Tickets[0], original); err != nil {
		t.Fatalf("input was modified: %v", err)
	}
}

func TestReceiptArithmeticFailClosed(t *testing.T) {
	bad := Ticket{ID:"T1", Lines: []Line{{ProductID:"x", Barcode:"12345678", Unit:"piece", Quantity:2, UnitPriceIQD:math.MaxInt64}}}
	if _, err := BuildReceiptPreview(bad); err == nil {
		t.Fatal("overflow receipt accepted")
	}
	if _, err := BuildReceiptPreview(Ticket{}); err == nil {
		t.Fatal("receipt without ticket ID accepted")
	}
}
