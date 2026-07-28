.PHONY: help build build-mcp build-mcp-bin build-server test test-race test-race-planner test-manual test-manual-persistent test-mcp-manual bench fmt lint run run-detail run-male run-persistent run-listen run-mcp run-observe run-observe-manual run-server sanity clean preview-mockup earshot-dev earshot-build earshot-test earshot-lint rvc-export rvc-export-shared rvc-worker-venv rvc-parity rvc-parity-gen rvc-fixtures-fetch rvc-fixtures-test rvc-fixtures-publish rvc-convert rvc-sanity voice-sanity mcp-voice-sanity gso-fetch-base gso-worker-venv gso-contract-test gso-warmproof gso-sanity gso-perf-baseline gso-g2p-check

SAMPLE ?= docs/samples/sample.md
OUT ?= /tmp/narrate-persistent-$(shell date +%s)
RVC_SANITY_OUT ?= /tmp/rvc-sanity-$(shell date +%s)
VOICE_SANITY_OUT ?= /tmp/voice-sanity-$(shell date +%s)
GSO_SANITY_OUT ?= /tmp/gso-sanity-$(shell date +%s)
GSO_PERF_OUT ?= /tmp/gso-perf-$(shell date +%s)
GSO_G2P_OUT ?= /tmp/gso-g2p-$(shell date +%s)
GSO_G2P_DOC ?= docs/samples/gso-g2p-coverage.md
GSO_G2P_LEVEL ?= 3
OBSERVE_FILE ?= /tmp/narrate-observe-manual.jsonl
ADDR ?= 127.0.0.1:8080
CORS_ORIGIN ?= http://localhost:5173
MOCKUP_PORT ?= 8137
MOCKUP_DIR ?= /tmp/earshot-preview
EARSHOT_DIR ?= earshot

help:
	@echo "Targets:"
	@echo "  build                  — go build ./..."
	@echo "  build-mcp              — go build ./cmd/narrate-mcp (compile MCP server in isolation)"
	@echo "  build-mcp-bin          — build the MCP server into bin/narrate-mcp (the binary .mcp.json's launcher execs)"
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
	@echo "  run-listen             — interactive raw-mode transport over \$$SAMPLE (n/b/space/g/q; in-process oto v3 player, true Pause/Resume; needs a tty + audio device)"
	@echo "  run-mcp                — start the MCP stdio server (Ctrl-C to stop)"
	@echo "  run-observe            — tail the Channel-2 live observer (-f \$$OBSERVE_FILE, else newest /tmp glob; Ctrl-C to stop)"
	@echo "  run-observe-manual     — 2-terminal live observer smoke: speak \$$SAMPLE emitting to \$$OBSERVE_FILE (real Kokoro + afplay)"
	@echo "  run-server             — start the localhost HTTP escalate server on \$$ADDR (Ctrl-C to stop)"
	@echo "  sanity                 — go build + check scripts/kokoro present"
	@echo "  clean                  — go clean -testcache"
	@echo ""
	@echo "Earshot #115 sign-off mockup (throwaway artifact):"
	@echo "  preview-mockup         — bundle earshot-mockup/EarshotMockup.jsx into a /tmp harness, serve on \$$MOCKUP_PORT, open browser (needs node/npx + network for esm.sh React)"
	@echo ""
	@echo "Earshot web app (earshot/, #111 — needs node/npm):"
	@echo "  earshot-dev            — Vite dev server (:5173) proxying /sessions,/narrate,/audio to narrate-server (run 'make run-server' first)"
	@echo "  earshot-build          — type-check + production bundle (COMPILE/SMOKE CHECK ONLY — no proxy in the bundle, not a runnable deploy)"
	@echo "  earshot-test           — Vitest unit + component + a11y suite (no audio)"
	@echo "  earshot-lint           — ESLint (flat config)"
	@echo ""
	@echo "RVC → ONNX export (#143 — needs the Applio venv w/ torch; artifacts gitignored):"
	@echo "  rvc-export VOICE=<slug> — export+validate one voice → assets/rvc-models/<slug>/onnx/ (net_g + index_vectors; runs rvc-export-shared first if _shared missing)"
	@echo "  rvc-export-shared      — (re)build voice-independent _shared/onnx/{contentvec,rmvpe}.onnx + mel basis (add FORCE=1 to rebuild)"
	@echo ""
	@echo "RVC torch-free inference worker (#144/#151 — needs .venv-rvc; parity fixtures fetched from a pinned GitHub Release):"
	@echo "  rvc-worker-venv        — build .venv-rvc (python3.12) from scripts/rvc-requirements.txt (asserts torch absent + prints freeze)"
	@echo "  rvc-fixtures-fetch     — fetch + hashlib-verify the hosted parity fixtures (prereq of rvc-parity; idempotent; fails loud on 404/hang/mismatch; while unpinned, falls back to LOCAL fixtures with a loud unverified notice)"
	@echo "  rvc-fixtures-test      — stdlib-only smokes for the fetch/verify + single-voice honesty machinery (stock python3; no venv/network/fixtures)"
	@echo "  rvc-parity             — always-on gate: per-stage refio corr + full-pipeline log-mel + protocol contract + arg/atomic/format"
	@echo "  rvc-parity-gen         — LOCAL (re)generate gate targets from the Applio torch ref (SOURCE=<clip.wav> [VOICE=<slug>]); gitignored, not the hosted bundle"
	@echo "  rvc-fixtures-publish   — MAINTAINER: regen PARITY_VOICES bundle → pin fixtures.sha256 → gh release create (gated on D0 license: I_HAVE_CLEARED_D0=1)"
	@echo "  rvc-convert VOICE=<slug> IN=<wav> OUT=<wav> — single by-ear smoke through scripts/rvc (INDEX_RATE optional)"
	@echo ""
	@echo "RVC voice wiring (#146 — needs the RVC worker: run 'make rvc-worker-venv' + 'make rvc-export'):"
	@echo "  rvc-sanity             — narrate \$$SAMPLE at both RVC voices → 40 kHz audio.wav + manifest per voice under \$$RVC_SANITY_OUT (for the #147 by-ear /verify)"
	@echo "  voice-sanity           — narrate \$$SAMPLE across the roster matrix (am-michael Kokoro 24 kHz + cool-jahns/confident-neal RVC 40 kHz) under \$$VOICE_SANITY_OUT (#156; RVC voices need the worker; for the #147 by-ear /verify)"
	@echo "  mcp-voice-sanity       — MCP runSpeak by-ear smoke through a roster VOICE (default cool-jahns → RVC 40 kHz via afplay); proves the MCP speak 'voice' arg end-to-end (#147)"
	@echo ""
	@echo "GPT-SoVITS (#161 — torch subprocess over TTS_infer_pack; consumes .ckpt/.pth directly, NO ONNX; 32 kHz):"
	@echo "  gso-fetch-base         — fetch + size-sanity the ~950MB shared base models into assets/gptsovits-models/_base/ (idempotent; needs network)"
	@echo "  gso-worker-venv        — build .venv-gso (python3.11) from scripts/gso-requirements.txt; gotchas 1-4 baked; asserts torch PRESENT (inverse of RVC) + prints freeze"
	@echo "  gso-contract-test      — torch-free wire/ERR-taxonomy contract test + shlex golden round-trip + warmproof negative dry-check (stock python3; no venv/network/models)"
	@echo "  gso-warmproof          — AC5 warm-load CORRECTNESS smoke: LOAD-once, non-silent 32 kHz, A,B,A determinism (per-response byte-buffering) + distinct B!=A, warm-vs-cold (distinct dirs); needs .venv-gso + real artifacts"
	@echo "  gso-sanity             — narrate \$$SAMPLE at cool-jahns-gso → 32 kHz audio.wav + manifest under \$$GSO_SANITY_OUT (#162 AC5 Timeline smoke; needs the GSO worker: .venv-gso + gso-fetch-base + a GSO_REPO clone)"
	@echo ""
	@echo "GPT-SoVITS go/no-go evidence (#164 — SURFACES/STAGES machine evidence for the human gate; NEVER self-verifies by ear):"
	@echo "  gso-perf-baseline      — OFFICIAL AC4 baseline: drive the warm worker over \$$SAMPLE, record cold/warm-per-block/peak-RSS vs ceilings (cold 30s / warm ~20s INFORMATIONAL, peak-RSS 8GB go/no-go). NEEDS the .venv-gso worker (real numbers only; absent worker → AC4 UNSATISFIED)"
	@echo "  gso-g2p-check          — AC3 machine coverage of \$$GSO_G2P_DOC: half A surfaces Segment.Text per structured class from plan.json (NO worker); half B dumps g2p_en ARPAbet phoneme STRINGS (NEEDS .venv-gso). Textual inspection only — acoustic realism is AC3-ear (staged for human ears)"
	@echo ""
	@echo "Override sample doc: make run SAMPLE=path/to/file.md"
	@echo "Override persistent out: make run-persistent OUT=path/to/dir"
	@echo "RVC voice slug: make rvc-export VOICE=cool-jahns  (or confident-neal)"

build:
	go build ./...

build-mcp:
	go build ./cmd/narrate-mcp

# Build the MCP server into bin/narrate-mcp — the binary that bin/narrate-mcp-launch
# execs when Claude Code starts the project-scoped "narrate" server from .mcp.json.
# bin/ is gitignored (build output), so a fresh clone must run this once before the
# MCP server is usable. The launcher script itself is force-tracked.
build-mcp-bin:
	go build -o bin/narrate-mcp ./cmd/narrate-mcp

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
# (n next, b back, space Pause/Resume, g go-to, q quit) via the in-process oto v3
# player (true device-level pause). Requires an interactive terminal and an
# openable audio device (macOS, phase one); refuses on a piped stdin.
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

# sanity proves the default build compiles, including the CGO_ENABLED=0 path for
# cmd/narrate (the in-process oto v3 player runs through purego, so it must link
# with no CGo — #101). The oto player is the default build now, so there is no
# separate -tags build to special-case.
sanity:
	go build ./... && CGO_ENABLED=0 go build ./cmd/narrate && test -x scripts/kokoro && echo "ok: build (default + CGO_ENABLED=0 cmd/narrate) + scripts/kokoro present"

clean:
	go clean -testcache

# Build + serve the throwaway #115 sign-off mockup. Generates an ephemeral harness
# in $(MOCKUP_DIR) (never committed): an entry that mounts EarshotMockup and an
# index.html whose importmap pulls React from esm.sh at runtime, so no node_modules
# is needed — only npx (for esbuild) and network. Ctrl-C the server to stop.
preview-mockup:
	@mkdir -p $(MOCKUP_DIR)
	@cp earshot-mockup/EarshotMockup.jsx $(MOCKUP_DIR)/EarshotMockup.jsx
	@printf 'import React from "react";\nimport { createRoot } from "react-dom/client";\nimport EarshotMockup from "./EarshotMockup.jsx";\ncreateRoot(document.getElementById("root")).render(<EarshotMockup />);\n' > $(MOCKUP_DIR)/entry.jsx
	@printf '<!doctype html><html><head><meta charset="utf-8"><title>Earshot mockup #115</title>\n<script type="importmap">\n{"imports":{"react":"https://esm.sh/react@18","react-dom/client":"https://esm.sh/react-dom@18/client"}}\n</script></head><body><div id="root"></div>\n<script type="module" src="./bundle.js"></script></body></html>\n' > $(MOCKUP_DIR)/index.html
	npx --yes esbuild@0.21.5 $(MOCKUP_DIR)/entry.jsx --bundle --format=esm --external:react --external:react-dom/client --outfile=$(MOCKUP_DIR)/bundle.js
	@echo "Serving http://localhost:$(MOCKUP_PORT) (Ctrl-C to stop)"
	@command -v open >/dev/null 2>&1 && (sleep 1 && open http://localhost:$(MOCKUP_PORT)) &
	@cd $(MOCKUP_DIR) && python3 -m http.server $(MOCKUP_PORT)

# ---- Earshot web app (earshot/, #111) ----
# These wrap the npm scripts in earshot/ so frontend actions stay consistent with
# the repo Makefile convention (D3). Each target installs node_modules on first
# run (gitignored). earshot-build is a COMPILE/SMOKE check only — the Vite dev
# proxy exists only under earshot-dev, so a built bundle cannot reach the server.
$(EARSHOT_DIR)/node_modules: $(EARSHOT_DIR)/package.json
	cd $(EARSHOT_DIR) && npm install
	@touch $(EARSHOT_DIR)/node_modules

earshot-dev: $(EARSHOT_DIR)/node_modules
	cd $(EARSHOT_DIR) && npm run dev

earshot-build: $(EARSHOT_DIR)/node_modules
	cd $(EARSHOT_DIR) && npm run build

earshot-test: $(EARSHOT_DIR)/node_modules
	cd $(EARSHOT_DIR) && npm run test

earshot-lint: $(EARSHOT_DIR)/node_modules
	cd $(EARSHOT_DIR) && npm run lint

# ---- RVC → ONNX export (#143) ----
# Productionize an Applio-trained RVC .pth voice into the ONNX + npy artifacts the
# torch-free runtime consumes. All heavy lifting + env (Applio venv python, MPS /
# OpenMP fixes) lives in scripts/rvc-export/rvc-export (mirrors scripts/kokoro).
# Outputs land under assets/rvc-models/ and are gitignored (large, regenerable —
# backed up to the vd09-projects/rvc-voices HF repo; see that dir's README).
RVC_FORCE_FLAG := $(if $(FORCE),--force,)

rvc-export:
	@test -n "$(VOICE)" || { echo "VOICE required, e.g. make rvc-export VOICE=cool-jahns"; exit 2; }
	scripts/rvc-export/rvc-export voice $(VOICE)

rvc-export-shared:
	scripts/rvc-export/rvc-export shared $(RVC_FORCE_FLAG)

# ---- RVC torch-free inference worker (#144) ----
# The ephemeral warm-load-once subprocess engine (#145 drives it later). Its venv
# is DEDICATED + torch-free (.venv-rvc, NOT the shared .venv) so the worker's
# startup guard — an explicit unconditional `if _TORCH_PRESENT: FATAL + sys.exit(78)`
# (EX_CONFIG), unstrippable by `python -O`/PYTHONOPTIMIZE unlike a bare assert — is a
# real guarantee. Built with python3.12 to match the pilot (onnxruntime/faiss wheels;
# the system python3 may be newer).
RVC_VENV := .venv-rvc
RVC_PY := $(RVC_VENV)/bin/python
PYTHON312 ?= python3.12

rvc-worker-venv:
	$(PYTHON312) -m venv $(RVC_VENV)
	$(RVC_PY) -m pip install --upgrade pip
	$(RVC_PY) -m pip install -r scripts/rvc-requirements.txt
	@$(RVC_PY) -c "import onnxruntime,numpy,faiss,scipy,librosa,soxr,soundfile; print('torch-free deps import OK')"
	@if $(RVC_PY) -c "import torch" 2>/dev/null; then echo "FAIL: torch present in $(RVC_VENV)"; exit 1; fi
	@if $(RVC_PY) -m pip freeze | grep -i '^torch'; then echo "FAIL: torch in pip freeze"; exit 1; fi
	@echo "ok: $(RVC_VENV) is torch-free. Freeze below — capture into scripts/rvc-requirements.txt provenance:"
	@$(RVC_PY) -m pip freeze

# ---- RVC parity fixtures: hosted bundle fetch + publish (#151) ----
# The full-pipeline gate fixtures (source.wav + the PARITY_VOICES ref WAV + log-mel
# target) are NOT committed (the repo forbids .wav/.npy binaries + personal audio).
# They are fetched from a pinned GitHub Release and hashlib-verified against the
# committed tests/rvc_parity/fixtures.sha256 — the trust root, NOT the owner-mutable
# release URL.
#
# TODO(#151): pin the real release tag + asset base URL below AFTER
# `make rvc-fixtures-publish` has been run and the release is live (plan Step 6 + the
# Step 8 merge gate — tag pin + fixtures.sha256 land in ONE commit that must NOT
# merge before the release is fetchable). They are EMPTY on purpose right now:
# BLOCKED on the D0 voice-model license gate (cool-jahns is a clone of a real public
# figure with no redistribution license — see tests/rvc_parity/fixtures/README.md).
# While empty, rvc-fixtures-fetch fails loud instead of fetching.
RVC_FIXTURES_TAG      ?=
RVC_FIXTURES_BASEURL  ?=
# Bounded curl — fail loud on hang/partition instead of blocking the gate forever.
RVC_FIXTURES_CONNECT_TIMEOUT ?= 10
RVC_FIXTURES_MAX_TIME        ?= 120
RVC_FIXTURES_RETRY           ?= 3
# Stdlib-only fetch/pin helpers — run under a stock python3 (no venv/torch to fetch).
PYTHON3 ?= python3

# Fetch + hashlib-verify the hosted parity fixtures. Idempotent (present+valid ->
# no network). Fails loud + non-zero on 404/unreachable, bounded-timeout hang,
# checksum mismatch, or a present-but-divergent file. While the release tag is the
# unpinned D0-not-cleared placeholder, it falls back to LOCAL fixtures: runs against
# them with a LOUD "unverified" notice if all are present, else fails loud pointing
# at `make rvc-parity-gen`. Prereq of rvc-parity so the gate can never go green on
# missing fixtures (Decision D5).
rvc-fixtures-fetch:
	$(PYTHON3) tests/rvc_parity/fetch_fixtures.py \
	  --tag "$(RVC_FIXTURES_TAG)" --base-url "$(RVC_FIXTURES_BASEURL)" \
	  --connect-timeout $(RVC_FIXTURES_CONNECT_TIMEOUT) \
	  --max-time $(RVC_FIXTURES_MAX_TIME) --retry $(RVC_FIXTURES_RETRY)

# Stdlib-only smoke suite for the fetch/verify + single-voice honesty machinery.
# Runs under a stock python3 (no venv/torch/network/fixtures), so this change's own
# guarantees have a green make path that can't silently rot.
rvc-fixtures-test:
	$(PYTHON3) tests/rvc_parity/fixtures_flow_test.py

rvc-parity: rvc-fixtures-fetch
	@test -x $(RVC_PY) || { echo "no $(RVC_VENV) — run 'make rvc-worker-venv'"; exit 2; }
	$(RVC_PY) tests/rvc_parity/parity_test.py

# LOCAL (re)generate gate targets from the Applio torch path (gitignored, NOT the
# hosted bundle). Pass SOURCE=<clip.wav> the first time to place fixtures/source.wav.
rvc-parity-gen:
	@test -x $(RVC_PY) || { echo "no $(RVC_VENV) — run 'make rvc-worker-venv'"; exit 2; }
	$(RVC_PY) tests/rvc_parity/gen_targets.py $(if $(SOURCE),--source $(SOURCE),) $(if $(VOICE),--voice $(VOICE),)

# MAINTAINER: regenerate the hosted PARITY_VOICES bundle, pin it, and cut a new
# release — one command (plan Step 6 / Decision D7). GATED on the D0 voice-model
# license: publishing converted output requires redistribution rights. cool-jahns is
# a clone of a real public figure with NO such license on record (#151 D0 = not
# cleared), so this refuses to run without an explicit I_HAVE_CLEARED_D0=1 override.
# Do NOT set that flag until a redistributable voice replaces cool-jahns.
#
# TODO(#151): RVC_PUBLISH_TAG is a template — pick the real tag (e.g.
# rvc-parity-fixtures-v1) and, after this runs + the release is live, pin
# RVC_FIXTURES_TAG/RVC_FIXTURES_BASEURL above + fixtures.sha256 in the SAME commit.
RVC_PUBLISH_TAG ?= rvc-parity-fixtures-vX
# Derived from PARITY_VOICES (single source) via parity_voices.py — NOT hardcoded —
# so a D0 pivot that swaps cool-jahns for a redistributable voice updates the
# pinned + uploaded asset list in lockstep. Recursively-expanded (=) so python3 only
# runs when this maintainer target actually references it, not on every make call.
RVC_BUNDLE_ASSETS = $(shell $(PYTHON3) tests/rvc_parity/parity_voices.py)
rvc-fixtures-publish:
	@test "$(I_HAVE_CLEARED_D0)" = "1" || { \
	  echo "BLOCKED: publishing needs the D0 voice-model redistribution license cleared."; \
	  echo "cool-jahns is a clone of a real public figure with NO redistribution license (#151 D0 = not cleared)."; \
	  echo "Do NOT publish until a redistributable voice replaces it. See tests/rvc_parity/fixtures/README.md."; \
	  echo "Re-run with I_HAVE_CLEARED_D0=1 ONLY after clearance."; exit 2; }
	@test -x $(RVC_PY) || { echo "no $(RVC_VENV) — run 'make rvc-worker-venv'"; exit 2; }
	@command -v gh >/dev/null || { echo "gh CLI required to create the release"; exit 2; }
	@test -n "$(SOURCE)" || { echo "SOURCE=<vetted-public-clip.wav> required"; exit 2; }
	$(RVC_PY) tests/rvc_parity/gen_targets.py --source $(SOURCE) --bundle
	$(PYTHON3) tests/rvc_parity/fixtures_io.py $(RVC_BUNDLE_ASSETS)
	gh release create $(RVC_PUBLISH_TAG) \
	  $(addprefix tests/rvc_parity/fixtures/,$(RVC_BUNDLE_ASSETS)) \
	  --title "RVC parity fixtures $(RVC_PUBLISH_TAG)" \
	  --notes "Torch-reference RVC parity fixtures for 'make rvc-parity' (#151). Trust root: tests/rvc_parity/fixtures.sha256 (committed)."
	@echo "Published $(RVC_PUBLISH_TAG). Now pin RVC_FIXTURES_TAG + RVC_FIXTURES_BASEURL + fixtures.sha256 in ONE commit (Step 8 merge gate)."

# Single by-ear smoke: one line through scripts/rvc. INDEX_RATE defaults per voice
# (cool-jahns 0.75, confident-neal 0.5). Pitch fixed to 0 (phase one).
rvc-convert:
	@test -n "$(VOICE)" || { echo "VOICE required, e.g. make rvc-convert VOICE=cool-jahns IN=in.wav OUT=out.wav"; exit 2; }
	@test -n "$(IN)" || { echo "IN required (source wav)"; exit 2; }
	@test -n "$(OUT)" || { echo "OUT required (dest wav)"; exit 2; }
	@ir="$(INDEX_RATE)"; if [ -z "$$ir" ]; then case "$(VOICE)" in confident-neal) ir=0.5;; *) ir=0.75;; esac; fi; \
	  printf '"%s" "%s" %s %s 0\n' "$(IN)" "$(OUT)" "$(VOICE)" "$$ir" | scripts/rvc && echo "wrote $(OUT) (voice=$(VOICE) index_rate=$$ir)"

# #146 by-ear /verify convenience (the exhaustive audio check is #147): render
# $(SAMPLE) once per RVC voice through the persistent sink so each dir carries a
# 40 kHz audio.wav + a manifest.json whose "voice" is the character slug (D6).
# Needs the RVC worker on disk (make rvc-worker-venv + make rvc-export VOICE=...);
# an unknown voice or a missing worker STOPS with a non-zero exit (honesty rule).
rvc-sanity:
	@mkdir -p $(RVC_SANITY_OUT)/cool-jahns $(RVC_SANITY_OUT)/confident-neal
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(RVC_SANITY_OUT)/cool-jahns --voice cool-jahns
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(RVC_SANITY_OUT)/confident-neal --voice confident-neal
	@echo "RVC sanity: wrote 40 kHz cool-jahns + confident-neal renders under $(RVC_SANITY_OUT)"
	@echo "Verify (#147): afplay $(RVC_SANITY_OUT)/cool-jahns/audio.wav — and confirm manifest.json \"voice\" == the slug (D6)."

# voice-sanity (#156) renders $(SAMPLE) across the unified roster matrix: a Kokoro
# slug (am-michael, 24 kHz — needs only Kokoro) plus both RVC slugs (40 kHz — need
# the RVC worker: make rvc-worker-venv + make rvc-export VOICE=...). Each --voice
# is the primary selector; an unknown slug or a missing worker STOPS non-zero
# (honesty rule). The Kokoro leg proves manifest.voice records the engine id
# (am_michael, underscore) at 24 kHz; the RVC legs prove the slug at 40 kHz.
voice-sanity:
	@mkdir -p $(VOICE_SANITY_OUT)/am-michael $(VOICE_SANITY_OUT)/cool-jahns $(VOICE_SANITY_OUT)/confident-neal $(VOICE_SANITY_OUT)/cool-jahns-gso
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(VOICE_SANITY_OUT)/am-michael --voice am-michael
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(VOICE_SANITY_OUT)/cool-jahns --voice cool-jahns
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(VOICE_SANITY_OUT)/confident-neal --voice confident-neal
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(VOICE_SANITY_OUT)/cool-jahns-gso --voice cool-jahns-gso
	@echo "Voice sanity: wrote am-michael (24 kHz Kokoro) + cool-jahns/confident-neal (40 kHz RVC) + cool-jahns-gso (32 kHz GPT-SoVITS) under $(VOICE_SANITY_OUT)"
	@echo "Verify (#147/#162): afplay each audio.wav; confirm manifest.json \"voice\" == am_michael (Kokoro engine id) / the RVC/GSO slug (D-D)."

# ---- MCP by-ear verify through the speak 'voice' arg (#147) ----
# Drives the production runSpeak (real pipeline + Kokoro + RVC worker + afplay)
# with NARRATE_SMOKE_VOICE set, so the MCP `speak` voice arg is exercised
# end-to-end — the acceptance-criterion-2 counterpart of rvc-sanity's CLI path.
# Default VOICE=cool-jahns (RVC 40 kHz); pass VOICE=<slug> for any roster voice.
MCP_VOICE ?= cool-jahns
mcp-voice-sanity:
	NARRATE_SMOKE_VOICE=$(MCP_VOICE) go test -tags manual -v -run TestSpeakManualSmoke ./cmd/narrate-mcp/...
	@echo "MCP by-ear (#147): heard $(MCP_VOICE) via the MCP speak 'voice' arg (runSpeak → BuildRenderer → RVC decorator → afplay)."

# ---- GPT-SoVITS (GSO) torch inference worker (#161) ----
# The ephemeral warm-load-once subprocess engine (#162 drives it later). Its venv is
# DEDICATED + torch-PRESENT (.venv-gso, NOT the shared .venv and NOT the torch-free
# .venv-rvc). Inverse of the RVC worker: the GSO worker infers WITH torch and asserts
# torch present at startup (explicit `if not importable: FATAL sys.exit(78)`, EX_CONFIG,
# unstrippable by `python -O`). Built with python3.11 (3.10-3.12 only; funasr/
# pyopenjtalk/jieba_fast target that range; NEVER system 3.14). Torch is reached ONLY
# across the stdin/stdout subprocess boundary — never linked into any Go binary.
GSO_VENV := .venv-gso
GSO_PY := $(GSO_VENV)/bin/python
PYTHON311 ?= python3.11

# Fetch the shared ~950MB base-model set (chinese-hubert-base + chinese-roberta-wwm-
# ext-large + the v2Pro sv embedding). Idempotent + size-sanity checked + fail-loud.
gso-fetch-base:
	scripts/gso-fetch-base

# Build .venv-gso with gotchas 1-4 baked as hardcoded recipe steps (NOT re-discovered):
#   1 install order — torch + torchaudio FIRST, before the rest resolves.
#   2 opencc build failure — filter a bare `opencc` pin out (keeping the reimpl), then
#     install opencc-python-reimplemented (same `opencc` import name).
#   3 torchcodec — pinned in gso-requirements.txt (arm64 wheel; needs Homebrew ffmpeg).
#   4 NLTK data — downloaded at BUILD time so the worker never hits network mid-job.
# Asserts torch PRESENT (inverse of rvc-worker-venv) + prints freeze for provenance.
gso-worker-venv:
	$(PYTHON311) -m venv $(GSO_VENV)
	$(GSO_PY) -m pip install --upgrade pip
	$(GSO_PY) -m pip install torch==2.13.0 torchaudio==2.11.0
	$(GSO_PY) -m pip install torchcodec==0.15.0
	grep -ivE '^opencc([[:space:]=<>!~]|$$)' scripts/gso-requirements.txt > $(GSO_VENV)/reqs.filtered.txt
	$(GSO_PY) -m pip install -r $(GSO_VENV)/reqs.filtered.txt
	$(GSO_PY) -m pip install opencc-python-reimplemented
	$(GSO_PY) -m nltk.downloader averaged_perceptron_tagger_eng averaged_perceptron_tagger cmudict punkt punkt_tab
	@$(GSO_PY) -c "import torch; print('torch present:', torch.__version__)" || { echo "FAIL: torch missing in $(GSO_VENV)"; exit 1; }
	@echo "ok: $(GSO_VENV) has torch. Freeze below — capture into scripts/gso-requirements.txt provenance:"
	@$(GSO_PY) -m pip freeze

# Torch-free contract gate — runs under a STOCK python3 (no venv/network/models):
#   * the wire/ERR-taxonomy + drift + minted-path + OK-after-write contract test
#     (includes the shlex golden round-trip fixture #162 must satisfy), AND
#   * the warmproof NEGATIVE DRY-CHECK — proves the AC5 determinism/distinctness oracle
#     CAN go RED on subtle + gross warm-state corruption and stale-cache reuse (and
#     stays GREEN on sub-threshold jitter) before it is ever trusted to pass green.
gso-contract-test:
	$(PYTHON3) scripts/gso_worker_contract_test.py
	$(PYTHON3) scripts/gso_warmproof.py --negative-dry-check

# AC5 warm-load CORRECTNESS smoke (needs .venv-gso + the real checkpoints + fetched
# base models). Feeds A,B,A to ONE warm worker, buffering each OK's WAV bytes before
# the next request (so A1 survives A2's idempotent overwrite on the shared content-
# addressed path), then asserts: LOAD-once, non-silent 32 kHz, determinism A1~=A2,
# distinct B!=A, and warm-vs-cold equivalence with DISTINCT GSO_OUT_DIRs. Latency is
# an informational note, not a gate.
gso-warmproof:
	@test -x $(GSO_PY) || { echo "no $(GSO_VENV) — run 'make gso-worker-venv'"; exit 2; }
	$(GSO_PY) scripts/gso_warmproof.py

# gso-sanity (#162 AC5) renders $(SAMPLE) at the GSO peer voice through the persistent
# sink → a 32 kHz audio.wav + manifest for the by-ear /verify. Real end-to-end: needs
# the GSO worker stack (make gso-worker-venv + make gso-fetch-base + a GSO_REPO clone
# of GPT-SoVITS). A missing worker STOPS non-zero (honesty rule: ErrWorkerMissing,
# never a silent Kokoro fallback). The unit suite (make test) does NOT depend on this
# — it drives the torch-free fake worker; this target is the manual Timeline-correctness
# smoke only (pronunciation quality is #164, not asserted here).
gso-sanity:
	@mkdir -p $(GSO_SANITY_OUT)/cool-jahns-gso
	go run ./cmd/narrate --file $(SAMPLE) --sink persistent --out $(GSO_SANITY_OUT)/cool-jahns-gso --voice cool-jahns-gso
	@echo "GSO sanity: wrote a 32 kHz cool-jahns-gso render under $(GSO_SANITY_OUT)"
	@echo "Verify (#162 AC5): afplay $(GSO_SANITY_OUT)/cool-jahns-gso/audio.wav; confirm one BlockTiming per plan block, monotonic non-overlapping offsets, EndMs-StartMs byte-consistent with the 32 kHz WAVs, and manifest.json \"voice\" == cool-jahns-gso (Timeline correctness only — pronunciation is #164)."

# ---- #164 go/no-go evidence: perf baseline + G2P coverage (machine-checkable) ----
# gso-perf-baseline (#164 AC4) records the OFFICIAL cold/warm-per-block/peak-RSS
# baseline by driving the REAL warm worker over $(SAMPLE) — a SEPARATE script from
# gso_warmproof.py (S2: warmproof is the correctness oracle, left byte-unchanged).
# cmd/plandump emits the engine-neutral plan.json (pure planner, NO worker, NO audio)
# so the worker is fed exactly the per-block spoken text the GSO renderer would send.
# Latency (cold 30s / warm ~20s) is INFORMATIONAL; peak RSS (8GB on a 16GB M1 Pro) is
# the HARD go/no-go input, sampled off the .venv-gso worker pid with the unified-memory
# caveat printed. A missing worker STOPS non-zero (honesty rule) → AC4 UNSATISFIED, and
# #164 does NOT substitute #165's smoke reading.
gso-perf-baseline:
	@test -x $(GSO_PY) || { echo "no $(GSO_VENV) — AC4 UNSATISFIED without the real worker; run 'make gso-worker-venv' (do NOT substitute #165's smoke number)"; exit 2; }
	@mkdir -p $(GSO_PERF_OUT)
	go run ./cmd/plandump --file $(SAMPLE) > $(GSO_PERF_OUT)/plan.json
	@echo "gso-perf-baseline: wrote engine-neutral plan.json (no worker/no audio) to $(GSO_PERF_OUT)/plan.json"
	$(GSO_PY) scripts/gso_perf_baseline.py --plan $(GSO_PERF_OUT)/plan.json | tee $(GSO_PERF_OUT)/baseline.txt
	@echo "gso-perf-baseline: baseline recorded at $(GSO_PERF_OUT)/baseline.txt (copy verbatim into the AC6 entry + PR body)."

# gso-g2p-check (#164 AC3, machine halves) surfaces two textual, NO-listening halves of
# the G2P coverage over $(GSO_G2P_DOC) at level $(GSO_G2P_LEVEL) (L3 so the table reads
# its header + rows deterministically): half A = Segment.Text per structured class from
# plan.json (pure planner, NO worker, NO audio); half B = the g2p_en ARPAbet phoneme
# STRING per Segment.Text (NEEDS .venv-gso). Any acoustic-only question is AC3-ear —
# staged for human ears, NEVER recorded here as a machine verdict. NO golden/fixture
# assertion (a #164 non-goal). Half A runs with or without .venv-gso; half B is skipped
# (with a loud notice) when the worker venv is absent.
gso-g2p-check:
	@mkdir -p $(GSO_G2P_OUT)
	go run ./cmd/plandump --file $(GSO_G2P_DOC) --level $(GSO_G2P_LEVEL) > $(GSO_G2P_OUT)/plan.json
	@echo "gso-g2p-check: wrote plan.json (half A Segment.Text source; no worker/no audio) to $(GSO_G2P_OUT)/plan.json"
	@if [ -x $(GSO_PY) ]; then \
		echo "gso-g2p-check: .venv-gso present — running half A + half B (g2p_en phoneme strings)"; \
		$(GSO_PY) scripts/gso_g2p_dump.py --plan $(GSO_G2P_OUT)/plan.json | tee $(GSO_G2P_OUT)/g2p.txt; \
	else \
		echo "gso-g2p-check: no $(GSO_VENV) — half B SKIPPED (needs 'make gso-worker-venv'); surfacing half A Segment.Text only"; \
		$(PYTHON3) scripts/gso_g2p_dump.py --plan $(GSO_G2P_OUT)/plan.json --no-phonemes | tee $(GSO_G2P_OUT)/g2p.txt; \
	fi
	@echo "gso-g2p-check: coverage surfaced at $(GSO_G2P_OUT)/g2p.txt (record the 5 target classes into docs/samples/gso-g2p-coverage.md; acoustic residue → AC3-ear, human-owned)."
