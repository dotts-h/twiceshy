// SPDX-License-Identifier: AGPL-3.0-only

// Package spool is the report intake queue (ADR-0013 §E1): report_outcome
// enqueues an outcome report here instead of returning markdown for a human to
// paste-PR, and the `intake-reports` CLI drains it into experience/ so adapt has
// nightly input. The queue stores the report REQUEST (not a built record), so the
// record id is allocated against the live corpus at intake time — never
// colliding across reports queued before a drain.
package spool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Report is one queued outcome report — the report_outcome arguments plus when it
// was filed. It is intentionally the request, not a record: the counter-record is
// built at intake against a fresh corpus id.
type Report struct {
	RecordID   string `json:"record_id"`
	Outcome    string `json:"outcome"`
	Evidence   string `json:"evidence,omitempty"`
	Author     string `json:"author"`
	Session    string `json:"session,omitempty"`
	ReportedAt string `json:"reported_at"`
}

// Enqueue writes r as a JSON file in dir (created if absent). It writes to a
// hidden temp file then renames to a unique `*.json` name, so a concurrent List
// never observes a half-written entry (rename is atomic within a directory). The
// filename is prefixed with ReportedAt so List returns roughly time order.
func Enqueue(dir string, r Report) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".enq-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, sanitize(r.ReportedAt)+"-"+filepath.Base(tmpName)+".json")
	if err := os.Rename(tmpName, final); err != nil {
		return "", err
	}
	return final, nil
}

// List returns the queued report file paths in dir, sorted (stable, roughly
// chronological). A missing dir is an empty queue, not an error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Read decodes a queued report file.
func Read(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return Report{}, err
	}
	return r, nil
}

// Remove deletes a processed queue file.
func Remove(path string) error { return os.Remove(path) }

// sanitize keeps a filename-safe prefix (the ReportedAt timestamp has ':' and
// '.' that are awkward in filenames on some systems).
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ':', '/', '\\', ' ':
			return '-'
		default:
			return r
		}
	}, s)
}
