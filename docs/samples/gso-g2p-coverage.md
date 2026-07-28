# G2P coverage — does the GSO worker's g2p_en pronounce the planner's Segment.Text correctly

This document is the fixed input for `make gso-g2p-check` (issue #164, AC3). It exercises every structured class the planner voices **deterministically** — spelled-out numbers, code, lists, tables, and headings — so the check can (A) confirm the planner's `Segment.Text` reads each class per this project's rules and (B) dump the ARPAbet phoneme string `g2p_en` produces for that exact text. It carries no image / bare-diagram refusals: this doc is about pronunciation coverage, not the honesty rule.

`make gso-g2p-check` runs it at **level 3** so the table reads its header and rows deterministically (at L1 a table is a one-line structural gist); numbers still spell out at any level because short prose degrades to a verbatim read.

The classes below are each labelled so the `plan.json` block that carries them is easy to find in the `gso-g2p-check` output.

## Numbers read as words

The narrator spells standalone numbers in read-aloud prose. We deployed 24,700 requests across 3 regions, saw a 3.5 percent error rate, and retried 42 of them; the long tail was 1,000,000 events, which stays as digits above the six-digit spell-out ceiling.

## Code read by structure

```go
replicas := 3
if replicas < 5 {
    scaleUp(replicas)
}
```

## List read item by item

- first bullet names the planner
- second bullet names the renderer
- third bullet names the sink

1. step one loads the plan
2. step two renders each block
3. step three writes the audio

## Table read row by row

| Knob | Default | Notes |
|---|---|---|
| Level | one | Re-requestable per block |
| Voice | jeremy jahns | zero shot clone |
| Sink | ephemeral | Plays through afplay |

## Headings are their own utterance

### A third-level heading

The five headings in this document — including this section title and the one above — are each planned as their own `heading` block, distinct from the prose around them.

---

## How the check reads this document

`make gso-g2p-check` runs in two machine-checkable halves plus one explicitly-staged acoustic residue. Nothing here is an ear check — halves A and B are text-only.

### Half A — `Segment.Text` correctness (no audio, no worker)

`cmd/plandump` renders this document through the pure Go planner and dumps `plan.json`. For each structured class we read the block's `Segment.Text` and confirm it matches the project's deterministic voicing rule (CLAUDE.md "Domain rules"): numbers spelled out in prose, code read as a structural gist, lists item by item with ordinals, tables header-and-rows, headings as their own utterance. This is the same engine-neutral spoken text any renderer (Kokoro, RVC, GSO) would receive, so half A is not GSO-specific and needs no `.venv-gso` worker.

### Half B — `g2p_en` phoneme string (textual, no listening)

`scripts/gso_g2p_dump.py` (run under `.venv-gso`, where GPT-SoVITS's `g2p_en` lives) feeds each block's `Segment.Text` through `g2p_en` and prints the ARPAbet phoneme STRING it produces. We inspect that string textually against the expected phonemes and flag any class whose string is wrong. Half B needs `.venv-gso`; it still never plays audio — it compares phoneme strings, not sound.

### Findings (from a real `make gso-g2p-check` run, M1 Pro, 2026-07-28)

One row per structured class. `Segment.Text` is copied verbatim from `plan.json` (level 3); the `g2p_en` string is copied verbatim from the `gso-g2p-check` half-B dump. Verdicts are by-inspection — there is deliberately **no golden/fixture assertion** (a #164 non-goal).

| Class | `Segment.Text` (from plan.json, L3) | Half A (reads per rule?) | `g2p_en` ARPAbet (excerpt) | Half B (phonemes right?) |
|---|---|---|---|---|
| numbers (prose) | "…deployed **twenty-four thousand seven hundred** requests across **three** regions, saw a **three point five** percent error rate, and retried **forty-two** of them; the long tail was **1,000,000** events…" | PASS — 24,700 / 3 / 3.5 / 42 spelled; 1,000,000 correctly left as digits (7 digits > the 6-digit ceiling) | `T W EH1 N T IY0 F UH1 R TH AW1 Z AH0 N D …`; `TH R IY1 P OY1 N T F AY1 V` ✓; `F AO0 R T UW1 T W AA2` (forty-two) | MIXED — "three point five" ✓, but the **hyphenated compound cardinals mis-phonemize**: "twenty-**four**" → `…F UH1 R` (should be `F AO1 R`), "forty-two" → `F AO0 R T UW1 T W AA2` (should be `F AO1 R T IY0 T UW1`). See CEILING 1. |
| code | "A **4-line** Go code block." | PASS — code read as a structural gist, never character-by-character | `AH0 F ER1 L AY0 N G OW1 K OW1 D B L AA1 K` | FLAG — the gist's "4-line" → `F ER1 L AY0 N` (digit-hyphen-word compound mis-tokenized, same class of bug as CEILING 1). The English words "Go code block" are correct. |
| list | "List of 3 items. **First**, … **Second**, … **Third**, …" | PASS — ordinals + item bodies read; "3" → "three" | `L IH1 S T AH1 V TH R IY1 AY1 T AH0 M Z . F ER1 S T , … S EH1 K AH0 N D , … TH ER1 D` | PASS — ordinals and bodies phonemize correctly. |
| table | "A 3-column, 3-row table. Row: Level, one, Re-requestable per block. Row: Voice, jeremy jahns, zero shot clone. Row: Sink, ephemeral, Plays through afplay." | PASS — at L3 the table reads its header structure + every row | `AH0 TH R IY1 K AH0 L M AH0 N , TH R IY1 R OW0 T EY1 B AH0 L . R OW1 L EH1 V AH0 L , W AH1 N , …` | PASS on plain English cell text (Level/one/Voice/jeremy jahns/ephemeral/afplay all read). Symbol-heavy cells were deliberately avoided here — see CEILING 2 for what happens to `L1` / `32 kHz` / slug tokens. |
| heading | "Section: Numbers read as words." (+ four more) | PASS — each heading is its own block, prefixed "Section:" | `S EH1 K SH AH0 N N AH1 M B ER0 Z R EH1 D AE1 Z W ER1 D Z` | PASS — clean. (Minor: "read" disambiguates to past-tense `R EH1 D`; harmless.) |

### CEILINGS surfaced by half B (documented, machine inputs to the go/no-go — NOT blockers-by-fabrication)

These are the biggest NEW risk vs RVC, which inherited Kokoro's pronunciation for free. They are documented and spun to follow-ups (#147 precedent), never hidden behind a fabricated pass:

- **CEILING 1 — g2p_en mis-phonemizes hyphenated spelled-out cardinals.** The planner's number pass emits hyphenated compounds ("twenty-four", "forty-two"); `g2p_en` mis-reads the second element ("twenty-four" → `…F UH1 R`, "forty-two" → `F AO0 R T UW1 T W AA2`), even though bare "four" alone → `F AO1 R` correctly. The hyphen is the trigger. Candidate fixes (out of scope for #164): emit space-separated cardinals in the planner's number pass, or attach a `VoicingDirective` phoneme hint. Recorded as a follow-up.
- **CEILING 2 — g2p_en mangles technical tokens / symbols.** `L1` → `L OW1 N` ("lone", not "L-one"); `32 kHz`, `GPT-SoVITS`, and hyphenated slugs like `cool-jahns-gso` phonemize as garbled runs. This is expected for abbreviations/identifiers and affects any engine relying on g2p_en; it is documented, not a blocker.

### AC3-ear — acoustic residue staged for human ears (NOT machine evidence)

A correct ARPAbet string only proves `g2p_en` tokenised and stressed the words correctly; whether the GSO model *realises* those phonemes as the intended sound — prosody, cadence, character likeness — can only be judged by listening. Those questions are staged here for the human by-ear session (alongside ACs 1 & 2) and are **never** recorded as a machine verdict (the B1 machine-vs-human boundary):

- Does the spelled cardinal ("twenty-four thousand seven hundred") sound natural, and does the CEILING-1 hyphen mis-phonemization ("forty-two") actually degrade the audio or does the model recover?
- Do the code/table gists ("A 4-line Go code block", "3-column, 3-row table") read intelligibly?
- Does the Jeremy Jahns character voice survive across the different classes?

These ride the human by-ear checklist in the PR; the machine go/no-go leans only on half A + half B.
