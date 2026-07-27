# GSO warm load needs os.chdir(GSO_REPO) into an external clone; the v2Pro TTS_Config sv_path/cnhubert keys are inert and were masked by chdir

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | gso, gpt-sovits, warm-load, chdir, gso-repo, sv-path, cnhubert, inert-config-keys, stdout-wire-isolation, fixed-seed, determinism, offline, m1-pro, machine-run, issue-165, issue-162 |

## Context

#161 shipped a torch-free-buildable GSO worker (`scripts/gptsovits_worker.py`) whose
`_GsoPipeline._build()` copied the config dict from `docs/gpt-sovits-inference-runbook.md`
verbatim — but dropped the runbook's `os.chdir(REPO)` preamble. #165 ran it on the real
M1 Pro against the real cool-jahns-gso checkpoints for the first time. The warm load
would not resolve until several gaps (all masked in the runbook by its `os.chdir`) were
fixed. This decision records what the machine run proved, because #162 (the Go peer
engine) binds to it.

## Options considered

### Option A: rely on the TTS_Config dict keys alone (runbook-style, no chdir)
- **Pros**: minimal; looks like the documented runbook config.
- **Cons**: PROVEN to silently no-op on the machine run. `from GPT_SoVITS...` isn't
  importable without the clone on `sys.path`; `GPT_SoVITS/sv.py` does
  `sys.path.append(f"{os.getcwd()}/GPT_SoVITS/eres2net")` + a module-global CWD-relative
  `sv_path`; `TTS_Config` never consumes `sv_path` (`SV(device, is_half)` takes no path
  arg) and reads `cnhuhbert_base_path` (extra `h`), so the runbook's `cnhubert_base_path`
  key was inert. All of this only worked in the runbook because `os.chdir(REPO)` made the
  CWD-relative defaults resolve against the clone.

### Option B: replicate the runbook's chdir + fix the inert keys explicitly (chosen)
- **Pros**: warm load resolves deterministically; base models come from the fetched
  `_base` (AC1) rather than the clone's copy; behavior is explicit, not accidental.
- **Cons**: couples the worker to an external clone path; needs a small amount of
  fd/stdout plumbing.

## Decision

The worker resolves an EXTERNAL GPT-SoVITS code clone via env `GSO_REPO` (default
`~/repos/GPT-SoVITS-local`), `os.chdir`es into it, and inserts it on `sys.path` before
importing GPT-SoVITS. Only CODE comes from the clone; all WEIGHTS stay under
`assets/gptsovits-models/`. On top of that, four inert/hidden behaviors are handled
explicitly:

1. **`sv_path` is INERT** — the SV embedding loads from `sv.py`'s module-global
   `sv_path`, not the config. The worker overrides `sv.sv_path` to the fetched `_base`
   embedding so it resolves deterministically.
2. **`cnhuhbert_base_path`** (not `cnhubert_base_path`) is the key TTS.py reads — fixed,
   so CNHuBERT loads from `_base`.
3. **stdout is a noise sink** — GPT-SoVITS prints copiously to stdout, which buries/loses
   the `OK`/`ERR` wire. The worker preserves the real stdout for the wire and shunts
   fd 1 → stderr; `run_loop` uses `readline()` so an interactive caller can't deadlock on
   text-iterator read-ahead.
4. **fixed `seed=42`** (+ runbook sampling params) — without it GPT-SoVITS randomizes the
   seed, which breaks the AC5 determinism/warm-vs-cold oracles.

## Consequences

- **#162 must** launch the worker with `GSO_REPO` available, expect 32 kHz output, read
  ONLY the fd-1 `OK`/`ERR` wire (all other lines are noise on stderr), and not assume the
  config-dict keys are honored.
- Deployment now has an external-clone dependency (documented in
  `assets/gptsovits-models/README.md`). A future hardening could vendor a pinned subset
  of the GPT-SoVITS code behind the same subprocess boundary.
- `os.chdir` is a process-global side effect; safe here because all worker-owned paths
  (out_base, models_root) are absolute and resolved at startup before the chdir.

## Related decisions

- [GSO worker wire contract is RVC-shaped but NOT verbatim](2026-07-27-gso-worker-wire-contract-rvc-shaped-not-verbatim.md) — this decision adds the runtime resolution + wire-isolation details that make that frozen wire contract actually work on hardware.
- [GSO warm-load proof is a correctness oracle](../convention/2026-07-27-gso-warmproof-warm-reuse-correctness-oracle.md) — the fixed-seed determinism fix is what makes that oracle pass green.

## Experiments

Machine run (M1 Pro, 16GB, macOS, MPS), `make gso-warmproof` on real cool-jahns-gso
checkpoints:
- (a) LOAD-once ✓  (b) non-silent 32 kHz ✓  (c) determinism A1≡A2 RMS 0.0, distinct B≠A ✓
  (d) warm-vs-cold RMS 0.0 ✓ → PASS.
- Perf baseline: cold request-1 ~17–22s (≤30s), warm/block ~3.6–3.8s (≤20s), peak RSS
  ~3.5 GB (≤8GB). Offline: warm load succeeds with an empty HF cache (nothing fetched).

## Revisit trigger

If GPT-SoVITS is upgraded/re-cloned and `TTS_Config` starts consuming `sv_path` /
`cnhubert_base_path`, or `sv.py` stops using CWD-relative paths, revisit whether the
chdir + overrides are still needed. Also revisit if the worker is ever packaged to vendor
the GPT-SoVITS code (removes the `GSO_REPO` clone dependency).
