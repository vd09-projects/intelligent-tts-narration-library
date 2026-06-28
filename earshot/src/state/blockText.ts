// blockText.ts — pure extractors for the two transcript views.
//
// Spoken view  = the final spoken words (Segment.Text joined). This is what the
//                listener actually hears — voicing happens in the planner, so
//                Segment.Text is authoritative (CLAUDE.md).
// Source view  = the original source excerpt (SourceMap.raw_excerpt).
//
// The Spoken/Source distinction is the core honesty surface: spoken text may
// differ from source text (gist mode), and the toggle lets the user see both.

import type { Block } from "../api/types";

export function spokenText(block: Block): string {
  return (block.segments ?? [])
    .filter((s) => s.kind === "speech")
    .map((s) => s.text ?? "")
    .join(" ")
    .trim();
}

export function sourceText(block: Block): string {
  return block.source_map.raw_excerpt ?? "";
}
