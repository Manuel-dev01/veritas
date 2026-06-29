# Veritas — Local Run Guide

**Environment model:** *build on Windows, run in WSL2.* The Canopy node + plugin need a
Linux runtime (Unix-domain socket, bash plugin launcher), so they run in **WSL2 Ubuntu**.
The plugin is pure Go and **cross-compiles from Windows to a Linux ELF**, so WSL needs no
Go/protoc. WSL2's network to GitHub object CDNs is slow/flaky, so large binaries are
downloaded on the **Windows** side (fast) and copied into WSL.

Ports: `50002` query/tx RPC · `50003` admin RPC (keystore). Custom plugin state is read over
core RPC `POST /v1/query/state`. Logs: `~/.canopy/logs/` (node), `/tmp/plugin/go-plugin.log` (plugin).

---

## 0. One-time toolchain (Windows)

- Go 1.25.x (already present).
- protoc 35.1 at `.tools/protoc/bin/protoc.exe` (Phase 1+ codegen only).
- `protoc-gen-go@v1.36.6` + `protoc-go-inject-tag` in `%USERPROFILE%\go\bin` (Phase 1+ only).

## 1. Canopy node binary (upstream, pinned)

On **Windows**:
```bash
gh release download "v0.1.18+beta" -R canopy-network/canopy -p cli-linux-amd64
```
Copy into WSL and mark executable:
```bash
mkdir -p ~/veritas/node && cp /mnt/c/.../cli-linux-amd64 ~/veritas/node/canopy && chmod +x ~/veritas/node/canopy
```

## 2. Build the plugin (Windows → Linux ELF)

```bash
cd plugin/go
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o go-plugin .
```
Stage it next to `pluginctl.sh` in the node's working dir (the controller runs
`plugin/go/pluginctl.sh start` relative to its CWD):
```bash
mkdir -p ~/veritas/node/plugin/go
cp /mnt/c/.../plugin/go/go-plugin       ~/veritas/node/plugin/go/go-plugin
cp /mnt/c/.../plugin/go/pluginctl.sh    ~/veritas/node/plugin/go/pluginctl.sh   # strip CRLF
chmod +x ~/veritas/node/plugin/go/{go-plugin,pluginctl.sh}
```

## 3. First-time node init (creates config/genesis/keys)

`canopy start` prompts interactively for a new key password; `deploy/pty-run.py` answers it.
```bash
cp /mnt/c/.../deploy/pty-run.py ~/veritas/pty-run.py     # strip CRLF
cd ~/veritas/node && python3 ~/veritas/pty-run.py 45 ~/veritas/init.log   # runs ~45s, then stops
```
Then edit `~/.canopy/config.json`:
```jsonc
"plugin": "go",        // enable the plugin
"autoUpdate": false,   // pin the binary
"runVDF": false        // faster dev blocks
```

## 4. Run the node + plugin (background)

```bash
cd ~/veritas/node && python3 ~/veritas/pty-run.py 0 ~/veritas/node-run.log &
```
`RUN_SECONDS=0` runs until killed. Canopy auto-launches the plugin and listens on
`/tmp/plugin/plugin.sock`. Block time ≈ 20s (single validator).

## 5. Verify

```bash
# node up?
curl -s -X POST localhost:50002/v1/query/height -d '{}'
# plugin connected? (expect "Handshaking with FSM" + begin/end each block)
tail -f /tmp/plugin/go-plugin.log
```

Submit a `send` from the genesis-funded validator account and watch state change:
```bash
cd plugin/go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o veritas-sign-linux ./cmd/veritas-sign
cp /mnt/c/.../plugin/go/veritas-sign-linux ~/veritas/veritas-sign && chmod +x ~/veritas/veritas-sign
~/veritas/veritas-sign -key ~/.canopy/validator_key.json -to 1111111111111111111111111111111111111111 -amount 250000
curl -s -X POST localhost:50002/v1/query/account -d '{"address":"1111111111111111111111111111111111111111"}'
```

**Genesis facts (this dev chain):** account `d46e9a2042b4f2bdb69362aec7398f2ec623faa8`
is funded with `1,000,000` uCNPY; its BLS private key is `~/.canopy/validator_key.json`.
`sendFee = 10000`. Address = `SHA256(blsPubKey)[:20]`. networkID = chainID = 1.

## Stop / reset

```bash
pkill -f pty-run.py ; ~/veritas/node/plugin/go/pluginctl.sh stop 2>/dev/null
rm -rf ~/.canopy ~/veritas/node/plugin/go/.. # full reset (re-run step 3)
```
