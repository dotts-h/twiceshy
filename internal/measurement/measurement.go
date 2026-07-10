// SPDX-License-Identifier: AGPL-3.0-only

// Package measurement produces privacy-preserving design-partner pilot reports
// from salted decision telemetry and explicit outcome judgements.
package measurement

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Config struct {
	Baseline  Window
	Treatment Window
	Cohorts   map[string]string
}
type Outcome struct {
	Time      string `json:"ts"`
	Session   string `json:"session_hash"`
	RecordID  string `json:"record_id"`
	Used      *bool  `json:"used,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
	Incorrect bool   `json:"incorrect,omitempty"`
}
type Rate struct {
	Successes int     `json:"successes"`
	Total     int     `json:"total"`
	Value     float64 `json:"value"`
	Low       float64 `json:"low_95"`
	High      float64 `json:"high_95"`
}
type Metrics struct {
	Decisions         int  `json:"decisions"`
	ExposedDecisions  int  `json:"exposed_decisions"`
	Exposures         int  `json:"exposures"`
	Judged            int  `json:"judged"`
	Used              int  `json:"used"`
	Confirmed         int  `json:"confirmed"`
	Incorrect         int  `json:"incorrect"`
	ErrorDecisions    int  `json:"error_decisions"`
	RepeatedErrors    int  `json:"repeated_errors"`
	HitRate           Rate `json:"hit_rate"`
	OutcomeCoverage   Rate `json:"outcome_coverage"`
	UsedRate          Rate `json:"used_rate"`
	HelpfulRate       Rate `json:"helpful_rate"`
	IncorrectRate     Rate `json:"incorrect_rate"`
	RepeatedErrorRate Rate `json:"repeated_error_rate"`
}
type ArmSummary struct {
	Arm     string  `json:"arm"`
	Window  Window  `json:"window"`
	Metrics Metrics `json:"metrics"`
}
type TeamSummary struct {
	Arm     string  `json:"arm"`
	Team    string  `json:"team"`
	Metrics Metrics `json:"metrics"`
}
type RecordSummary struct {
	Arm      string  `json:"arm"`
	Team     string  `json:"team"`
	RecordID string  `json:"record_id"`
	Metrics  Metrics `json:"metrics"`
}
type Report struct {
	Baseline  ArmSummary      `json:"baseline"`
	Treatment ArmSummary      `json:"treatment"`
	Teams     []TeamSummary   `json:"teams"`
	Records   []RecordSummary `json:"records"`
}

type bucket struct {
	metrics  Metrics
	errors   map[string]int
	exposure map[string]int
}

func Generate(cfg Config, decisions []telemetry.Decision, outcomes []Outcome) (Report, error) {
	if err := validateWindows(cfg); err != nil {
		return Report{}, err
	}
	arms := map[string]*bucket{"baseline": {errors: map[string]int{}, exposure: map[string]int{}}, "treatment": {errors: map[string]int{}, exposure: map[string]int{}}}
	teams := map[string]*bucket{}
	records := map[string]*bucket{}
	teamOf := func(session string) string {
		if t := cfg.Cohorts[session]; t != "" {
			return t
		}
		return "unattributed"
	}
	for _, d := range decisions {
		t, err := time.Parse(time.RFC3339, d.Time)
		if err != nil {
			continue
		}
		arm := armAt(cfg, t)
		if arm == "" {
			continue
		}
		team := teamOf(d.Session)
		keys := []string{arm + "\x00" + team}
		bs := []*bucket{arms[arm], getBucket(teams, keys[0])}
		for _, b := range bs {
			addDecision(b, d)
		}
		for _, hit := range d.Served {
			if !record.ValidID(hit.ID) {
				continue
			}
			key := arm + "\x00" + team + "\x00" + hit.ID
			b := getBucket(records, key)
			b.metrics.Exposures++
			b.metrics.ExposedDecisions++
			for _, parent := range bs {
				parent.exposure[d.Session+"\x00"+hit.ID]++
			}
			b.exposure[d.Session+"\x00"+hit.ID]++
		}
	}
	// Outcomes are attributable only up to the number of actual served exposures.
	usedExposure := map[string]int{}
	sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Time < outcomes[j].Time })
	for _, o := range outcomes {
		t, err := time.Parse(time.RFC3339, o.Time)
		if err != nil || !record.ValidID(o.RecordID) {
			continue
		}
		arm := armAt(cfg, t)
		if arm == "" {
			continue
		}
		team := teamOf(o.Session)
		expKey := o.Session + "\x00" + o.RecordID
		capKey := arm + "\x00" + expKey
		if arms[arm].exposure[expKey] <= usedExposure[capKey] {
			continue
		}
		usedExposure[capKey]++
		for _, b := range []*bucket{arms[arm], getBucket(teams, arm+"\x00"+team), getBucket(records, arm+"\x00"+team+"\x00"+o.RecordID)} {
			addOutcome(b, o)
		}
	}

	rep := Report{Baseline: ArmSummary{Arm: "baseline", Window: cfg.Baseline}, Treatment: ArmSummary{Arm: "treatment", Window: cfg.Treatment}}
	rep.Baseline.Metrics = finish(arms["baseline"])
	rep.Treatment.Metrics = finish(arms["treatment"])
	for key, b := range teams {
		parts := splitKey(key)
		rep.Teams = append(rep.Teams, TeamSummary{Arm: parts[0], Team: parts[1], Metrics: finish(b)})
	}
	for key, b := range records {
		parts := splitKey(key)
		rep.Records = append(rep.Records, RecordSummary{Arm: parts[0], Team: parts[1], RecordID: parts[2], Metrics: finish(b)})
	}
	sort.Slice(rep.Teams, func(i, j int) bool {
		if rep.Teams[i].Team != rep.Teams[j].Team {
			return rep.Teams[i].Team < rep.Teams[j].Team
		}
		return rep.Teams[i].Arm < rep.Teams[j].Arm
	})
	sort.Slice(rep.Records, func(i, j int) bool {
		a, b := rep.Records[i], rep.Records[j]
		if a.Team != b.Team {
			return a.Team < b.Team
		}
		if a.RecordID != b.RecordID {
			return a.RecordID < b.RecordID
		}
		return a.Arm < b.Arm
	})
	return rep, nil
}

func validateWindows(c Config) error {
	for _, w := range []Window{c.Baseline, c.Treatment} {
		if w.Start.IsZero() || !w.Start.Before(w.End) {
			return errors.New("measurement: each window requires start < end")
		}
	}
	if c.Baseline.Start.Before(c.Treatment.End) && c.Treatment.Start.Before(c.Baseline.End) {
		return errors.New("measurement: baseline and treatment windows overlap")
	}
	return nil
}
func armAt(c Config, t time.Time) string {
	if !t.Before(c.Baseline.Start) && t.Before(c.Baseline.End) {
		return "baseline"
	}
	if !t.Before(c.Treatment.Start) && t.Before(c.Treatment.End) {
		return "treatment"
	}
	return ""
}
func getBucket(m map[string]*bucket, k string) *bucket {
	if m[k] == nil {
		m[k] = &bucket{errors: map[string]int{}, exposure: map[string]int{}}
	}
	return m[k]
}
func addDecision(b *bucket, d telemetry.Decision) {
	b.metrics.Decisions++
	if len(d.Served) > 0 {
		b.metrics.ExposedDecisions++
	}
	b.metrics.Exposures += len(d.Served)
	if d.Channel == "push" && d.Trigger == "error" {
		b.metrics.ErrorDecisions++
		if d.Session != "" && d.QueryHash != "" {
			k := d.Session + "\x00" + d.QueryHash
			if b.errors[k] > 0 {
				b.metrics.RepeatedErrors++
			}
			b.errors[k]++
		}
	}
}
func addOutcome(b *bucket, o Outcome) {
	if o.Used != nil || o.Incorrect {
		b.metrics.Judged++
		if o.Used != nil && *o.Used {
			b.metrics.Used++
		}
	}
	if o.Confirmed {
		b.metrics.Confirmed++
	}
	if o.Incorrect {
		b.metrics.Incorrect++
	}
}
func finish(b *bucket) Metrics {
	m := b.metrics
	m.HitRate = wilson(m.ExposedDecisions, m.Decisions)
	m.OutcomeCoverage = wilson(m.Judged, m.Exposures)
	m.UsedRate = wilson(m.Used, m.Judged)
	m.HelpfulRate = wilson(m.Confirmed, m.Exposures)
	m.IncorrectRate = wilson(m.Incorrect, m.Judged)
	m.RepeatedErrorRate = wilson(m.RepeatedErrors, m.ErrorDecisions)
	return m
}
func wilson(success, total int) Rate {
	r := Rate{Successes: success, Total: total}
	if total == 0 {
		return r
	}
	r.Value = float64(success) / float64(total)
	z := 1.959963984540054
	n := float64(total)
	den := 1 + z*z/n
	center := (r.Value + z*z/(2*n)) / den
	half := z * math.Sqrt(r.Value*(1-r.Value)/n+z*z/(4*n*n)) / den
	r.Low = math.Max(0, center-half)
	r.High = math.Min(1, center+half)
	return r
}
func splitKey(k string) []string {
	var out []string
	for {
		i := -1
		for j := 0; j < len(k); j++ {
			if k[j] == 0 {
				i = j
				break
			}
		}
		if i < 0 {
			return append(out, k)
		}
		out = append(out, k[:i])
		k = k[i+1:]
	}
}

func (w Window) String() string {
	return fmt.Sprintf("%s/%s", w.Start.Format(time.RFC3339), w.End.Format(time.RFC3339))
}
