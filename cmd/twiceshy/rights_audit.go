// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"

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
	if err := rejectSymlinkComponents(*corpus, false); err != nil {
		return fmt.Errorf("rights-audit: corpus path: %w", err)
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
	root := filepath.Clean(filepath.Dir(manifestPath))
	for _, artifact := range []struct{ label, path string }{
		{"manifest", manifestPath}, {"notices", noticesPath}, {"pack license", packLicensePath},
	} {
		if err := rejectSymlinkComponents(artifact.path, false); err != nil {
			return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: %s path: %w", artifact.label, err)
		}
		if filepath.Clean(filepath.Dir(artifact.path)) != root {
			return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: %s must be in the manifest pack root", artifact.label)
		}
	}
	if filepath.Base(manifestPath) != "MANIFEST.json" || filepath.Base(noticesPath) != "ATTRIBUTION.md" || filepath.Base(packLicensePath) != "LICENSE" {
		return rightsaudit.ArtifactValidation{}, errors.New("rights-audit: pack artifacts must use canonical names MANIFEST.json, ATTRIBUTION.md, and LICENSE")
	}
	packRoot, err := os.OpenRoot(root)
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: opening pack root: %w", err)
	}
	defer func() { _ = packRoot.Close() }()
	manifestData, err := packRoot.ReadFile("MANIFEST.json")
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
	notices, err := packRoot.ReadFile("ATTRIBUTION.md")
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading notices: %w", err)
	}
	packLicense, err := packRoot.ReadFile("LICENSE")
	if err != nil {
		return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading pack license: %w", err)
	}
	materials := make(map[string][]byte)
	for _, entry := range manifest.Attribution {
		for _, rel := range []string{entry.LicenseFile, entry.CopyrightFile, entry.NoticeFile} {
			if rel == "" {
				continue
			}
			path, err := safeJoin(filepath.Dir(manifestPath), rel)
			if err != nil {
				return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: invalid material path %q: %w", rel, err)
			}
			if err := rejectSymlinkComponents(path, false); err != nil {
				return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: material path %q: %w", rel, err)
			}
			body, err := packRoot.ReadFile(filepath.FromSlash(rel))
			if err != nil {
				return rightsaudit.ArtifactValidation{}, fmt.Errorf("rights-audit: reading material %q: %w", rel, err)
			}
			materials[rel] = body
		}
	}
	errs := pack.ValidateCommercialArtifacts(recs, manifest, notices, packLicense, materials)
	errs = append(errs, validatePackInventory(packRoot, recs)...)
	sort.Strings(errs)
	return rightsaudit.ArtifactValidation{Requested: true, Valid: len(errs) == 0, Errors: errs}, nil
}

func validatePackInventory(root *os.Root, recs []*record.Record) []string {
	expected := map[string]bool{"MANIFEST.json": true, "ATTRIBUTION.md": true, "LICENSE": true}
	manifest := pack.BuildManifest(recs, true, false)
	byID := make(map[string]*record.Record, len(recs))
	for _, rec := range recs {
		byID[rec.ID] = rec
	}
	for _, id := range manifest.Included {
		if rec := byID[id]; rec != nil {
			expected[filepath.ToSlash(rec.Path)] = true
		}
	}
	for _, entry := range manifest.Attribution {
		for _, rel := range []string{entry.LicenseFile, entry.CopyrightFile, entry.NoticeFile} {
			if rel != "" {
				expected[filepath.ToSlash(rel)] = true
			}
		}
	}

	actual := make(map[string]bool)
	var errs []string
	walkErr := iofs.WalkDir(root.FS(), ".", func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		rel := filepath.ToSlash(path)
		if entry.Type()&os.ModeSymlink != 0 {
			errs = append(errs, fmt.Sprintf("pack artifact path is a symbolic link: %s", rel))
			if entry.IsDir() {
				return iofs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			errs = append(errs, fmt.Sprintf("pack artifact is not a regular file: %s", rel))
			return nil
		}
		actual[rel] = true
		return nil
	})
	if walkErr != nil {
		errs = append(errs, fmt.Sprintf("cannot inventory pack root: %v", walkErr))
	}
	for rel := range expected {
		if !actual[rel] {
			errs = append(errs, "pack artifact is missing: "+rel)
		}
	}
	for rel := range actual {
		if !expected[rel] {
			errs = append(errs, "unreferenced pack artifact: "+rel)
		}
	}
	sort.Strings(errs)
	return errs
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
	if err := rejectSymlinkComponents(path, true); err != nil {
		return fmt.Errorf("rights-audit: remediation queue path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("rights-audit: creating remediation queue directory: %w", err)
	}
	if err := rejectSymlinkComponents(filepath.Dir(path), false); err != nil {
		return fmt.Errorf("rights-audit: remediation queue path: %w", err)
	}
	if err := writeFileAtomic(path, append(body, '\n'), 0o644); err != nil {
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
