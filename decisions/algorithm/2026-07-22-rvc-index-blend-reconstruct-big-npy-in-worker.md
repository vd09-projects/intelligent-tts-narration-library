# RVC index-blend reconstructs big_npy in-worker from the .index (reconstruct_n + make_direct_map fallback), not the index_vectors.npy artifact

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-22       |
| Status   | accepted         |
| Category | algorithm        |
| Tags     | rvc, voice-conversion, faiss, ivfflat, index-blend, reconstruct-n, make-direct-map, big-npy, index-vectors-npy, engine-faithful, go-knn-deferred, issue-143, issue-144, issue-145 |

## Context

The RVC index-blend stage (PILOT_REPORT §4.5b) needs the source vectors `big_npy[ix]` for the k=8 IVFFlat neighbours it retrieves, to compute `npy = Σ big_npy[ix]·w` and blend `feats = index_rate·npy + (1-index_rate)·feats`. There are **two** ways to get those vectors, and #143 export tooling produced artifacts for both paths:

- `assets/rvc-models/<slug>/<Name>.index` — the original faiss IVFFlat index the pilot searched directly.
- `assets/rvc-models/<slug>/onnx/index_vectors.npy` — a `reconstruct_n` dump of the same vectors, produced by #143 export tooling.

The `index_vectors.npy` dump is a large artifact (the parent decision notes ~557MB of index vectors). The open question at plan time (the one remaining OQ after round 1) was whether the `.index` still carries reconstructable vectors so `big_npy` can be rebuilt in-worker without also loading the `.npy` dump.

## Options considered

### Option A: load index_vectors.npy as the blend source
- **Pros**: a flat array, no faiss reconstruct call.
- **Cons**: duplicates data already inside the `.index` the worker must open anyway (it has faiss and searches the `.index` directly, engine-faithful to the pilot); the `.npy` dump is explicitly reserved for the FUTURE in-process Go kNN path (Option D endgame), where Go has no faiss to reconstruct from. Using it in the Python worker blurs that separation.

### Option B: reconstruct big_npy in-worker from the .index — CHOSEN
- **Pros**: engine-faithful to the pilot (the Python worker HAS faiss and searches the `.index` directly, so the vectors are already reachable); no second large artifact loaded into the warm worker; keeps `index_vectors.npy` cleanly reserved for the future Go kNN fallback where faiss reconstruct isn't available.
- **Cons**: depends on the IVFFlat index carrying (or being able to build) a direct map.

## Decision

The worker reconstructs `big_npy` **in-process from the original `.index`** via faiss `reconstruct_n`, and if the IVF index lacks a direct map it enables one at load with `make_direct_map()`. It does **not** load `index_vectors.npy` — that `reconstruct_n` dump stays reserved for the deferred in-process Go kNN path (#145's Option-D endgame), where Go cannot call faiss reconstruct. Both resolutions are torch-free and cheap; the `.index` was confirmed reconstruct_n-able, and the build implemented the blend with `big_npy` held as warm read-only cross-call state alongside the ORT sessions, index, and melbasis. Parity is judged by full-pipeline log-mel corr ≥0.98 (not raw samples), so faiss IVFFlat search not being bit-identical to torch is an accepted sub-0.999-raw-corr noise floor, not a defect.

## Consequences

- The warm worker holds one big_npy per loaded voice (both if a job mixes voices), reconstructed once and reused — no per-block reconstruct.
- `index_vectors.npy` remains an unused-by-the-worker artifact whose sole consumer is the future Go kNN path; a future engineer seeing two big_npy sources should not "simplify" by pointing the worker at the `.npy` — that would couple the worker to the artifact meant for the Go fallback.
- If a future faiss/index format drops direct-map support, the documented fallback is to load `index_vectors.npy` instead (still torch-free).

## Related decisions

- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent decision; its deferred Option D (in-process Go kNN) is exactly what `index_vectors.npy` is reserved for, which is why the Python worker deliberately does not consume it.

## Revisit trigger

Reconsider when the in-process Go kNN path (Option D) is built (#145's deferred endgame / the sherpa-onnx-go CGo migration) — at that point `index_vectors.npy` becomes the live blend source for Go, and the Python-worker reconstruct path may be retired. Also revisit if a faiss upgrade changes `reconstruct_n`/`make_direct_map` availability for IVFFlat indexes.
