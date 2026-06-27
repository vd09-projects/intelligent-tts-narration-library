# HUGINN CHECKPOINT 2 — THE OPINION (Stage 2) — earshot-ui-design-108 (#108)

**Persisted:** 2026-06-28 (recovered verbatim from user paste after context loss across resume)
**Status:** Opinion approved by user with verdict = VERIFY. Stage 3 adversarial verification in progress.
**Intent class:** UI → after Stage 3, Stage 4 (design loop) is in play.

---

## RECOMMENDATION (provisional — not yet verified)

### AC1 — mockup production method (ranked, no pre-bias)
1. **Claude Artifacts (free)** — $0, outputs interactive React (target stack), downloadable into the eventual Earshot codebase. Fast path to clickable sign-off.
2. **Hand-coded React prototype** — strongest reuse (output IS the eventual code), $0, full fidelity; loses on speed-to-first-mockup. Best as the step after an Artifact is approved.
3. **Figma free tier** — highest static + clickable-prototype fidelity at $0, but output is a design file, not React.
4. **AI app-builders (v0 / bolt / Lovable)** — React-friendly but full-app-heavy; free tiers credit-constrained.
5. **claude.ai Design** — most polished, but NO free tier (requires Claude Pro $20/mo) → fails no-recurring-spend constraint unless already subscribed.
- **ASCII wireframes**: $0 precursor for drafting interaction spec in text, not an approval deliverable on its own.

### AC2 — layout
List-detail canonical layout (Material): LEFT collapsible session pane (rows: title/date/length + one status chip), CENTER transcript pane (ordered blocks), persistent full-width transport bar (bottom-anchored — defensible, not settled; sources split top vs bottom). Sync is block-level only → tap-a-block-to-seek + "return to current block" resync control.

### AC3 — interaction spec (W3C APG-grounded)
- transport as `role="toolbar"` (roving tabindex)
- scrubber as `role="slider"` with `aria-valuetext` spoken-time
- play/pause via media action-label model (label flips, no `aria-pressed`)
- session list as listbox if selection, list-of-links if navigation
- per-block L1/L2/L3 as radio-group / segmented control (exposes all three states honestly — plain disclosure can't model L2 middle)

### SQ4 gap analysis (resolved against player/src)
- Session pane → **NET-NEW**. No session/history/list concept exists (grep clean; single `active` triple, one dir at a time).
- File/transcript pane → **RENAME/REFACTOR** of existing BlockList+BlockRow (and SourcePane for raw source). Earshot must decide whether "transcript" = spoken blocks or raw source — repo has both.
- Playback transport → logic **REUSE-AS-IS** (usePlayback), UI **NET-NEW** (today raw `<audio controls>`).
- Per-block leveling → **RENAME/REFACTOR** — full L1/L2/L3 escalate already exists end-to-end.

---

## CONTRADICTIONS LEFT STANDING (not averaged)
- **Claude Artifacts on Free:** official support article says React artifacts supported on Free; pricing matrix appears to gate "Artifacts" to paid. Likely: basic artifacts free, advanced (live/persistent) paid. LOAD-BEARING for #1 pick — must verify.
- **Transport anchor:** top (Readwise) vs bottom (Speechify); design systems decline to prescribe.
- **Reuse weight vs #107:** if player/ deleted, "reuse existing components" weakens — but gap analysis still tells Earshot what patterns/logic to port.

---

## LOAD-BEARING CLAIMS — STAGE 3 MUST TRY TO BREAK
1. **Claude Artifacts free tier supports interactive React components, downloadable for reuse** (the contradiction above).
2. **claude.ai Design has no free tier / requires Claude Pro $20/mo** (the disqualifier).
3. **v0 free = $5/mo token credits; Figma free = 3-file team cap, Professional $16/editor/mo** (cost claims driving the ranking).
4. **Material list-detail canonical layout is the documented pattern** for list+detail (session+transcript).
5. **APG: radio-group (not disclosure) is correct accessible pattern for a 3-state level control**; slider/toolbar/listbox roles as specced.
6. **Gap: no authoritative source documents a per-block L1/L2/L3 voicing UI affordance** — flag as model/inference, not a cited pattern (weakest-grounded surface).
7. **SQ4 internal claims** (session pane net-new; transport logic reusable) — code-cited, high confidence.

---

## USER STEER (verbatim — apply through verify + design)
> "Yeah you don't need to explicitly delete everything. If something is reusable, please use it but make it more dynamic, very easy to use, this kind of UI. I am more expecting a very static, very difficult-to-build UI and I actually don't like our current UI. If there are some components that are reusable, sure go ahead with it but I saw multiple bugs in it, multiple other issues also. If building from scratch is too much of a hazard then sure reuse it. If we can design a production-level, real-life, proper product-designed UI that is way better than just sticking to some bad decisions we made earlier, then sure. If you still think that is good enough and we just need to do some beautification on top of it, I'll leave that decision to you."

**Interpretation:** reuse permitted but NOT load-bearing — quality wins over reuse; don't anchor to prior bad decisions/bugs; target a production-level, real, properly product-designed UI explicitly better than the current player; make reused components more dynamic/easier; build-vs-beautify-vs-rebuild delegated to huginn's judgment.
