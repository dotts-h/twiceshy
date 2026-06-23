// SPDX-License-Identifier: AGPL-3.0-only

package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// servedScanBuf bounds one decision line the reader will accept — generous over any
// honest decision (a few served ids + tokens), well above bufio's 64 KiB default so a
// long-but-valid line is not silently truncated mid-parse.
const servedScanBuf = 1 << 20

// ServedInSession returns the set of record ids that were served (search or push) to
// the session whose salted hash is sessionHash (#0069), unioned across the active log
// and its one rotated generation (<path>.1). The retro helpfulness join uses it to
// confirm only cards that were actually served in the session being judged, rather
// than trusting a model's verdict ids blindly.
//
// An empty sessionHash matches nothing — a decision recorded without a session
// correlation key is never attributable. A missing log is not an error (no decisions
// were logged → empty set). The read is best-effort: a torn/garbled line is skipped,
// and a line beyond the buffer cap (corruption only — honest decision lines are tiny)
// stops that file's scan without failing the join.
//
// It assumes a QUIESCED log (the retro join runs offline). It is not safe to read
// concurrently with a live Recorder mid-rotation, which could under-count a session
// that spans the rotation — close/flush the Recorder first, as the tests do.
func ServedInSession(path, sessionHash string) (map[string]bool, error) {
	served := map[string]bool{}
	if sessionHash == "" {
		return served, nil
	}
	// Read the prior generation first, then the active log — order is immaterial to a
	// set union, but covers a session that spans a rotation.
	for _, p := range []string{path + ".1", path} {
		if err := scanServed(p, sessionHash, served); err != nil {
			return nil, err
		}
	}
	return served, nil
}

// scanServed adds, to served, every served id from the decisions in one log file whose
// Session equals sessionHash. A non-existent file is a no-op (no error).
func scanServed(path, sessionHash string, served map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("telemetry: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), servedScanBuf)
	for sc.Scan() {
		var d Decision
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			continue // skip a torn/garbled line; the read is best-effort
		}
		if d.Session != sessionHash {
			continue
		}
		for _, s := range d.Served {
			served[s.ID] = true
		}
	}
	// A line beyond servedScanBuf (corruption — honest lines are tiny) stops this
	// file's scan; treat it as best-effort, not fatal, so the join still runs.
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return nil
}
