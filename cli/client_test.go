package main

import (
	"strings"
	"testing"

	"github.com/canopy-network/go-plugin/contract"
	"github.com/canopy-network/go-plugin/crypto"
)

func testKey(t *testing.T) *crypto.BLS12381PrivateKey {
	t.Helper()
	priv, err := crypto.StringToBLS12381PrivateKey(strings.Repeat("01", 32))
	if err != nil {
		t.Fatalf("load test key: %v", err)
	}
	return priv
}

func TestAddressOf_20Bytes(t *testing.T) {
	if got := AddressOf(testKey(t)); len(got) != 20 {
		t.Fatalf("address must be 20 bytes, got %d", len(got))
	}
}

func TestBuildTx_PluginTypeUsesMsgBytes(t *testing.T) {
	priv := testKey(t)
	c := &Client{NetID: 1, ChainID: 1, Fee: 10000}
	msg := &contract.MessageSubmitClaim{Submitter: AddressOf(priv), ContentHash: []byte{0xab}, Text: "x", Nonce: 1}
	tx, err := c.buildTx("submit_claim", msg, priv, 100, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tx["msgTypeUrl"]; !ok {
		t.Fatal("plugin type must set msgTypeUrl")
	}
	if _, ok := tx["msgBytes"]; !ok {
		t.Fatal("plugin type must set msgBytes")
	}
	if _, ok := tx["msg"]; ok {
		t.Fatal("plugin type must NOT set msg")
	}
}

func TestBuildTx_SendUsesMsgObject(t *testing.T) {
	priv := testKey(t)
	c := &Client{NetID: 1, ChainID: 1, Fee: 10000}
	msg := &contract.MessageSend{FromAddress: AddressOf(priv), ToAddress: make([]byte, 20), Amount: 5}
	tx, err := c.buildTx("send", msg, priv, 100, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tx["msg"]; !ok {
		t.Fatal("send must set the msg object")
	}
	if _, ok := tx["msgBytes"]; ok {
		t.Fatal("send must NOT set msgBytes")
	}
}

func TestBuildTx_DeterministicSignature(t *testing.T) {
	priv := testKey(t)
	c := &Client{NetID: 1, ChainID: 1, Fee: 10000}
	msg := &contract.MessageRateNote{Rater: AddressOf(priv), NoteId: make([]byte, 32), Value: contract.RatingValue_RATING_HELPFUL, Nonce: 7}
	a, err := c.buildTx("rate_note", msg, priv, 200, 99)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.buildTx("rate_note", msg, priv, 200, 99)
	if err != nil {
		t.Fatal(err)
	}
	if a["signature"].(map[string]string)["signature"] != b["signature"].(map[string]string)["signature"] {
		t.Fatal("BLS signature over identical sign-bytes must be deterministic")
	}
	if a["msgBytes"] != b["msgBytes"] {
		t.Fatal("msgBytes must be deterministic")
	}
}

func TestBuildTx_UnknownType(t *testing.T) {
	c := &Client{NetID: 1, ChainID: 1, Fee: 10000}
	if _, err := c.buildTx("nope", &contract.MessageSend{}, testKey(t), 1, 1); err == nil {
		t.Fatal("expected error for unknown msg type")
	}
}
