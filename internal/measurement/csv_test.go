// SPDX-License-Identifier: AGPL-3.0-only
package measurement_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/measurement"
)

func TestCSVHasEveryConfidenceIntervalAndCorrectRecordFields(t *testing.T) {
	rate := measurement.Rate{Successes: 1, Total: 2, Value: .5, Low: .1, High: .9}
	rep := measurement.Report{Records: []measurement.RecordSummary{{Arm: "treatment", Team: "team-a", RecordID: "exp-0001", Metrics: measurement.RecordMetrics{Exposures: 2, Judged: 2, Used: 1, OutcomeCoverage: rate, UsedRate: rate, HelpfulRate: rate, IncorrectRate: rate}}}}
	var out bytes.Buffer
	if err := measurement.WriteCSV(&out, rep); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"outcome_coverage_low_95", "outcome_coverage_high_95", "used_low_95", "used_high_95", "helpful_low_95", "helpful_high_95", "incorrect_low_95", "incorrect_high_95", "repeated_error_low_95", "repeated_error_high_95"} {
		if !containsCell(rows[0], name) {
			t.Errorf("missing CSV confidence column %s", name)
		}
	}
	record := rows[len(rows)-1]
	if len(record) != len(rows[0]) {
		t.Fatalf("record columns=%d header=%d", len(record), len(rows[0]))
	}
	if record[4] != "" || record[5] != "" || record[6] != "2" {
		t.Fatalf("record row uses invalid decision denominators: %v", record[:7])
	}
}
func containsCell(row []string, want string) bool {
	for _, v := range row {
		if v == want {
			return true
		}
	}
	return false
}
