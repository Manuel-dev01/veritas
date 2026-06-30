// veritas is the Veritas command-line client: it BLS12-381-signs and submits all four custom
// transaction types to a local Canopy node, and queries+decodes the on-chain event lifecycle.
// It is the reference implementation the Phase 6 TypeScript frontend mirrors (see client.go/query.go).
//
//	veritas <command> [flags]
//	  send   -key K -to HEX20 -amount N
//	  claim  -key K -text "..." [-url U] [-content-hash HEX] [-nonce N]
//	  note   -key K -claim-id HEX -body "..." [-content-hash HEX] [-nonce N]
//	  rate   -key K -note-id HEX -rating helpful|somewhat|not [-nonce N]
//	  query  (-address HEX20 | -height H) [-event claim|note|rating|score|reputation|all] [-json]
//	  keys   new -name NICK | get -address HEX20   [-password PW]   (dev keystore convenience)
//
// Global flags (on every subcommand): -rpc -admin-rpc -net -chain -fee -json -key.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/canopy-network/go-plugin/contract"
	"github.com/canopy-network/go-plugin/crypto"
	"google.golang.org/protobuf/proto"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "send":
		cmdSend(args)
	case "claim":
		cmdClaim(args)
	case "note":
		cmdNote(args)
	case "rate":
		cmdRate(args)
	case "query":
		cmdQuery(args)
	case "keys":
		cmdKeys(args)
	case "serve":
		cmdServe(args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `veritas — Veritas BLS12-381 CLI

commands:
  send   -key K -to HEX20 -amount N
  claim  -key K -text "..." [-url U] [-content-hash HEX] [-nonce N]
  note   -key K -claim-id HEX -body "..." [-content-hash HEX] [-nonce N]
  rate   -key K -note-id HEX -rating helpful|somewhat|not [-nonce N]
  query  (-address HEX20 | -height H) [-event claim|note|rating|score|reputation|all] [-json]
  keys   new -name NICK | get -address HEX20 [-password PW]
  serve  -port 8080 [-validator-key PATH]   (gateway: indexer + signer + CORS for the frontend)

global flags: -rpc -admin-rpc -net -chain -fee -json -key
`)
}

// common holds the shared flags registered on every subcommand.
type common struct {
	rpc, admin, key   *string
	net, chain, fee   *uint64
	asJSON            *bool
}

func addCommon(fs *flag.FlagSet) *common {
	return &common{
		rpc:    fs.String("rpc", "http://localhost:50002", "query/tx RPC base URL"),
		admin:  fs.String("admin-rpc", "http://localhost:50003", "admin RPC base URL (keystore)"),
		key:    fs.String("key", "", "BLS private key hex, or path to a *_key.json file"),
		net:    fs.Uint64("net", 1, "network id"),
		chain:  fs.Uint64("chain", 1, "chain id"),
		fee:    fs.Uint64("fee", 10000, "fee in uCNPY"),
		asJSON: fs.Bool("json", false, "emit JSON output"),
	}
}

func (cm *common) client() *Client {
	return &Client{RPC: *cm.rpc, AdminRPC: *cm.admin, NetID: *cm.net, ChainID: *cm.chain, Fee: *cm.fee}
}

// mustKeyCrypto loads the signing key and returns it with its 20-byte address; fatal on error.
func mustKeyCrypto(cm *common) (*crypto.BLS12381PrivateKey, []byte) {
	p, err := LoadKey(*cm.key)
	if err != nil {
		fatal(err)
	}
	return p, AddressOf(p)
}

// ---- tx subcommands ----

func cmdSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	cm := addCommon(fs)
	to := fs.String("to", "", "recipient address (20-byte hex)")
	amount := fs.Uint64("amount", 100000, "amount in uCNPY")
	_ = fs.Parse(args)
	priv, from := mustKeyCrypto(cm)
	toB := mustHex20(*to, "-to")
	msg := &contract.MessageSend{FromAddress: from, ToAddress: toB, Amount: *amount}
	txHash, height := submit(cm, priv, "send", msg)
	emit(cm, map[string]interface{}{"type": "send", "from": hexs(from), "to": hexs(toB), "amount": *amount, "txHash": txHash, "height": height})
}

func cmdClaim(args []string) {
	fs := flag.NewFlagSet("claim", flag.ExitOnError)
	cm := addCommon(fs)
	text := fs.String("text", "", "short claim text")
	url := fs.String("url", "", "optional url")
	ch := fs.String("content-hash", "", "optional content hash (hex)")
	nonce := fs.Uint64("nonce", 1, "nonce for deterministic id")
	_ = fs.Parse(args)
	priv, from := mustKeyCrypto(cm)
	msg := &contract.MessageSubmitClaim{Submitter: from, ContentHash: decodeHexFlag(*ch, "-content-hash", false), Url: *url, Text: *text, Nonce: *nonce}
	txHash, height := submit(cm, priv, "submit_claim", msg)
	claimID := contract.DeriveClaimID(from, *nonce, height)
	emit(cm, map[string]interface{}{"type": "submit_claim", "from": hexs(from), "claimId": hexs(claimID), "txHash": txHash, "height": height})
}

func cmdNote(args []string) {
	fs := flag.NewFlagSet("note", flag.ExitOnError)
	cm := addCommon(fs)
	claimID := fs.String("claim-id", "", "claim id (hex) the note attaches to")
	body := fs.String("body", "", "note body text")
	ch := fs.String("content-hash", "", "optional content hash (hex)")
	url := fs.String("url", "", "optional source url")
	nonce := fs.Uint64("nonce", 1, "nonce for deterministic id")
	_ = fs.Parse(args)
	priv, from := mustKeyCrypto(cm)
	claimB := decodeHexFlag(*claimID, "-claim-id", true)
	msg := &contract.MessageSubmitNote{Author: from, ClaimId: claimB, Body: *body, ContentHash: decodeHexFlag(*ch, "-content-hash", false), Url: *url, Nonce: *nonce}
	txHash, height := submit(cm, priv, "submit_note", msg)
	noteID := contract.DeriveNoteID(from, claimB, *nonce, height)
	emit(cm, map[string]interface{}{"type": "submit_note", "from": hexs(from), "claimId": hexs(claimB), "noteId": hexs(noteID), "txHash": txHash, "height": height})
}

func cmdRate(args []string) {
	fs := flag.NewFlagSet("rate", flag.ExitOnError)
	cm := addCommon(fs)
	noteID := fs.String("note-id", "", "note id (hex) being rated")
	rating := fs.String("rating", "helpful", "helpful | somewhat | not")
	nonce := fs.Uint64("nonce", 1, "nonce")
	_ = fs.Parse(args)
	priv, from := mustKeyCrypto(cm)
	noteB := decodeHexFlag(*noteID, "-note-id", true)
	val, ok := parseRating(*rating)
	if !ok {
		fatal(fmt.Errorf("-rating must be helpful|somewhat|not, got %q", *rating))
	}
	msg := &contract.MessageRateNote{Rater: from, NoteId: noteB, Value: val, Nonce: *nonce}
	txHash, height := submit(cm, priv, "rate_note", msg)
	emit(cm, map[string]interface{}{"type": "rate_note", "from": hexs(from), "noteId": hexs(noteB), "rating": *rating, "txHash": txHash, "height": height})
}

// ---- query subcommand ----

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	cm := addCommon(fs)
	addr := fs.String("address", "", "filter events by account address (20-byte hex)")
	height := fs.Uint64("height", 0, "fetch events at this block height")
	event := fs.String("event", "all", "claim|note|rating|score|reputation|all")
	_ = fs.Parse(args)
	cl := cm.client()
	var (
		events []rawEvent
		err    error
	)
	switch {
	case *addr != "":
		events, err = cl.EventsByAddress(*addr)
	case *height > 0:
		events, err = cl.EventsByHeight(*height)
	default:
		fatal(fmt.Errorf("query requires -address HEX20 or -height H"))
	}
	if err != nil {
		fatal(err)
	}
	out, err := renderEvents(events, *event, *cm.asJSON)
	if err != nil {
		fatal(err)
	}
	fmt.Print(out)
}

// ---- keys subcommand (dev keystore convenience) ----

func cmdKeys(args []string) {
	if len(args) < 1 {
		fatal(fmt.Errorf("keys requires a subcommand: new | get"))
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("keys "+sub, flag.ExitOnError)
	cm := addCommon(fs)
	name := fs.String("name", "", "new: keystore nickname")
	addr := fs.String("address", "", "get: account address (hex)")
	pw := fs.String("password", "testpassword123", "keystore password")
	_ = fs.Parse(rest)
	cl := cm.client()
	switch sub {
	case "new":
		a, err := cl.KeystoreNew(*name, *pw)
		if err != nil {
			fatal(err)
		}
		emit(cm, map[string]interface{}{"address": a})
	case "get":
		kg, err := cl.KeystoreGet(*addr, *pw)
		if err != nil {
			fatal(err)
		}
		emit(cm, map[string]interface{}{"address": kg.Address, "publicKey": kg.PublicKey, "privateKey": kg.PrivateKey})
	default:
		fatal(fmt.Errorf("unknown keys subcommand %q (new|get)", sub))
	}
}

// ---- helpers ----

// submit signs+submits and returns (txHash, signedHeight); fatal on error.
func submit(cm *common, priv *crypto.BLS12381PrivateKey, msgType string, msg proto.Message) (string, uint64) {
	txHash, height, err := cm.client().BuildAndSubmit(msgType, msg, priv)
	if err != nil {
		fatal(err)
	}
	return txHash, height
}

// emit prints a result map as JSON (-json) or as aligned key=value lines.
func emit(cm *common, m map[string]interface{}) {
	if *cm.asJSON {
		b, _ := json.MarshalIndent(m, "", "  ")
		fmt.Println(string(b))
		return
	}
	// stable, readable order
	order := []string{"type", "from", "to", "amount", "claimId", "noteId", "rating", "address", "publicKey", "privateKey", "height", "txHash"}
	seen := map[string]bool{}
	for _, k := range order {
		if v, ok := m[k]; ok {
			fmt.Printf("%-12s %v\n", k+":", v)
			seen[k] = true
		}
	}
	for k, v := range m {
		if !seen[k] {
			fmt.Printf("%-12s %v\n", k+":", v)
		}
	}
}

func hexs(b []byte) string { return hex.EncodeToString(b) }

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// parseRating maps a CLI rating word to the RatingValue enum.
func parseRating(s string) (contract.RatingValue, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "helpful":
		return contract.RatingValue_RATING_HELPFUL, true
	case "somewhat":
		return contract.RatingValue_RATING_SOMEWHAT, true
	case "not", "not_helpful", "nothelpful":
		return contract.RatingValue_RATING_NOT_HELPFUL, true
	default:
		return contract.RatingValue_RATING_UNSPECIFIED, false
	}
}

func decodeHexFlag(s, name string, required bool) []byte {
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			fatal(fmt.Errorf("%s is required (hex)", name))
		}
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		fatal(fmt.Errorf("bad %s hex: %w", name, err))
	}
	return b
}

func mustHex20(s, name string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != 20 {
		fatal(fmt.Errorf("%s must be 20-byte hex", name))
	}
	return b
}
