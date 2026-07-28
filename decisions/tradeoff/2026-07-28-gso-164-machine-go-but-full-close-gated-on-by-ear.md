# GSO #164 go/no-go — machine-evidence GO on the hard RAM ceiling, but the full close is gated on human by-ear

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-28       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | gso, gpt-sovits, go-no-go, by-ear, peak-rss, ram-ceiling, latency, g2p-en, pronunciation-ceiling, mcp-blocked, honesty-over-optimism, cool-jahns-gso, issue-164 |

## Update 2026-07-29 — AC2 unblocked inline (supersedes the "AC2 BLOCKED" snapshot below)

The AC2 blocker was a renderer per-block timeout that was too short for a legit
long verbatim-prose block, and the smoke test hardcoding its source. Per user
direction, both were folded into PR #170 **inline** this session rather than
deferred to follow-ups:

- `render/gptsovits/config.go`: `defaultPerBlockTimeout` **30s → 5min**
  (first-block 45s → 6min, wall-clock 10min → 15min). Generous-local bound — a
  genuinely stuck worker still errors, without falsely killing the measured
  161s worst-case legit block. Latency stays informational, never a gate.
- `cmd/narrate-mcp/mcp_manual_smoke_test.go`: added `NARRATE_SMOKE_SOURCE`
  override (+ `MCP_SOURCE` Makefile var) so the MCP by-ear can target a short
  doc.

Result: `make mcp-voice-sanity MCP_VOICE=cool-jahns-gso MCP_SOURCE=<short doc>`
now **PASSES** (65.51s, 5 blocks voiced through the GSO peer via the MCP `speak`
path). **AC2 is now STAGED, pending human ears — same status as AC1**, not
blocked. The go/no-go verdict below is unchanged (GO on the RAM gate). The two
g2p_en pronunciation ceilings remain documented-only, not folded in.

## Context

#164 (GSO P4) is the go/no-go gate + verify close-out of the GPT-SoVITS integration —
the final GSO chain item (#161/#162/#163/#165 done). AC6 requires this decision be
recorded **regardless of outcome**, mirroring the #147 honesty-over-optimism precedent.
This is an honest-ceiling outcome, not a clean pass: the machine-checkable evidence
clears the hard resource gate, but the artifact's acoustic sign-off is human-owned and
cannot be a machine verdict.

The gate has two kinds of input that must not be conflated: **machine-checkable**
signals (peak RSS, latency, g2p coverage, objective audio signals) that the agent can
measure this session, and **by-ear** judgments (character likeness, whether the g2p
ceilings actually degrade the audio, whether gists read intelligibly) that only a human
listener can render. The go/no-go decision runs on the machine inputs; the by-ear
judgments are explicitly **not** inputs to it.

## What was measured (machine-checkable)

### Peak RSS — the HARD go/no-go input → GO

Official `make gso-perf-baseline` (M1 Pro, over `docs/samples/sample.md`, 15 blocks,
2026-07-28): **peak RSS 2.13 GB vs the ≤8 GB ceiling → GO**, with comfortable headroom.
Cross-checked against getrusage `ru_maxrss` at **2.12 GB**. Apple-Silicon
unified-memory / MPS caveat noted — RSS can under-count GPU-side allocations, so
near-ceiling reads are reported conservatively; at 2.13 GB the headroom is not
near-ceiling, so the caveat does not flip the verdict.

### Latency — INFORMATIONAL, never a gate

Per the warm-load-proof correctness-oracle standing order, latency is recorded but never
gates go/no-go. Cold load **22.67s** (≤30s, OK). Warm-per-block mean **24.49s** is
**OVER** the ~20s advisory ceiling — driven by long verbatim-prose outliers
(block 3 = **161.12s**, block 1 = **59.80s**; median ~3s; spread **2.41–161.12s**).
Documented honestly; not a blocker.

### G2P coverage (AC3) — mechanized, no listening

**Half A (Segment.Text correctness from `plan.json`): PASS** across all 5 structured
classes — spelled-out numbers (incl. 24,700 → "twenty-four thousand seven hundred", and
the 6-digit spell-out ceiling leaving 1,000,000 as digits); code gist; list item-by-item;
table header + rows at L3; heading prefixed "Section:".

**Half B (g2p_en phoneme-string vs expected ARPAbet, textual): two documented CEILINGS.**
This is the NEW risk RVC never had — RVC inherited Kokoro's pronunciation for free; GSO's
own g2p_en does not.
- **CEILING 1 — hyphenated spelled-out cardinals mis-phonemized.** "twenty-four" →
  …F UH1 R vs the correct F AO1 R; "forty-two" wrong; the code-gist "4-line" also wrong.
  The **hyphen is the trigger** — bare "four" is correct.
- **CEILING 2 — technical tokens / identifiers mangled.** "L1" → "lone"; "32 kHz",
  "GPT-SoVITS", "cool-jahns-gso" garbled.

## What is staged / blocked (by-ear, human-owned — never a machine verdict)

- **AC1 CLI by-ear: STAGED, pending human ears.** `audio.wav` produced; objective signals
  all pass — 32 kHz mono s16le, manifest `voice == cool-jahns-gso`, one BlockTiming per
  block, non-silent (RMS 0.104 / peak 0.883).
- **AC2 MCP by-ear: BLOCKED, not staged.** `make mcp-voice-sanity MCP_VOICE=cool-jahns-gso`
  failed at 49.66s — `TestSpeakManualSmoke` hardcodes `docs/samples/sample.md`, whose
  long-prose blocks exceed the renderer's 30s per-block timeout (context deadline
  exceeded). Half-staged, named honestly, not faked → follow-ups (timeout scaling;
  smoke-source override).
- **All acoustic judgments** — Jeremy Jahns character likeness, whether the g2p_en
  ceilings actually degrade the audio, whether gists read intelligibly — are PENDING
  HUMAN EARS and are explicitly NOT inputs to the machine go/no-go.

## Decision

Ship the faithful, machine-checkable work behind the human by-ear gate — the #147
disposition, applied again.

**Machine go/no-go = GO.** The hard go/no-go input is peak RSS: the official
`make gso-perf-baseline` run (M1 Pro, over `docs/samples/sample.md`, 15 blocks,
2026-07-28) measured peak RSS **2.13 GB** against the **≤8 GB** ceiling — comfortable
headroom, cross-checked by getrusage `ru_maxrss` at **2.12 GB** (the Apple-Silicon
unified-memory / MPS caveat is noted: RSS can under-count GPU-side allocations, so
near-ceiling reads would be reported conservatively; at 2.13 GB the headroom is not
near-ceiling). Latency is informational, never a gate (warm-load-proof correctness-oracle
standing order): cold load **22.67s** (≤30s, OK); warm-per-block mean **24.49s** is OVER
the ~20s advisory ceiling, driven by long verbatim-prose outliers (block 3 = **161.12s**,
block 1 = **59.80s**; median ~3s; spread **2.41–161.12s**) — documented, not a blocker.
G2P coverage (AC3, mechanized) passes half A (Segment.Text correctness across all 5
structured classes) and documents two half-B g2p_en phoneme ceilings (hyphenated
spelled-out cardinals mis-phonemized; technical tokens / identifiers mangled) — the new
risk RVC never carried, since RVC inherited Kokoro's pronunciation for free while GSO's
own g2p_en does not. The GO is a machine verdict only; the FULL close depends on the
human's by-ear sign-off of the CLI artifact — the CLI by-ear leg (AC1) is staged with all
objective signals passing, the MCP by-ear leg (AC2) is BLOCKED (not staged) on a
per-block-timeout failure, and no by-ear pass is recorded that the agent cannot hear.

Character/quality fixes (the g2p_en ceilings, likeness) and the MCP leg spin out to
follow-ups. Honesty over optimism — the latency over-run and the g2p_en ceilings are
documented, not fabricated blockers, and no by-ear pass the agent cannot hear is claimed.

## Consequences

- #164's machine-checkable deliverables are complete: the RAM go/no-go is GO with
  headroom, and the CLI artifact is staged for the human to hear.
- The **full close is not machine-closable** — it waits on the human's by-ear sign-off of
  the CLI `audio.wav`. That sign-off is the acceptance criterion "hear it in the
  cool-jahns-gso voice", and it is deferred, not skipped.
- The **MCP by-ear leg (AC2) is blocked**, not merely unstaged: the smoke test's hardcoded
  long-prose source trips the 30s per-block renderer timeout. Follow-ups: scale the
  per-block timeout and/or let the smoke test override its source.
- **g2p_en pronunciation is a standing GSO risk** with no RVC analogue. Any future
  "GSO voice mispronounces X" report should first check whether X is a hyphenated
  spelled-out cardinal or a technical identifier — the two documented ceilings — before
  suspecting the renderer or roster.
- Latency being over the advisory ceiling on long verbatim-prose blocks is recorded but
  does not gate; if it later becomes a product concern it is a separate performance ticket.

## Related decisions

- [RVC pipeline is objectively faithful to torch, but a Kokoro-synthetic source ≠ recognizable target voice by ear](2026-07-25-rvc-faithful-pipeline-but-synthetic-source-mismatch.md) — the #147 precedent this mirrors: ship the machine-faithful work, defer the by-ear character sign-off, honesty over optimism. #164 applies the same disposition to the GSO engine.
- [GSO warm-load proof is a correctness oracle, not a latency check](../convention/2026-07-27-gso-warmproof-warm-reuse-correctness-oracle.md) — the standing order that makes latency informational-only and never a go/no-go gate.
- [GSO warm load needs os.chdir(GSO_REPO); v2Pro TTS_Config keys are inert](../architecture/2026-07-27-gso-warm-load-chdir-repo-inert-config-keys.md) — established the perf-baseline expectations (cold ≤30s, warm/block, peak RSS ≤8 GB) and the 32 kHz output this gate re-measured on the official baseline.

## Experiments

- **Perf baseline** (`make gso-perf-baseline`, M1 Pro, `docs/samples/sample.md`, 15 blocks,
  2026-07-28): peak RSS **2.13 GB** (getrusage `ru_maxrss` **2.12 GB**) vs ≤8 GB → GO;
  cold load **22.67s** (≤30s); warm-per-block mean **24.49s** (advisory ~20s), outliers
  block 3 **161.12s** / block 1 **59.80s**, median ~3s, spread **2.41–161.12s**.
- **G2P coverage (AC3)** — half A Segment.Text from `plan.json`: PASS all 5 structured
  classes (24,700 → "twenty-four thousand seven hundred"; 1,000,000 left as digits at the
  6-digit ceiling; code gist; list item-by-item; table header+rows at L3; heading
  "Section:"-prefixed). Half B g2p_en phoneme vs ARPAbet: CEILING 1 hyphenated cardinals
  ("twenty-four" → …F UH1 R vs correct F AO1 R; "forty-two"; "4-line"; bare "four" correct
  — hyphen is the trigger); CEILING 2 technical tokens ("L1" → "lone"; "32 kHz";
  "GPT-SoVITS"; "cool-jahns-gso" garbled).
- **AC1 CLI artifact** (staged, pending ears): `audio.wav` — 32 kHz mono s16le, manifest
  `voice == cool-jahns-gso`, one BlockTiming per block, non-silent RMS **0.104** / peak
  **0.883**.
- **AC2 MCP** (`make mcp-voice-sanity MCP_VOICE=cool-jahns-gso`): FAILED at **49.66s** —
  `TestSpeakManualSmoke` hardcodes `docs/samples/sample.md`; long-prose blocks exceed the
  30s per-block renderer timeout (context deadline exceeded). Blocked, not staged.

## Revisit trigger

When the human returns a by-ear verdict on the staged CLI `audio.wav`: a pass closes
#164's acceptance and this tradeoff no longer binds; a fail re-scopes the character work.
Also revisit when the MCP smoke-source / per-block-timeout follow-ups land (AC2 becomes
stageable) or when the two g2p_en ceilings are addressed (hyphenated-cardinal and
technical-identifier phonemization).
