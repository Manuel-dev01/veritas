package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/canopy-network/go-plugin/contract"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

/*
query.go — the read path the frontend mirrors: fetch events over RPC and decode the custom payloads.

v0.1.18 has no plugin-state read RPC, so events ARE the read API. The node renders our custom Any
payload under the (cosmetically mislabeled) msg.orderId hex string; we decode it via the protobuf
registry (anypb.Any.UnmarshalNew) — the contract types register themselves on import, so no
hand-rolled wire-byte parsing is needed.

Output convention: byte fields (ids, addresses, hashes) are rendered as HEX — identical to what the
tx subcommands print — so an id you submit matches an id you read back. (protojson would render bytes
as base64 and omit zero-valued scalars, which is why we build the payload map by hand here.)
*/

// eventShortToType maps a friendly -event filter to the on-chain eventType string.
var eventShortToType = map[string]string{
	"claim":      "claim_created",
	"note":       "note_created",
	"rating":     "note_rated",
	"score":      "note_scored",
	"reputation": "reputation_changed",
}

type rawEvent struct {
	EventType string `json:"eventType"`
	Msg       struct {
		OrderID string `json:"orderId"`
	} `json:"msg"`
	Height    uint64 `json:"height"`
	Reference string `json:"reference"`
	Address   string `json:"address"`
}

type eventsPage struct {
	Results []rawEvent `json:"results"`
}

// EventsByAddress / EventsByHeight fetch a page of events from the query RPC.
func (c *Client) EventsByAddress(addr string) ([]rawEvent, error) {
	return c.fetchEvents("/v1/query/events-by-address", fmt.Sprintf(`{"address":%q,"perPage":300}`, addr))
}
func (c *Client) EventsByHeight(h uint64) ([]rawEvent, error) {
	return c.fetchEvents("/v1/query/events-by-height", fmt.Sprintf(`{"height":%d,"perPage":300}`, h))
}
func (c *Client) fetchEvents(path, body string) ([]rawEvent, error) {
	rb, err := postJSON(c.RPC+path, body)
	if err != nil {
		return nil, err
	}
	var page eventsPage
	if err := json.Unmarshal(rb, &page); err != nil {
		return nil, fmt.Errorf("parse events: %w", err)
	}
	return page.Results, nil
}

// decodeEventPayload decodes one custom event's Any payload into a concrete proto message, or nil if
// the event carries no custom payload (e.g. a core "reward" event).
func decodeEventPayload(e rawEvent) (proto.Message, error) {
	if e.Msg.OrderID == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(e.Msg.OrderID)
	if err != nil {
		return nil, err
	}
	any := &anypb.Any{}
	if err := proto.Unmarshal(b, any); err != nil {
		return nil, err
	}
	return any.UnmarshalNew() // resolves via the global proto registry (contract types register on import)
}

// eventPayloadMap converts a decoded Veritas event into an ordered-friendly map with HEX byte fields
// and enum names, including zero-valued scalars. Returns nil for an unrecognized message.
func eventPayloadMap(m proto.Message) map[string]interface{} {
	switch e := m.(type) {
	case *contract.ClaimCreatedEvent:
		return map[string]interface{}{
			"claimId": hex.EncodeToString(e.ClaimId), "submitter": hex.EncodeToString(e.Submitter),
			"contentHash": hex.EncodeToString(e.ContentHash), "url": e.Url, "createdHeight": e.CreatedHeight,
			"text": e.Text,
		}
	case *contract.NoteCreatedEvent:
		return map[string]interface{}{
			"noteId": hex.EncodeToString(e.NoteId), "claimId": hex.EncodeToString(e.ClaimId),
			"author": hex.EncodeToString(e.Author), "body": e.Body,
			"contentHash": hex.EncodeToString(e.ContentHash), "createdHeight": e.CreatedHeight,
			"status": e.Status.String(), "url": e.Url,
		}
	case *contract.NoteRatedEvent:
		return map[string]interface{}{
			"noteId": hex.EncodeToString(e.NoteId), "rater": hex.EncodeToString(e.Rater),
			"value": e.Value.String(), "createdHeight": e.CreatedHeight,
		}
	case *contract.NoteScoredEvent:
		return map[string]interface{}{
			"noteId": hex.EncodeToString(e.NoteId), "claimId": hex.EncodeToString(e.ClaimId),
			"status": e.Status.String(), "noteIntercept": e.NoteIntercept, "noteFactor": e.NoteFactor,
			"globalMu": e.GlobalMu, "numRaters": e.NumRaters,
			"bridgeScore": e.BridgeScore, "meanA": e.MeanA, "meanB": e.MeanB,
			"countA": e.CountA, "countB": e.CountB, "height": e.Height,
		}
	case *contract.ReputationChangedEvent:
		return map[string]interface{}{
			"account": hex.EncodeToString(e.Account), "scoreFP": e.ScoreFp,
			"delta": e.Delta, "reason": e.Reason, "height": e.Height,
		}
	default:
		return nil
	}
}

// renderEvents decodes + filters the events and writes them (human-readable, or a JSON array).
// short is "" or "all" for no filter, else one of eventShortToType.
func renderEvents(events []rawEvent, short string, asJSON bool) (string, error) {
	wantType := ""
	if short != "" && short != "all" {
		t, ok := eventShortToType[short]
		if !ok {
			return "", fmt.Errorf("unknown -event %q (claim|note|rating|score|reputation|all)", short)
		}
		wantType = t
	}

	type outItem struct {
		EventType string                 `json:"eventType"`
		Height    uint64                 `json:"height"`
		Reference string                 `json:"reference"`
		Address   string                 `json:"address"`
		Payload   map[string]interface{} `json:"payload"`
	}
	var items []outItem
	for _, e := range events {
		if wantType != "" && e.EventType != wantType {
			continue
		}
		msg, err := decodeEventPayload(e)
		if err != nil {
			return "", err
		}
		if msg == nil {
			continue // non-custom (core) event
		}
		payload := eventPayloadMap(msg)
		if payload == nil {
			continue
		}
		items = append(items, outItem{EventType: e.EventType, Height: e.Height, Reference: e.Reference, Address: e.Address, Payload: payload})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Height < items[j].Height }) // oldest-first

	if asJSON {
		b, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	out := ""
	for _, it := range items {
		out += fmt.Sprintf("h=%-6d %-20s %s  (ref=%s)\n", it.Height, it.EventType, summarize(it.EventType, it.Payload), it.Reference)
	}
	if out == "" {
		out = "(no matching custom events)\n"
	}
	return out, nil
}

// summarize produces a compact one-line view of a decoded payload for the human-readable mode.
func summarize(eventType string, m map[string]interface{}) string {
	get := func(k string) string {
		if v, ok := m[k]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	short := func(k string) string {
		s := get(k)
		if len(s) > 16 {
			return s[:16] + ".."
		}
		return s
	}
	switch eventType {
	case "claim_created":
		return fmt.Sprintf("claim=%s url=%q text=%q", short("claimId"), get("url"), get("text"))
	case "note_created":
		return fmt.Sprintf("note=%s claim=%s status=%s body=%q", short("noteId"), short("claimId"), get("status"), get("body"))
	case "note_rated":
		return fmt.Sprintf("note=%s rater=%s value=%s", short("noteId"), short("rater"), get("value"))
	case "note_scored":
		return fmt.Sprintf("note=%s status=%s intercept=%s factor=%s mu=%s raters=%s (latent A=%s B=%s)",
			short("noteId"), get("status"), get("noteIntercept"), get("noteFactor"), get("globalMu"), get("numRaters"), get("countA"), get("countB"))
	case "reputation_changed":
		return fmt.Sprintf("acct=%s scoreFP=%s delta=%s reason=%s", short("account"), get("scoreFP"), get("delta"), get("reason"))
	default:
		return fmt.Sprintf("%v", m)
	}
}
