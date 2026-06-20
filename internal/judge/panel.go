// SPDX-License-Identifier: AGPL-3.0-only

package judge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PanelMember pairs a judge with its model id (for family diversity checks and
// audit stamping). Each member SHOULD already be the caller's majority-wrapped
// judge — Panel does not double-wrap.
type PanelMember struct {
	Model string
	Judge Judge
}

// PanelJudge requires UNANIMOUS approval from ≥2 judges of DISTINCT model families
// (ADR-0016 §1-2, anti-monoculture). It fails safe: any member that errors,
// rejects, or is absent yields a non-approving verdict. The combined Verdict's
// Model is the joined member ids; its Checks union the members'.
type PanelJudge struct {
	members     []PanelMember
	lastMembers []Verdict
}

// NewPanel builds a panel judge. It errors if there are fewer than two members
// or if any two members share a model family.
func NewPanel(members ...PanelMember) (Judge, error) {
	if len(members) < 2 {
		return nil, errors.New("judge panel: at least 2 members required")
	}
	seen := make(map[string]string, len(members))
	for _, m := range members {
		model := strings.TrimSpace(m.Model)
		if model == "" {
			return nil, errors.New("judge panel: member model id required")
		}
		if m.Judge == nil {
			return nil, fmt.Errorf("judge panel: member %q has nil judge", model)
		}
		fam := FamilyOf(model)
		if fam == "" {
			return nil, fmt.Errorf("judge panel: model %q has no recognizable family", model)
		}
		if other, ok := seen[fam]; ok {
			return nil, fmt.Errorf("judge panel: models %q and %q share family %q — members must differ", other, model, fam)
		}
		seen[fam] = model
	}
	return &PanelJudge{members: members}, nil
}

// PanelMembers returns each member's verdict from the most recent Judge call.
func (p *PanelJudge) PanelMembers() []Verdict {
	return append([]Verdict(nil), p.lastMembers...)
}

// Judge consults every member and returns a unanimous approving verdict, or a
// non-approving verdict / error per the fail-safe invariants.
func (p *PanelJudge) Judge(ctx context.Context, req Request) (Verdict, error) {
	p.lastMembers = nil
	models := make([]string, 0, len(p.members))
	memberVerdicts := make([]Verdict, 0, len(p.members))
	checksByName := make(map[CheckName]Check)

	for _, m := range p.members {
		v, err := m.Judge.Judge(ctx, req)
		if err != nil {
			return Verdict{}, err
		}
		v.Model = m.Model
		memberVerdicts = append(memberVerdicts, v)
		models = append(models, m.Model)
		if !v.Approved() {
			p.lastMembers = memberVerdicts
			return v, nil
		}
		for _, c := range v.Checks {
			if prev, ok := checksByName[c.Name]; !ok || (c.Pass && !prev.Pass) {
				checksByName[c.Name] = c
			}
		}
	}
	p.lastMembers = memberVerdicts
	checks := make([]Check, 0, len(Checks))
	for _, name := range Checks {
		checks = append(checks, checksByName[name])
	}
	return Verdict{
		Decision: Approve,
		Model:    strings.Join(models, "+"),
		Checks:   checks,
	}, nil
}
