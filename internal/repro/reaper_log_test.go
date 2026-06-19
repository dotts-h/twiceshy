// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// parseReaperLogEvents decodes slog JSON lines from the reaper/cleanup path.
func parseReaperLogEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var events []map[string]any
	sc := bufio.NewScanner(bytes.NewBufferString(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func TestCleanup_VolumeRemoveFailure_LogsStructuredWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	s := &stubRunner{
		responder: func(rc recordedCall) (execResult, error) {
			if len(rc.args) >= 1 && rc.args[0] == "ps" {
				return execResult{}, nil
			}
			if len(rc.args) >= 2 && rc.args[0] == "volume" && rc.args[1] == "rm" {
				return execResult{}, errors.New("volume still in use")
			}
			return execResult{}, nil
		},
	}

	b := NewBroker([]string{PinnedGoImage},
		withRunner(s),
		WithLogger(logger),
		withIDFunc(func() (string, error) { return "runid", nil }),
	).(*dockerBroker)

	const runID = "run-abc"
	const vol = "twiceshy-repro-vol-xyz"
	b.cleanup(runID, vol)

	events := parseReaperLogEvents(t, buf.String())
	if len(events) != 1 {
		t.Fatalf("expected 1 log event, got %d: %q", len(events), buf.String())
	}
	ev := events[0]

	level, _ := ev["level"].(string)
	if level != "WARN" {
		t.Fatalf("level = %q, want WARN: %v", level, ev)
	}
	msg, _ := ev["msg"].(string)
	if !strings.Contains(msg, "reaper") {
		t.Fatalf("msg %q should contain reaper", msg)
	}
	if ev["volume"] != vol {
		t.Fatalf("volume = %v, want %q", ev["volume"], vol)
	}
	if ev["run"] != runID {
		t.Fatalf("run = %v, want %q", ev["run"], runID)
	}
	if ev["error"] != "volume still in use" {
		t.Fatalf("error = %v, want %q", ev["error"], "volume still in use")
	}
	if ev["retry"] != true {
		t.Fatalf("retry = %v, want true", ev["retry"])
	}

	// No bare log.Printf-style prose lines.
	if strings.Contains(buf.String(), "repro: WARNING") {
		t.Fatalf("unexpected bare log.Printf output: %q", buf.String())
	}
}

func TestRemoveContainersByLabel_Failures_LogsStructuredWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	const label = labelKey + "=run-xyz"

	t.Run("list containers", func(t *testing.T) {
		buf.Reset()
		s := &stubRunner{
			responder: func(rc recordedCall) (execResult, error) {
				if len(rc.args) >= 1 && rc.args[0] == "ps" {
					return execResult{}, errors.New("daemon unreachable")
				}
				return execResult{}, nil
			},
		}
		b := NewBroker([]string{PinnedGoImage}, withRunner(s), WithLogger(logger)).(*dockerBroker)
		b.removeContainersByLabel(context.Background(), label)

		events := parseReaperLogEvents(t, buf.String())
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d: %q", len(events), buf.String())
		}
		ev := events[0]
		if ev["level"] != "WARN" {
			t.Fatalf("level = %v, want WARN", ev["level"])
		}
		if !strings.Contains(ev["msg"].(string), "reaper") {
			t.Fatalf("msg = %v", ev["msg"])
		}
		if ev["label"] != label {
			t.Fatalf("label = %v, want %q", ev["label"], label)
		}
		if ev["error"] != "daemon unreachable" {
			t.Fatalf("error = %v", ev["error"])
		}
		if ev["retry"] != true {
			t.Fatalf("retry = %v", ev["retry"])
		}
	})

	t.Run("remove container", func(t *testing.T) {
		buf.Reset()
		const cid = "deadbeef"
		s := &stubRunner{
			responder: func(rc recordedCall) (execResult, error) {
				if len(rc.args) >= 1 && rc.args[0] == "ps" {
					return execResult{stdout: cid}, nil
				}
				if len(rc.args) >= 1 && rc.args[0] == "rm" {
					return execResult{}, errors.New("container locked")
				}
				return execResult{}, nil
			},
		}
		b := NewBroker([]string{PinnedGoImage}, withRunner(s), WithLogger(logger)).(*dockerBroker)
		b.removeContainersByLabel(context.Background(), label)

		events := parseReaperLogEvents(t, buf.String())
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d: %q", len(events), buf.String())
		}
		ev := events[0]
		if ev["level"] != "WARN" {
			t.Fatalf("level = %v, want WARN", ev["level"])
		}
		if !strings.Contains(ev["msg"].(string), "reaper") {
			t.Fatalf("msg = %v", ev["msg"])
		}
		if ev["container"] != cid {
			t.Fatalf("container = %v, want %q", ev["container"], cid)
		}
		if ev["label"] != label {
			t.Fatalf("label = %v, want %q", ev["label"], label)
		}
		if ev["error"] != "container locked" {
			t.Fatalf("error = %v", ev["error"])
		}
		if ev["retry"] != true {
			t.Fatalf("retry = %v", ev["retry"])
		}
	})
}
