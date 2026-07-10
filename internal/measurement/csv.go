// SPDX-License-Identifier: AGPL-3.0-only
package measurement

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

func WriteCSV(w io.Writer, rep Report) error {
	c := csv.NewWriter(w)
	header := []string{"scope", "arm", "team", "record_id", "decisions", "exposed_decisions", "exposures", "judged", "used", "confirmed", "incorrect", "error_decisions", "repeated_errors", "hit_rate", "hit_low_95", "hit_high_95", "outcome_coverage", "used_rate", "helpful_rate", "incorrect_rate", "repeated_error_rate"}
	if err := c.Write(header); err != nil {
		return err
	}
	write := func(scope, arm, team, id string, m Metrics) error {
		row := []string{scope, arm, team, id}
		for _, n := range []int{m.Decisions, m.ExposedDecisions, m.Exposures, m.Judged, m.Used, m.Confirmed, m.Incorrect, m.ErrorDecisions, m.RepeatedErrors} {
			row = append(row, strconv.Itoa(n))
		}
		for _, v := range []float64{m.HitRate.Value, m.HitRate.Low, m.HitRate.High, m.OutcomeCoverage.Value, m.UsedRate.Value, m.HelpfulRate.Value, m.IncorrectRate.Value, m.RepeatedErrorRate.Value} {
			row = append(row, strconv.FormatFloat(v, 'f', 6, 64))
		}
		return c.Write(row)
	}
	if err := write("aggregate", "baseline", "", "", rep.Baseline.Metrics); err != nil {
		return err
	}
	if err := write("aggregate", "treatment", "", "", rep.Treatment.Metrics); err != nil {
		return err
	}
	for _, s := range rep.Teams {
		if err := write("team", s.Arm, s.Team, "", s.Metrics); err != nil {
			return err
		}
	}
	for _, s := range rep.Records {
		if err := write("record", s.Arm, s.Team, s.RecordID, s.Metrics); err != nil {
			return err
		}
	}
	c.Flush()
	if err := c.Error(); err != nil {
		return fmt.Errorf("measurement: write CSV: %w", err)
	}
	return nil
}
