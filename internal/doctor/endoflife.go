// SPDX-License-Identifier: AGPL-3.0-only

package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DefaultEOLBase is the public endoflife.date API base (MIT-licensed data).
const DefaultEOLBase = "https://endoflife.date"

// endoflifeSource fetches release cycles from an endoflife.date-compatible API.
type endoflifeSource struct {
	base   string
	client *http.Client
}

// NewEndOfLifeSource returns an EOLSource backed by endoflife.date (or a
// compatible base URL — used to point tests at an httptest server).
func NewEndOfLifeSource(base string) EOLSource {
	if base == "" {
		base = DefaultEOLBase
	}
	return endoflifeSource{base: base, client: &http.Client{Timeout: 10 * time.Second}}
}

// eolField captures endoflife.date's `eol`, which is a date string, or a
// boolean (true = already EOL with no date, false = not EOL).
type eolField struct {
	date string
	eol  bool
}

func (e *eolField) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		e.date, e.eol = s, true
		return nil
	}
	var ok bool
	if err := json.Unmarshal(b, &ok); err == nil {
		e.eol = ok // true → already EOL (no date); false → not EOL
		return nil
	}
	return fmt.Errorf("eol field is neither string nor bool: %s", b)
}

// normalized returns the EOL date string for a Cycle: the explicit date, a
// far-past sentinel when eol==true with no date, or "" when not EOL/unknown.
func (e eolField) normalized() string {
	if e.date != "" {
		return e.date
	}
	if e.eol {
		return "0001-01-01" // already EOL, date unknown — definitely past
	}
	return ""
}

func (s endoflifeSource) Cycles(ctx context.Context, product string) ([]Cycle, error) {
	url := s.base + "/api/" + product + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // unknown product — caller skips (no false flag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endoflife %s: status %d", product, resp.StatusCode)
	}
	var raw []struct {
		Cycle string   `json:"cycle"`
		EOL   eolField `json:"eol"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("endoflife %s: %w", product, err)
	}
	cycles := make([]Cycle, 0, len(raw))
	for _, r := range raw {
		cycles = append(cycles, Cycle{Cycle: r.Cycle, EOL: r.EOL.normalized()})
	}
	return cycles, nil
}
