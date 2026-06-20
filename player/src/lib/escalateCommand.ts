import type { Block, NarrationPlan } from '../types/plan.ts'

// escalateCommand returns the CLI invocation the user can run on their host
// to re-narrate one block at L3 detail. The reference player does not
// execute it — phase one shows the literal string with a Copy button (plan
// Decision D5; per-block in-app re-render is out of scope until #14 grows a
// streaming companion).
//
// Shape (mirrors cmd/narrate flags):
//   narrate --block <id> --level 3 --file <plan.source.uri>
//
// When plan.source.kind !== "file" the file path doesn't apply (e.g. MCP-text
// or raw-text input); we substitute the placeholder `<source>` so the user
// sees the shape but is reminded that this scope did not originate from a
// file on disk and they must supply the input themselves.
export function escalateCommand(block: Block, plan: NarrationPlan): string {
  const blockArg = block.id
  if (plan.source.kind === 'file' && plan.source.uri && plan.source.uri.length > 0) {
    return `narrate --block ${blockArg} --level 3 --file ${plan.source.uri}`
  }
  return `narrate --block ${blockArg} --level 3 --file <source>`
}
