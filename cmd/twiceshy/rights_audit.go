// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/rightsaudit"
)

func runRightsAudit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("rights-audit", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	asJSON := fs.Bool("json", false, "emit the complete deterministic report as JSON")
	queue := fs.String("queue", "", "write the unresolved-evidence remediation queue as JSON")
	failUnknown := fs.Bool("fail-on-unknown", false, "exit non-zero when rights evidence is missing, incomplete, or unrecognized")
	manifestPath := fs.String("manifest", "", "optional commercial pack MANIFEST.json to validate")
	noticesPath := fs.String("notices", "", "optional commercial pack ATTRIBUTION.md/source-license notice document to validate")
	packLicensePath := fs.String("pack-license", "", "optional commercial pack LICENSE terms to validate")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	artifactArgs := 0
	for _, value := range []string{*manifestPath, *noticesPath, *packLicensePath} {
		if value != "" {
			artifactArgs++
		}
	}
	if artifactArgs != 0 && artifactArgs != 3 {
		return errors.New("rights-audit requires -manifest, -notices, and -pack-license together")
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("rights-audit: loading corpus: %w", err)
	}
	rep := rightsaudit.Build(*corpus, recs)
	if *manifestPath != "" {
		validation, err := validateRightsArtifacts(recs, *manifestPath, *noticesPath, *packLicensePath)
		if err != nil {
			return err
		}
		rep.ArtifactValidation = &validation
	}

	if *asJSON {
		if err := writeJSON(out, rep); err != nil {
			return err
		}
	} else {
		printRightsAudit(out, rep)
	}
	if *queue != "" {
		if err := writeRemediationQueue(*queue, rep.RemediationQueue); err != nil {
			return err
		}
	}
	if rep.ArtifactValidation != nil && !rep.ArtifactValidation.Valid {
		return fmt.Errorf("rights-audit: commercial pack artifacts failed validation (%d finding(s))", len(rep.ArtifactValidation.Errors))
	}
	if *failUnknown && rep.UnresolvedEvidence > 0 {
		return fmt.Errorf("rights-audit: %d record(s) have missing, incomplete, or unrecognized rights evidence", rep.UnresolvedEvidence)
	}
	return nil
}

func validateRightsArtifacts(recs []*record.Record, manifestPath, noticesPath, packLicensePath string) (rightsaudit.ArtifactValidation, error) {
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading manifest: %w", err)
	}
	var manifest pack.Manifest
	dec := json.NewDecoder(bytes.NewReader(manifestData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: parsing manifest: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected second JSON value")
		}
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: manifest has trailing data: %w", err)
	}
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading notices: %w", err)
	}
	packLicense, err := os.ReadFile(packLicensePath)
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading pack license: %w", err)
	}
	errs := pack.ValidateCommercialArtifacts(recs, manifest, notices, packLicense)
	return rightsaudit.ArtifactValidation{Requested: true, Valid: len(errs) == 0, Errors: errs}, nil
}

func writeJSON(out io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

func writeRemediationQueue(path string, queue []rightsaudit.Remediation) error {
	body, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("rights-audit: creating remediation queue directory: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("rights-audit: writing remediation queue: %w", err)
	}
	return nil
}

func printRightsAudit(out io.Writer, rep rightsaudit.Report) {
	_, _ = fmt.Fprintf(out, "rights audit: %d records; %d commercial-eligible; %d unresolved evidence\n", rep.TotalRecords, rep.CommercialEligible, rep.UnresolvedEvidence)
	for _, bucket := range rep.ReasonBuckets {
		_, _ = fmt.Fprintf(out, "  %-24s %d\n", bucket.Code, bucket.Count)
	}
	if rep.ArtifactValidation != nil {
		_, _ = fmt.Fprintf(out, "pack artifacts valid: %v\n", rep.ArtifactValidation.Valid)
		for _, finding := range rep.ArtifactValidation.Errors {
			_, _ = fmt.Fprintf(out, "  - %s\n", finding)
		}
	}
}
