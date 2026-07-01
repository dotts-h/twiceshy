// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

// #0110: every V2 system prompt must pin the fifth canonical check —
// USEFULNESS — so a panel stops approving content-shaped non-lessons (a
// record that merely narrates work done, with no trap and no dead-end, that
// would never change a future agent's action; the motivating example is
// exp-2845 "Use Selftests for Argument Parsing Invariants", panel-approved
// 2026-06-28 despite naming no trap). Each prompt still names all five checks
// in order and the "approve only if all N pass" contract is updated to five.
func TestSystemPromptsV2_PinUsefulnessCheck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt string
	}{
		{"ProseSystemV2", judge.ProseSystemV2},
		{"AdvisorySystemV2", judge.AdvisorySystemV2},
		{"ProsePanelSystemV2", judge.ProsePanelSystemV2},
		{"RubricSystemV2", judge.RubricSystemV2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lower := strings.ToLower(tc.prompt)
			if !strings.Contains(lower, "usefulness") {
				t.Fatalf("%s must name the usefulness check:\n%s", tc.name, tc.prompt)
			}
			if !strings.Contains(lower, "next action") {
				t.Fatalf("%s must ask whether the record would change a future agent's next action:\n%s", tc.name, tc.prompt)
			}
			if !strings.Contains(lower, "meaning, scope, usefulness, license, poison") {
				t.Fatalf("%s must list all five checks in canonical order (meaning, scope, usefulness, license, poison):\n%s", tc.name, tc.prompt)
			}
			if !strings.Contains(tc.prompt, "all five") {
				t.Fatalf("%s must gate approval on all FIVE checks, not four:\n%s", tc.name, tc.prompt)
			}
		})
	}
}

// The rubric's worked examples ground the usefulness boundary in two concrete
// cases: the exp-2845 narrative-only convention (FAIL — nothing an agent would
// do differently) and a genuine trap shape, a typed-nil Go error (PASS — a
// non-obvious escape a future agent needs).
func TestRubricSystemV2_HasUsefulnessWorkedExamples(t *testing.T) {
	if !strings.Contains(judge.RubricSystemV2, "exp-2845") {
		t.Fatalf("RubricSystemV2 must cite exp-2845 as the usefulness FAIL example:\n%s", judge.RubricSystemV2)
	}
	if !strings.Contains(strings.ToLower(judge.RubricSystemV2), "typed-nil") {
		t.Fatalf("RubricSystemV2 must include a typed-nil-error trap as the usefulness PASS example:\n%s", judge.RubricSystemV2)
	}
}

// Supersede, never delete (CLAUDE.md): the V1 constants stay byte-identical so
// a past A/B measurement quoted against them ("0 false-approve", etc.) remains
// auditable, and V2 must not silently collapse back onto V1's four-check text.
func TestSystemPromptsV1_StillFourChecksNoUsefulness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt string
	}{
		{"ProseSystemV1", judge.ProseSystemV1},
		{"AdvisorySystemV1", judge.AdvisorySystemV1},
		{"ProsePanelSystemV1", judge.ProsePanelSystemV1},
		{"RubricSystemV1", judge.RubricSystemV1},
	} {
		if strings.Contains(strings.ToLower(tc.prompt), "usefulness") {
			t.Errorf("%s is the pre-#0110 baseline and must stay four-check (no usefulness) for the audit trail", tc.name)
		}
	}
}
