// SPDX-License-Identifier: AGPL-3.0-only

// Package corpusquality derives deterministic quality and rights-coverage
// signals from an experience corpus. It reports facts only; it does not mutate
// records or decide whether a pack may be distributed.
package corpusquality

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Missing is the LicenseCounts key used for records without source_license.
const Missing = "(missing)"

// Coverage counts records carrying the minimum provenance/right fields named.
// Provenance means a non-empty provenance.source.author.
type Coverage struct {
	Provenance    int `json:"provenance"`
	SourceLicense int `json:"source_license"`
	SourceURL     int `json:"source_url"`
}

// Report is a deterministic corpus-quality snapshot.
type Report struct {
	TotalRecords                  int            `json:"total_records"`
	StatusCounts                  map[string]int `json:"status_counts"`
	KindCounts                    map[string]int `json:"kind_counts"`
	ValidatedActionableBehavioral int            `json:"validated_actionable_behavioral"`
	RecordsWithGuard              int            `json:"records_with_guard"`
	DeclaredRepros                int            `json:"declared_repros"`
	RunnableRepros                int            `json:"runnable_repros"`
	Coverage                      Coverage       `json:"coverage"`
	LicenseCounts                 map[string]int `json:"license_counts"`
}

// IsValidatedActionableBehavioral identifies the high-value behavioral subset:
// a served behavioral kind, excluding imported vulnerability advisories. The
// advisory decision deliberately delegates to record.IsAdvisoryClass, the same
// deterministic content predicate used by promotion routing.
func IsValidatedActionableBehavioral(rec *record.Record) bool {
	if rec == nil || rec.Status != "validated" || record.IsAdvisoryClass(rec) {
		return false
	}
	switch rec.Kind {
	case "trap", "fix", "dead-end":
		return true
	default:
		return false
	}
}

// Build derives a report from parsed records. corpusRoot is used only to check
// whether declared repro artifacts are stageable by the execution harness.
func Build(corpusRoot string, recs []*record.Record) Report {
	rep := Report{
		TotalRecords:  len(recs),
		StatusCounts:  seededCounts(record.Statuses),
		KindCounts:    seededCounts(record.Kinds),
		LicenseCounts: make(map[string]int),
	}
	for _, rec := range recs {
		if rec == nil {
			continue
		}
		rep.StatusCounts[rec.Status]++
		rep.KindCounts[rec.Kind]++
		if IsValidatedActionableBehavioral(rec) {
			rep.ValidatedActionableBehavioral++
		}
		if rec.Guard != nil {
			rep.RecordsWithGuard++
			for _, path := range declaredRepros(rec.Guard) {
				rep.DeclaredRepros++
				if stageable(corpusRoot, path) {
					rep.RunnableRepros++
				}
			}
		}
		if strings.TrimSpace(rec.Provenance.Source.Author) != "" {
			rep.Coverage.Provenance++
		}
		license := strings.TrimSpace(rec.Provenance.SourceLicense)
		if license == "" {
			license = Missing
		} else {
			rep.Coverage.SourceLicense++
		}
		rep.LicenseCounts[license]++
		if strings.TrimSpace(rec.Provenance.SourceURL) != "" {
			rep.Coverage.SourceURL++
		}
	}
	return rep
}

func seededCounts(values []string) map[string]int {
	out := make(map[string]int, len(values))
	for _, value := range values {
		out[value] = 0
	}
	return out
}

func declaredRepros(guard *record.Guard) []string {
	var paths []string
	if guard.Repro != nil && strings.TrimSpace(*guard.Repro) != "" {
		paths = append(paths, *guard.Repro)
	}
	for _, repro := range guard.Repros {
		if strings.TrimSpace(repro.Path) != "" {
			paths = append(paths, repro.Path)
		}
	}
	return paths
}

func stageable(root, path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	abs := filepath.Join(root, clean)
	info, err := os.Stat(abs)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return info.Mode().IsRegular()
	}
	repro, err := os.Stat(filepath.Join(abs, "repro.sh"))
	return err == nil && repro.Mode().IsRegular()
}
