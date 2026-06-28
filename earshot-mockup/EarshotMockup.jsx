/*
 * EarshotMockup.jsx — Earshot clickable fidelity mockup (issue #115)
 * ============================================================================
 * CONSTRAINTS (all four are load-bearing — do not violate):
 *   1. $0 — free Claude Artifact. No paid/Live/AI/MCP/persistent Artifact
 *      features. Pasteable as-is into a FREE Claude Artifact.
 *   2. no Live/AI/MCP/persistent features. All data is hard-coded fixtures;
 *      playback + escalation are SIMULATED in local state (timers only).
 *   3. no network/CDN — inline styles only. The only import is React. No
 *      external stylesheet, font, image, or script is loaded. Visible focus
 *      relies on the native browser outline (never removed anywhere here).
 *   4. throwaway fidelity mockup — DO NOT wire into earshot/, hand-port the
 *      approved patterns instead. This file never talks to narrate-server or
 *      the Go core. It exists only to drive a recorded sign-off before #111.
 * ============================================================================
 * Purpose: make the two weakest surfaces of the approved Earshot design
 * clickable and user-testable before #111 opens:
 *   - the per-block L1/L2/L3 segmented control (a novel 3-state synthesis), and
 *   - the bottom-anchored transport (a delegated top-vs-bottom call).
 *
 * The two probes that decide sign-off live in the plan's "Probe Protocol"
 * (.claude/handoff/earshot-mockup-artifact-115/planner-task.md). Sign-off
 * itself is a HUMAN action recorded in the project's decision journal AFTER
 * the evaluator runs Probe 1 (L1/L2/L3 intuitiveness) and Probe 2 (transport
 * anchor) — the mockup cannot produce the sign-off, only enable it.
 *
 * Honesty rule (non-negotiable, mirrored from CLAUDE.md):
 *   - refused block  -> spoken refusal notice + source map, and NO level control.
 *   - degraded block -> the REAL verbatim prose as spoken text PLUS a distinct
 *                       reason chip. Never a fabricated gist at any level.
 *   - status enum is exactly: voiced | degraded | refused.
 *
 * Escalation contract (Model A): the L1/L2/L3 radiogroup IS the escalation
 * surface. There is no standalone escalate card / popover / region anywhere.
 * Escalation is inline, role="status" (polite), non-dismissable.
 */

import React, { useState, useRef, useEffect, useCallback, useMemo } from "react";

/* ----------------------------------------------------------------------------
 * Fixtures (module constants — no server, no glob, no localStorage).
 * Block ids are namespaced per session (s1-b1, s2-b1, ...) so any cross-session
 * state leak is immediately visible. Each non-refused block carries L1/L2/L3
 * variants; the degraded block's variants are all the SAME verbatim prose by
 * construction (honesty: degraded prose has no intelligence to gist — escalating
 * it must never fabricate a summary).
 * --------------------------------------------------------------------------*/

const SESSIONS = [
  {
    id: "s1",
    title: "Kubernetes Deployment Guide",
    date: "2026-06-24",
    length: "2:20",
    statusChip: null,
    blocks: [
      {
        id: "s1-b1",
        klass: "heading",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 0, endMs: 8000 },
        sourceText: "# Deployment Overview",
        levels: {
          1: { spokenText: "Heading. Deployment Overview.", timeRange: { startMs: 0, endMs: 8000 } },
          2: { spokenText: "Section heading: Deployment Overview — the chapter on rolling out workloads.", timeRange: { startMs: 0, endMs: 9000 } },
          3: { spokenText: "Top-level section heading, Deployment Overview, introducing how workloads are rolled out across the cluster.", timeRange: { startMs: 0, endMs: 11000 } },
        },
      },
      {
        // PROBE 1 HERO: a voiced prose block, opens at L1.
        id: "s1-b2",
        klass: "prose",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 8000, endMs: 30000 },
        sourceText:
          "A Deployment provides declarative updates for Pods and ReplicaSets. You describe a desired state in a Deployment, and the Deployment Controller changes the actual state to the desired state at a controlled rate. Deployments are well suited to stateless applications.",
        levels: {
          1: { spokenText: "Deployments declaratively manage stateless Pods at a controlled rollout rate.", timeRange: { startMs: 8000, endMs: 16000 } },
          2: { spokenText: "A Deployment declares the desired state for Pods and ReplicaSets; its controller reconciles actual state toward that, rolling changes out at a controlled rate. Best for stateless apps.", timeRange: { startMs: 8000, endMs: 24000 } },
          3: { spokenText: "A Deployment provides declarative updates for Pods and ReplicaSets. You describe a desired state, and the Deployment Controller changes the actual state to match at a controlled rate — pausing, resuming, or rolling back as needed. This pattern suits stateless applications where any replica can serve any request.", timeRange: { startMs: 8000, endMs: 30000 } },
        },
      },
      {
        // structured class (config) — deterministic at all levels, no intelligence needed.
        id: "s1-b3",
        klass: "config",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 30000, endMs: 52000 },
        sourceText: "spec:\n  replicas: 3\n  strategy:\n    type: RollingUpdate",
        levels: {
          1: { spokenText: "Config. Replicas set to three. Strategy rolling update.", timeRange: { startMs: 30000, endMs: 38000 } },
          2: { spokenText: "Deployment spec: replicas set to three, update strategy is rolling update.", timeRange: { startMs: 30000, endMs: 44000 } },
          3: { spokenText: "Deployment spec block. Replicas set to three. Strategy type rolling update, which replaces Pods incrementally rather than all at once.", timeRange: { startMs: 30000, endMs: 52000 } },
        },
      },
      {
        // DEGRADED prose: verbatim words + reason chip. Variants are identical
        // verbatim text at every level — escalating must NOT fabricate a gist.
        id: "s1-b4",
        klass: "prose",
        status: "degraded",
        defaultLevel: 1,
        timeRange: { startMs: 52000, endMs: 70000 },
        degradedReason: "No intelligence adapter available — prose read verbatim",
        sourceText:
          "Note that horizontal pod autoscaling and the Deployment's own replica count can conflict; if both manage the same workload, the autoscaler should own the replica count and the Deployment field should be omitted.",
        levels: {
          1: { spokenText: "Note that horizontal pod autoscaling and the Deployment's own replica count can conflict; if both manage the same workload, the autoscaler should own the replica count and the Deployment field should be omitted.", timeRange: { startMs: 52000, endMs: 70000 } },
          2: { spokenText: "Note that horizontal pod autoscaling and the Deployment's own replica count can conflict; if both manage the same workload, the autoscaler should own the replica count and the Deployment field should be omitted.", timeRange: { startMs: 52000, endMs: 70000 } },
          3: { spokenText: "Note that horizontal pod autoscaling and the Deployment's own replica count can conflict; if both manage the same workload, the autoscaler should own the replica count and the Deployment field should be omitted.", timeRange: { startMs: 52000, endMs: 70000 } },
        },
      },
      {
        id: "s1-b5",
        klass: "list",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 70000, endMs: 82000 },
        sourceText: "- Pause rollout\n- Resume rollout\n- Roll back to previous revision",
        levels: {
          1: { spokenText: "List. Pause rollout. Resume rollout. Roll back to previous revision.", timeRange: { startMs: 70000, endMs: 78000 } },
          2: { spokenText: "Three rollout actions: pause the rollout, resume it, or roll back to the previous revision.", timeRange: { startMs: 70000, endMs: 80000 } },
          3: { spokenText: "A list of three rollout controls — pausing a rollout to inspect it, resuming a paused rollout, and rolling back to the previously known-good revision.", timeRange: { startMs: 70000, endMs: 82000 } },
        },
      },
      {
        // REFUSED image: refusal notice + source map, NO level control.
        id: "s1-b6",
        klass: "image",
        status: "refused",
        defaultLevel: null,
        timeRange: { startMs: 82000, endMs: 90000 },
        sourceText: "[image] fig-2.png",
        refusal: {
          notice: "Image cannot be voiced — no description available. Skipping.",
          source: "fig-2.png",
          reason: "RefuseBareImage",
        },
      },
      {
        id: "s1-b7",
        klass: "prose",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 90000, endMs: 120000 },
        sourceText:
          "Readiness and liveness probes let the cluster route traffic only to Pods that are ready and restart Pods that have wedged, which is essential during a rolling update so that traffic is never sent to a Pod that is still starting up.",
        levels: {
          1: { spokenText: "Readiness and liveness probes gate traffic and restart wedged Pods during rollouts.", timeRange: { startMs: 90000, endMs: 100000 } },
          2: { spokenText: "Readiness probes keep traffic off Pods that aren't ready; liveness probes restart wedged Pods. Both matter during a rolling update.", timeRange: { startMs: 90000, endMs: 110000 } },
          3: { spokenText: "Readiness and liveness probes let the cluster route traffic only to Pods that are ready and restart Pods that have wedged. This is essential during a rolling update so traffic is never sent to a Pod still starting up.", timeRange: { startMs: 90000, endMs: 120000 } },
        },
      },
      {
        id: "s1-b8",
        klass: "table",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 120000, endMs: 140000 },
        sourceText: "| Strategy | Downtime |\n| Recreate | yes |\n| RollingUpdate | no |",
        levels: {
          1: { spokenText: "Table. Recreate strategy has downtime. Rolling update has none.", timeRange: { startMs: 120000, endMs: 128000 } },
          2: { spokenText: "Two-row table comparing strategies: Recreate incurs downtime; RollingUpdate does not.", timeRange: { startMs: 120000, endMs: 134000 } },
          3: { spokenText: "A comparison table of two deployment strategies. The Recreate strategy causes downtime because it tears down old Pods first. RollingUpdate avoids downtime by replacing Pods incrementally.", timeRange: { startMs: 120000, endMs: 140000 } },
        },
      },
    ],
  },
  {
    id: "s2",
    title: "API Migration Notes",
    date: "2026-06-20",
    length: "0:48",
    statusChip: "stale", // persistent-sink stale; no auto-regenerate affordance
    blocks: [
      {
        id: "s2-b1",
        klass: "heading",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 0, endMs: 9000 },
        sourceText: "# v2 Endpoint Changes",
        levels: {
          1: { spokenText: "Heading. v2 Endpoint Changes.", timeRange: { startMs: 0, endMs: 9000 } },
          2: { spokenText: "Section heading: v2 Endpoint Changes — what moved between API versions.", timeRange: { startMs: 0, endMs: 10000 } },
          3: { spokenText: "Top-level heading, v2 Endpoint Changes, covering every endpoint that moved between API versions one and two.", timeRange: { startMs: 0, endMs: 12000 } },
        },
      },
      {
        id: "s2-b2",
        klass: "prose",
        status: "degraded",
        defaultLevel: 1,
        timeRange: { startMs: 9000, endMs: 30000 },
        degradedReason: "Source exceeded inline budget — read verbatim, not summarized",
        sourceText:
          "The v1 list endpoint returned an array at the top level; v2 wraps results in an object with a data array and a pagination cursor, so clients that destructured the array directly must be updated before cutover.",
        levels: {
          1: { spokenText: "The v1 list endpoint returned an array at the top level; v2 wraps results in an object with a data array and a pagination cursor, so clients that destructured the array directly must be updated before cutover.", timeRange: { startMs: 9000, endMs: 30000 } },
          2: { spokenText: "The v1 list endpoint returned an array at the top level; v2 wraps results in an object with a data array and a pagination cursor, so clients that destructured the array directly must be updated before cutover.", timeRange: { startMs: 9000, endMs: 30000 } },
          3: { spokenText: "The v1 list endpoint returned an array at the top level; v2 wraps results in an object with a data array and a pagination cursor, so clients that destructured the array directly must be updated before cutover.", timeRange: { startMs: 9000, endMs: 30000 } },
        },
      },
      {
        id: "s2-b3",
        klass: "image",
        status: "refused",
        defaultLevel: null,
        timeRange: { startMs: 30000, endMs: 48000 },
        sourceText: "[image] sequence-diagram.png",
        refusal: {
          notice: "Diagram image cannot be voiced — no text alternative supplied. Skipping.",
          source: "sequence-diagram.png",
          reason: "RefuseBareImage",
        },
      },
    ],
  },
  {
    id: "s3",
    title: "Release Checklist",
    date: "2026-06-12",
    length: "0:30",
    statusChip: "error", // a non-stale error chip, to show the second chip kind
    blocks: [
      {
        id: "s3-b1",
        klass: "heading",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 0, endMs: 8000 },
        sourceText: "# Pre-flight",
        levels: {
          1: { spokenText: "Heading. Pre-flight.", timeRange: { startMs: 0, endMs: 8000 } },
          2: { spokenText: "Section heading: Pre-flight checks before release.", timeRange: { startMs: 0, endMs: 9000 } },
          3: { spokenText: "Top-level heading, Pre-flight, listing the checks that must pass before a release is cut.", timeRange: { startMs: 0, endMs: 10000 } },
        },
      },
      {
        id: "s3-b2",
        klass: "list",
        status: "voiced",
        defaultLevel: 1,
        timeRange: { startMs: 8000, endMs: 30000 },
        sourceText: "- Tag the release\n- Run the smoke suite\n- Notify on-call",
        levels: {
          1: { spokenText: "List. Tag the release. Run the smoke suite. Notify on-call.", timeRange: { startMs: 8000, endMs: 18000 } },
          2: { spokenText: "Three pre-flight steps: tag the release, run the smoke suite, notify on-call.", timeRange: { startMs: 8000, endMs: 24000 } },
          3: { spokenText: "A checklist of three pre-flight steps — tag the release in version control, run the full smoke suite against staging, and notify the on-call engineer before cutover.", timeRange: { startMs: 8000, endMs: 30000 } },
        },
      },
    ],
  },
];

const LEVEL_META = {
  1: { label: "L1 — gist", short: "L1" },
  2: { label: "L2 — summary", short: "L2" },
  3: { label: "L3 — detail", short: "L3" },
};

const ESCALATION_LATENCY_MS = 900; // believable simulated re-render latency
const TICK_MS = 250;

/* ---- helpers (pure, module scope) ---- */
function fmt(ms) {
  const total = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function statusGlyph(status) {
  // Non-color status signal: a text glyph + a text label always accompany color.
  if (status === "voiced") return "✓"; // check
  if (status === "degraded") return "⚠"; // warning
  if (status === "refused") return "⊘"; // circled minus
  return "•";
}

function chipGlyph(chip) {
  if (chip === "stale") return "⦰"; // reversed tilde-ish
  if (chip === "error") return "✕"; // x
  return "•";
}

/* ----------------------------------------------------------------------------
 * Component
 * --------------------------------------------------------------------------*/
export default function EarshotMockup() {
  // ---- App-root state (lifted because read across transcript/transport) ----
  const [selectedSessionId, setSelectedSessionId] = useState(SESSIONS[0].id);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playheadMs, setPlayheadMs] = useState(0);
  const [perBlockLevel, setPerBlockLevel] = useState({}); // blockId -> 1|2|3
  const [escalatingBlockId, setEscalatingBlockId] = useState(null);
  const [escalationMsg, setEscalationMsg] = useState({}); // blockId -> string
  const [transportAnchor, setTransportAnchor] = useState("bottom"); // accepted default
  const [transcriptView, setTranscriptView] = useState("spoken"); // spoken | source
  const [sessionPaneCollapsed, setSessionPaneCollapsed] = useState(false);

  // ---- session-ID entry (design §4: "input a session ID → message list") ----
  // The mock can only resolve the hard-coded fixtures, so an ID that does not
  // match a fixture yields the HONEST glob-miss notice — never a fabricated
  // "loaded" session. This mirrors the real limitation: Earshot reaches only
  // sessions whose .jsonl is on this machine.
  const [sessionIdInput, setSessionIdInput] = useState("");
  const [sessionIdNotice, setSessionIdNotice] = useState(null); // null | {kind, msg}

  // ---- local UI focus / menu state ----
  const [listboxFocusIndex, setListboxFocusIndex] = useState(0);
  const [toolbarFocusIndex, setToolbarFocusIndex] = useState(0); // 0=prev 1=play 2=slider 3=return
  const [loadMenuOpen, setLoadMenuOpen] = useState(false);
  const [skipFocused, setSkipFocused] = useState(false);
  const [activeInView, setActiveInView] = useState(true);

  // ---- refs ----
  const escTimers = useRef({}); // blockId -> timeout id
  const intervalRef = useRef(null);
  const toolbarRefs = useRef([]); // 4 control nodes
  const blockRefs = useRef({}); // blockId -> row node
  const playBtnRefs = useRef({}); // blockId -> play-from-here button
  const radioRefs = useRef({}); // blockId -> {1,2,3} radio nodes
  const transcriptScrollRef = useRef(null);

  const session = useMemo(
    () => SESSIONS.find((s) => s.id === selectedSessionId) || SESSIONS[0],
    [selectedSessionId]
  );
  const blocks = session.blocks;
  const totalMs = blocks[blocks.length - 1].timeRange.endMs;

  // ---- derived: active block index from playheadMs (single source of truth) ----
  const activeBlockIndex = useMemo(() => {
    for (let i = 0; i < blocks.length; i++) {
      if (playheadMs < blocks[i].timeRange.endMs) return i;
    }
    return blocks.length - 1;
  }, [blocks, playheadMs]);
  const activeBlock = blocks[activeBlockIndex];
  const activeBlockId = activeBlock.id;

  const levelOf = useCallback(
    (b) => (b.status === "refused" ? null : perBlockLevel[b.id] ?? b.defaultLevel),
    [perBlockLevel]
  );

  // ---- simulated playhead: setInterval with a FUNCTIONAL updater (no stale ms) ----
  useEffect(() => {
    if (!isPlaying) return undefined;
    intervalRef.current = setInterval(() => {
      setPlayheadMs((prev) => Math.min(prev + TICK_MS, totalMs));
    }, TICK_MS);
    return () => {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    };
  }, [isPlaying, totalMs]);

  // stop at end of document
  useEffect(() => {
    if (isPlaying && playheadMs >= totalMs) setIsPlaying(false);
  }, [isPlaying, playheadMs, totalMs]);

  // ---- clear ALL simulated-escalation timers on unmount (no orphan timers) ----
  useEffect(() => {
    const timers = escTimers.current;
    return () => {
      Object.values(timers).forEach((t) => clearTimeout(t));
    };
  }, []);

  // ---- session switch: reset shared playback state + escalation, namespaced ids ----
  const selectSession = useCallback((id) => {
    // clear any in-flight escalation timers from the outgoing session
    Object.values(escTimers.current).forEach((t) => clearTimeout(t));
    escTimers.current = {};
    setSelectedSessionId(id);
    setActiveInView(true);
    setIsPlaying(false);
    setPlayheadMs(0);
    setPerBlockLevel({});
    setEscalatingBlockId(null);
    setEscalationMsg({});
    setToolbarFocusIndex(0);
    setSessionIdNotice(null);
  }, []);

  // ---- resolve a typed session ID (design §4 + §6 glob model) ----
  // Match is case-insensitive against the fixture ids (also accept a "load-by-id"
  // alias of the title for convenience). A miss is reported honestly, not faked.
  const loadSessionById = useCallback(() => {
    const raw = sessionIdInput.trim();
    if (!raw) {
      setSessionIdNotice({ kind: "error", msg: "Enter a session ID to load." });
      return;
    }
    const q = raw.toLowerCase();
    const hit = SESSIONS.find(
      (s) => s.id.toLowerCase() === q || s.title.toLowerCase() === q
    );
    if (hit) {
      selectSession(hit.id);
      setSessionIdInput("");
      setSessionIdNotice({ kind: "ok", msg: `Loaded "${hit.title}".` });
      return;
    }
    // Honest glob-miss: do not fabricate a "loaded" session for an unknown ID.
    setSessionIdNotice({
      kind: "error",
      msg: `No local transcript found for "${raw}". Earshot reaches only sessions whose .jsonl is on this machine (glob ~/.claude/projects/*/{id}.jsonl). Mock fixtures: ${SESSIONS.map((s) => s.id).join(", ")}.`,
    });
  }, [sessionIdInput, selectSession]);

  // ---- compute whether the active block is within the transcript viewport ----
  const computeInView = useCallback(() => {
    const container = transcriptScrollRef.current;
    const row = blockRefs.current[activeBlockId];
    if (!container || !row) return;
    const c = container.getBoundingClientRect();
    const r = row.getBoundingClientRect();
    const visible = r.bottom > c.top + 4 && r.top < c.bottom - 4;
    setActiveInView(visible);
  }, [activeBlockId]);

  useEffect(() => {
    computeInView();
  }, [computeInView, playheadMs, activeBlockId, sessionPaneCollapsed, transportAnchor]);

  // ---- seek helpers (block-quantized) ----
  const seekToBlock = useCallback(
    (i) => {
      const idx = Math.max(0, Math.min(blocks.length - 1, i));
      setPlayheadMs(blocks[idx].timeRange.startMs);
    },
    [blocks]
  );

  const playFromBlock = useCallback(
    (i) => {
      seekToBlock(i);
      setIsPlaying(true);
    },
    [seekToBlock]
  );

  // ---- escalation simulation (Model A — inline role=status, non-dismissable) ----
  const setLevel = useCallback(
    (block, newLevel) => {
      const current = perBlockLevel[block.id] ?? block.defaultLevel;
      if (newLevel === current) return;
      // supersede any in-flight timer for this block
      if (escTimers.current[block.id]) {
        clearTimeout(escTimers.current[block.id]);
        delete escTimers.current[block.id];
      }
      if (newLevel > current) {
        // higher fidelity -> simulate a re-render with a polite status message
        setEscalatingBlockId(block.id);
        setEscalationMsg((m) => ({ ...m, [block.id]: `Re-rendering block at ${LEVEL_META[newLevel].short}…` }));
        escTimers.current[block.id] = setTimeout(() => {
          setPerBlockLevel((m) => ({ ...m, [block.id]: newLevel }));
          setEscalatingBlockId((cur) => (cur === block.id ? null : cur));
          setEscalationMsg((m) => ({ ...m, [block.id]: `Block now at ${LEVEL_META[newLevel].short}` }));
          delete escTimers.current[block.id];
        }, ESCALATION_LATENCY_MS);
      } else {
        // lower fidelity -> instant cached variant, no re-bill, no spinner
        setPerBlockLevel((m) => ({ ...m, [block.id]: newLevel }));
        setEscalatingBlockId((cur) => (cur === block.id ? null : cur));
        setEscalationMsg((m) => ({ ...m, [block.id]: `Block now at ${LEVEL_META[newLevel].short}` }));
      }
    },
    [perBlockLevel]
  );

  // ---- displayed text per block (honesty branch) ----
  const displayText = (b) => {
    if (transcriptView === "source") return b.sourceText;
    if (b.status === "refused") return b.refusal.notice;
    return b.levels[levelOf(b)].spokenText;
  };
  const displayRange = (b) => {
    if (b.status === "refused") return b.timeRange;
    return b.levels[levelOf(b)].timeRange;
  };

  // ===========================================================================
  // Toolbar (role=toolbar) — roving tabindex, 4 controls.
  // ===========================================================================
  const TOOLBAR_COUNT = 4;
  const focusToolbar = useCallback((i) => {
    const idx = Math.max(0, Math.min(TOOLBAR_COUNT - 1, i));
    setToolbarFocusIndex(idx);
    const node = toolbarRefs.current[idx];
    if (node) node.focus();
  }, []);

  const onToolbarBtnKey = (e, myIndex) => {
    // non-slider controls: both axes move between controls (APG toolbar)
    if (["ArrowLeft", "ArrowUp"].includes(e.key)) {
      e.preventDefault();
      focusToolbar(myIndex - 1);
    } else if (["ArrowRight", "ArrowDown"].includes(e.key)) {
      e.preventDefault();
      focusToolbar(myIndex + 1);
    } else if (e.key === "Home") {
      e.preventDefault();
      focusToolbar(0);
    } else if (e.key === "End") {
      e.preventDefault();
      focusToolbar(TOOLBAR_COUNT - 1);
    }
  };

  const onSliderKey = (e) => {
    // slider OWNS left/right (block-step); up/down EXIT to neighbor controls.
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      seekToBlock(activeBlockIndex - 1);
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      seekToBlock(activeBlockIndex + 1);
    } else if (e.key === "Home") {
      e.preventDefault();
      seekToBlock(0);
    } else if (e.key === "End") {
      e.preventDefault();
      seekToBlock(blocks.length - 1);
    } else if (e.key === "PageUp") {
      e.preventDefault();
      seekToBlock(activeBlockIndex + 5);
    } else if (e.key === "PageDown") {
      e.preventDefault();
      seekToBlock(activeBlockIndex - 5);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      focusToolbar(2 - 1); // exit up to play/pause
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      focusToolbar(2 + 1); // exit down to return-to-playing
    }
  };

  const returnToPlaying = () => {
    const row = blockRefs.current[activeBlockId];
    if (row && row.scrollIntoView) row.scrollIntoView({ block: "center" });
    const btn = playBtnRefs.current[activeBlockId];
    if (btn) btn.focus();
    setActiveInView(true);
  };

  // ===========================================================================
  // Radiogroup (per non-refused block) — arrows move+select; one tab stop.
  // ===========================================================================
  const onRadiogroupKey = (e, block) => {
    const cur = perBlockLevel[block.id] ?? block.defaultLevel;
    let next = null;
    if (["ArrowRight", "ArrowDown"].includes(e.key)) next = Math.min(3, cur + 1);
    else if (["ArrowLeft", "ArrowUp"].includes(e.key)) next = Math.max(1, cur - 1);
    else if (e.key === "Home") next = 1;
    else if (e.key === "End") next = 3;
    if (next !== null) {
      e.preventDefault();
      setLevel(block, next);
      const node = radioRefs.current[block.id] && radioRefs.current[block.id][next];
      if (node) node.focus();
    }
  };

  // ---- styles (inline only) ----
  const C = {
    ink: "#1c2024",
    sub: "#51606a",
    line: "#c9d2d9",
    bg: "#ffffff",
    panel: "#f4f7f9",
    accent: "#1f6feb",
    accentInk: "#ffffff",
    voiced: "#1a7f37",
    degraded: "#9a6700",
    refused: "#b42318",
    stale: "#8250df",
  };
  const st = {
    app: { fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, sans-serif", color: C.ink, background: C.bg, minHeight: "100vh", display: "flex", flexDirection: "column", fontSize: 15, lineHeight: 1.45 },
    header: { display: "flex", alignItems: "center", gap: 12, padding: "10px 16px", borderBottom: `1px solid ${C.line}`, background: C.panel, position: "sticky", top: 0, zIndex: 5 },
    h1: { fontSize: 18, margin: 0, fontWeight: 700 },
    headerSpacer: { flex: 1 },
    btn: { font: "inherit", padding: "6px 10px", border: `1px solid ${C.line}`, background: C.bg, borderRadius: 6, cursor: "pointer", color: C.ink },
    btnPrimary: { font: "inherit", padding: "6px 12px", border: `1px solid ${C.accent}`, background: C.accent, color: C.accentInk, borderRadius: 6, cursor: "pointer" },
    body: { flex: 1, display: "flex", minHeight: 0 },
    sidePane: { width: 280, borderRight: `1px solid ${C.line}`, background: C.panel, padding: 12, overflowY: "auto" },
    sidePaneCollapsed: { width: 44, borderRight: `1px solid ${C.line}`, background: C.panel, padding: 8 },
    main: { flex: 1, display: "flex", flexDirection: "column", minWidth: 0 },
    transcriptScroll: { flex: 1, overflowY: "auto", padding: "16px 20px", position: "relative" },
    paneTitle: { fontSize: 14, margin: "0 0 8px", textTransform: "uppercase", letterSpacing: 0.5, color: C.sub },
    listbox: { listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 6 },
    option: (sel) => ({ padding: "8px 10px", border: `1px solid ${sel ? C.accent : C.line}`, background: sel ? "#e8f0fe" : C.bg, borderRadius: 6, cursor: "pointer" }),
    row: (active) => ({ border: `1px solid ${active ? C.accent : C.line}`, borderLeft: `4px solid ${active ? C.accent : "transparent"}`, borderRadius: 8, padding: 12, marginBottom: 12, background: active ? "#f5f9ff" : C.bg }),
    meta: { display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap", fontSize: 12, color: C.sub, marginBottom: 6 },
    chip: (color) => ({ display: "inline-flex", gap: 4, alignItems: "center", padding: "1px 7px", borderRadius: 999, border: `1px solid ${color}`, color, fontSize: 12, fontWeight: 600, background: "#fff" }),
    klassTag: { padding: "1px 7px", borderRadius: 4, background: "#eceff2", color: C.sub, fontWeight: 600 },
    text: { margin: "4px 0 10px", whiteSpace: "pre-wrap" },
    refusalBox: { background: "#fdf2f0", border: `1px solid ${C.refused}`, borderRadius: 6, padding: 10, color: C.refused },
    radiogroup: { display: "inline-flex", border: `1px solid ${C.line}`, borderRadius: 8, overflow: "hidden" },
    radio: (checked) => ({ font: "inherit", padding: "5px 12px", border: "none", borderRight: `1px solid ${C.line}`, background: checked ? C.accent : C.bg, color: checked ? C.accentInk : C.ink, cursor: "pointer", fontWeight: checked ? 700 : 400 }),
    status: { fontSize: 12, color: C.sub, minHeight: 16, marginTop: 4 },
    transportWrap: (anchor) => ({ position: "sticky", [anchor === "top" ? "top" : "bottom"]: 0, zIndex: 4, background: C.panel, borderTop: anchor === "bottom" ? `1px solid ${C.line}` : "none", borderBottom: anchor === "top" ? `1px solid ${C.line}` : "none", padding: "8px 16px" }),
    toolbar: { display: "flex", alignItems: "center", gap: 10 },
    sliderTrack: { position: "relative", flex: 1, height: 10, background: "#dfe6ea", borderRadius: 999, cursor: "pointer" },
    sliderFill: (pct) => ({ position: "absolute", left: 0, top: 0, bottom: 0, width: `${pct}%`, background: C.accent, borderRadius: 999 }),
    sliderThumb: (pct) => ({ position: "absolute", top: "50%", left: `${pct}%`, transform: "translate(-50%,-50%)", width: 16, height: 16, borderRadius: "50%", background: "#fff", border: `2px solid ${C.accent}` }),
    skip: (focused) => (focused
      ? { position: "absolute", left: 8, top: 8, zIndex: 20, padding: "6px 10px", background: C.ink, color: "#fff", borderRadius: 6 }
      : { position: "absolute", left: -9999, top: "auto", width: 1, height: 1, overflow: "hidden" }),
  };

  const chipColor = (chip) => (chip === "stale" ? C.stale : chip === "error" ? C.refused : C.sub);
  const statusColor = (s) => (s === "voiced" ? C.voiced : s === "degraded" ? C.degraded : C.refused);

  const scrubPct = totalMs > 0 ? (activeBlock.timeRange.startMs / totalMs) * 100 : 0;
  const sliderValueText = `${fmt(playheadMs)}, block ${activeBlockIndex + 1} of ${blocks.length}, ${activeBlock.klass}`;

  return (
    <div style={st.app}>
      {/* skip-link — first focusable element */}
      <a
        href="#transport"
        style={st.skip(skipFocused)}
        onFocus={() => setSkipFocused(true)}
        onBlur={() => setSkipFocused(false)}
        onClick={(e) => {
          e.preventDefault();
          focusToolbar(0);
        }}
      >
        Skip to transport
      </a>

      {/* ---- app header (landmark) ---- */}
      <header style={st.header}>
        <h1 style={st.h1}>Earshot</h1>
        <div style={{ position: "relative" }}>
          <button
            type="button"
            style={st.btn}
            aria-haspopup="menu"
            aria-expanded={loadMenuOpen}
            onClick={() => setLoadMenuOpen((o) => !o)}
          >
            Load directory ▾
          </button>
          {loadMenuOpen && (
            <div
              role="menu"
              aria-label="Load directory"
              style={{ position: "absolute", top: "100%", left: 0, marginTop: 4, background: "#fff", border: `1px solid ${C.line}`, borderRadius: 6, padding: 6, minWidth: 200, boxShadow: "0 4px 16px rgba(0,0,0,0.12)" }}
            >
              <div role="menuitem" tabIndex={-1} style={{ padding: "6px 8px", color: C.sub }}>
                (mockup — directory loading is out of scope)
              </div>
            </div>
          )}
        </div>
        <div style={st.headerSpacer} />
        {/* AC4 — transport anchor toggle (also reachable here as a setting) */}
        <div role="group" aria-label="Transport anchor" style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <span style={{ fontSize: 12, color: C.sub }}>Transport:</span>
          <button
            type="button"
            style={transportAnchor === "top" ? st.btnPrimary : st.btn}
            aria-pressed={transportAnchor === "top"}
            onClick={() => setTransportAnchor("top")}
          >
            Top
          </button>
          <button
            type="button"
            style={transportAnchor === "bottom" ? st.btnPrimary : st.btn}
            aria-pressed={transportAnchor === "bottom"}
            onClick={() => setTransportAnchor("bottom")}
          >
            Bottom
          </button>
        </div>
        <button type="button" style={st.btn} aria-label="Settings">
          ⚙ Settings
        </button>
      </header>

      {/* transport rendered at TOP when anchored top (single instance below via render flag) */}
      {transportAnchor === "top" && (
        <TransportDeck
          st={st}
          C={C}
          anchor="top"
          isPlaying={isPlaying}
          setIsPlaying={setIsPlaying}
          activeBlockIndex={activeBlockIndex}
          blocks={blocks}
          playheadMs={playheadMs}
          scrubPct={scrubPct}
          sliderValueText={sliderValueText}
          totalMs={totalMs}
          toolbarRefs={toolbarRefs}
          toolbarFocusIndex={toolbarFocusIndex}
          onToolbarBtnKey={onToolbarBtnKey}
          onSliderKey={onSliderKey}
          seekToBlock={seekToBlock}
          returnToPlaying={returnToPlaying}
          activeInView={activeInView}
        />
      )}

      <div style={st.body}>
        {/* ---- session pane (aside/nav landmark) ---- */}
        <nav aria-label="Sessions" style={sessionPaneCollapsed ? st.sidePaneCollapsed : st.sidePane}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            {!sessionPaneCollapsed && <h2 style={st.paneTitle}>Sessions</h2>}
            <button
              type="button"
              style={st.btn}
              aria-expanded={!sessionPaneCollapsed}
              aria-label={sessionPaneCollapsed ? "Expand sessions pane" : "Collapse sessions pane"}
              onClick={() => setSessionPaneCollapsed((c) => !c)}
            >
              {sessionPaneCollapsed ? "»" : "«"}
            </button>
          </div>

          {/* ---- session-ID entry (design §4) — load a session not in the list ---- */}
          {!sessionPaneCollapsed && (
            <form
              style={{ margin: "4px 0 12px" }}
              onSubmit={(e) => {
                e.preventDefault();
                loadSessionById();
              }}
            >
              <label htmlFor="session-id-input" style={{ ...st.paneTitle, display: "block", marginBottom: 4 }}>
                Load by session ID
              </label>
              <div style={{ display: "flex", gap: 6 }}>
                <input
                  id="session-id-input"
                  type="text"
                  value={sessionIdInput}
                  onChange={(e) => setSessionIdInput(e.target.value)}
                  placeholder="paste session ID…"
                  aria-describedby="session-id-notice"
                  style={{ flex: 1, minWidth: 0, padding: "6px 8px", border: `1px solid ${C.line}`, borderRadius: 6, fontSize: 13 }}
                />
                <button type="submit" style={st.btnPrimary} aria-label="Load session by ID">
                  Load
                </button>
              </div>
              {sessionIdNotice && (
                <div
                  id="session-id-notice"
                  role="status"
                  aria-live="polite"
                  style={{ marginTop: 6, fontSize: 12, lineHeight: 1.4, color: sessionIdNotice.kind === "error" ? C.refused : C.sub }}
                >
                  {sessionIdNotice.msg}
                </div>
              )}
            </form>
          )}

          {!sessionPaneCollapsed && (
            <ul
              role="listbox"
              aria-label="Sessions"
              style={st.listbox}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setListboxFocusIndex((i) => Math.min(SESSIONS.length - 1, i + 1));
                } else if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setListboxFocusIndex((i) => Math.max(0, i - 1));
                } else if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  selectSession(SESSIONS[listboxFocusIndex].id);
                }
              }}
            >
              {SESSIONS.map((s, i) => {
                const selected = s.id === selectedSessionId;
                return (
                  <li
                    key={s.id}
                    role="option"
                    aria-selected={selected}
                    tabIndex={i === listboxFocusIndex ? 0 : -1}
                    style={st.option(selected)}
                    onClick={() => {
                      setListboxFocusIndex(i);
                      selectSession(s.id);
                    }}
                    onFocus={() => setListboxFocusIndex(i)}
                  >
                    <div style={{ fontWeight: 600 }}>{s.title}</div>
                    <div style={{ fontSize: 12, color: C.sub, display: "flex", gap: 8, alignItems: "center", marginTop: 2 }}>
                      <span>{s.date}</span>
                      <span>{s.length}</span>
                      {s.statusChip && (
                        <span style={st.chip(chipColor(s.statusChip))}>
                          <span aria-hidden="true">{chipGlyph(s.statusChip)}</span>
                          {s.statusChip}
                        </span>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </nav>

        {/* ---- transcript (main landmark) ---- */}
        <main style={st.main} aria-label="Transcript">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 20px", borderBottom: `1px solid ${C.line}` }}>
            <h2 style={st.paneTitle}>{session.title}</h2>
            {/* Spoken | Source toggle (default Spoken) */}
            <div role="group" aria-label="Transcript view" style={{ display: "inline-flex", border: `1px solid ${C.line}`, borderRadius: 8, overflow: "hidden" }}>
              <button
                type="button"
                aria-pressed={transcriptView === "spoken"}
                style={st.radio(transcriptView === "spoken")}
                onClick={() => setTranscriptView("spoken")}
              >
                Spoken
              </button>
              <button
                type="button"
                aria-pressed={transcriptView === "source"}
                style={{ ...st.radio(transcriptView === "source"), borderRight: "none" }}
                onClick={() => setTranscriptView("source")}
              >
                Source
              </button>
            </div>
          </div>

          <div ref={transcriptScrollRef} style={st.transcriptScroll} onScroll={computeInView}>
            {blocks.map((b, i) => {
              const active = i === activeBlockIndex;
              const lvl = levelOf(b);
              const range = displayRange(b);
              return (
                <section
                  key={b.id}
                  ref={(n) => (blockRefs.current[b.id] = n)}
                  aria-current={active ? "true" : undefined}
                  style={st.row(active)}
                >
                  <div style={st.meta}>
                    <span style={st.klassTag}>{b.klass}</span>
                    <span style={st.chip(statusColor(b.status))}>
                      <span aria-hidden="true">{statusGlyph(b.status)}</span>
                      {b.status}
                    </span>
                    {lvl && <span style={{ color: C.sub }}>{LEVEL_META[lvl].short}</span>}
                    <span style={{ color: C.sub }}>
                      {fmt(range.startMs)}–{fmt(range.endMs)}
                    </span>
                    {active && <span style={{ color: C.accent, fontWeight: 600 }}>▶ playing</span>}
                  </div>

                  {/* honesty branch */}
                  {b.status === "refused" ? (
                    <div style={st.refusalBox} role="note">
                      <div style={{ fontWeight: 600 }}>
                        <span aria-hidden="true">{statusGlyph("refused")} </span>
                        {transcriptView === "source" ? b.sourceText : b.refusal.notice}
                      </div>
                      <div style={{ fontSize: 12, marginTop: 4 }}>
                        Source: {b.refusal.source} · reason: {b.refusal.reason}
                      </div>
                    </div>
                  ) : (
                    <p style={st.text}>{displayText(b)}</p>
                  )}

                  {/* degraded reason chip — distinct from the verbatim spoken text */}
                  {b.status === "degraded" && (
                    <div style={{ ...st.chip(C.degraded), marginBottom: 8 }}>
                      <span aria-hidden="true">{statusGlyph("degraded")}</span>
                      {b.degradedReason}
                    </div>
                  )}

                  <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
                    <button
                      type="button"
                      ref={(n) => (playBtnRefs.current[b.id] = n)}
                      style={st.btn}
                      aria-label="Play from this block"
                      onClick={() => playFromBlock(i)}
                    >
                      ▶ Play from here
                    </button>

                    {/* L1/L2/L3 control — ONLY for non-refused rows (Model A escalation surface) */}
                    {b.status !== "refused" && (
                      <div>
                        <div
                          role="radiogroup"
                          aria-label="Detail level"
                          title="L1 gist · L2 summary · L3 detail"
                          style={st.radiogroup}
                          onKeyDown={(e) => onRadiogroupKey(e, b)}
                        >
                          {[1, 2, 3].map((n) => {
                            const checked = lvl === n;
                            return (
                              <button
                                key={n}
                                type="button"
                                role="radio"
                                aria-checked={checked}
                                aria-label={LEVEL_META[n].label}
                                tabIndex={checked ? 0 : -1}
                                ref={(node) => {
                                  if (!radioRefs.current[b.id]) radioRefs.current[b.id] = {};
                                  radioRefs.current[b.id][n] = node;
                                }}
                                style={{ ...st.radio(checked), borderRight: n === 3 ? "none" : `1px solid ${C.line}` }}
                                onClick={() => setLevel(b, n)}
                              >
                                {LEVEL_META[n].short}
                              </button>
                            );
                          })}
                        </div>
                        {/* inline, non-dismissable polite live region (mounted always) */}
                        <div role="status" aria-live="polite" style={st.status}>
                          {escalatingBlockId === b.id && <span aria-hidden="true">⧗ </span>}
                          {escalationMsg[b.id] || ""}
                        </div>
                      </div>
                    )}
                  </div>
                </section>
              );
            })}
          </div>
        </main>
      </div>

      {/* transport rendered at BOTTOM when anchored bottom (default) */}
      {transportAnchor === "bottom" && (
        <TransportDeck
          st={st}
          C={C}
          anchor="bottom"
          isPlaying={isPlaying}
          setIsPlaying={setIsPlaying}
          activeBlockIndex={activeBlockIndex}
          blocks={blocks}
          playheadMs={playheadMs}
          scrubPct={scrubPct}
          sliderValueText={sliderValueText}
          totalMs={totalMs}
          toolbarRefs={toolbarRefs}
          toolbarFocusIndex={toolbarFocusIndex}
          onToolbarBtnKey={onToolbarBtnKey}
          onSliderKey={onSliderKey}
          seekToBlock={seekToBlock}
          returnToPlaying={returnToPlaying}
          activeInView={activeInView}
        />
      )}
    </div>
  );
}

/* ----------------------------------------------------------------------------
 * TransportDeck — single component, rendered from exactly one of two anchor
 * positions. State lives in the parent so flipping top<->bottom preserves it.
 * role=toolbar with roving tabindex; the scrubber is a block-quantized slider.
 * --------------------------------------------------------------------------*/
function TransportDeck(props) {
  const {
    st, C, anchor, isPlaying, setIsPlaying, activeBlockIndex, blocks, playheadMs,
    scrubPct, sliderValueText, toolbarRefs, toolbarFocusIndex, onToolbarBtnKey,
    onSliderKey, seekToBlock, returnToPlaying, activeInView,
  } = props;

  const atStart = activeBlockIndex === 0;
  const atEnd = activeBlockIndex === blocks.length - 1;

  return (
    <div id="transport" style={st.transportWrap(anchor)}>
      <div role="toolbar" aria-label="Playback transport" style={st.toolbar}>
        {/* 0 — Prev */}
        <button
          type="button"
          ref={(n) => (toolbarRefs.current[0] = n)}
          tabIndex={toolbarFocusIndex === 0 ? 0 : -1}
          style={st.btn}
          aria-label="Previous block"
          aria-disabled={atStart}
          onKeyDown={(e) => onToolbarBtnKey(e, 0)}
          onClick={() => !atStart && seekToBlock(activeBlockIndex - 1)}
        >
          ⏮ Prev
        </button>

        {/* 1 — Play/Pause (label flips, NO aria-pressed) */}
        <button
          type="button"
          ref={(n) => (toolbarRefs.current[1] = n)}
          tabIndex={toolbarFocusIndex === 1 ? 0 : -1}
          style={st.btnPrimary}
          onKeyDown={(e) => onToolbarBtnKey(e, 1)}
          onClick={() => setIsPlaying((p) => !p)}
        >
          {isPlaying ? "⏸ Pause" : "▶ Play"}
        </button>

        {/* 2 — block-quantized scrubber (role=slider) */}
        <div
          ref={(n) => (toolbarRefs.current[2] = n)}
          role="slider"
          aria-label="Scrubber"
          tabIndex={toolbarFocusIndex === 2 ? 0 : -1}
          aria-valuemin={0}
          aria-valuemax={blocks.length - 1}
          aria-valuenow={activeBlockIndex}
          aria-valuetext={sliderValueText}
          style={st.sliderTrack}
          onKeyDown={onSliderKey}
          onClick={(e) => {
            const rect = e.currentTarget.getBoundingClientRect();
            const pct = (e.clientX - rect.left) / rect.width;
            seekToBlock(Math.round(pct * (blocks.length - 1)));
          }}
        >
          <div style={st.sliderFill(scrubPct)} />
          <div style={st.sliderThumb(scrubPct)} />
        </div>

        {/* time readout */}
        <span style={{ fontSize: 13, color: C.sub, minWidth: 96, textAlign: "center" }}>
          {fmt(playheadMs)} · blk {activeBlockIndex + 1}/{blocks.length}
        </span>

        {/* 3 — Next */}
        <button
          type="button"
          ref={(n) => (toolbarRefs.current[3] = n)}
          tabIndex={toolbarFocusIndex === 3 ? 0 : -1}
          style={st.btn}
          aria-label="Next block"
          aria-disabled={atEnd}
          onKeyDown={(e) => onToolbarBtnKey(e, 3)}
          onClick={() => !atEnd && seekToBlock(activeBlockIndex + 1)}
        >
          Next ⏭
        </button>

        {/* Return to playing — re-uses roving slot via keyboard from neighbors;
            kept outside the 4-slot roving set as a plain control reachable by
            Tab after the toolbar, to keep the roving model simple and correct. */}
        <button
          type="button"
          style={st.btn}
          aria-label="Return to playing block"
          aria-disabled={activeInView}
          onClick={() => !activeInView && returnToPlaying()}
        >
          ⌖ Return to playing
        </button>
      </div>
    </div>
  );
}
