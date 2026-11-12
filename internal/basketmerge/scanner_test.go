package basketmerge

import (
	"strings"
	"testing"
)

func feedDigits(t *testing.T, startMS int64, code string) (ScannerState, int64) {
	t.Helper()
	state := ScannerState{}
	at := startMS
	for _, digit := range code {
		var value string
		var emitted bool
		var err error
		state, value, emitted, err = FeedScannerKey(state, digit, at)
		if err != nil || emitted || value != "" {
			t.Fatalf("unexpected intermediate scan: %q emitted=%t err=%v", value, emitted, err)
		}
		at += 10
	}
	return state, at - 10
}

func TestScannerWithEnterAndCRLF(t *testing.T) {
	state, at := feedDigits(t, 100, "670237553985")
	next, code, done, err := FeedScannerKey(state, '\r', at+10)
	if err != nil || !done || code != "670237553985" || next.Active {
		t.Fatalf("Enter scan not finalized: %q done=%t err=%v", code, done, err)
	}
	_, code, done, err = FeedScannerKey(next, '\n', at+11)
	if err != nil || done || code != "" {
		t.Fatalf("CRLF produced an extra scan: %q err=%v", code, err)
	}
}

func TestScannerWithoutEnterRequiresIdle(t *testing.T) {
	state, at := feedDigits(t, 1000, "0670237553985")
	before, code, done, err := FlushScannerIdle(state, at+HIDIdleMS-1)
	if err != nil || done || code != "" || before != state {
		t.Fatal("no-Enter scan finalized too early")
	}
	after, code, done, err := FlushScannerIdle(state, at+HIDIdleMS)
	if err != nil || !done || code != "0670237553985" || after.Active {
		t.Fatalf("no-Enter scan failed: %q done=%t err=%v", code, done, err)
	}
}

func TestScannerGapRequiresExplicitDecision(t *testing.T) {
	state, at := feedDigits(t, 100, "670237553985")
	next, code, done, err := FeedScannerKey(state, '5', at+HIDIdleMS+1)
	if err == nil || done || code != "" || next != state {
		t.Fatalf("interrupted scan was silently merged: %+v %v", next, err)
	}
	after, code, done, err := FlushScannerIdle(state, at+HIDIdleMS+1)
	if err != nil || !done || code != "670237553985" || after.Active {
		t.Fatalf("pending scan not explicitly flushed: %q err=%v", code, err)
	}
}

func TestScannerInvalidOrPartialInputNeverEmitted(t *testing.T) {
	short, at := feedDigits(t, 0, "123")
	if _, _, done, err := FeedScannerKey(short, '\r', at+1); err == nil || done {
		t.Fatal("short barcode accepted with Enter")
	}
	if _, _, done, err := FlushScannerIdle(short, at+HIDIdleMS); err == nil || done {
		t.Fatal("short barcode accepted with timeout")
	}
	invalid := []rune{'a', ' ', '\t', '-'}
	for _, key := range invalid {
		if _, _, done, err := FeedScannerKey(ScannerState{}, key, 0); err == nil || done {
			t.Fatalf("invalid key %q accepted", key)
		}
	}
	long := strings.Repeat("1", MaxBarcodeDigits)
	state, at := feedDigits(t, 1, long)
	if _, _, done, err := FeedScannerKey(state, '9', at+1); err == nil || done {
		t.Fatal("oversized barcode accepted")
	}
	if _, _, done, err := FeedScannerKey(state, '9', at-1); err == nil || done {
		t.Fatal("out-of-order timestamp accepted")
	}
	if _, _, done, err := FeedScannerKey(state, '9', -1); err == nil || done {
		t.Fatal("negative timestamp accepted")
	}
	if DiscardScannerInput(state).Active {
		t.Fatal("explicit discard failed")
	}
}

func TestBarcodeMergeRequiresCatalogAndRejectsScalePrefix(t *testing.T) {
	if !BarcodeCanBeAutoMerged("670237553985") || !BarcodeCanBeAutoMerged("0670237553985") {
		t.Fatal("standard barcode unexpectedly rejected")
	}
	for _, code := range []string{"2701234567890", "27", "", "abc", "12 34"} {
		if BarcodeCanBeAutoMerged(code) {
			t.Fatalf("unverified barcode %q accepted for automatic merge", code)
		}
	}
}
