# DefaultLexicon shipped frozen + user-overridable via WithLexicon

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, voice, lexicon, phase-one, user-override |

## Context

`planner/voice.go` turns raw source text into the final spoken words inside `Segment.Text`. Symbols (`->`, `==`, `&&`), system paths (`/etc/hosts`), and dev acronyms (`API`, `JSON`, `K8s`) need to be voiced — `->` should become "arrow", not "dash greater than". CLAUDE.md `A8` calls for a default lexicon plus a user-overridable map.

Two extremes were available: ship empty (every project supplies its own) or ship opinionated (project starts with sensible defaults).

## Options considered

### Option A: ship `DefaultLexicon` frozen + `WithLexicon(extra)` overlay (chosen)
- **Pros**: out-of-the-box voicings are correct; users override only what they disagree with; user entries always win on key collision.
- **Cons**: the shipped baseline is opinionated; users may be surprised by a default they didn't expect.

### Option B: ship empty default; require every project to supply a lexicon
- **Pros**: no opinion baked in.
- **Cons**: every project produces silent silly voicings on day one (`->` literally as "dash greater than"); discoverability of "you need a lexicon" is bad.

### Option C: auto-detect lexicon entries from training data / corpus
- **Pros**: scales to large symbol vocabularies.
- **Cons**: not deterministic — forbidden by CLAUDE.md `A13`; lexicon shape changes silently between versions.

## Decision

Ship `DefaultLexicon` as an exported map covering:

- Arrow / comparison operators: `->`, `=>`, `<-`, `==`, `!=`, `<=`, `>=`, `&&`, `||`
- Common system paths: `/etc/hosts`, `/usr/local/bin`, `~/.ssh/config`, `/var/log`, `/dev/null`
- Dev acronyms: `API`, `URL`, `URI`, `HTTP`, `HTTPS`, `JSON`, `YAML`, `SQL`, `CLI`, `TTS`, `MCP`, `gRPC`, `K8s`, `CI`, `CD`

Expose `Lexicon` (map type), `WithLexicon(extra Lexicon) VoiceOption` (user-overridable overlay), and `compileLexicon(opts...)` (longest-match-first compilation — `>=` beats `>`).

User entries overlay on `DefaultLexicon` and win on key collision. Calling `WithLexicon` multiple times merges all extras; later calls win on collision.

## Consequences

- A project that disagrees with a default does one line: `planner.Plan(..., WithLexicon(Lexicon{"->": "to"}))`.
- A project that wants a new acronym does one line: `WithLexicon(Lexicon{"WAT": "what a tangle"})`.
- Every default entry has a table-driven test exercising it in a wrapping sentence (`planner/voice_test.go`).
- Longest-match-first is enforced at compile time via length-descending sort; an explicit test pins this behaviour.
- Whitespace padding around replacements: a single space is inserted on either side when the replacement would otherwise butt up against a non-space character (e.g. "->b" → "arrow b"). The exception list for punctuation chars is hand-rolled — flagged as future tech-debt cleanup.

## Related decisions

- [Planner classifier sniff order](2026-06-18-planner-classifier-sniff-order.md) — same anti-magic discipline: deterministic rules, every default exercised by a test.

## Revisit trigger

- A real user project ships a `WithoutLexicon(keys...)` style request (need to delete defaults rather than overlay).
- Multi-locale support lands — at that point the lexicon may need to become `map[Locale]Lexicon`.
- A lexicon entry is observed to produce wrong voicings consistently across multiple users.
