package basketmerge

import (
	"errors"
	"fmt"
)

// ReceiptLine is a display-only projection, NOT a print command or ledger.
type ReceiptLine struct {
	ProductID    string
	Barcode      string
	Unit         string
	Quantity     int64
	UnitPriceIQD int64
	LineTotalIQD int64
}

// ReceiptPreview is an in-memory preview. It does not send any print job,
// process payment, decrement stock or write to SQL.
type ReceiptPreview struct {
	TicketID string
	Lines    []ReceiptLine
	TotalIQD int64
}

// BuildReceiptPreview verifies arithmetic and uses the same immutable ticket
// input as the basket. Line totals and grand total use checked integer math.
func BuildReceiptPreview(t Ticket) (ReceiptPreview, error) {
	total, err := TotalIQD(t)
	if err != nil {
		return ReceiptPreview{}, err
	}
	if t.ID == "" {
		return ReceiptPreview{}, errors.New("receipt ticket ID required")
	}
	p := ReceiptPreview{TicketID: t.ID, Lines: make([]ReceiptLine, 0, len(t.Lines)), TotalIQD: total}
	for _, l := range t.Lines {
		p.Lines = append(p.Lines, ReceiptLine{
			ProductID: l.ProductID, Barcode: l.Barcode, Unit: l.Unit,
			Quantity: l.Quantity, UnitPriceIQD: l.UnitPriceIQD,
			LineTotalIQD: l.Quantity * l.UnitPriceIQD,
		})
	}
	return p, nil
}

// CheckReceiptParity refuses a proposed receipt whose rows or total differ
// from the basket snapshot. It never modifies or prints the receipt.
func CheckReceiptParity(t Ticket, proposed ReceiptPreview) error {
	expected, err := BuildReceiptPreview(t)
	if err != nil {
		return err
	}
	if expected.TicketID != proposed.TicketID || expected.TotalIQD != proposed.TotalIQD || len(expected.Lines) != len(proposed.Lines) {
		return errors.New("basket and receipt header/total/line count mismatch")
	}
	for i, line := range expected.Lines {
		if line != proposed.Lines[i] {
			return fmt.Errorf("basket and receipt line %d mismatch", i+1)
		}
	}
	return nil
}
