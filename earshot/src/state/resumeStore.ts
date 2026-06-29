// resumeStore.ts — pure, framework-agnostic localStorage persistence of the
// BLOCK-LEVEL resume position. No React, no DOM beyond `localStorage`.
//
// Load-bearing rules (CLAUDE.md + #112 plan):
//   - Block-level ONLY. We persist `blockId` (+ `blockOrder` for display) and a
//     `blockSignature` staleness guard — NEVER a raw ms offset. On restore the
//     caller re-derives `startMs` live from the timeline by blockId, so a
//     re-planned / escalated document can never resume into a stale byte
//     position. (No ms field exists in the value at all.)
//   - Every access is wrapped in try/catch: private-mode, disabled storage, and
//     quota throws all no-op silently. Resume is a convenience; losing it must
//     never break playback.
//   - Validate on read: drop the entry on schemaVersion mismatch or
//     blockSignature mismatch (the document changed under us — Decision
//     2026-06-22), and on any malformed JSON.

export const RESUME_SCHEMA_VERSION = 1;
const KEY_PREFIX = "earshot:resume:v1:";
/** Cap on stored resume keys; prune oldest beyond this on write (quota guard). */
const MAX_ENTRIES = 20;

/** The persisted shape. Block-level only — deliberately NO ms / time offset. */
export interface ResumeEntry {
  schemaVersion: number;
  blockId: string;
  blockOrder: number;
  /** Sorted-block-id signature of the document at write time (staleness guard). */
  blockSignature: string;
  updatedAt: number;
}

/** Namespaced key for a resume scope (e.g. an entry id). */
export function resumeKey(scope: string): string {
  return KEY_PREFIX + scope;
}

function storage(): Storage | null {
  try {
    // Touch the global; in private mode access itself can throw.
    return typeof localStorage !== "undefined" ? localStorage : null;
  } catch {
    return null;
  }
}

function isValidEntry(v: unknown): v is ResumeEntry {
  if (typeof v !== "object" || v === null) return false;
  const e = v as Record<string, unknown>;
  return (
    typeof e.schemaVersion === "number" &&
    typeof e.blockId === "string" &&
    typeof e.blockOrder === "number" &&
    typeof e.blockSignature === "string" &&
    typeof e.updatedAt === "number"
  );
}

/**
 * Read + validate a resume entry. Returns null (and self-heals by clearing the
 * key) on: missing/unavailable storage, malformed JSON, shape mismatch,
 * schemaVersion mismatch, or — when `expectedSignature` is supplied — a
 * blockSignature that no longer matches the current document.
 */
export function readResume(key: string, expectedSignature?: string): ResumeEntry | null {
  const s = storage();
  if (!s) return null;
  try {
    const raw = s.getItem(key);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!isValidEntry(parsed) || parsed.schemaVersion !== RESUME_SCHEMA_VERSION) {
      clearResume(key);
      return null;
    }
    if (expectedSignature !== undefined && parsed.blockSignature !== expectedSignature) {
      // Stale: the document changed (re-plan / escalation). Drop it.
      clearResume(key);
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

/** Write a resume entry. No-ops on any storage failure (private mode / quota). */
export function writeResume(key: string, entry: ResumeEntry): void {
  const s = storage();
  if (!s) return;
  try {
    s.setItem(key, JSON.stringify(entry));
    prune(s);
  } catch {
    /* private mode / quota — resume silently disabled, playback unaffected */
  }
}

/** Remove a resume entry. No-ops on failure. */
export function clearResume(key: string): void {
  const s = storage();
  if (!s) return;
  try {
    s.removeItem(key);
  } catch {
    /* ignore */
  }
}

/**
 * Bound localStorage growth: each new scope (entry/file) mints a fresh key, so
 * without a cap the ~5MB quota fills and the swallowed quota-throw would
 * silently kill resume. Keep the MAX_ENTRIES most-recent (by updatedAt); drop
 * the rest. Best-effort; any failure is ignored.
 */
function prune(s: Storage): void {
  try {
    const ours: { key: string; updatedAt: number }[] = [];
    for (let i = 0; i < s.length; i++) {
      const k = s.key(i);
      if (!k || !k.startsWith(KEY_PREFIX)) continue;
      let updatedAt = 0;
      try {
        const parsed: unknown = JSON.parse(s.getItem(k) ?? "");
        if (isValidEntry(parsed)) updatedAt = parsed.updatedAt;
      } catch {
        /* malformed — treat as oldest so it is pruned first */
      }
      ours.push({ key: k, updatedAt });
    }
    if (ours.length <= MAX_ENTRIES) return;
    ours.sort((a, b) => b.updatedAt - a.updatedAt); // newest first
    for (const { key } of ours.slice(MAX_ENTRIES)) {
      s.removeItem(key);
    }
  } catch {
    /* ignore */
  }
}
