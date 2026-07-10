// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dotts-h/twiceshy/internal/measurement"
)

type telemetryInputs []string

func (v *telemetryInputs) String() string { return fmt.Sprint([]string(*v)) }
func (v *telemetryInputs) Set(value string) error {
	if value == "" {
		return errors.New("empty telemetry path")
	}
	*v = append(*v, value)
	return nil
}

func runPilotReport(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("pilot-report", flag.ContinueOnError)
	var tele telemetryInputs
	fs.Var(&tele, "telemetry", "gate-decision JSONL path; repeat for every explicit archive generation")
	coh := fs.String("cohorts", "", "CSV team,session_hash cohort map")
	outs := fs.String("outcomes", "", "strict privacy-safe outcome JSONL")
	bs := fs.String("baseline-start", "", "baseline start RFC3339 (inclusive)")
	be := fs.String("baseline-end", "", "baseline end RFC3339 (exclusive)")
	ts := fs.String("treatment-start", "", "treatment start RFC3339 (inclusive)")
	te := fs.String("treatment-end", "", "treatment end RFC3339 (exclusive)")
	format := fs.String("format", "json", "json or csv")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if len(tele) == 0 || *coh == "" || *outs == "" {
		return errors.New("pilot-report requires -telemetry, -cohorts, and -outcomes")
	}
	parse := func(name, value string) (time.Time, error) {
		v, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("pilot-report: %s: %w", name, err)
		}
		return v, nil
	}
	bstart, err := parse("baseline-start", *bs)
	if err != nil {
		return err
	}
	bend, err := parse("baseline-end", *be)
	if err != nil {
		return err
	}
	tstart, err := parse("treatment-start", *ts)
	if err != nil {
		return err
	}
	tend, err := parse("treatment-end", *te)
	if err != nil {
		return err
	}
	decisions, err := measurement.LoadDecisions(tele)
	if err != nil {
		return err
	}
	cohorts, err := measurement.LoadCohorts(*coh)
	if err != nil {
		return err
	}
	outcomes, err := measurement.LoadOutcomes(*outs)
	if err != nil {
		return err
	}
	rep, err := measurement.Generate(measurement.Config{Baseline: measurement.Window{Start: bstart, End: bend}, Treatment: measurement.Window{Start: tstart, End: tend}, Cohorts: cohorts}, decisions, outcomes)
	if err != nil {
		return err
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "csv":
		return measurement.WriteCSV(out, rep)
	default:
		return fmt.Errorf("pilot-report: unknown format %q (want json or csv)", *format)
	}
}
