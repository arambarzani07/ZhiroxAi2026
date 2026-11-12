package basketmerge

import (
	"errors"
	"strings"
)

// ScannerState is a deterministic, in-memory HID input buffer for a focused
// scanner field. It does NOT install global keyboard hooks or read device input.
// The caller owns focus, timestamps and the uniqueness of the scan EventID.
type ScannerState struct {
	Digits    string
	LastKeyMS int64
	Active    bool
}

const (
	// HIDIdleMS must be validated against the real scanner hardware before use.
	HIDIdleMS int64 = 250
	MinBarcodeDigits = 4
	MaxBarcodeDigits = 32
)

// FeedScannerKey accepts one ASCII digit or CR/LF terminator. A scan with Enter
// emits immediately. A scan without Enter is emitted ONLY by FlushScannerIdle.
// No partial scan is silently accepted following an interrupted key sequence.
func FeedScannerKey(before ScannerState, key rune, atMS int64) (ScannerState, string, bool, error) {
	if atMS < 0 {
		return before, "", false, errors.New("negative timestamp")
	}
	if before.Active && atMS < before.LastKeyMS {
		return before, "", false, errors.New("non-monotonic scanner timestamp")
	}
	if key == '\r' || key == '\n' {
		if !before.Active {
			return before, "", false, nil // optional LF after a CR
		}
		if len(before.Digits) < MinBarcodeDigits {
			return before, "", false, errors.New("barcode too short")
		}
		return ScannerState{}, before.Digits, true, nil
	}
	if key < '0' || key > '9' {
		return before, "", false, errors.New("non-digit scanner input")
	}
	if before.Active && atMS-before.LastKeyMS > HIDIdleMS {
		return before, "", false, errors.New("scanner input gap: flush or discard pending scan before accepting another key")
	}
	if len(before.Digits) >= MaxBarcodeDigits {
		return before, "", false, errors.New("barcode exceeds maximum length")
	}
	next := ScannerState{Digits: before.Digits + string(key), LastKeyMS: atMS, Active: true}
	return next, "", false, nil
}

// FlushScannerIdle emits a no-Enter barcode only after a quiet interval.
// Scans are NOT automatically bound to any Cashier ticket or sale.
func FlushScannerIdle(before ScannerState, atMS int64) (ScannerState, string, bool, error) {
	if !before.Active {
		return before, "", false, nil
	}
	if atMS < before.LastKeyMS {
		return before, "", false, errors.New("non-monotonic scanner timestamp")
	}
	if atMS-before.LastKeyMS < HIDIdleMS {
		return before, "", false, nil
	}
	if len(before.Digits) < MinBarcodeDigits {
		return before, "", false, errors.New("barcode too short")
	}
	return ScannerState{}, before.Digits, true, nil
}

// DiscardScannerInput explicitly clears a malformed/aborted input without
// generating a sale event.
func DiscardScannerInput(_ ScannerState) ScannerState {
	return ScannerState{}
}

// BarcodeCanBeAutoMerged deliberately rejects unverified variable-weight labels.
// An authoritative catalog resolution is still REQUIRED before ApplyScan.
func BarcodeCanBeAutoMerged(code string) bool {
	if len(code) < MinBarcodeDigits || len(code) > MaxBarcodeDigits {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	// The legacy store's prefix-27 scale format requires an isolated decoding
	// and weight calculation workflow, not blind basket coalescing.
	return !strings.HasPrefix(code, "27")
}
