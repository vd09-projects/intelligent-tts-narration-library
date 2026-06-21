.PHONY: help build build-mcp build-server test test-race test-manual test-manual-persistent test-mcp-manual bench lint run run-detail run-male run-persistent run-mcp run-server sanity clean player-dev player-build player-test player-fixture-silent player-fixture-kokoro

SAMPLE ?= docs/samples/sample.md
OUT ?= /tmp/narrate-persistent-$(shell date +%s)
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
	@echo "  test-manual            — end-to-end smoke with real Kokoro + afplay"
	@echo "  test-manual-persistent — sink/persistent smoke with real Kokoro (writes \$$OUT)"
	@echo "  test-mcp-manual        — MCP runSpeak smoke against \$$SAMPLE (real Kokoro + afplay)"
	@echo "  bench                  — planner-only + end-to-end benchmarks"
	@echo "  lint                   — golangci-lint run"
	@echo "  run                    — narrate \$$SAMPLE at level 1, female voice, ephemeral sink"
	@echo "  run-detail             — narrate \$$SAMPLE at level 3, male voice"
	@echo "  run-male               — narrate \$$SAMPLE at level 1, male voice"
	@echo "  run-persistent         — narrate \$$SAMPLE via persistent sink → \$$OUT"
	@echo "  run-mcp                — start the MCP stdio server (Ctrl-C to stop)"
	@echo "  run-server             — start the localhost HTTP escalate server on \$$ADDR (Ctrl-C to stop)"
	@echo "  sanity                 — go build + check scripts/kokoro present"
	@echo "  clean                  — go clean -testcache"
	@echo ""
	@echo "Reference player (player/):"
	@echo "  player-dev             — cd player && pnpm install && pnpm dev"
	@echo "  player-build           — cd player && pnpm install && pnpm build"
	@echo "  player-test            — cd player && pnpm install && pnpm test"
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

test-manual:
	go test -tags manual ./pipeline/...

test-manual-persistent:
	go test -tags manual -v ./sink/persistent/...

test-mcp-manual:
	go test -tags manual -v ./cmd/narrate-mcp/...

bench:
	go test -bench=BenchmarkNarrate -benchmem ./pipeline/...

lint:
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

run-mcp:
	go run ./cmd/narrate-mcp

run-server:
	go run ./cmd/narrate-server --addr $(ADDR) --cors-origin $(CORS_ORIGIN)

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
