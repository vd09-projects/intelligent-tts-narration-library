.PHONY: help build build-mcp test test-manual test-mcp-manual bench lint run run-detail run-male run-persistent run-mcp sanity clean

SAMPLE ?= docs/samples/sample.md

help:
	@echo "Targets:"
	@echo "  build             — go build ./..."
	@echo "  build-mcp         — go build ./cmd/narrate-mcp (compile MCP server in isolation)"
	@echo "  test              — unit + golden fixtures (no audio, no subprocess)"
	@echo "  test-manual       — end-to-end smoke with real Kokoro + afplay"
	@echo "  test-mcp-manual   — MCP runSpeak smoke against \$$SAMPLE (real Kokoro + afplay)"
	@echo "  bench             — planner-only + end-to-end benchmarks"
	@echo "  lint              — golangci-lint run"
	@echo "  run               — narrate \$$SAMPLE at level 1, female voice, ephemeral sink"
	@echo "  run-detail        — narrate \$$SAMPLE at level 3, male voice"
	@echo "  run-male          — narrate \$$SAMPLE at level 1, male voice"
	@echo "  run-persistent    — confirm persistent sink rejects (expect exit 2)"
	@echo "  run-mcp           — start the MCP stdio server (Ctrl-C to stop)"
	@echo "  sanity            — go build + check scripts/kokoro present"
	@echo "  clean             — go clean -testcache"
	@echo ""
	@echo "Override sample doc: make run SAMPLE=path/to/file.md"

build:
	go build ./...

build-mcp:
	go build ./cmd/narrate-mcp

test:
	go test ./...

test-manual:
	go test -tags manual ./pipeline/...

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
	-go run ./cmd/narrate --file $(SAMPLE) --sink persistent; echo "exit=$$?"

run-mcp:
	go run ./cmd/narrate-mcp

sanity:
	go build ./... && test -x scripts/kokoro && echo "ok: build + scripts/kokoro present"

clean:
	go clean -testcache
