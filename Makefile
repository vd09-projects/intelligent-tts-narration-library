.PHONY: help build build-mcp build-server test test-race test-race-planner test-manual test-manual-persistent test-mcp-manual bench fmt lint run run-detail run-male run-persistent run-listen run-mcp run-observe run-observe-manual run-server run-companion sanity clean player-dev player-build player-test player-fixture-silent player-fixture-kokoro

SAMPLE ?= docs/samples/sample.md
OUT ?= /tmp/narrate-persistent-$(shell date +%s)
OBSERVE_FILE ?= /tmp/narrate-observe-manual.jsonl
ADDR ?= 127.0.0.1:8080
CORS_ORIGIN ?= http://localhost:5173
PLAYER_FIXTURE_DIR ?= player/public/fixtures/sample
PLAYER_FIXTURE_DURATION ?= 2.0

help:
	@echo "Targets:"
	@echo "  build                  — go build ./..."
	@echo "  build-mcp              — go build ./cmd/narrate-mcp (compile MCP server in isolation)"
	@echo "  build-server           — go build ./cmd/narrate-server (compile HTTP escalate edge in isolation)"
	@echo "  test                   — unit + golden fixtures (no audio, no subprocess)"
	@echo "  test-race              — unit tests under the race detector (go test -race ./...)"
	@echo "  test-race-planner      — planner-only race gate (go test -race ./planner/...)"
	@echo "  test-manual            — end-to-end smoke with real Kokoro + afplay"
	@echo "  test-manual-persistent — sink/persistent smoke with real Kokoro (writes \$$OUT)"
	@echo "  test-mcp-manual        — MCP runSpeak smoke against \$$SAMPLE (real Kokoro + afplay)"
	@echo "  bench                  — planner-only + end-to-end benchmarks"
	@echo "  fmt                    — gofmt -w . (format module-wide)"
	@echo "  lint                   — gofmt drift gate + golangci-lint run"
	@echo "  run                    — narrate \$$SAMPLE at level 1, female voice, ephemeral sink"
	@echo "  run-detail             — narrate \$$SAMPLE at level 3, male voice"
	@echo "  run-male               — narrate \$$SAMPLE at level 1, male voice"
	@echo "  run-persistent         — narrate \$$SAMPLE via persistent sink → \$$OUT"
	@echo "  run-listen             — interactive raw-mode transport over \$$SAMPLE (n/b/space/g/q; needs a tty + afplay)"
	@echo "  run-mcp                — start the MCP stdio server (Ctrl-C to stop)"
	@echo "  run-observe            — tail the Channel-2 live observer (-f \$$OBSERVE_FILE, else newest /tmp glob; Ctrl-C to stop)"
	@echo "  run-observe-manual     — 2-terminal live observer smoke: speak \$$SAMPLE emitting to \$$OBSERVE_FILE (real Kokoro + afplay)"
	@echo "  run-server             — start the localhost HTTP escalate server on \$$ADDR (Ctrl-C to stop)"
	@echo "  sanity                 — go build + check scripts/kokoro present"
	@echo "  clean                  — go clean -testcache"
	@echo ""
	@echo "Reference player (player/):"
	@echo "  player-dev             — cd player && pnpm install && pnpm dev"
	@echo "  player-build           — cd player && pnpm install && pnpm build"
	@echo "  player-test            — cd player && pnpm install && pnpm test"
	@echo "  run-companion          — one-click visual companion: render \$$SAMPLE → \$$OUT (separate --sink persistent), start narrate-server, launch player against \$$OUT (#76)"
	@echo "  player-fixture-silent  — regenerate $(PLAYER_FIXTURE_DIR)/audio.wav as silent 24kHz mono PCM-16"
	@echo "  player-fixture-kokoro  — narrate \$$SAMPLE via persistent sink → $(PLAYER_FIXTURE_DIR)"
	@echo ""
	@echo "Override sample doc: make run SAMPLE=path/to/file.md"
	@echo "Override persistent out: make run-persistent OUT=path/to/dir"

build:
	go build ./...

build-mcp:
	go build ./cmd/narrate-mcp

build-server:
	go build ./cmd/narrate-server

test:
	go test ./...

test-race:
	go test -race ./...

test-race-planner:
	go test -race ./planner/...

test-manual:
	go test -tags manual ./pipeline/...

test-manual-persistent:
	go test -tags manual -v ./sink/persistent/...

test-mcp-manual:
	go test -tags manual -v ./cmd/narrate-mcp/...

bench:
	go test -bench=BenchmarkNarrate -benchmem ./pipeline/...

fmt:
	gofmt -w .

lint:
	@drift=$$(gofmt -l .); if [ -n "$$drift" ]; then echo "gofmt drift (run 'make fmt'):"; echo "$$drift"; exit 1; fi
	golangci-lint run

run:
	go run ./cmd/narrate --file $(SAMPLE)

run-detail:
	go run ./cmd/narrate --file $(SAMPLE) --level 3 --gender male

run-male:
	go run ./cmd/narrate --file $(SAMPLE) --gender male

run-persistent:
	@mkdir -p $(OUT)
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(OUT)
	@echo "Wrote audio.wav + plan.json + manifest.json to $(OUT)"

# Interactive: renders the whole doc then hands you single-key block transport
# (n next, b back, space Stop/Replay block, g go-to, q quit). Requires an
# interactive terminal and afplay (macOS, phase one); refuses on a piped stdin.
run-listen:
	go run ./cmd/narrate --file $(SAMPLE) --listen

run-mcp:
	go run ./cmd/narrate-mcp

# Channel-2 live observer (issue #81). Terminal 2: tail the scratch file the
# speak handler writes per block. Defaults to -f $(OBSERVE_FILE); drop it to
# fall back to the newest /tmp/narrate-observe-*.jsonl (e.g. an auto-temp run).
run-observe:
	go run ./cmd/narrate-observe -f $(OBSERVE_FILE)

# Two-terminal manual flow for the live observer:
#   Terminal 1:  make run-observe                 # starts tailing $(OBSERVE_FILE)
#   Terminal 2:  make run-observe-manual          # speaks $(SAMPLE), emitting live
# The speak side runs the real Kokoro + afplay manual smoke with
# NARRATE_OBSERVE_FILE pointed at $(OBSERVE_FILE), so terminal 1 prints one
# progress line per block WHILE audio plays.
run-observe-manual:
	NARRATE_OBSERVE_FILE=$(OBSERVE_FILE) go test -tags manual -v -run TestSpeakManualSmoke ./cmd/narrate-mcp/...

run-server:
	go run ./cmd/narrate-server --addr $(ADDR) --cors-origin $(CORS_ORIGIN)

# One-click optional React visual companion (#76). Composes EXISTING pieces, no
# new render path and never a `speak` tee (Risk 6 / AC6): a SEPARATE
# `cmd/narrate --sink persistent` render into $(OUT), the localhost narrate-server
# (which the player polls + re-fetches over HTTP), and the player itself targeting
# $(OUT) via VITE_COMPANION_DIR. mkdir -p $(OUT) FIRST so the dir exists before
# the player polls — `no_out_dir` then fires only on a genuine bad path, and the
# spinner-then-load path is exercised because the render runs in the background
# while the player spins.
#
# Cleanup: the whole recipe runs in ONE shell (backslash-joined) so `trap 'kill
# 0' INT EXIT` reaps BOTH backgrounded `go run` processes (server + render) when
# the foreground `pnpm dev` exits on Ctrl-C. Without this they were orphaned by
# their already-exited per-line sub-shells, leaving the narrate-server port held
# so a second `make run-companion` failed with "address already in use". `kill 0`
# signals the whole process group, so the trap fires on both interrupt and a
# normal exit.
run-companion:
	@mkdir -p $(OUT)
	@trap 'kill 0' INT EXIT; \
		go run ./cmd/narrate-server --addr $(ADDR) --cors-origin $(CORS_ORIGIN) & \
		go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(OUT) & \
		cd player && VITE_ESCALATE_BASE_URL=http://$(ADDR) VITE_COMPANION_DIR=$(OUT) pnpm install && pnpm dev

sanity:
	go build ./... && test -x scripts/kokoro && echo "ok: build + scripts/kokoro present"

clean:
	go clean -testcache

# ----- Reference player (player/) ------------------------------------------

player-dev:
	cd player && pnpm install && pnpm dev

player-build:
	cd player && pnpm install && pnpm build

player-test:
	cd player && pnpm install && pnpm test

player-fixture-silent:
	@mkdir -p $(PLAYER_FIXTURE_DIR)
	python3 player/public/fixtures/make_silent_wav.py $(PLAYER_FIXTURE_DURATION) $(PLAYER_FIXTURE_DIR)/audio.wav

player-fixture-kokoro:
	@mkdir -p $(PLAYER_FIXTURE_DIR)
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(PLAYER_FIXTURE_DIR)
	@echo "Refreshed $(PLAYER_FIXTURE_DIR) from $(SAMPLE)"
