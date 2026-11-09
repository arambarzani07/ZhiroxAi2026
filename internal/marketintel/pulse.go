// Package marketintel provides deterministic, read-only analyses of existing
// PC Relay ItemName delta events. It intentionally has no database, network,
// process-management, or filesystem write capability.
package marketintel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	maxInputBytes = 32 << 20
	maxEvents = 10000
	maxChangesPerEvent = 1000
	maxAlerts = 100
	largeRetailIncreasePercent = int64(35)
)

// Event is the selected, non-secret subset of a pc_os_events record.
type Event struct {
	ID        string          `json:"id"`
	CreatedAt string          `json:"created_at"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

type deltaPayload struct {
	EventKey string  `json:"event_key"`
	ReadOnly *bool   `json:"read_only"`
	Changes  []Delta `json:"changes"`
}

type Delta struct {
	Kind       string `json:"kind"`
	ItemNameID string `json:"itemNameID"`
	ItemCode   string `json:"itemCode"`
	ItemName   string `json:"itemName"`
	PriceField string `json:"priceField"`
	OldPrice   string `json:"oldPrice"`
	NewPrice   string `json:"newPrice"`
}

// Alert is an advisory only; it cannot modify or authorize a sale.
type Alert struct {
	Code       string `json:"code"`
	ItemNameID string `json:"itemNameID,omitempty"`
	ItemCode   string `json:"itemCode,omitempty"`
	PriceField string `json:"priceField,omitempty"`
	OldPrice   string `json:"oldPrice,omitempty"`
	NewPrice   string `json:"newPrice,omitempty"`
}

// Summary counts changes observed in event data, not all business activity.
// NewProducts and ProductsWithPriceChanges are distinct products in input window.
type Summary struct {
	ReadOnly                 bool    `json:"read_only"`
	Source                   string  `json:"source"`
	EventsSeen               int     `json:"events_seen"`
	EventsAnalyzed           int     `json:"events_analyzed"`
	DuplicateEventsSkipped   int     `json:"duplicate_events_skipped"`
	NewProducts              int     `json:"new_products"`
	ProductsWithPriceChanges int     `json:"products_with_price_changes"`
	RetailPriceChanges       int     `json:"retail_price_changes"`
	WholesalePriceChanges    int     `json:"wholesale_price_changes"`
	OtherPriceChanges        int     `json:"other_price_changes"`
	LargeRetailIncreases     int     `json:"large_retail_increases"`
	SuspiciousRetailPrices   int     `json:"suspicious_retail_prices"`
	UnparseablePrices        int     `json:"unparseable_prices"`
	LatestEventUTC           string  `json:"latest_event_utc,omitempty"`
	Alerts                   []Alert `json:"alerts"`
}

// AnalyzeJSON accepts a JSON array of read-only event snapshots.
// It rejects untrusted oversized inputs and non-read-only payloads.
func AnalyzeJSON(data []byte) (Summary, error) {
	out := Summary{ReadOnly: true, Source: "pc_os_events.itemname_delta", Alerts: []Alert{}}
	if len(data) > maxInputBytes {
		return out, errors.New("input size limit exceeded")
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return out, fmt.Errorf("decode events: %w", err)
	}
	if len(events) > maxEvents {
		return out, errors.New("event count limit exceeded")
	}
	seen := make(map[string]bool)
	newProducts := make(map[string]bool)
	changedProducts := make(map[string]bool)
	out.EventsSeen = len(events)
	for _, e := range events {
		if e.EventType != "itemname_delta" {
			continue
		}
		var p deltaPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return out, fmt.Errorf("decode event payload: %w", err)
		}
		if p.ReadOnly == nil || !*p.ReadOnly {
			return out, errors.New("event missing read_only=true attestation")
		}
		if len(p.Changes) > maxChangesPerEvent {
			return out, errors.New("event change count limit exceeded")
		}
		key := strings.TrimSpace(p.EventKey)
		if key == "" {
			key = strings.TrimSpace(e.ID)
		}
		if key == "" {
			return out, errors.New("itemname_delta event has no deduplication key")
		}
		if seen[key] {
			out.DuplicateEventsSkipped++
			continue
		}
		seen[key] = true
		out.EventsAnalyzed++
		if e.CreatedAt > out.LatestEventUTC {
			out.LatestEventUTC = e.CreatedAt
		}
		for _, c := range p.Changes {
			itemID := strings.TrimSpace(c.ItemNameID)
			if itemID == "" {
				itemID = "barcode:" + strings.TrimSpace(c.ItemCode)
			}
			if itemID == "barcode:" {
				continue
			}
			switch c.Kind {
			case "added":
				newProducts[itemID] = true
			case "price_changed":
				changedProducts[itemID] = true
				switch c.PriceField {
				case "SalePrice":
					out.RetailPriceChanges++
				case "WholeSalePrice":
					out.WholesalePriceChanges++
				default:
					out.OtherPriceChanges++
				}
				oldPrice, errOld := strconv.ParseInt(strings.TrimSpace(c.OldPrice), 10, 64)
				newPrice, errNew := strconv.ParseInt(strings.TrimSpace(c.NewPrice), 10, 64)
				if errOld != nil || errNew != nil {
					out.UnparseablePrices++
					out.appendAlert(Alert{Code: "invalid_price_format", ItemNameID: c.ItemNameID, ItemCode: c.ItemCode, PriceField: c.PriceField})
					continue
				}
				if c.PriceField == "SalePrice" {
					if newPrice <= 0 {
						out.SuspiciousRetailPrices++
						out.appendAlert(Alert{Code: "nonpositive_retail_price", ItemNameID: c.ItemNameID, ItemCode: c.ItemCode, PriceField: c.PriceField, OldPrice: c.OldPrice, NewPrice: c.NewPrice})
					}
					if oldPrice > 0 && newPrice > oldPrice &&
						float64(newPrice)/float64(oldPrice) >= 1+float64(largeRetailIncreasePercent)/100 {
						out.LargeRetailIncreases++
						out.appendAlert(Alert{Code: "large_retail_price_increase", ItemNameID: c.ItemNameID, ItemCode: c.ItemCode, PriceField: c.PriceField, OldPrice: c.OldPrice, NewPrice: c.NewPrice})
					}
				}
			}
		}
	}
	out.NewProducts = len(newProducts)
	out.ProductsWithPriceChanges = len(changedProducts)
	return out, nil
}

func (s *Summary) appendAlert(a Alert) {
	if len(s.Alerts) < maxAlerts {
		s.Alerts = append(s.Alerts, a)
	}
}
