# SESSION — narrate-block-render-id-state-110 (ticket #110) — CLOSED

**Status:** COMPLETE. GATE 1 approved by user → Option 1. Decision recorded, follow-up
tickets created/updated, research PR raised. This file is retained as the on-disk research
trail (frame + verdicts + recommendation) because the harness forbade writing report.md;
the full narrative report was delivered inline to the user.
**Outcome:**
- Decision: decisions/architecture/2026-06-28-post-narrate-block-escalation-persists-a-3-file.md
  (supersedes the single-wav claim in 2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md)
- Tickets: #110 annotated with Option 1 + AC3 (build resumes here); #125 (dev/high) dir-aware
  reaper/orphan/serve; #126 (research/low) dormant content-hash escalation cache.
- Branch: research/110-narrate-block-render-id-state.
**Step:** 5→9 done. (Was: STAGE 1-3 COMPLETE; awaited GATE 1.)
**Date:** 2026-06-28

## CONTEXT RECOVERY (the prior agent ad40e36f03c94ec1d lost its frame — never persisted)
Prior agent held Stage 0 frame + SQ1-SQ5 in context only; nothing was on disk. This agent
RECONSTRUCTED the frame from ground truth (the #110 ticket, the named Go source, and the
journaled #109/#49/#28/#14/#72-73 decisions). User decision at Checkpoint 1 was GO; "switch
to 3-file sink" was kept SOFT (candidate), genuinely weighed against in-memory / lazy-
materialization / content-hash-cache.

## FRAME (reconstructed)
#110 = POST /narrate/block {render_id, block_id, level}: re-render ONE block of a prior
POST /narrate render, byte-preserve the rest, reuse persistent.PatchBlock. Crux: /narrate
(#109) persists ONLY a combined wav + a createdAt; PatchBlock needs a full 3-file dir.
SQ1 reuse-fit · SQ2 storage model · SQ3 GC/lifecycle · SQ4 security/coupling · SQ5 endpoint
shape + supersession/blast-radius. All 3 ACs covered.

## VERDICTS (all load-bearing claims verified)
- POSIX open-fd survival across unlink (Linux+macOS): VERIFIED.
- os.Rename atomic same-dir/same-fs (Linux+macOS), EXDEV cross-fs, durability needs fsync: VERIFIED.
- PatchBlock requires all 3 files (ErrNothingToPatch on absence); byte ranges DERIVED from
  manifest timing: VERIFIED (passing patch tests).
- /narrate persists ONLY combined wav + createdAt; plan/timeline/per-block wavs discarded;
  source text NOT stored either: VERIFIED.

## RECOMMENDATION (presented to user)
Option 1 — write a 3-file persistent-sink dir per render_id under tempRoot/{render_id}/,
keyed by render_id (NOT a user dir); reuse PatchBlock + readBack; serve via existing
/audio/{render_id}.wav; return escalateResponse field set with audio_url (not audio_ref+dir).
Supersedes the single-wav claim in the #109 render-id-lifecycle decision; trips (not
violates) the WAVFileSink-no-sidecars revisit trigger. Option 2 (in-memory) reopens seam-gap
R1 + high RSS; Option 3 (lazy) is a trap (discarded plan/source + new crash window +
file/dir heterogeneity). Content-hash escalation cache NOT recommended phase one (#14 already
rejected per-block hashes; regen is sub-100ms).

## ON NEXT RESUME / parent action
1. If user picked an option at Gate 1 → record decision-journal entry for it (supersede the
   #109 render-id-wav-lifecycle decision's single-wav claim), then create follow-up tickets,
   then (optional) PR on branch research/110-narrate-block-render-id-state.
2. The build can then resume #110 against the chosen option.
3. Reaper/orphan/serve rework spots for Option 1: audiostore.go:203 (os.Remove->os.RemoveAll),
   :117 (open path -> {id}/audio.wav), :209-229 (orphan scan dir-aware). narrate.go:78-81,140-145
   (WAVFileSink -> 3-file persistent.Sink; stop RemoveAll'ing per-block state).
