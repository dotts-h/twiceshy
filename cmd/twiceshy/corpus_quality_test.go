// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dotts-h/twiceshy/internal/corpusquality"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

func TestCorpusQualityJSON(t *testing.T) {
	var out bytes.Buffer
	if err := runCorpusQuality([]string{"-corpus", testcorpus.Root(), "-json"}, &out); err != nil {
		t.Fatalf("runCorpusQuality: %v", err)
	}
	var got corpusquality.Report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("JSON output: %v\n%s", err, out.String())
	}
	if got.TotalRecords == 0 || got.StatusCounts["validated"] == 0 {
		t.Fatalf("unexpected fixture report: %+v", got)
	}
}

func TestCorpusQualityProseIsStable(t *testing.T) {
	var first, second bytes.Buffer
	args := []string{"-corpus", testcorpus.Root()}
	if err := runCorpusQuality(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := runCorpusQuality(args, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatalf("output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
}

func TestRunDispatchesCorpusQuality(t *testing.T) {
	var out bytes.Buffer
	if err := run(t.Context(), []string{"corpus-quality", "-corpus", testcorpus.Root(), "-json"}, &out, noEnv); err != nil {
		t.Fatalf("run: %v", err)
	}
}
