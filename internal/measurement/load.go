// SPDX-License-Identifier: AGPL-3.0-only
package measurement

import (
	"bufio"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
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
		_, timeErr := time.Parse(time.RFC3339, o.Time)
		confirmedWithoutUse := o.Confirmed && (o.Used == nil || !*o.Used)
		if !validHash(o.Session) || timeErr != nil || !record.ValidID(o.RecordID) ||
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

func LoadDecisions(path string) ([]telemetry.Decision, error) {
	var out []telemetry.Decision
	for _, p := range []string{path + ".1", path} {
		f, err := os.Open(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = scanDecisions(f, &out)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("measurement: read decisions: %w", err)
		}
	}
	return out, nil
}
func scanDecisions(r io.Reader, out *[]telemetry.Decision) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var d telemetry.Decision
		if json.Unmarshal(sc.Bytes(), &d) == nil {
			d.QueryText = ""
			*out = append(*out, d)
		}
	}
	return sc.Err()
}
