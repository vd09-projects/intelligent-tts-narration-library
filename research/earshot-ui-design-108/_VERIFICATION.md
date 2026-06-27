# HUGINN STAGE 3 — ADVERSARIAL VERIFICATION VERDICTS — earshot-ui-design-108 (#108)

**Run:** 2026-06-28 — 8 blind claim-verifiers (depth-1), refutation-first, no opinion attached.
**Headline:** All 7 load-bearing claims hold. No claim killed. Several carry recommendation-shaping caveats. One notable correction on claim B (the "Design" feature is real and named "Claude Design").

---

## A — Claude Artifacts free tier → interactive React, downloadable for reuse
**Grade: VERIFIED** (first-party Anthropic support doc)
- Free, Pro, Max, Team, Enterprise all get Artifacts incl. "Interactive React components"; "View/Download underlying code".
- Source: https://support.claude.com/en/articles/9487310 (fetched 2026-06-28). Free artifacts tier introduced ~Feb 2026.
- **CAVEAT (load-bearing for #1 pick):** the paywall is on a DIFFERENT class — "AI-powered" / Live Artifacts (call Claude API at runtime), MCP-connected, persistent-storage are Pro+. A static clickable React mockup that runs pre-written JS is NOT paywalled. Recommendation must not promise Live/AI-powered artifacts on free.
- The original Checkpoint-2 contradiction (support doc vs pricing matrix) is RESOLVED: basic interactive artifacts free; advanced/live paid.

## B — claude.ai "Design" has no free tier / requires Pro $20/mo
**Grade: VERIFIED** (Anthropic primary)
- Feature exists, officially **"Claude Design" (Anthropic Labs)**, claude.ai/design, launched 2026-04-17, **research preview**.
- "Available for Claude Pro, Max, Team, and Enterprise subscribers." Free = $0, not listed. Pro = $20/mo.
- Sources: anthropic.com/news/claude-design-anthropic-labs ; claude.com/pricing (2026-06-28).
- **CAVEAT:** not Pro *specifically* — Max/Team/Enterprise also unlock; Pro ($20/mo) is just the cheapest qualifying paid tier. Disqualifier (no free tier) HOLDS. Terms may change (research preview).

## C1 — v0 free = $5/mo token credits
**Grade: VERIFIED** (v0 official pricing + docs)
- "$5 of included monthly credits" (v0.app/pricing & v0.app/docs/pricing, 2026-06-28). ~7 messages/day cap (secondary).
- Pricing-volatile, pinned to date.

## C2 — Figma free = 3-file team cap; Professional $16/editor/mo
**Grade: VERIFIED** (Figma help + pricing, both halves)
- "A single team with 3 files" (Starter). "Full seat: $16/mo" (Professional).
- **CAVEAT:** post-March-2025 seat model — $16 = **Full seat**; Dev seat $12, Collab seat $3. Third-party shows $15 (annual) / $20 (true monthly). First-party $16/mo governs. The "editor" in the claim = Full seat.

## D — Material list-detail canonical layout is documented pattern
**Grade: VERIFIED** (Material Design 3 primary)
- "list-detail" named exactly under MD3 "Canonical layouts": two side-by-side panes (list + detail) for a collection + selected item's detail. "ideal for ... email client or messaging app."
- Sources: m3.material.io/foundations/layout/canonical-layouts/list-detail ; developer.android.com mirror (2026-06-28).
- Note: "supporting pane" is a distinct canonical layout — claim correctly names list-detail.

## E — W3C APG roles (radio-group / slider / toolbar / listbox)
**Grade: VERIFIED** (W3C APG primary, all 4 sub-patterns quote-pinned)
- radio group = one-of-many checkable; slider role + aria-valuetext for human-friendly value (APG media-player/scrubber example); toolbar role + **roving tabindex** (explicitly, not aria-activedescendant); listbox = list of selectable options.
- Sources: w3.org/WAI/ARIA/apg/patterns/{radio,slider,toolbar,listbox}/ (2026-06-28). No sub-pattern mischaracterized.

## F — No authoritative source for per-block 3-level (L1/L2/L3) voicing UI
**Grade: VERIFIED (bounded negative)** — claim of absence holds; weakest-grounded design surface.
- No single authoritative source matches all 3 attributes: (a) voicing/TTS, (b) per-block granularity, (c) 3 escalating content-fidelity levels as a named pattern.
- Closest lineage (each captures ONE axis):
  - Shneiderman "Overview first, zoom and filter, details-on-demand" — visual 3-level detail, not voicing.
  - Screen-reader verbosity (JAWS/NVDA high/medium/low) — TTS 3-level but structural-chrome, not content summaries; global not per-block.
  - Summarizer length sliders (QuillBot, GetDigest) — content levels but document-level text, not voicing.
- **CAVEAT (must shape rationale):** absence cannot be exhaustively proven. Design rationale must position the affordance as a **novel synthesis** of documented patterns (details-on-demand + screen-reader verbosity + multi-level summarization) — NOT claim pure invention, NOT claim a cited established pattern. This is the honesty-rule-aligned framing.

## G — Repo code: session pane net-new; usePlayback logic reusable
**Grade: VERIFIED** (direct source inspection, file:line cited)
- (a) Single active document: App.tsx:61-70 scalar `plan`/`manifest` (never arrays); :94-102 new plan_id REPLACES not appends; grep for session|history|recent|past|localStorage|indexedDB across player/src = ZERO matches. Session/history concept is NET-NEW.
- (b) Transport logic in exported hook `usePlayback` (player/src/hooks/usePlayback.ts:30-33), exposes activeBlockId/currentTimeMs/seekToBlock/play/pause; App.tsx:143 consumes it; component renders only thin `<audio controls>` (:468-476). Logic NOT inlined.
- **CAVEAT:** "reusable as-is" = reusable within this app's data model. Hook is domain-coupled to project Manifest/ManifestBlock types + block-sync (findActiveBlock). Not a generic drop-in. Encapsulation claim fully verified.

---

## NET EFFECT ON THE RECOMMENDATION
- **AC1 ranking stands.** #1 Claude Artifacts (free, interactive React, downloadable) confirmed — with the Live/AI-powered caveat. #5 Claude Design correctly disqualified on no-recurring-spend (now confirmed real + paid). Cost claims for v0/Figma confirmed.
- **AC2 layout** (list-detail) is on documented ground. Transport anchor top-vs-bottom remains a genuine design-loop decision (not a verifiable fact).
- **AC3 interaction spec** fully grounded in W3C APG — strongest-cited surface.
- **Weakest surface = the per-block L1/L2/L3 control (F):** ship it as an explicit novel synthesis with cited lineage, honesty-rule aligned. This is the one to flag to the user and carry carefully into the Stage 4 design loop.
- **Reuse (G)** confirmed but per user steer is NOT load-bearing: usePlayback logic is portable; everything UI-facing is fair game to rebuild to production quality.
