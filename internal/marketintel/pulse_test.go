package marketintel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnalyzeJSONDedupAndPriceAnomalies(t *testing.T) {
	events := []Event{
		{ID: "event-1", EventType: "itemname_delta", CreatedAt: "2026-11-09T20:21:32Z", Payload: json.RawMessage(`{"event_key":"seq-1","read_only":true,"changes":[{"kind":"added","itemNameID":"54807","itemCode":"6291003022999","itemName":"sample"},{"kind":"price_changed","itemNameID":"9907","itemCode":"8680417200655","priceField":"SalePrice","oldPrice":"1500","newPrice":"1750"},{"kind":"price_changed","itemNameID":"9907","itemCode":"8680417200655","priceField":"WholeSalePrice","oldPrice":"1291","newPrice":"1600"}]}`)},
		{ID: "event-1-copy", EventType: "itemname_delta", CreatedAt: "2026-11-09T20:21:32Z", Payload: json.RawMessage(`{"event_key":"seq-1","read_only":true,"changes":[]}`)},
		{ID: "event-2", EventType: "itemname_delta", CreatedAt: "2026-11-09T20:51:32Z", Payload: json.RawMessage(`{"read_only":true,"changes":[{"kind":"price_changed","itemNameID":"9907","itemCode":"8680417200655","priceField":"SalePrice","oldPrice":"100","newPrice":"140"},{"kind":"price_changed","itemNameID":"44","priceField":"SalePrice","oldPrice":"100","newPrice":"0"}]}`)},
		{ID: "other", EventType: "agent_health", Payload: json.RawMessage(`{}`)},
	}
	b, _ := json.Marshal(events)
	s, err := AnalyzeJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if s.EventsSeen != 4 || s.EventsAnalyzed != 2 || s.DuplicateEventsSkipped != 1 {
		t.Fatalf("bad event counts: %+v", s)
	}
	if s.NewProducts != 1 || s.ProductsWithPriceChanges != 2 {
		t.Fatalf("bad product counts: %+v", s)
	}
	if s.RetailPriceChanges != 3 || s.WholesalePriceChanges != 1 {
		t.Fatalf("bad price counts: %+v", s)
	}
	if s.LargeRetailIncreases != 1 || s.SuspiciousRetailPrices != 1 || len(s.Alerts) != 2 {
		t.Fatalf("bad alerts: %+v", s)
	}
	if !s.ReadOnly || s.LatestEventUTC != "2026-11-09T20:51:32Z" {
		t.Fatalf("bad source: %+v", s)
	}
}

func TestAnalyzeJSONRejectsUnsafeAndMalformed(t *testing.T) {
	cases := []string{
		`[ {"id":"1","event_type":"itemname_delta","payload":{"event_key":"1","read_only":false,"changes":[]}} ]`,
		`[ {"id":"1","event_type":"itemname_delta","payload":{"event_key":"1","changes":[]}} ]`,
		`[ {"event_type":"itemname_delta","payload":{"read_only":true,"changes":[]}} ]`,
		`invalid`,
	}
	for _, c := range cases {
		if _, err := AnalyzeJSON([]byte(c)); err == nil {
			t.Fatalf("accepted unsafe input: %s", c)
		}
	}
	if _, err := AnalyzeJSON([]byte(strings.Repeat(" ", maxInputBytes+1))); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestZeroBaselineNotCountedAsPercentageIncrease(t *testing.T) {
	b := []byte(`[{"id":"x","event_type":"itemname_delta","payload":{"read_only":true,"changes":[{"kind":"price_changed","itemNameID":"7","priceField":"SalePrice","oldPrice":"0","newPrice":"4000"}]}}]`)
	s, err := AnalyzeJSON(b)
	if err != nil || s.LargeRetailIncreases != 0 || s.RetailPriceChanges != 1 {
		t.Fatalf("unexpected result: %+v, %v", s, err)
	}
}
