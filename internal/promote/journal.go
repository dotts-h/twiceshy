// SPDX-License-Identifier: AGPL-3.0-only

package promote

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Journal is the incremental run log for promote/adapt corpus walks (#0054).
// A mid-run abort leaves Complete false with StoppedAt set; the next invocation
// resumes by skipping record ids already present in Actions.
type Journal struct {
	RunID     string         `json:"run_id"`
	Stage     string         `json:"stage"` // "promote" | "adapt"
	Complete  bool           `json:"complete"`
	StoppedAt *JournalStop   `json:"stopped_at,omitempty"`
	Actions   []RecordAction `json:"actions"`
}

// JournalStop records where a broker-outage abort halted the walk.
type JournalStop struct {
	RecordID string `json:"record_id"`
	Error    string `json:"error"`
}

// JournalPath is the on-disk location for a stage's run journal under corpus.
func JournalPath(corpus, stage string) string {
	return filepath.Join(corpus, "runs", stage+".journal.json")
}

// LoadJournal reads a journal from path. A missing file returns (nil, nil).
func LoadJournal(path string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// Save writes the journal atomically: indented JSON to path.tmp then rename.
func (j *Journal) Save(path string) error {
	if j.Actions == nil {
		j.Actions = []RecordAction{}
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DoneIDs returns the set of record ids with a terminal decision in Actions.
func (j *Journal) DoneIDs() map[string]bool {
	done := make(map[string]bool, len(j.Actions))
	for _, a := range j.Actions {
		done[a.ID] = true
	}
	return done
}
