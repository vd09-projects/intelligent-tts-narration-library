// useNarrationSession.ts — THE single owner of shared narration state.
// Entry-based model: each (session message / file) gets its own NarrationEntry
// keyed by a stable string. Multiple entries can be loading in parallel.
// Selecting an entry switches the transcript view. Audio follows the selection.
//
// Per-block leveling (#113): setBlockLevel re-narrates ONE block at a new
// fidelity via POST /narrate/block and swaps its audio in place. The server
// rewrites the single combined wav and shifts every downstream offset, so the
// client adopts the returned full `timeline` WHOLESALE (no client-side offset
// math). A per-(key,blockId,level) snapshot cache — seeded for every block on
// every transcript set, each level paired with its authoritative timeline —
// makes returning to any previously-seen level a zero-network, zero-rebill swap.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ClientError, postNarrate, postNarrateBlock, postNarrateFile } from "../api/client";
import type { Block, Gender, Level, NarrateResponse, Timeline } from "../api/types";
import { deriveActiveBlockId } from "../state/activeBlock";
import { useAudio } from "./useAudio";

const NARRATE_TIMEOUT_MS = 120_000; // 2 minutes

export interface NarrationEntry {
  status: "loading" | "ready" | "error";
  transcript: NarrateResponse | null;
  error: string | null;
}

/** Per-block leveling UI state, keyed `${entryKey}::${blockId}`. */
export interface BlockLevelingState {
  /** idle (incl. just-committed), loading (POST in flight), error, or an
   *  escalation-returned refusal surfaced inline (block NOT flipped to refused). */
  phase: "idle" | "loading" | "error" | "refused-inline";
  /** Inline message: a polite "now at Ln" (idle) or an assertive error/refusal. */
  message: string | null;
  /** In-flight target level — held as aria-checked across the async window. */
  target: Level | null;
}

/** A cached block render paired with the timeline authoritative AT that level.
 *  The block's own offsets live inside `timelineSnapshot.blocks` (keyed by
 *  block_id); the cache-hit restore re-applies that whole timeline, so no
 *  separate per-block `timing` is stored. */
interface LevelSnapshot {
  block: Block;
  timelineSnapshot: Timeline;
}

const IDLE_LEVELING: BlockLevelingState = { phase: "idle", message: null, target: null };

export interface NarrationSession {
  /** All narrated items keyed by caller-supplied stable key. */
  entries: Map<string, NarrationEntry>;
  /** Which entry is currently displayed in the transcript pane. */
  selectedEntryId: string | null;
  /** Derived selected entry (null when nothing selected). */
  selectedEntry: NarrationEntry | null;
  /** Derived transcript for the selected entry. */
  currentTranscript: NarrateResponse | null;
  /** Switch the transcript view to an existing entry. */
  selectEntry: (key: string) => void;
  /** DERIVED active block id (aria-current), null when none under playhead. */
  activeBlockId: string | null;
  /** Per-block leveling status keyed `${entryKey}::${blockId}`. */
  blockLeveling: Map<string, BlockLevelingState>;
  // playback (from useAudio)
  audioRef: React.RefObject<HTMLAudioElement>;
  activeAudioUrl: string | null;
  playbackState: ReturnType<typeof useAudio>["playbackState"];
  audioError: string | null;
  play: () => void;
  pause: () => void;
  /** Seek the shared clip to a position in ms (block-start, re-derived live). */
  seek: (ms: number) => void;
  /** Seek the shared clip to a block's start and play. */
  playFromBlock: (blockId: string) => void;
  /**
   * Narrate text keyed by `key` (e.g. message.id).
   * If an entry for `key` already exists and is loading, the call is a no-op.
   * Otherwise starts a new narration and auto-selects the entry.
   */
  narrateText: (key: string, text: string, level: Level, gender?: Gender) => Promise<void>;
  /**
   * Narrate a file keyed by `key` (e.g. file.name).
   * Same idempotency rule as narrateText.
   */
  narrateFile: (key: string, file: File, level: Level, gender?: Gender) => Promise<void>;
  /**
   * Re-level ONE block of entry `key`. A cache hit (previously-seen level,
   * including the original) restores the block + its paired timeline snapshot
   * with NO network and NO model re-bill; a miss posts /narrate/block once,
   * replaces only that block, and adopts the response timeline wholesale.
   */
  setBlockLevel: (key: string, blockId: string, level: Level) => void;
}

export function useNarrationSession(): NarrationSession {
  const [entries, setEntries] = useState<Map<string, NarrationEntry>>(() => new Map());
  const [selectedEntryId, setSelectedEntryId] = useState<string | null>(null);
  const [blockLeveling, setBlockLeveling] = useState<Map<string, BlockLevelingState>>(
    () => new Map(),
  );
  const [reloadNonce, setReloadNonce] = useState(0);

  const audio = useAudio();
  const { setSource, reload } = audio;

  const selectedEntry = selectedEntryId ? (entries.get(selectedEntryId) ?? null) : null;
  const currentTranscript = selectedEntry?.transcript ?? null;

  // Refs so the async setBlockLevel closure reads CURRENT state without
  // re-creating the callback on every entry/selection change.
  const entriesRef = useRef(entries);
  const selectedRef = useRef(selectedEntryId);
  // Snapshot cache keyed `${key}::${blockId}::${level}` and the per-block
  // last-selected target (re-entrancy: last-selected-wins).
  const cacheRef = useRef<Map<string, LevelSnapshot>>(new Map());
  const pendingTargets = useRef<Map<string, Level>>(new Map());

  useEffect(() => {
    entriesRef.current = entries;
  }, [entries]);
  useEffect(() => {
    selectedRef.current = selectedEntryId;
  }, [selectedEntryId]);

  // Source the shared <audio> from the OPAQUE url STRING (not the transcript
  // object): an in-place block swap keeps audio_url stable, so this effect does
  // NOT re-fire (and would reset position); the reload nonce drives the
  // position-preserving re-fetch instead. A genuine url change (new narration or
  // a server-drifted audio_url) DOES re-source.
  const currentAudioUrl = currentTranscript?.audio_url ?? "";
  useEffect(() => {
    setSource(currentAudioUrl);
  }, [currentAudioUrl, setSource]);

  // Reload nonce → one position-preserving re-fetch of the rewritten bytes.
  // Skip the mount run (nonce starts at 0); only a bump after an in-place swap
  // on the CURRENTLY-selected entry reloads.
  const mountedRef = useRef(false);
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true;
      return;
    }
    reload();
  }, [reloadNonce, reload]);

  const activeBlockId = useMemo(
    () => deriveActiveBlockId(currentTranscript?.timeline, audio.currentTimeMs),
    [currentTranscript, audio.currentTimeMs],
  );

  // Seed the snapshot cache for EVERY block of a transcript, pairing each block's
  // current level with the timeline authoritative at that level. Runs on initial
  // load AND after each escalate/de-escalate so return-to-any-seen-level is a
  // pure client swap that restores matching offsets (F1 never reintroduced).
  //
  // TODO: harden the (key,blockId,level) snapshot cache for SIMULTANEOUS
  // multi-block divergence in useNarrationSession; a single (blockId,level)→
  // timeline snapshot can't capture a state where two+ blocks are off-baseline at
  // once, so a cross-block cache-hit could restore a timeline that pairs with a
  // different combination. The planner chose this single-snapshot model and every
  // transcript set re-seeds all current levels, so the tested single-divergence
  // flows stay correct; only the untested 2+-block case is affected. Low-pri.
  const seedCache = useCallback((key: string, transcript: NarrateResponse) => {
    for (const block of transcript.blocks) {
      cacheRef.current.set(`${key}::${block.id}::${block.level}`, {
        block,
        timelineSnapshot: transcript.timeline,
      });
    }
  }, []);

  const setLeveling = useCallback((blockKey: string, state: BlockLevelingState) => {
    setBlockLeveling((prev) => new Map(prev).set(blockKey, state));
  }, []);

  const run = useCallback(
    async (key: string, fn: (signal: AbortSignal) => Promise<NarrateResponse>) => {
      // Don't re-narrate if already in flight.
      setEntries((prev) => {
        if (prev.get(key)?.status === "loading") return prev;
        return new Map(prev).set(key, { status: "loading", transcript: null, error: null });
      });
      // Auto-select so the transcript pane shows this entry's loading state.
      setSelectedEntryId(key);

      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), NARRATE_TIMEOUT_MS);
      try {
        const resp = await fn(controller.signal);
        seedCache(key, resp); // seed the original level for every block
        setEntries((prev) =>
          new Map(prev).set(key, { status: "ready", transcript: resp, error: null }),
        );
      } catch (err) {
        const message =
          err instanceof ClientError
            ? err.message
            : err instanceof Error
              ? err.message
              : "Something went wrong";
        setEntries((prev) =>
          new Map(prev).set(key, { status: "error", transcript: null, error: message }),
        );
      } finally {
        clearTimeout(timer);
      }
    },
    [seedCache],
  );

  const narrateText = useCallback(
    (key: string, text: string, level: Level, gender?: Gender) =>
      run(key, (signal) => postNarrate({ text, level, gender, signal })),
    [run],
  );

  const narrateFile = useCallback(
    (key: string, file: File, level: Level, gender?: Gender) =>
      run(key, (signal) => postNarrateFile({ file, level, gender, signal })),
    [run],
  );

  const setBlockLevel = useCallback(
    (key: string, blockId: string, level: Level) => {
      const entry = entriesRef.current.get(key);
      const transcript = entry?.transcript;
      if (!transcript) return;
      // Guard: an older server lacking render_id cannot drive /narrate/block.
      if (!transcript.render_id) return;
      const block = transcript.blocks.find((b) => b.id === blockId);
      if (!block) return;

      const blockKey = `${key}::${blockId}`;
      // Tag this commit by its target level (last-selected-wins re-entrancy).
      pendingTargets.current.set(blockKey, level);

      const cached = cacheRef.current.get(`${key}::${blockId}::${level}`);
      if (cached) {
        // CACHE HIT — no network, no model re-bill. Restore the block AND its
        // paired timeline snapshot (a no-network de-escalate must not reintroduce
        // post-escalate offsets — F1).
        setEntries((prev) => {
          const e = prev.get(key);
          if (!e?.transcript) return prev;
          const next: NarrateResponse = {
            ...e.transcript,
            blocks: e.transcript.blocks.map((b) => (b.id === blockId ? cached.block : b)),
            timeline: cached.timelineSnapshot,
          };
          seedCache(key, next);
          return new Map(prev).set(key, { ...e, transcript: next });
        });
        setLeveling(blockKey, { phase: "idle", message: `Block now at L${level}`, target: null });
        if (selectedRef.current === key) setReloadNonce((n) => n + 1);
        return;
      }

      // CACHE MISS — POST. Inline loading; hold the target as aria-checked.
      setLeveling(blockKey, { phase: "loading", message: null, target: level });
      void (async () => {
        try {
          const resp = await postNarrateBlock({
            render_id: transcript.render_id,
            block_id: blockId,
            level,
          });
          // Re-entrancy: ignore a resolved response whose target is no longer the
          // block's currently-selected level (last-selected-wins).
          if (pendingTargets.current.get(blockKey) !== level) return;

          if (resp.refusal) {
            // Escalation-returned refusal — treat like the ERROR path (F4): keep
            // the prior committed level + audio playable; surface inline via the
            // alert region. Do NOT flip the block to refused.
            setLeveling(blockKey, {
              phase: "refused-inline",
              message:
                resp.refusal.message ||
                `Block can't be voiced at L${level}; still at L${block.level}`,
              target: null,
            });
            return;
          }

          // SUCCESS — replace ONLY this block; adopt the server timeline WHOLESALE.
          setEntries((prev) => {
            const e = prev.get(key);
            if (!e?.transcript) return prev;
            if (resp.audio_url !== e.transcript.audio_url) {
              // audio_url is expected stable (combined wav rewritten in place);
              // trust the server response if it drifts and re-point. Drift is the
              // warned/unexpected case: the changed URL re-fires the currentAudioUrl
              // setSource effect, which resets playback position — and on the drift
              // path that position RESET is intentional (a different URL is a
              // genuinely new clip, not the in-place rewrite the reload-nonce
              // position-preserve was designed for). The nonce bump below is then a
              // harmless no-op re-load of the freshly-sourced element.
              console.warn(
                `narrate/block returned a different audio_url (${resp.audio_url} != ${e.transcript.audio_url}); re-pointing`,
              );
            }
            const next: NarrateResponse = {
              ...e.transcript,
              audio_url: resp.audio_url,
              blocks: e.transcript.blocks.map((b) => (b.id === blockId ? resp.block : b)),
              timeline: resp.timeline,
            };
            seedCache(key, next);
            return new Map(prev).set(key, { ...e, transcript: next });
          });
          setLeveling(blockKey, {
            phase: "idle",
            message: `Block now at L${level}`,
            target: null,
          });
          if (selectedRef.current === key) setReloadNonce((n) => n + 1);
        } catch {
          if (pendingTargets.current.get(blockKey) !== level) return;
          // Keep the prior committed level/audio; surface inline via role=alert.
          setLeveling(blockKey, {
            phase: "error",
            message: `Couldn't re-narrate; still at L${block.level}`,
            target: null,
          });
        }
      })();
    },
    [seedCache, setLeveling],
  );

  const selectEntry = useCallback((key: string) => {
    setSelectedEntryId(key);
  }, []);

  const { seek, play } = audio;
  const playFromBlock = useCallback(
    (blockId: string) => {
      const timing = currentTranscript?.timeline.blocks.find((b) => b.block_id === blockId);
      if (timing) {
        seek(timing.start_ms);
      }
      play();
    },
    [currentTranscript, seek, play],
  );

  return {
    entries,
    selectedEntryId,
    selectedEntry,
    currentTranscript,
    selectEntry,
    activeBlockId,
    blockLeveling,
    audioRef: audio.audioRef,
    activeAudioUrl: currentTranscript?.audio_url ?? null,
    playbackState: audio.playbackState,
    audioError: audio.audioError,
    play: audio.play,
    pause: audio.pause,
    seek: audio.seek,
    playFromBlock,
    narrateText,
    narrateFile,
    setBlockLevel,
  };
}

export { IDLE_LEVELING };
