package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/canopy-network/go-plugin/contract"
	"github.com/canopy-network/go-plugin/crypto"
)

/*
serve.go — the Veritas gateway (`veritas serve`). The browser cannot sign for this chain (BDN/kyber
BLS) nor read it directly (no CORS, no state-read RPC), so this gateway is the bridge: an in-memory
EVENT INDEXER (reconstructs the board from decoded events) + a SIGNER/SUBMITTER (BLS-signs txs with
seeded demo identities and broadcasts) + permissive CORS. It reuses the proven Client.BuildAndSubmit
and the event decoder; it is off-chain tooling, so no determinism constraints apply here.
*/

// ---- in-memory index model ----

type scoreRec struct {
	Intercept int64  `json:"intercept"`
	Factor    int64  `json:"factor"`
	Mu        int64  `json:"mu"`
	MeanA     int64  `json:"meanA"`
	MeanB     int64  `json:"meanB"`
	CountA    int    `json:"countA"`
	CountB    int    `json:"countB"`
	NumRaters int    `json:"numRaters"`
	Status    string `json:"status"`
	Height    uint64 `json:"height"`
}

type noteRec struct {
	ID      string    `json:"id"`
	ClaimID string    `json:"claimId"`
	Author  string    `json:"author"`
	Body    string    `json:"body"`
	URL     string    `json:"url"`
	Status  string    `json:"status"`
	Height  uint64    `json:"height"`
	Score   *scoreRec `json:"score"`
}

type claimRec struct {
	ID        string     `json:"id"`
	Submitter string     `json:"submitter"`
	URL       string     `json:"url"`
	Text      string     `json:"text"`
	Height    uint64     `json:"height"`
	Notes     []*noteRec `json:"notes"`
	noteSet   map[string]bool
}

type logLine struct {
	Height uint64 `json:"height"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
}

type ident struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Camp    string `json:"camp"`
	Address string `json:"address"`
	priv    *crypto.BLS12381PrivateKey
}

type gateway struct {
	cli     *Client
	valPriv *crypto.BLS12381PrivateKey
	valAddr []byte

	idents    []*ident
	identByID map[string]*ident

	mu          sync.Mutex
	height      uint64
	lastScanned uint64
	claims      map[string]*claimRec
	claimOrder  []string
	notes       map[string]*noteRec
	reps        map[string]int64
	repLedger   map[string][]logLine
	log         []logLine
	logSeen     map[string]bool
	nonce       uint64
}

func newGateway(cli *Client, valKeyPath string) *gateway {
	priv, err := LoadKey(valKeyPath)
	if err != nil {
		log.Fatalf("load validator key %q: %v", valKeyPath, err)
	}
	return &gateway{
		cli: cli, valPriv: priv, valAddr: AddressOf(priv),
		identByID: map[string]*ident{},
		claims:    map[string]*claimRec{}, notes: map[string]*noteRec{},
		reps: map[string]int64{}, repLedger: map[string][]logLine{},
		logSeen: map[string]bool{}, nonce: 1_000_000,
	}
}

// seedIdentities derives deterministic demo keys (restart-stable) and funds them from the validator.
func (g *gateway) seedIdentities() {
	defs := []struct{ id, name, camp string }{
		{"you", "you", "neutral"},
		{"a1", "rater_a1", "A"}, {"a2", "rater_a2", "A"}, {"a3", "rater_a3", "A"},
		{"b1", "rater_b1", "B"}, {"b2", "rater_b2", "B"}, {"b3", "rater_b3", "B"},
	}
	for _, d := range defs {
		seed := sha256.Sum256([]byte("veritas/demo/" + d.id))
		seed[0] = 0 // clear the top byte so the scalar is always < the BLS12-381 group order r
		priv, err := crypto.BytesToBLS12381PrivateKey(seed[:])
		if err != nil {
			log.Fatalf("derive demo key %s: %v", d.id, err)
		}
		addr := AddressOf(priv)
		it := &ident{ID: d.id, Name: d.name, Camp: d.camp, Address: hex.EncodeToString(addr), priv: priv}
		g.idents = append(g.idents, it)
		g.identByID[d.id] = it
		// fund from the validator (idempotent enough for a demo; balances just accumulate)
		send := &contract.MessageSend{FromAddress: g.valAddr, ToAddress: addr, Amount: 2_000_000}
		if _, _, err := g.cli.BuildAndSubmit("send", send, g.valPriv); err != nil {
			log.Printf("warn: funding %s failed: %v", d.name, err)
		}
	}
	log.Printf("seeded %d demo identities (funding lands next block)", len(g.idents))
}

func (g *gateway) nextNonce() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nonce++
	return g.nonce
}

// ---- indexer ----

func (g *gateway) indexerLoop() {
	for {
		h, err := g.cli.Height()
		if err == nil && h > 0 {
			from := uint64(1)
			g.mu.Lock()
			g.height = h
			if g.lastScanned > 5 {
				from = g.lastScanned - 4 // re-scan a trailing window (idempotent) to catch late inclusions
			}
			g.mu.Unlock()
			for hh := from; hh <= h; hh++ {
				events, e := g.cli.EventsByHeight(hh)
				if e != nil {
					break
				}
				g.fold(hh, events)
			}
			g.mu.Lock()
			g.lastScanned = h
			g.mu.Unlock()
		}
		time.Sleep(2 * time.Second)
	}
}

func (g *gateway) fold(height uint64, events []rawEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, e := range events {
		msg, err := decodeEventPayload(e)
		if err != nil || msg == nil {
			continue
		}
		switch x := msg.(type) {
		case *contract.ClaimCreatedEvent:
			id := hex.EncodeToString(x.ClaimId)
			if _, ok := g.claims[id]; !ok {
				g.claims[id] = &claimRec{ID: id, Submitter: hex.EncodeToString(x.Submitter), URL: x.Url, Text: x.Text, Height: x.CreatedHeight, Notes: []*noteRec{}, noteSet: map[string]bool{}}
				g.claimOrder = append(g.claimOrder, id)
			}
		case *contract.NoteCreatedEvent:
			id := hex.EncodeToString(x.NoteId)
			cid := hex.EncodeToString(x.ClaimId)
			if _, ok := g.notes[id]; !ok {
				g.notes[id] = &noteRec{ID: id, ClaimID: cid, Author: hex.EncodeToString(x.Author), Body: x.Body, URL: x.Url, Status: x.Status.String(), Height: x.CreatedHeight}
			}
			if c := g.claims[cid]; c != nil && !c.noteSet[id] {
				c.noteSet[id] = true
				c.Notes = append(c.Notes, g.notes[id])
			}
		case *contract.NoteScoredEvent:
			if n := g.notes[hex.EncodeToString(x.NoteId)]; n != nil {
				n.Status = x.Status.String()
				n.Score = &scoreRec{Intercept: x.NoteIntercept, Factor: x.NoteFactor, Mu: x.GlobalMu, MeanA: x.MeanA, MeanB: x.MeanB, CountA: int(x.CountA), CountB: int(x.CountB), NumRaters: int(x.NumRaters), Status: x.Status.String(), Height: x.Height}
			}
			g.addLog(e.Reference, height, "score", fmt.Sprintf("%s  intercept %s  → %s", short12(hex.EncodeToString(x.NoteId)), fp2(x.NoteIntercept), x.Status.String()))
		case *contract.ReputationChangedEvent:
			acct := hex.EncodeToString(x.Account)
			g.reps[acct] = x.ScoreFp
			ll := logLine{Height: x.Height, Kind: "rep", Text: fmt.Sprintf("%s  %s  %s", short12(acct), signFp(x.Delta), x.Reason)}
			g.repLedger[acct] = appendCapped(g.repLedger[acct], ll, 30)
			g.addLog(e.Reference+"rep", height, "rep", ll.Text)
		}
	}
}

func (g *gateway) addLog(ref string, height uint64, kind, text string) {
	key := fmt.Sprintf("%d/%s/%s", height, kind, ref)
	if g.logSeen[key] {
		return
	}
	g.logSeen[key] = true
	g.log = append(g.log, logLine{Height: height, Kind: kind, Text: text})
	sort.SliceStable(g.log, func(i, j int) bool { return g.log[i].Height < g.log[j].Height })
	if len(g.log) > 60 {
		g.log = g.log[len(g.log)-60:]
	}
}

// ---- HTTP ----

func (g *gateway) cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// "*" (valid with a wildcard origin / no credentials) so the browser preflight also accepts
		// custom headers like ngrok-skip-browser-warning, which the frontend sends to bypass the
		// ngrok-free interstitial.
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (g *gateway) handleState(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	claims := make([]*claimRec, 0, len(g.claimOrder))
	// newest first (board reads top-down)
	for i := len(g.claimOrder) - 1; i >= 0; i-- {
		claims = append(claims, g.claims[g.claimOrder[i]])
	}
	logCopy := append([]logLine(nil), g.log...)
	writeJSON(w, map[string]interface{}{"height": g.height, "claims": claims, "log": logCopy})
}

// handleRoot answers "/" with a friendly status page so visiting the tunnel URL confirms the gateway is
// live (instead of a bare 404). This gateway only serves /api/*; anything else stays a 404.
func (g *gateway) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	g.mu.Lock()
	h := g.height
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]interface{}{
		"service":   "veritas-gateway",
		"status":    "ok",
		"height":    h,
		"app":       "https://frontend-kappa-flax-87.vercel.app/app",
		"endpoints": []string{"/api/state", "/api/identities", "/api/account", "/api/claim", "/api/note", "/api/rate", "/api/seed"},
		"note":      "This is the Veritas API gateway, not the web app. Open the app URL above.",
	})
}

func (g *gateway) handleIdentities(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]map[string]interface{}, 0, len(g.idents))
	for _, it := range g.idents {
		out = append(out, map[string]interface{}{"id": it.ID, "name": it.Name, "camp": it.Camp, "address": it.Address, "reputation": g.reps[it.Address]})
	}
	writeJSON(w, out)
}

func (g *gateway) handleAccount(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	g.mu.Lock()
	defer g.mu.Unlock()
	camp := "neutral"
	for _, it := range g.idents {
		if it.Address == addr {
			camp = it.Camp
		}
	}
	writeJSON(w, map[string]interface{}{"address": addr, "scoreFp": g.reps[addr], "camp": camp, "ledger": g.repLedger[addr]})
}

// resolveIdent maps a request "identity" id to its key+address.
func (g *gateway) resolveIdent(id string) (*crypto.BLS12381PrivateKey, []byte, error) {
	it := g.identByID[id]
	if it == nil {
		return nil, nil, fmt.Errorf("unknown identity %q", id)
	}
	addr, _ := hex.DecodeString(it.Address)
	return it.priv, addr, nil
}

func (g *gateway) handleClaim(w http.ResponseWriter, r *http.Request) {
	var req struct{ Identity, Text, URL string }
	if !decodeBody(w, r, &req) {
		return
	}
	priv, addr, err := g.resolveIdent(req.Identity)
	if err != nil {
		httpErr(w, err)
		return
	}
	ch := sha256.Sum256([]byte(req.Text))
	nonce := g.nextNonce()
	msg := &contract.MessageSubmitClaim{Submitter: addr, ContentHash: ch[:8], Url: req.URL, Text: req.Text, Nonce: nonce}
	txHash, height, err := g.cli.BuildAndSubmit("submit_claim", msg, priv)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"claimId": hex.EncodeToString(contract.DeriveClaimID(addr, nonce, height)), "txHash": txHash})
}

func (g *gateway) handleNote(w http.ResponseWriter, r *http.Request) {
	var req struct{ Identity, ClaimId, Body, URL string }
	if !decodeBody(w, r, &req) {
		return
	}
	priv, addr, err := g.resolveIdent(req.Identity)
	if err != nil {
		httpErr(w, err)
		return
	}
	claimB, err := hex.DecodeString(req.ClaimId)
	if err != nil {
		httpErr(w, fmt.Errorf("bad claimId"))
		return
	}
	ch := sha256.Sum256([]byte(req.Body))
	nonce := g.nextNonce()
	msg := &contract.MessageSubmitNote{Author: addr, ClaimId: claimB, Body: req.Body, ContentHash: ch[:8], Url: req.URL, Nonce: nonce}
	txHash, height, err := g.cli.BuildAndSubmit("submit_note", msg, priv)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"noteId": hex.EncodeToString(contract.DeriveNoteID(addr, claimB, nonce, height)), "txHash": txHash})
}

func (g *gateway) handleRate(w http.ResponseWriter, r *http.Request) {
	var req struct{ Identity, NoteId, Value string }
	if !decodeBody(w, r, &req) {
		return
	}
	priv, addr, err := g.resolveIdent(req.Identity)
	if err != nil {
		httpErr(w, err)
		return
	}
	noteB, err := hex.DecodeString(req.NoteId)
	if err != nil {
		httpErr(w, fmt.Errorf("bad noteId"))
		return
	}
	val, ok := parseRating(req.Value)
	if !ok {
		httpErr(w, fmt.Errorf("bad rating value"))
		return
	}
	msg := &contract.MessageRateNote{Rater: addr, NoteId: noteB, Value: val, Nonce: g.nextNonce()}
	txHash, _, err := g.cli.BuildAndSubmit("rate_note", msg, priv)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"txHash": txHash})
}

// ---- seed scenario (one-click polarized demo) ----

func (g *gateway) submitClaimAs(id, text, url string) ([]byte, error) {
	priv, addr, err := g.resolveIdent(id)
	if err != nil {
		return nil, err
	}
	nonce := g.nextNonce()
	ch := sha256.Sum256([]byte(text))
	_, h, err := g.cli.BuildAndSubmit("submit_claim", &contract.MessageSubmitClaim{Submitter: addr, ContentHash: ch[:8], Url: url, Text: text, Nonce: nonce}, priv)
	if err != nil {
		return nil, err
	}
	return contract.DeriveClaimID(addr, nonce, h), nil
}

func (g *gateway) submitNoteAs(id string, claimID []byte, body, url string) ([]byte, error) {
	priv, addr, err := g.resolveIdent(id)
	if err != nil {
		return nil, err
	}
	nonce := g.nextNonce()
	ch := sha256.Sum256([]byte(body))
	_, h, err := g.cli.BuildAndSubmit("submit_note", &contract.MessageSubmitNote{Author: addr, ClaimId: claimID, Body: body, ContentHash: ch[:8], Url: url, Nonce: nonce}, priv)
	if err != nil {
		return nil, err
	}
	return contract.DeriveNoteID(addr, claimID, nonce, h), nil
}

func (g *gateway) rateAs(id string, noteID []byte, v contract.RatingValue) {
	priv, addr, err := g.resolveIdent(id)
	if err != nil {
		return
	}
	if _, _, e := g.cli.BuildAndSubmit("rate_note", &contract.MessageRateNote{Rater: addr, NoteId: noteID, Value: v, Nonce: g.nextNonce()}, priv); e != nil {
		log.Printf("seed rate %s failed: %v", id, e)
	}
}

// handleSeed establishes a polarized cohort history (camp A vs camp B disagree on seed notes) and
// posts a fresh target note pre-rated by camp A only — so a single camp-B rating bridges it to
// HELPFUL. Synchronous (~20 txs); they land over the next blocks and the MF picks up polarity.
func (g *gateway) handleSeed(w http.ResponseWriter, _ *http.Request) {
	H, X := contract.RatingValue_RATING_HELPFUL, contract.RatingValue_RATING_NOT_HELPFUL
	A := []string{"a1", "a2", "a3"}
	B := []string{"b1", "b2", "b3"}
	rateAll := func(ids []string, n []byte, v contract.RatingValue) {
		for _, id := range ids {
			g.rateAs(id, n, v)
		}
	}
	cs, err := g.submitClaimAs("you", "Seed claim — macro chart with a shifted baseline", "https://example.org/seed")
	if err != nil {
		httpErr(w, fmt.Errorf("seed claim: %w (are identities funded yet?)", err))
		return
	}
	s1, err1 := g.submitNoteAs("you", cs, "Seed note A — establishes the polarity axis.", "https://example.org/s1")
	s2, err2 := g.submitNoteAs("you", cs, "Seed note B — establishes the polarity axis.", "https://example.org/s2")
	if err1 != nil || err2 != nil {
		httpErr(w, fmt.Errorf("seed notes failed"))
		return
	}
	// polarize: camp A and camp B disagree, in both directions
	rateAll(A, s1, H)
	rateAll(B, s1, X)
	rateAll(A, s2, X)
	rateAll(B, s2, H)
	// target: a real-looking claim + note, pre-rated HELPFUL by camp A only (mid-bridge)
	ct, err := g.submitClaimAs("you", "This chart proves unemployment tripled under the new policy.", "https://bls.gov/data")
	if err != nil {
		httpErr(w, fmt.Errorf("target claim: %w", err))
		return
	}
	t, err := g.submitNoteAs("you", ct, "The chart drops the 2019 baseline — the full series shows +14%, not 200%.", "https://bls.gov/series")
	if err != nil {
		httpErr(w, fmt.Errorf("target note: %w", err))
		return
	}
	rateAll(A, t, H)
	writeJSON(w, map[string]interface{}{
		"seedClaim":  hex.EncodeToString(cs),
		"targetClaim": hex.EncodeToString(ct),
		"targetNote": hex.EncodeToString(t),
		"hint":       "Polarity is seeding (~1 min for inclusion + scoring). Then switch to a camp-B identity and rate the target note Helpful to watch it bridge to HELPFUL.",
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpErr(w, fmt.Errorf("bad json: %w", err))
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ---- small helpers (display) ----

func short12(h string) string {
	if len(h) > 12 {
		return h[:12] + ".."
	}
	return h
}
func fp2(v int64) string { return fmt.Sprintf("%.2f", float64(v)/1_000_000) } // display only (gateway is off-chain)
func signFp(v int64) string {
	if v >= 0 {
		return "+" + fp2(v)
	}
	return fp2(v)
}
func appendCapped(s []logLine, e logLine, max int) []logLine {
	s = append(s, e)
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}

// cmdServe runs the gateway.
func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cm := addCommon(fs)
	port := fs.String("port", "8080", "http port to listen on")
	valKey := fs.String("validator-key", os.Getenv("HOME")+"/.canopy/validator_key.json", "validator key (funds demo identities)")
	_ = fs.Parse(args)

	g := newGateway(cm.client(), *valKey)
	g.seedIdentities()
	go g.indexerLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/", g.cors(g.handleRoot))
	mux.HandleFunc("/api/state", g.cors(g.handleState))
	mux.HandleFunc("/api/identities", g.cors(g.handleIdentities))
	mux.HandleFunc("/api/account", g.cors(g.handleAccount))
	mux.HandleFunc("/api/claim", g.cors(g.handleClaim))
	mux.HandleFunc("/api/note", g.cors(g.handleNote))
	mux.HandleFunc("/api/rate", g.cors(g.handleRate))
	mux.HandleFunc("/api/seed", g.cors(g.handleSeed))
	log.Printf("veritas gateway listening on :%s (rpc=%s)", *port, g.cli.RPC)
	if err := http.ListenAndServe(":"+*port, mux); err != nil {
		log.Fatal(err)
	}
}
