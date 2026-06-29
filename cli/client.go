package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/canopy-network/go-plugin/contract"
	"github.com/canopy-network/go-plugin/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

/*
client.go — the canonical Veritas "build → BLS12-381 sign → submit over RPC" flow.

This is the reference the Phase 6 TypeScript frontend mirrors. The signing bytes are produced by the
SAME crypto.GetSignBytes the node uses (imported via the plugin module), so a tx signed here is
byte-for-byte acceptable to the chain. The CLI is off-chain tooling, so non-determinism (time, etc.)
is fine here — determinism only matters inside the plugin's state transitions.
*/

// typeURLs maps a tx type name to its protobuf Any type URL (must match the plugin's ContractConfig).
var typeURLs = map[string]string{
	"send":         "type.googleapis.com/types.MessageSend",
	"submit_claim": "type.googleapis.com/types.MessageSubmitClaim",
	"submit_note":  "type.googleapis.com/types.MessageSubmitNote",
	"rate_note":    "type.googleapis.com/types.MessageRateNote",
}

// Client submits transactions and reads events against a local Canopy node.
type Client struct {
	RPC      string // query/tx RPC base URL (default :50002)
	AdminRPC string // admin RPC base URL (default :50003, keystore)
	NetID    uint64
	ChainID  uint64
	Fee      uint64
}

// Height returns the current block height (used as the tx createdHeight + deterministic-id input).
func (c *Client) Height() (uint64, error) {
	rb, err := postJSON(c.RPC+"/v1/query/height", "{}")
	if err != nil {
		return 0, err
	}
	var r struct {
		Height uint64 `json:"height"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return 0, fmt.Errorf("parse height %q: %w", string(rb), err)
	}
	return r.Height, nil
}

// BuildAndSubmit runs the full signing flow and returns the tx hash and the height it was signed at.
//
// Steps the frontend mirrors:
//  1. height = current chain height (also feeds DeriveClaimID/DeriveNoteID).
//  2. any    = Any{typeURL, proto.Marshal(msg)}.
//  3. signBytes = crypto.GetSignBytes(msgType, any, time, height, fee, "", net, chain).
//  4. sig    = BLS12-381 sign(signBytes) with the account's private key.
//  5. tx JSON: the core "send" type uses the "msg" object; plugin-only types use
//     "msgTypeUrl" + "msgBytes"(hex) for exact byte control.
//  6. POST /v1/tx.
func (c *Client) BuildAndSubmit(msgType string, msg proto.Message, priv *crypto.BLS12381PrivateKey) (txHash string, height uint64, err error) {
	if _, ok := typeURLs[msgType]; !ok {
		return "", 0, fmt.Errorf("unknown msg type %q", msgType)
	}
	height, err = c.Height()
	if err != nil {
		return "", 0, err
	}
	tx, err := c.buildTx(msgType, msg, priv, height, uint64(time.Now().UnixMicro()))
	if err != nil {
		return "", 0, err
	}
	body, _ := json.Marshal(tx)
	rb, err := postJSON(c.RPC+"/v1/tx", string(body))
	if err != nil {
		return "", height, err
	}
	// the tx RPC returns the tx hash as a JSON string
	_ = json.Unmarshal(rb, &txHash)
	if txHash == "" {
		txHash = strings.TrimSpace(string(rb))
	}
	return txHash, height, nil
}

// buildTx assembles the signed transaction object (pure: no network) — the core of the signing flow.
func (c *Client) buildTx(msgType string, msg proto.Message, priv *crypto.BLS12381PrivateKey, height, txTime uint64) (map[string]interface{}, error) {
	if _, ok := typeURLs[msgType]; !ok {
		return nil, fmt.Errorf("unknown msg type %q", msgType)
	}
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	any := &anypb.Any{TypeUrl: typeURLs[msgType], Value: msgBytes}
	signBytes, err := crypto.GetSignBytes(msgType, any, txTime, height, c.Fee, "", c.NetID, c.ChainID)
	if err != nil {
		return nil, err
	}
	sig := priv.Sign(signBytes)
	tx := map[string]interface{}{
		"type": msgType,
		"signature": map[string]string{
			"publicKey": hex.EncodeToString(priv.PublicKey().Bytes()),
			"signature": hex.EncodeToString(sig),
		},
		"time":          txTime,
		"createdHeight": height,
		"fee":           c.Fee,
		"memo":          "",
		"networkID":     c.NetID,
		"chainID":       c.ChainID,
	}
	if msgType == "send" {
		// core-registered type: submit via the JSON "msg" object (base64 bytes per protojson)
		s := msg.(*contract.MessageSend)
		tx["msg"] = map[string]interface{}{
			"fromAddress": base64.StdEncoding.EncodeToString(s.FromAddress),
			"toAddress":   base64.StdEncoding.EncodeToString(s.ToAddress),
			"amount":      s.Amount,
		}
	} else {
		// plugin-only type: submit raw bytes for exact byte control
		tx["msgTypeUrl"] = typeURLs[msgType]
		tx["msgBytes"] = hex.EncodeToString(msgBytes)
	}
	return tx, nil
}

// LoadKey accepts a raw BLS12-381 private key hex or a path to a *_key.json file (a quoted hex string).
func LoadKey(hexOrPath string) (*crypto.BLS12381PrivateKey, error) {
	s := strings.TrimSpace(hexOrPath)
	if s == "" {
		return nil, fmt.Errorf("key is required (hex or path to *_key.json)")
	}
	if fi, err := os.Stat(s); err == nil && !fi.IsDir() {
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, err
		}
		s = strings.Trim(strings.TrimSpace(string(b)), "\"")
	}
	return crypto.StringToBLS12381PrivateKey(s)
}

// AddressOf returns the 20-byte account address for a key: sha256(publicKey)[:20].
func AddressOf(priv *crypto.BLS12381PrivateKey) []byte {
	sum := sha256.Sum256(priv.PublicKey().Bytes())
	return sum[:20]
}

// postJSON POSTs a JSON body and returns the response bytes (error on HTTP >= 400).
func postJSON(url, body string) ([]byte, error) {
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return rb, nil
}
