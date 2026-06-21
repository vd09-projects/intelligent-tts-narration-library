import type { Class, Level } from '../types/plan.ts'

// escalateTargets returns the levels a block can be escalated TO, given its
// current level and class. Pure + table-testable.
//
// Domain rule (CLAUDE.md "Deterministic L1 for structured classes"): code,
// config, table, diagram_as_text, heading, and list voice IDENTICALLY across
// L1/L2/L3 with no intelligence adapter — escalating them produces
// content-identical output. So they get NO escalate targets; only prose (and
// other non-structured classes) truly benefit from a higher level.
//
//   prose @ L1 → [2, 3]
//   prose @ L2 → [3]
//   prose @ L3 → []
//   structured @ any level → []
//
// canDowngrade is always false — downgrade-to-L1 is out of scope (Q5).
export interface EscalateTargets {
  up: Level[]
  canDowngrade: false
}

// STRUCTURED_CLASSES are the deterministic-L1 classes that voice the same at
// every level. Kept as a Set for O(1) membership; mirrors the planner's
// deterministic-class list.
const STRUCTURED_CLASSES: ReadonlySet<string> = new Set<Class>([
  'code',
  'config',
  'table',
  'diagram_as_text',
  'heading',
  'list',
])

export function escalateTargets(
  currentLevel: Level,
  blockClass: Class,
): EscalateTargets {
  if (STRUCTURED_CLASSES.has(blockClass)) {
    return { up: [], canDowngrade: false }
  }
  const up: Level[] = ([1, 2, 3] as const).filter(
    (l): l is Level => l > currentLevel,
  )
  return { up, canDowngrade: false }
}
