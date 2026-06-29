# `veritas` — Veritas BLS12-381 CLI

The command-line client for the Veritas app-chain. It BLS12-381-signs and submits all four custom
transaction types to a local Canopy node, and queries + decodes the on-chain event lifecycle. It is
the **reference implementation the Phase 6 TypeScript frontend mirrors** — the signing flow lives in
[`client.go`](client.go) and the read/decode flow in [`query.go`](query.go).

This is a separate Go module that reuses the plugin's real `contract` proto types and
`crypto.GetSignBytes` via a `replace` directive (`replace github.com/canopy-network/go-plugin =>
../plugin/go`), so a transaction signed here is byte-for-byte acceptable to the chain.

## Build

```bash
cd cli
go build -o veritas .
# cross-compile to run next to the node in WSL/Linux:
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o veritas .
```

## Commands

```
veritas <command> [flags]
  send   -key K -to HEX20 -amount N
  claim  -key K -text "..." [-url U] [-content-hash HEX] [-nonce N]      → prints claimId
  note   -key K -claim-id HEX -body "..." [-content-hash HEX] [-nonce N] → prints noteId
  rate   -key K -note-id HEX -rating helpful|somewhat|not [-nonce N]
  query  (-address HEX20 | -height H) [-event claim|note|rating|score|reputation|all] [-json]
  keys   new -name NICK | get -address HEX20 [-password PW]   (dev keystore convenience)
```

Global flags (every subcommand): `-rpc` (default `:50002`), `-admin-rpc` (`:50003`), `-net`,
`-chain`, `-fee`, `-json`, `-key`.

## Example lifecycle

```bash
K=~/.canopy/validator_key.json
veritas claim -key $K -text "the earth is flat" -content-hash deadbeef -nonce 1   # → claimId
veritas note  -key $K -claim-id <claimId> -body "no, it's an oblate spheroid" -nonce 1   # → noteId
veritas rate  -key <raterKey> -note-id <noteId> -rating helpful -nonce 1
veritas query -address <validatorAddr> -event score          # watch the bridging recompute
veritas query -address <raterAddr>     -event reputation     # watch reputation move
veritas query -height 1357 --json                            # machine-readable
```

## The signing flow (what the frontend mirrors)

1. `height` = current chain height (also the input to `DeriveClaimID`/`DeriveNoteID`).
2. `any = Any{typeURL, proto.Marshal(msg)}`.
3. `signBytes = GetSignBytes(msgType, any, time, height, fee, "", net, chain)` — the canonical
   `lib.Transaction`-shaped deterministic marshal (no signature field).
4. `sig = BLS12-381 sign(signBytes)` with the account key (address = `sha256(pubKey)[:20]`).
5. Transaction JSON: the core `send` type carries a `msg` object; the plugin-only types carry
   `msgTypeUrl` + `msgBytes` (hex) for exact byte control.
6. `POST /v1/tx`.

## The read flow

v0.1.18 has no plugin-state read RPC, so **events are the read API**. The node renders our custom
`Any` payload under the (cosmetically mislabeled) `msg.orderId` hex; `query.go` hex-decodes it,
`proto.Unmarshal`s the `Any`, and resolves the concrete type via the protobuf registry
(`Any.UnmarshalNew`) — no hand wire-parsing. Decoded events: `claim_created`, `note_created`,
`note_rated`, `note_scored`, `reputation_changed`.
