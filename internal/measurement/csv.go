// SPDX-License-Identifier: AGPL-3.0-only
package measurement

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

var csvHeader = []string{"scope", "arm", "team", "record_id", "decisions", "exposed_decisions", "exposures", "judged", "used", "confirmed", "incorrect", "error_decisions", "repeated_errors", "hit_rate", "hit_low_95", "hit_high_95", "outcome_coverage", "outcome_coverage_low_95", "outcome_coverage_high_95", "used_rate", "used_low_95", "used_high_95", "helpful_rate", "helpful_low_95", "helpful_high_95", "incorrect_rate", "incorrect_low_95", "incorrect_high_95", "repeated_error_rate", "repeated_error_low_95", "repeated_error_high_95"}

func WriteCSV(w io.Writer, rep Report) error {
	c := csv.NewWriter(w)
	if err := c.Write(csvHeader); err != nil {
		return err
	}
	write := func(row []string) error { return c.Write(row) }
	if err := write(metricsRow("aggregate", "baseline", "", "", rep.Baseline.Metrics)); err != nil {
		return err
	}
	if err := write(metricsRow("aggregate", "treatment", "", "", rep.Treatment.Metrics)); err != nil {
		return err
	}
	for _, s := range rep.Teams {
		if err := write(metricsRow("team", s.Arm, s.Team, "", s.Metrics)); err != nil {
			return err
		}
	}
	for _, s := range rep.Records {
		if err := write(recordRow(s)); err != nil {
			return err
		}
	}
	c.Flush()
	if err := c.Error(); err != nil {
		return fmt.Errorf("measurement: write CSV: %w", err)
	}
	return nil
}

func metricsRow(scope, arm, team, id string, m Metrics) []string {
	row := []string{scope, arm, team, id}
	for _, n := range []int{m.Decisions, m.ExposedDecisions, m.Exposures, m.Judged, m.Used, m.Confirmed, m.Incorrect, m.ErrorDecisions, m.RepeatedErrors} {
		row = append(row, strconv.Itoa(n))
	}
	for _, r := range []Rate{m.HitRate, m.OutcomeCoverage, m.UsedRate, m.HelpfulRate, m.IncorrectRate, m.RepeatedErrorRate} {
		row = appendRate(row, r)
	}
	return row
}
func recordRow(s RecordSummary) []string {
	m := s.Metrics
	row := []string{"record", s.Arm, s.Team, s.RecordID, "", "", strconv.Itoa(m.Exposures), strconv.Itoa(m.Judged), strconv.Itoa(m.Used), strconv.Itoa(m.Confirmed), strconv.Itoa(m.Incorrect), "", "", "", "", ""}
	for _, r := range []Rate{m.OutcomeCoverage, m.UsedRate, m.HelpfulRate, m.IncorrectRate} {
		row = appendRate(row, r)
	}
	row = append(row, "", "", "")
	return row
}
func appendRate(row []string, r Rate) []string {
	return append(row, float(r.Value), float(r.Low), float(r.High))
}
func float(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
