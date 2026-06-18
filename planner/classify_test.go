package planner

import (
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestClassify_PriorityChain — one case per rule in classify()'s sniff
// order. Each case names the deciding rule so a future maintainer can
// reorder with intent.
func TestClassify_PriorityChain(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rule string
		rb   rawBlock
		want plan.Class
	}{
		{
			name: "heading hint wins",
			rule: "rule 1 — hintHeading → ClassHeading",
			rb:   rawBlock{text: "# Hello", hint: hintHeading},
			want: plan.ClassHeading,
		},
		{
			name: "list hint wins",
			rule: "rule 2 — hintList → ClassList",
			rb:   rawBlock{text: "- a\n- b", hint: hintList},
			want: plan.ClassList,
		},
		{
			name: "image hint → unknown",
			rule: "rule 3 — hintImage → ClassUnknown (refused downstream)",
			rb:   rawBlock{text: "![](x.png)", hint: hintImage},
			want: plan.ClassUnknown,
		},
		{
			name: "table hint wins",
			rule: "rule 4 — hintTable → ClassTable",
			rb:   rawBlock{text: "| a | b |", hint: hintTable},
			want: plan.ClassTable,
		},
		{
			name: "fenced go → code",
			rule: "rule 5 default — fenced code with unknown lang → ClassCode",
			rb:   rawBlock{text: "func main() {}", hint: hintCode, fenceInfo: "go"},
			want: plan.ClassCode,
		},
		{
			name: "fenced mermaid → diagram",
			rule: "rule 5a — fenced code with diagram dialect → ClassDiagramAsText",
			rb:   rawBlock{text: "graph TD; A-->B", hint: hintCode, fenceInfo: "mermaid"},
			want: plan.ClassDiagramAsText,
		},
		{
			name: "fenced yaml → config",
			rule: "rule 5b — fenced code with config dialect → ClassConfig",
			rb:   rawBlock{text: "key: value", hint: hintCode, fenceInfo: "yaml"},
			want: plan.ClassConfig,
		},
		{
			name: "plaintext pipe-table → table",
			rule: "rule 6 — plaintext sniff for pipe table → ClassTable",
			rb:   rawBlock{text: "| a | b |\n|---|---|\n| 1 | 2 |\n", hint: hintProse},
			want: plan.ClassTable,
		},
		{
			name: "plaintext YAML-shaped → config",
			rule: "rule 7 — plaintext config sniff → ClassConfig",
			rb:   rawBlock{text: "name: alice\nage: 30\n", hint: hintProse},
			want: plan.ClassConfig,
		},
		{
			name: "plaintext ASCII diagram → diagram",
			rule: "rule 8 — ASCII diagram sniff → ClassDiagramAsText",
			rb:   rawBlock{text: "+---+\n| A |\n+---+\n", hint: hintProse},
			want: plan.ClassDiagramAsText,
		},
		{
			name: "default prose",
			rule: "rule 9 — fallback → ClassProse",
			rb:   rawBlock{text: "A simple sentence.", hint: hintProse},
			want: plan.ClassProse,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.rb)
			if got != tt.want {
				t.Errorf("%s: want %v, got %v", tt.rule, tt.want, got)
			}
		})
	}
}
