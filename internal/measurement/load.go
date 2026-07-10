// SPDX-License-Identifier: AGPL-3.0-only
package measurement

import (
	"bufio"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

var teamPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func validHash(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func LoadCohorts(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("measurement: open cohorts: %w", err)
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("measurement: read cohorts: %w", err)
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "team" || rows[0][1] != "session_hash" {
		return nil, fmt.Errorf("measurement: cohorts header must be team,session_hash")
	}
	out := map[string]string{}
	for i, row := range rows[1:] {
		if len(row) != 2 || !teamPattern.MatchString(row[0]) || !validHash(row[1]) {
			return nil, fmt.Errorf("measurement: invalid cohorts row %d", i+2)
		}
		if prior := out[row[1]]; prior != "" && prior != row[0] {
			return nil, fmt.Errorf("measurement: session appears in multiple teams")
		}
		out[row[1]] = row[0]
	}
	return out, nil
}

func LoadOutcomes(path string) ([]Outcome, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("measurement: open outcomes: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []Outcome
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for line := 1; sc.Scan(); line++ {
		var o Outcome
		dec := json.NewDecoder(strings.NewReader(sc.Text()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return nil, fmt.Errorf("measurement: outcomes line %d: %w", line, err)
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("measurement: outcomes line %d: trailing content is forbidden", line)
		}
		_, timeErr := time.Parse(time.RFC3339, o.Time)
		confirmedWithoutUse := o.Confirmed && (o.Used == nil || !*o.Used)
		if !validHash(o.Session) || timeErr != nil || !record.ValidID(o.RecordID) || (o.ExposureID != "" && !validHash(o.ExposureID)) ||
			(o.Used == nil && !o.Confirmed && !o.Incorrect) || o.Confirmed && o.Incorrect || confirmedWithoutUse {
			return nil, fmt.Errorf("measurement: invalid outcome line %d", line)
		}
		out = append(out, o)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type TelemetryInputMode string

const (
	TelemetryDisjoint         TelemetryInputMode = "disjoint"
	TelemetryOverlapSnapshots TelemetryInputMode = "overlap-snapshot"
)

func LoadDecisions(paths []string, requested ...TelemetryInputMode) ([]telemetry.Decision, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("measurement: at least one telemetry input is required")
	}
	if len(requested) > 1 {
		return nil, fmt.Errorf("measurement: exactly one telemetry input mode may be selected")
	}
	mode := TelemetryDisjoint
	if len(requested) == 1 {
		mode = requested[0]
	}
	if mode != TelemetryDisjoint && mode != TelemetryOverlapSnapshots {
		return nil, fmt.Errorf("measurement: unknown telemetry input mode %q", mode)
	}
	var out []telemetry.Decision
	maxCounts := map[string]int{}
	prototypes := map[string]telemetry.Decision{}
	seenPaths := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("measurement: resolve telemetry %s: %w", p, err)
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("measurement: resolve telemetry %s: %w", p, err)
		}
		if seenPaths[resolved] {
			return nil, fmt.Errorf("measurement: duplicate telemetry path %s", p)
		}
		seenPaths[resolved] = true
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("measurement: open telemetry %s: %w", p, err)
		}
		var fileDecisions []telemetry.Decision
		err = scanDecisions(f, p, &fileDecisions)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("measurement: read decisions: %w", err)
		}
		if mode == TelemetryDisjoint {
			out = append(out, fileDecisions...)
			continue
		}
		counts := map[string]int{}
		for _, d := range fileDecisions {
			id := decisionIdentity(d)
			counts[id]++
			prototypes[id] = d
		}
		for id, n := range counts {
			if n > maxCounts[id] {
				maxCounts[id] = n
			}
		}
	}
	if mode == TelemetryOverlapSnapshots {
		for id, n := range maxCounts {
			for range n {
				out = append(out, prototypes[id])
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time != out[j].Time {
			return out[i].Time < out[j].Time
		}
		return decisionIdentity(out[i]) < decisionIdentity(out[j])
	})
	return out, nil
}
func scanDecisions(r io.Reader, path string, out *[]telemetry.Decision) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		if len(strings.TrimSpace(sc.Text())) == 0 {
			return fmt.Errorf("%s line %d: empty telemetry line", path, line)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(sc.Bytes(), &fields); err != nil {
			return fmt.Errorf("%s line %d: malformed telemetry: %w", path, line, err)
		}
		if _, ok := fields["query_text"]; ok {
			return fmt.Errorf("%s line %d: query_text is forbidden in measurement input", path, line)
		}
		var d telemetry.Decision
		dec := json.NewDecoder(strings.NewReader(sc.Text()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			return fmt.Errorf("%s line %d: invalid telemetry: %w", path, line, err)
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			return fmt.Errorf("%s line %d: trailing telemetry content is forbidden", path, line)
		}
		if err := validateDecision(d); err != nil {
			return fmt.Errorf("%s line %d: %w", path, line, err)
		}
		*out = append(*out, d)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if line == 0 {
		return fmt.Errorf("%s: telemetry input is empty", path)
	}
	return nil
}

func validateDecision(d telemetry.Decision) error {
	if _, err := time.Parse(time.RFC3339, d.Time); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if !validHash(d.QueryHash) {
		return fmt.Errorf("invalid query_hash")
	}
	if d.Session != "" && !validHash(d.Session) {
		return fmt.Errorf("invalid session hash")
	}
	if d.Channel != "push" && d.Channel != "search" {
		return fmt.Errorf("invalid channel %q", d.Channel)
	}
	if d.Channel == "push" && (d.Trigger != "prompt" && d.Trigger != "error") {
		return fmt.Errorf("invalid push trigger %q", d.Trigger)
	}
	if d.Channel == "search" && d.Trigger != "" {
		return fmt.Errorf("search decision carries trigger")
	}
	if d.Count != len(d.Served) {
		return fmt.Errorf("count does not match served")
	}
	for _, h := range d.Served {
		if !record.ValidID(h.ID) || math.IsNaN(h.Score) || math.IsInf(h.Score, 0) {
			return fmt.Errorf("invalid served hit")
		}
	}
	return nil
}
