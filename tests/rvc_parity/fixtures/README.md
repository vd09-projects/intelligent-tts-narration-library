# RVC parity gate fixtures (issues #144 / #151)

The always-on full-pipeline log-mel gate (`make rvc-parity`) compares the
torch-free ONNX worker's output against a torch-reference log-mel target derived
from a fixed source clip. Those binaries are **not committed** — the repo forbids
`.wav`/`.npy` binaries and the source may be personal audio — so they are
**fetched from a pinned GitHub Release** and hashlib-verified before the gate runs.

## The bundle — three binaries (single voice)

`make rvc-parity` depends on `make rvc-fixtures-fetch`, which downloads and verifies:

| file | what | how produced |
|---|---|---|
| `source.wav` | the fixed source clip fed to BOTH the torch reference and the ONNX worker | a public, permissively-licensed speech clip (provenance recorded below) |
| `cool-jahns_ref.wav` | torch-reference output for `cool-jahns` | `assets/rvc-models/rvc-convert.sh cool-jahns source.wav …` (Applio torch venv) |
| `cool-jahns_logmel_target.npy` | pinned log-mel of `cool-jahns_ref.wav` — the assertion target | `gen_targets.py` (loaded with `allow_pickle=False`) |

(An `ATTRIBUTION.txt` also travels in the bundle if the source clip or the voice
model's license requires attribution — release consumers never see this README.)

### Why one voice (`confident-neal` excluded)

`make rvc-parity` is a **pipeline FLOW gate**, not a per-voice correctness matrix:
it proves the RVC conversion *pipeline* reproduces byte-for-byte on a fresh clone.
One voice — `cool-jahns` — exercises the entire path (source → `rvc-convert.sh` →
ONNX worker → log-mel compare), so shipping a second voice would double the bundle
for zero added flow coverage. `confident-neal` is therefore **deliberately dropped**
from the bundle. This is the single canonical statement of that rationale — the
code points here rather than repeating it (`EXCLUDED_PARITY_VOICES` in
`parity_voices.py`, the `PARITY_VOICES` narrowing in `parity_test.py`).

The drop is matrix-narrowed, not skipped: `parity_test.py` parametrizes over
`PARITY_VOICES = ("cool-jahns",)`, so `confident-neal`'s full-pipeline case is never
generated (no silent per-run skip), and `test_parity_voice_coverage` fails loud if
any repo voice is absent from both `PARITY_VOICES` and `EXCLUDED_PARITY_VOICES`.
**By design, this gate will NOT catch a `confident-neal`-specific conversion
regression** — only the shared pipeline is covered; a voice-specific regression in
`confident-neal` would need its own fixture, which #151 intentionally does not ship.

## License gate (D0) — status: NOT CLEARED (publishing blocked)

Before anything is hosted, the `cool-jahns` voice-model license must permit
**public redistribution of converted output** — the `cool-jahns_ref.wav` is
converted output and `cool-jahns_logmel_target.npy` is a log-mel derivative of it.

**Current disposition — not cleared:** per `assets/rvc-models/README.md`,
`cool-jahns` is an RVC voice **cloned from ~59 min of a real, named public figure**,
there is **no license, consent, or model-card file** anywhere under
`assets/rvc-models/`, and the models live only in a **private** HF backup repo
(`vd09-projects/rvc-voices`). No permission to publicly redistribute the cloned
voice's converted output is on record. **Until a redistributable voice replaces
`cool-jahns` (or explicit redistribution rights are obtained), the bundle must NOT
be published:** `make rvc-fixtures-publish` refuses to run without an explicit
`I_HAVE_CLEARED_D0=1` override, and `fixtures.sha256` ships empty (no pins), so
`make rvc-fixtures-fetch` fails loud rather than fetching unlicensed output.

## Trust path — `fixtures.sha256`

`tests/rvc_parity/fixtures.sha256` (committed, text) is the **trust root**, not the
release URL: GitHub release assets are owner-mutable, so integrity is guaranteed by
the committed SHA-256 pins, verified by the ONE shared `hashlib` implementation in
`fixtures_io.py` (platform-neutral — never GNU `sha256sum`, which is absent on
macOS). The fetch stages each download into a `.part` temp **inside** this dir
(same filesystem), verifies it, then `os.replace`s it atomically into place; any
404 / bounded-timeout hang / checksum mismatch / present-but-divergent file is a
hard, non-zero failure — never a silent green.

## Fetching (fresh clone)

```
make rvc-worker-venv     # once: build .venv-rvc (torch-free)
make rvc-parity          # runs rvc-fixtures-fetch first, then the gate
```

`rvc-fixtures-fetch` is idempotent: a present-and-valid fixture is skipped with no
network. A present-but-divergent fixture (bytes ≠ pin) is a hard fail telling you to
`rm` it and re-fetch — it is never silently overwritten.

**While the release is unpinned (blocked by the D0 gate above), there is no bundle to
download.** In that state `rvc-fixtures-fetch` falls back to your **local** fixtures:
if the full bundle set is present it runs the gate against them but prints a **loud
"unverified — not checked against a published checksum" notice** (never a silent
green); if any is missing it **fails loud** and points you at
`make rvc-parity-gen SOURCE=<clip.wav>` to regenerate locally (ungated, gitignored).
So the interim local-fixtures gate is runnable via `make rvc-parity` itself — the
loud notice is your reminder that a green run is not yet reproducible from a published
bundle.

> **The 3-file bundle is necessary but NOT sufficient for a full `make rvc-parity`.**
> `test_perstage_parity` also loads the per-voice `net_g.onnx` + `net_g_refio.npz`
> (and the shared contentvec/rmvpe ONNX + `*_refio.npz`) from the **gitignored**
> `assets/rvc-models/` tree — those per-stage models are **not** in this bundle and
> are **not hosted** (they are themselves D0-gated voice models). A fresh clone with
> only the fetched bundle therefore still fails at the per-stage stage until those
> models are present locally. The full fresh-clone-verify follow-up tracks this.

The stdlib-only machinery behind all of this (fetch/verify + the single-voice honesty
partition) has its own always-runnable smoke suite: `make rvc-fixtures-test` (stock
`python3`, no venv/network/fixtures needed).

## Re-publishing (maintainer)

Changing `logmel.py` params or the source clip means a new bundle:

```
# after D0 is cleared for a redistributable voice:
I_HAVE_CLEARED_D0=1 make rvc-fixtures-publish SOURCE=<vetted-public-clip.wav> \
  RVC_PUBLISH_TAG=rvc-parity-fixtures-v1
```

This regenerates the `PARITY_VOICES` bundle (`gen_targets.py --bundle`), pins it via
`fixtures_io.py`, and `gh release create`s a new tag. Then pin `RVC_FIXTURES_TAG` +
`RVC_FIXTURES_BASEURL` (Makefile) **and** the updated `fixtures.sha256` in the **same
commit**, and do not merge that commit until the release is live and fetchable (the
Step 8 merge gate) — otherwise a fresh clone 404s at the pinned tag.

## Source clip provenance

> Recorded here once a public, permissively-licensed clip (Apache/CC/public-domain)
> is selected and vetted (plan Step 1): exact source URL, license, and upstream
> SHA-256. Not personal / E_baseline audio.
