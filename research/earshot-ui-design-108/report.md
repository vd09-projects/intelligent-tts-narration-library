---
title: "Earshot — Production UI Design (research report)"
slug: earshot-ui-design-108
ticket: "#108"
version: 1
status: complete
intent_class: UI
date: 2026-06-28
stages_run: [0-frame, 1-fanout, 2-opinion, 3-adversarial-verify, 4-design-loop, 5-report]
load_bearing_claims: 7
verdicts: { verified: 7, single_source: 0, contested: 0, unsupported: 0, model_only: 0 }
weakest_surface: "per-block L1/L2/L3 voicing control (novel synthesis, bounded-negative grounding)"
transport_anchor_decision: bottom
---

# Earshot — Production UI Design

> Research deliverable for ticket **#108** — "Earshot UI design (production-grade)".
> This is a **design report**, not an implementation. The build is a follow-up `dev` ticket.
> Every load-bearing claim was adversarially re-verified in Stage 3 (see Evidence Grades);
> the design surfaces grounded in those claims are marked with their grade inline.

---

## 1. Problem restated & intent

**X (have):** an existing React reference player (`player/src`) — a single-document viewer
with raw `<audio controls>`, a flat block list (`BlockList`/`BlockRow`), inline escalation
(`EscalateCard`), and a top bar for directory loading. The user finds it static, hard to use,
and buggy.

**Y (want):** a production-grade, properly product-designed UI — "Earshot" — that is *honestly
better* than the current player, not a beautification pass over prior bad decisions. Three
panes: a **net-new session pane**, a **transcript pane**, and a **persistent transport bar**.
Per-control interaction must be accessible. Reuse is **permitted but not load-bearing** —
quality wins over reuse (user steer, §10).

**Intent class:** UI design. After verification (Stage 3), the design loop (Stage 4) produced
the concrete layout, interaction spec, and the per-block level control below.

**Acceptance criteria coverage:**

| AC | Description | Status | Where answered |
|----|-------------|--------|----------------|
| AC1 | Recommend a mockup production method + produce the mockup | Answered | §4, §8 |
| AC2 | Concrete finalized layout (session / transcript / transport) | Answered | §5 |
| AC3 | Per-control interaction spec (APG-grounded) | Answered | §6, §7 |

---

## 2. The opinion (now verified)

The Stage 2 opinion held after adversarial verification. In one line:

> **Build Earshot as a Material list-detail layout (left session pane, center spoken
> transcript, bottom persistent transport), mock it first as a free Claude Artifact
> (interactive, downloadable React), ground every control in W3C APG roles, and ship the
> per-block L1/L2/L3 control as an explicit novel synthesis of three documented patterns.
> Reuse `usePlayback` logic; rebuild all UI to production quality.**

Nothing in the opinion was overturned by verification. One factual correction landed (claim B:
"Claude Design" is a real, named Anthropic Labs feature — and it remains paid-only, so the
disqualifier still holds).

---

## 3. Evidence grades (Stage 3 verdicts)

All 7 load-bearing claims **VERIFIED**. None killed. Full detail in `_VERIFICATION.md`.

| # | Claim | Grade | Primary source (fetched 2026-06-28) | Caveat carried into design |
|---|-------|-------|-------------------------------------|----------------------------|
| A | Claude Artifacts free tier → interactive React, downloadable | **VERIFIED** | support.claude.com/en/articles/9487310 | Paywall is on *Live/AI-powered/MCP/persistent* artifacts, NOT static clickable React. Mockup unaffected. |
| B | Claude Design has no free tier (cheapest qualifying = Pro $20/mo) | **VERIFIED** | anthropic.com/news/claude-design-anthropic-labs; claude.com/pricing | Real feature ("Claude Design", Anthropic Labs, research preview). Max/Team/Enterprise also unlock. Disqualifier holds. |
| C1 | v0 free = $5/mo token credits | **VERIFIED** | v0.app/pricing, v0.app/docs/pricing | Pricing-volatile, pinned to date. ~7 msgs/day. |
| C2 | Figma free = 3-file team cap; Pro $16/Full seat/mo | **VERIFIED** | figma.com help + pricing | $16 = Full seat (post-Mar-2025 seat model; Dev $12 / Collab $3). |
| D | Material list-detail is the documented canonical layout | **VERIFIED** | m3.material.io/foundations/layout/canonical-layouts/list-detail | "supporting pane" is a distinct layout; we correctly use list-detail. |
| E | W3C APG roles (radio / slider / toolbar / listbox) as specced | **VERIFIED** | w3.org/WAI/ARIA/apg/patterns/{radio,slider,toolbar,listbox}/ | Toolbar uses **roving tabindex** (not activedescendant); slider uses `aria-valuetext`. |
| F | No authoritative source for a per-block 3-level voicing UI | **VERIFIED (bounded negative)** | absence across TTS + per-block + 3-level-fidelity | Cannot prove exhaustively → frame as **novel synthesis**, not invention, not cited pattern. **Weakest surface.** |
| G | Session pane net-new; `usePlayback` logic reusable | **VERIFIED** | direct source inspection (file:line) | `usePlayback` is domain-coupled (Manifest types), not a generic drop-in. |

**Grading legend:** VERIFIED = multiple/first-party authoritative; bounded-negative = claim of
absence holds within a defined search frame but cannot be proven exhaustively.

---

## 4. AC1 — Recommended mockup production method

**Recommendation: Claude Artifacts (free tier).** [grade A: VERIFIED]

Ranked, no pre-bias:

| Rank | Method | Cost | Output | Why / why not |
|------|--------|------|--------|---------------|
| **1** | **Claude Artifacts (free)** | **$0** | Interactive React, downloadable | Target stack; clickable sign-off fast; code drops into the eventual Earshot codebase. *Caveat:* only static/pre-written-JS artifacts are free — no Live/AI-powered/MCP artifacts on free [A]. A mockup needs none of those. |
| 2 | Hand-coded React prototype | $0 | Production React | Strongest reuse (output *is* the code); loses on speed-to-first-mockup. Best as the step *after* an Artifact is approved. |
| 3 | Figma free tier | $0 | Design file (not React) | Highest static + clickable-prototype fidelity, but output is not code [C2: 3-file cap]. |
| 4 | AI app-builders (v0 / bolt / Lovable) | credit-limited | React-ish full app | Full-app-heavy; free tiers credit-constrained [C1: $5/mo]. |
| 5 | Claude Design | $20/mo (Pro, cheapest) | Most polished | **Disqualified** — no free tier; fails no-recurring-spend [B]. |

**Production path:** (1) this report's ASCII wireframe + interaction tables → (2) a **free
clickable Claude Artifact** built from the spec in §6–§8 → (3) user sign-off → (4) hand-port
the approved Artifact React into `player/` (the build follow-up ticket). The clickable Artifact
is a **follow-up step** (§11), not produced inline here; this report delivers the concrete
wireframe + interaction tables that the Artifact is built from.

---

## 5. AC2 — Finalized layout

Material **list-detail** canonical layout [grade D: VERIFIED], extended with a third
persistent region (the transport deck).

### 5.1 Wireframe (desktop, ≥1024px)

```
┌───────────────────────────────────────────────────────────────────────────┐
│  Earshot            [ Load directory ▾ ]  [ ⚙ ]            (app header bar)  │  ← app chrome (top)
├──────────────┬────────────────────────────────────────────────────────────┤
│ SESSIONS  «  │  TRANSCRIPT — design-doc.md            [ Spoken | Source ]   │
│ (net-new)    │                                                             │
│ ┌──────────┐ │  ┌──────────────────────────────────────────────────────┐  │
│ │● design… │ │  │ heading · L1 · voiced            0:00–0:04            │  │
│ │  06-28 4m│ │  │ "Solution phase design"                              │  │
│ ├──────────┤ │  │              [ L1 ·L2· L3 ]   ⏯ from here            │  │  ← active block
│ │  api-ref │ │  ├──────────────────────────────────────────────────────┤  │     (aria-current)
│ │  06-27 9m│ │  │ prose · L2 · degraded            0:04–0:31  ⚠         │  │
│ ├──────────┤ │  │ "Read verbatim — no intelligence adapter configured" │  │
│ │  readme  │ │  │              [·L1· L2  L3 ]   ⏯ from here            │  │
│ │  ⚠ stale │ │  ├──────────────────────────────────────────────────────┤  │
│ └──────────┘ │  │ image · — · refused              ——                  │  │
│              │  │ 🔇 "Image not voiced: no description available"      │  │  ← refusal (spoken+shown)
│              │  │     source: fig-2.png                                │  │
│              │  └──────────────────────────────────────────────────────┘  │
├──────────────┴────────────────────────────────────────────────────────────┤
│  ⏮  ⏯  ⏭   ●━━━━━━━━━━━○────────────  0:12 / 4:03   ⟲ return to playing   │  ← persistent transport
└───────────────────────────────────────────────────────────────────────────┘     (BOTTOM-anchored)
```

Mobile (<768px): session pane collapses to a top sheet ("Sessions ▾"); transcript is
full-width; transport stays bottom-fixed (thumb reach).

### 5.2 Regions

| Region | Anchor | Reuse verdict | Notes |
|--------|--------|---------------|-------|
| App header | top, fixed | refactor `TopBar` | Directory load + settings only. App chrome, not transport. |
| **Session pane** | left, collapsible | **NET-NEW** [G] | Rows: title · date · length · one status chip (stale/error). No history concept exists in `player/src` today (grep clean) [G]. |
| **Transcript pane** | center, scroll | rebuild `BlockList`/`BlockRow`; keep `SourcePane` behind toggle | Ordered spoken blocks. `[ Spoken | Source ]` segmented toggle resolves the spoken-vs-source ambiguity (§9). |
| **Transport bar** | **bottom**, fixed full-width | logic REUSE (`usePlayback`), UI NET-NEW [G] | Replaces raw `<audio controls>`. |

### 5.3 Transport anchor decision: **BOTTOM** (delegated to design; justified)

Sources split and design systems decline to prescribe (Readwise Reader anchors top; Speechify
anchors bottom — contradiction left standing in §10). The user delegated this call. **Decision:
bottom-anchored, full-width, `position: fixed`.**

Justification:
1. **Domain convention.** Audio-first products converge on a persistent bottom "now-playing"
   deck — Spotify, Apple Podcasts, SoundCloud, YouTube Music, Speechify. Earshot is audio-first;
   matching the dominant audio mental model lowers learning cost more than matching a
   read-it-later app (Readwise).
2. **Reading flow.** The transcript is the primary surface and scrolls vertically. Top space is
   spent on app chrome + load controls; pinning transport to the bottom keeps the scrolling
   reading column clean and keeps playhead controls out of the content's way.
3. **Block-level sync pairing.** Sync is block-level only, so "return to current block"
   (`⟲`) is a first-class control. It lives in the bottom deck exactly like Spotify's
   "now playing" jump — you scroll the transcript freely, the deck always shows + returns to
   the playing block.
4. **Touch reach.** On mobile the bottom bar is within thumb reach; a top bar is not.
5. **Accessibility.** A bottom `role="toolbar"` is fully reachable; we add a skip-link
   ("Skip to transport") and the toolbar participates in tab order via roving tabindex [E].

**Tradeoff acknowledged (not settled):** on very short content a top bar is visible without any
scroll, and some read-it-later users expect controls at top. Mitigated by `position: fixed` (the
bar is always visible regardless of scroll) and by the `⟲ return to playing` control. If user
testing of the Artifact shows confusion, top-anchor is a cheap flip (one CSS region).

---

## 6. AC3 — Per-control interaction spec (W3C APG-grounded) [grade E: VERIFIED]

Every control below maps to a verified APG pattern. Roles/attributes are quote-pinned in
`_VERIFICATION.md` claim E.

| Control | Role / pattern | Keyboard | ARIA | Behavior |
|---------|----------------|----------|------|----------|
| **Transport bar** | `role="toolbar"`, **roving tabindex** | ←/→ move between controls; Tab enters/exits as one stop | `aria-label="Playback transport"` | Single tab stop; arrows traverse buttons (APG toolbar) [E]. |
| **Play / Pause** | button, media action-label model | Space/Enter toggles | label flips "Play"↔"Pause"; **no `aria-pressed`** | Proxies `usePlayback.play()/pause()` [G]. Icon + visible label. |
| **Prev / Next block** | button | Space/Enter | `aria-label="Previous block"/"Next block"` | `seekToBlock(prev/next id)` [G]. Disabled at ends (`aria-disabled`). |
| **Scrubber** | `role="slider"` | ←/→ step block; Home/End first/last block; PageUp/Down ±5 blocks | `aria-valuemin/max/now` + **`aria-valuetext`="0:12, block 2 of 8, prose"** | Block-quantized (sync is block-level, never word-level). Drag/keyboard snaps to nearest block start [E]. |
| **Return to current block** (`⟲`) | button | Space/Enter | `aria-label="Return to playing block"`; `aria-disabled` when already in view | Scrolls transcript to `activeBlockId` and moves focus there. First-class because sync is block-level. |
| **Session load** (header) | menu button → file/dir picker | Space/Enter opens | `aria-haspopup`, `aria-expanded` | Refactor of existing `useDirectoryLoader`/`useFixture`. |
| **Session list** (left pane) | `role="listbox"` (selection model) | ↑/↓ move, Enter activate | `aria-selected` on current; `aria-label="Sessions"` | Listbox (not list-of-links) because selecting a session *changes the detail pane* — it is selection, not navigation [E]. |
| **Escalate → inline card** | `aria-expanded` + `aria-controls` on trigger; card `role="region"` | Space/Enter toggles; Esc dismisses | trigger `aria-expanded`; region `aria-label="Escalate command"` | **Inline only, never modal/toast** (honesty rule). Keep current `EscalateCard` contract (`player/src/components/EscalateCard.tsx`) — already correct. |
| **L1/L2/L3 level control** | `role="radiogroup"` (see §7) | ←/→ or ↑/↓ move + select; Tab = one stop | `aria-label="Detail level"`; each option `role="radio"` `aria-checked` | The novel-synthesis surface — full spec in §7. |

### 6.1 Per-block row (rebuilt `BlockRow`)

The current `BlockRow` has two production weaknesses the user flagged: (a) the click-to-seek
wrapper is `aria-hidden="true"` and mouse-only, redundant with a separate "Seek" button —
confusing dual affordance; (b) escalation is **up-only** ("Escalate L{n}" buttons) so a user
cannot return to a lower level or even *see* that three levels exist.

Rebuilt row contract:
- One **"⏯ from here"** affordance per row (replaces the dual click-wrapper + Seek button):
  a real focusable button, `aria-label="Play from this block"`, calling `seekToBlock(id)` then
  `play()` [G]. The whole row is still mouse-clickable, but the keyboard path and the visible
  control are the *same* button (no `aria-hidden` ghost).
- The **L1/L2/L3 segmented control** (§7) replaces the up-only escalate buttons — exposes all
  three states, current state checked.
- `aria-current="true"` on the active block (kept from current `BlockRow`).
- Refused blocks: spoken refusal text + source map shown, no level control (nothing to
  escalate to) — honesty rule, §9.

---

## 7. The per-block L1/L2/L3 control — explicit novel synthesis [grade F: bounded-negative]

**This is the weakest-grounded surface in the design.** Verification found **no single
authoritative source** for a UI affordance that is simultaneously (a) about voicing/TTS,
(b) per-block granular, and (c) three escalating *content-fidelity* levels [F]. We therefore
design it **honestly as a novel synthesis** of three documented patterns — not as an invention,
and not by falsely citing an established pattern.

### 7.1 Cited lineage (each contributes one axis)

| Source pattern | What it contributes | What it lacks here |
|----------------|---------------------|--------------------|
| **Shneiderman — "Overview first, zoom and filter, details-on-demand"** (visual information-seeking mantra) | The 3-level *detail-on-demand* progression | Visual, not voicing; document-level, not per-block |
| **Screen-reader verbosity levels** (JAWS / NVDA high·medium·low) | A 3-level control *over speech* | Controls structural chrome, not content summaries; global, not per-block |
| **Summarizer length sliders** (QuillBot, GetDigest short/medium/long) | 3 levels of *content fidelity* | Document-level text output, not voiced, not per-block |

Earshot's synthesis = **per-block × voiced × content-fidelity**, the intersection none of the
three covers alone. The design rationale (and the decision-journal entry) state this plainly.

### 7.2 Why a radio-group (segmented control), not a disclosure

A plain disclosure ("show more" / progressive expand) can only model two states — collapsed and
expanded. L1/L2/L3 has a **middle** state (L2 summary) that is neither the terse gist nor the
full detail. A disclosure cannot honestly represent "you are currently at the middle level."
A `radiogroup` of three radios exposes **all three states at once with exactly one checked** —
the honest representation [E].

### 7.3 Control spec

```
   ┌─────────────────────────────┐
   │  Detail:  [·L1·] [ L2 ] [ L3 ]   │   ← role="radiogroup" aria-label="Detail level"
   └─────────────────────────────┘       each cell role="radio", one aria-checked="true"
```

| Aspect | Spec |
|--------|------|
| Role | `role="radiogroup"`, `aria-label="Detail level"`; cells `role="radio"` with `aria-checked` [E] |
| Keyboard | Tab = single stop into the group; ←/→ (and ↑/↓) move *and* select the focused radio (APG radio pattern) [E] |
| Visual | Segmented control; checked segment filled; level meaning in tooltip ("L1 gist · L2 summary · L3 detail") |
| Selecting a *higher* level | escalation: `planner.Plan` re-runs for that one block → `RenderBlock` patches just that block's audio + timing (domain rule). Inline loading state on the row (`role="status"`), never a toast. |
| Selecting a *lower* level | returns to a cached level (intelligence cache keyed by content-hash/level/model — escalation doesn't re-bill, domain rule). |
| Structured classes (code/config/table/heading/list) | deterministic L1; L2/L3 enrich but never block voicing. The control still shows three states; lower levels are always available. |
| Refused block | control hidden — nothing to voice at any level (§9). |
| Honesty | the control reflects the block's **actual** `status` (`voiced`/`degraded`/`refused`) — e.g. prose read verbatim shows `degraded` at the chosen level, never a faked gist. |

This control **reuses the existing escalation machinery** (`useEscalation`, `escalateCommand`,
`escalateTargets`, `EscalateCard`) but replaces the up-only button UI with the honest
three-state segmented control.

---

## 8. AC1 deliverable — the mockup (this report's wireframe + the Artifact follow-up)

Per §4, the recommended method is a **free Claude Artifact**. The concrete, build-ready
artifacts delivered *in this report* are:
- The **desktop + mobile wireframe** (§5.1) and the **level-control wireframe** (§7.3).
- The **interaction tables** (§6, §7.3) — every control's role, keys, ARIA, behavior.
- The **reuse map** (§5.2, §9) — what to reuse, refactor, rebuild.

The **clickable interactive Artifact** is the next production step (follow-up ticket §11): build
a static React Artifact (no Live/AI features needed → free [A]) from these tables, get user
sign-off, then hand-port into `player/`.

---

## 9. Honesty rule in the UI + reuse map

**Honesty rule (non-negotiable, project invariant):** refusals are *spoken and surfaced*, never
silent, never fabricated.

| UI surface | Honesty behavior |
|------------|------------------|
| Refused block | Spoken notice ("Image not voiced: no description available") rendered in the row + `SourceMap` shown; no level control. Keep `RefusalBadge`. |
| Degraded block | `degraded` status chip + reason ("read verbatim — no intelligence adapter"). Never presented as a real gist. |
| Stale session/block | `manifest.json` stale flag → visible "⚠ stale" chip in the session pane + row; **no auto-regenerate** (domain rule). Keep `StaleBadge`. |
| Escalate | Inline card / inline status only — never a toast that could imply success that didn't happen. Keep `EscalateCard` contract. |
| Spoken vs source | `[ Spoken | Source ]` toggle. **Default = Spoken** (what was actually voiced; spoken text ≠ source text by design). Source view (existing `SourcePane`) lets the user verify against the raw input. This resolves the spoken-vs-source ambiguity flagged in the opinion — *decision made, not blocked.* |

**Reuse map** (user steer: reuse permitted, not load-bearing; quality wins):

| Existing | Verdict | Action |
|----------|---------|--------|
| `usePlayback` (hook) | **REUSE logic as-is** [G] | Block-level seek + play/pause are correct; domain-coupled but that's fine within Earshot. |
| `useEscalation`, `escalate*` libs | REUSE logic | Drives the L1/L2/L3 control. |
| `useDirectoryLoader`, `useFixture`, `loadFromServer` | REUSE logic | Feeds session pane + load menu. |
| `findActiveBlock`, `reconcileManifest` | REUSE | Sync + patch logic. |
| `EscalateCard`, `RefusalBadge`, `StaleBadge` | REUSE component (minor restyle) | Already honesty-rule-correct. |
| `BlockRow`, `BlockList` | **REBUILD** | Fix dual click/seek affordance + up-only escalate; new row contract §6.1. |
| `TopBar` | REFACTOR | Strip transport concerns; keep load + settings. |
| Raw `<audio controls>` | **REPLACE** | New bottom transport deck §5.3. |
| Session pane | **NET-NEW** [G] | No prior code. |

---

## 10. Contradictions left standing & open questions

- **Transport anchor (top vs bottom).** Sources split (Readwise top / Speechify bottom);
  design systems decline. **Resolved by decision** (§5.3, bottom) with an acknowledged tradeoff
  and a cheap flip path. Not a verifiable fact — a design call.
- **Per-block L1/L2/L3 grounding [F].** Bounded-negative: absence cannot be proven exhaustively.
  Shipped as novel synthesis with cited lineage (§7). Carries the most design risk; the Artifact
  user-test should specifically probe whether the three-state control reads as intuitive.
- **Reuse weight vs #107.** If `player/` is deleted by another track, "reuse existing
  components" weakens — but the gap analysis (§9) still tells the build which logic/patterns to
  port. Reuse is not load-bearing (user steer).
- **`usePlayback` is domain-coupled** to project Manifest types [G] — fine for Earshot, but it is
  not a generic drop-in if the data model changes.

None of these block the design. They are recorded as the honest edges of it.

---

## 11. Follow-up work (the research deliverable)

Proposed follow-up tickets (created via task-manager, after user approval at the session gate):

1. **[dev / high] Build Earshot UI — bottom-transport list-detail shell.** Implement the §5
   layout (net-new session pane, rebuilt transcript pane, bottom persistent transport deck),
   reusing `usePlayback` logic. AC: matches wireframe §5.1; transport `role="toolbar"` + roving
   tabindex; replaces raw `<audio controls>`.
2. **[dev / high] Per-block L1/L2/L3 segmented control (novel synthesis).** Replace up-only
   escalate buttons with the `radiogroup` three-state control (§7), reusing `useEscalation`. AC:
   all three states exposed, one `aria-checked`; honest status reflection; reuses escalation
   machinery.
3. **[dev / med] Clickable Claude Artifact mockup (free) for user sign-off.** Build the static
   React Artifact from §6–§8 tables; capture sign-off before the hand-port. AC: clickable
   prototype of all controls; $0 (no Live/AI features).
4. **[dev / med] Rebuild BlockRow — fix dual seek affordance + add "⏯ from here".** Remove the
   `aria-hidden` mouse-only click wrapper; single focusable play-from-here control (§6.1).
5. **[research / low] Validate the L1/L2/L3 control with user testing.** Probe the weakest
   surface [F]: does the three-state segmented control read intuitively? Feeds an anchor-flip /
   pattern-revision decision. AC: usability signal on the novel-synthesis control + transport
   anchor.

Final rune classification, priority, and duplicate-check happen in task-manager at creation.

---

## 12. Sources (all fetched 2026-06-28)

- Claude Artifacts (free, interactive React, download): https://support.claude.com/en/articles/9487310
- Claude Design (Anthropic Labs, paid): https://www.anthropic.com/news/claude-design-anthropic-labs ; https://claude.com/pricing
- v0 pricing: https://v0.app/pricing ; https://v0.app/docs/pricing
- Figma pricing/seats: figma.com help center + pricing page
- Material Design 3 list-detail canonical layout: https://m3.material.io/foundations/layout/canonical-layouts/list-detail
- W3C APG patterns: https://www.w3.org/WAI/ARIA/apg/patterns/{radio,slider,toolbar,listbox}/
- Shneiderman, "The Eyes Have It" (visual information-seeking mantra: overview, zoom/filter, details-on-demand)
- Screen-reader verbosity levels (JAWS / NVDA documentation)
- Summarizer length controls (QuillBot, GetDigest)
- Repo code: `player/src/hooks/usePlayback.ts`, `player/src/components/{BlockRow,EscalateCard,SourcePane,RefusalBadge,StaleBadge}.tsx`, `player/src/App.tsx`

---

## Changelog

- **v1 (2026-06-28)** — Initial report. Stages 0–5 complete. 7/7 load-bearing claims verified.
  Stage 4 design loop: finalized list-detail layout; **transport anchor decided = bottom**
  (delegated, justified §5.3); full APG-grounded interaction spec; per-block L1/L2/L3 control
  designed as explicit novel synthesis (§7); AC1 method = free Claude Artifact; spoken-vs-source
  resolved (spoken default, source toggle). 5 follow-ups proposed.
