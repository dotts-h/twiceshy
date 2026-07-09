// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// mkProspectRecord builds a minimal validated trap/fix record for prospector tests
// — just the fields prospectEligible, renderProspectCard, and the leak guard read.
func mkProspectRecord(id, kind, status, author string) *record.Record {
	return &record.Record{
		ID:         id,
		Kind:       kind,
		Status:     status,
		Title:      "title for " + id,
		Provenance: record.Provenance{Source: record.Source{Author: author}},
		Symptom:    &record.Symptom{Summary: "symptom summary for " + id},
		Resolution: &record.Resolution{RootCause: "root cause for " + id, Fix: "fix text for " + id},
	}
}

// stubDrafter maps a record ID to either a canned TaskCase or an error (e.g.
// ErrTaskUnsupported), so Prospect's per-record branches are testable without a
// real off-pool model.
type stubDrafter struct {
	tasks map[string]TaskCase
	errs  map[string]error
}

func (d *stubDrafter) Name() string { return "stub-drafter" }

func (d *stubDrafter) DraftTask(_ context.Context, rec *record.Record) (TaskCase, error) {
	if err, ok := d.errs[rec.ID]; ok {
		return TaskCase{}, err
	}
	if tc, ok := d.tasks[rec.ID]; ok {
		return tc, nil
	}
	return TaskCase{}, errors.New("stubDrafter: no task configured for " + rec.ID)
}

// prospectStubRunner records every OFF-arm (card == "") and ON-arm (card != "")
// prompt it was called with, so tests can assert whether either arm ran at all —
// the load-bearing branches ("OFF avoided" and now "control voided" must skip
// arm runs entirely).
type prospectStubRunner struct {
	offCalls []string
	onCalls  []string
}

func (r *prospectStubRunner) Run(_ context.Context, prompt, card string) (Result, error) {
	if card == "" {
		r.offCalls = append(r.offCalls, prompt)
		return Result{Output: "OFF:" + prompt, Tokens: 10}, nil
	}
	r.onCalls = append(r.onCalls, prompt)
	return Result{Output: "ON:" + prompt, Tokens: 20}, nil
}

// prospectStubVerifier maps a TaskCase's Prompt to the avoidance verdict for its
// OFF and ON arm, keyed by the OFF:/ON: prefix prospectStubRunner's Output
// carries. Prospect now also runs the SAME Avoided call directly against
// tc.Control (no runner involved, so the output carries neither prefix) — that
// call is resolved via controlAvoided/controlErr, keyed by the plain control
// text itself, distinct from the OFF:/ON:-prefixed runner outputs. An unset
// control entry defaults to "avoided" so every pre-existing test in this file
// (none of which populate controlAvoided) keeps its original behavior.
type prospectStubVerifier struct {
	offAvoided     map[string]bool
	offErr         map[string]error
	onAvoided      map[string]bool
	onErr          map[string]error
	controlAvoided map[string]bool
	controlErr     map[string]error
}

func (v *prospectStubVerifier) Avoided(_ context.Context, c TaskCase, output string) (bool, error) {
	switch {
	case strings.HasPrefix(output, "OFF:"):
		if err, ok := v.offErr[c.Prompt]; ok {
			return false, err
		}
		return v.offAvoided[c.Prompt], nil
	case strings.HasPrefix(output, "ON:"):
		if err, ok := v.onErr[c.Prompt]; ok {
			return false, err
		}
		return v.onAvoided[c.Prompt], nil
	default:
		if err, ok := v.controlErr[output]; ok {
			return false, err
		}
		if got, ok := v.controlAvoided[output]; ok {
			return got, nil
		}
		return true, nil
	}
}

func TestProspect_EligibilitySkipsImporterWrongKindAndStatus(t *testing.T) {
	recs := []*record.Record{
		mkProspectRecord("exp-0001", "trap", "validated", "twiceshy-importer"), // importer origin
		mkProspectRecord("exp-0002", "convention", "validated", "horia"),       // wrong kind
		mkProspectRecord("exp-0003", "trap", "quarantined", "horia"),           // wrong status
		mkProspectRecord("exp-0004", "trap", "validated", "horia"),             // eligible
	}
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0004": {TrapID: "exp-0004", Prompt: "eligible prompt", VerifyID: "gobuild"},
	}}
	runner := &prospectStubRunner{}
	verifier := &prospectStubVerifier{offAvoided: map[string]bool{"eligible prompt": true}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Scanned != 4 {
		t.Errorf("Scanned = %d, want 4", rep.Scanned)
	}
	if rep.Eligible != 1 {
		t.Errorf("Eligible = %d, want 1", rep.Eligible)
	}
	if rep.Skipped["ineligible"] != 3 {
		t.Errorf("Skipped[ineligible] = %d, want 3; skipped=%v", rep.Skipped["ineligible"], rep.Skipped)
	}
	if rep.Drafted != 1 {
		t.Errorf("Drafted = %d, want 1", rep.Drafted)
	}
}

// The aggregate Skipped counts answer "how many were skipped and why-category",
// but auditing a specific record ("why was exp-X skipped?") required a one-record
// re-run (#0144). SkipReasons carries the per-record reason so every future audit
// is cheap: each skipped record id maps to its skip category.
func TestProspect_SkipReasonsArePerRecord(t *testing.T) {
	recs := []*record.Record{
		mkProspectRecord("exp-0001", "convention", "validated", "horia"), // ineligible (wrong kind)
		mkProspectRecord("exp-0002", "trap", "validated", "horia"),       // unsupported (drafter declines)
		mkProspectRecord("exp-0003", "trap", "validated", "horia"),       // control-fail
	}
	drafter := &stubDrafter{
		tasks: map[string]TaskCase{
			"exp-0003": {TrapID: "exp-0003", Prompt: "control-fail prompt", VerifyID: "gobuild", Control: "ctrl"},
		},
		errs: map[string]error{"exp-0002": ErrTaskUnsupported},
	}
	runner := &prospectStubRunner{}
	// exp-0003's control does NOT verify as avoided → control-fail skip.
	verifier := &prospectStubVerifier{controlAvoided: map[string]bool{"ctrl": false}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	want := map[string]string{
		"exp-0001": "ineligible",
		"exp-0002": "unsupported",
		"exp-0003": "control",
	}
	for id, reason := range want {
		if got := rep.SkipReasons[id]; got != reason {
			t.Errorf("SkipReasons[%s] = %q, want %q; all=%v", id, got, reason, rep.SkipReasons)
		}
	}
	// A record that was NOT skipped must not appear.
	if _, ok := rep.SkipReasons["exp-9999"]; ok {
		t.Errorf("SkipReasons must only contain skipped records; got %v", rep.SkipReasons)
	}
}

func TestProspect_DrafterUnsupportedCountedAsSkip(t *testing.T) {
	recs := []*record.Record{mkProspectRecord("exp-0001", "trap", "validated", "horia")}
	drafter := &stubDrafter{errs: map[string]error{"exp-0001": ErrTaskUnsupported}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: recs, Runner: &prospectStubRunner{}, Verifier: &prospectStubVerifier{}, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Skipped["unsupported"] != 1 {
		t.Errorf("Skipped[unsupported] = %d, want 1; skipped=%v", rep.Skipped["unsupported"], rep.Skipped)
	}
	if rep.Drafted != 0 {
		t.Errorf("Drafted = %d, want 0", rep.Drafted)
	}
}

// A drafter error that is NOT ErrTaskUnsupported is a real failure (a transport
// error, say) and must abort the run — mirroring drafter.Pipeline's "the first
// that does not decline owns the candidate; anything else surfaces" contract.
func TestProspect_DrafterHardErrorAborts(t *testing.T) {
	recs := []*record.Record{mkProspectRecord("exp-0001", "trap", "validated", "horia")}
	wantErr := errors.New("boom")
	drafter := &stubDrafter{errs: map[string]error{"exp-0001": wantErr}}

	_, err := Prospect(context.Background(), ProspectConfig{
		Records: recs, Runner: &prospectStubRunner{}, Verifier: &prospectStubVerifier{}, Drafter: drafter, Max: 10,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("want the hard drafter error to surface, got %v", err)
	}
}

func TestProspect_LeakGuardSkipsCopiedPrompt(t *testing.T) {
	rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
	rec.Resolution.Fix = "escape embedded quotes before matching each token safely against the corpus"
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		// The drafted prompt quotes the fix nearly verbatim — the leak the guard exists for.
		"exp-0001": {TrapID: "exp-0001", Prompt: "Please " + rec.Resolution.Fix, VerifyID: "gobuild"},
	}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: []*record.Record{rec}, Runner: &prospectStubRunner{}, Verifier: &prospectStubVerifier{}, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Skipped["leak"] != 1 {
		t.Errorf("Skipped[leak] = %d, want 1; skipped=%v", rep.Skipped["leak"], rep.Skipped)
	}
	if rep.Drafted != 0 {
		t.Errorf("Drafted = %d, want 0 (leaked draft must not count as drafted)", rep.Drafted)
	}
}

// The control check runs right after the leak guard and before rep.Drafted is
// incremented. When the control does NOT verify as avoided, the case is voided:
// it never reaches the OFF or ON arm (the runner is never called for it), it
// never appears in rep.ModelHard or rep.OffAvoided, rep.Drafted does not count
// it, and rep.Skipped["control"] — a new reason alongside "ineligible",
// "unsupported", and "leak" — is incremented instead.
func TestProspect_ControlFailsVoidsCase(t *testing.T) {
	rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0001": {
			TrapID: "exp-0001", Prompt: "solve it cleanly", VerifyID: "gobuild",
			Control: "control answer text",
		},
	}}
	runner := &prospectStubRunner{}
	// offAvoided/onAvoided are deliberately set to true here: if the control gate
	// were a no-op, this record would sail through as an ordinary OFF-avoided
	// case. The assertions below prove that does NOT happen — the control gate
	// overrides, and the OFF/ON verdicts configured here are never even consulted.
	verifier := &prospectStubVerifier{
		controlAvoided: map[string]bool{"control answer text": false},
		offAvoided:     map[string]bool{"solve it cleanly": true},
		onAvoided:      map[string]bool{"solve it cleanly": true},
	}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: []*record.Record{rec}, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Skipped["control"] != 1 {
		t.Errorf("Skipped[control] = %d, want 1; skipped=%v", rep.Skipped["control"], rep.Skipped)
	}
	if rep.Drafted != 0 {
		t.Errorf("Drafted = %d, want 0 (a control-voided case must not count as drafted)", rep.Drafted)
	}
	if len(rep.ModelHard) != 0 {
		t.Errorf("ModelHard = %v, want empty (a control-voided case never reaches an arm)", rep.ModelHard)
	}
	if len(rep.OffAvoided) != 0 {
		t.Errorf("OffAvoided = %v, want empty (a control-voided case never reaches an arm)", rep.OffAvoided)
	}
	if len(runner.offCalls) != 0 || len(runner.onCalls) != 0 {
		t.Errorf("runner must never be called for a control-voided case; offCalls=%v onCalls=%v", runner.offCalls, runner.onCalls)
	}
}

// A transport/verify error on the control check aborts the run, exactly like the
// OFF/ON verify calls already do.
func TestProspect_ControlVerifyErrorAborts(t *testing.T) {
	rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0001": {
			TrapID: "exp-0001", Prompt: "solve it cleanly", VerifyID: "gobuild",
			Control: "control answer text",
		},
	}}
	wantErr := errors.New("verify transport boom")
	verifier := &prospectStubVerifier{
		controlErr: map[string]error{"control answer text": wantErr},
	}

	_, err := Prospect(context.Background(), ProspectConfig{
		Records: []*record.Record{rec}, Runner: &prospectStubRunner{}, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("want the control verify error to abort the run, got %v", err)
	}
}

// When the control DOES verify as avoided, behavior is byte-for-byte unchanged
// from today: the existing OFF-hit/ON-run path runs exactly as it did before the
// control gate existed.
func TestProspect_ControlPassesRunsExistingOffHitOnArmPath(t *testing.T) {
	rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0001": {
			TrapID: "exp-0001", Prompt: "solve it cleanly", VerifyID: "gobuild",
			Control: "control answer text",
		},
	}}
	runner := &prospectStubRunner{}
	verifier := &prospectStubVerifier{
		controlAvoided: map[string]bool{"control answer text": true},
		offAvoided:     map[string]bool{"solve it cleanly": false},
		onAvoided:      map[string]bool{"solve it cleanly": true},
	}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: []*record.Record{rec}, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Skipped["control"] != 0 {
		t.Errorf("Skipped[control] = %d, want 0; skipped=%v", rep.Skipped["control"], rep.Skipped)
	}
	if rep.Drafted != 1 {
		t.Errorf("Drafted = %d, want 1", rep.Drafted)
	}
	if len(runner.offCalls) != 1 {
		t.Fatalf("want the OFF arm run exactly once, got %d", len(runner.offCalls))
	}
	if len(runner.onCalls) != 1 {
		t.Fatalf("want the ON arm run exactly once, got %d", len(runner.onCalls))
	}
	if len(rep.OffAvoided) != 0 {
		t.Errorf("OffAvoided = %v, want empty (the OFF arm hit)", rep.OffAvoided)
	}
	if len(rep.ModelHard) != 1 {
		t.Fatalf("ModelHard len = %d, want 1", len(rep.ModelHard))
	}
	if got := rep.ModelHard[0]; got.TrapID != "exp-0001" || !got.OnAvoided {
		t.Errorf("ModelHard[0] = %+v, want TrapID exp-0001 OnAvoided true", got)
	}
}

func TestProspect_OffAvoidedSkipsOnArm(t *testing.T) {
	rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0001": {TrapID: "exp-0001", Prompt: "solve it cleanly", VerifyID: "gobuild"},
	}}
	runner := &prospectStubRunner{}
	verifier := &prospectStubVerifier{offAvoided: map[string]bool{"solve it cleanly": true}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: []*record.Record{rec}, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if len(runner.onCalls) != 0 {
		t.Errorf("ON arm must not run when OFF already avoided; onCalls=%v", runner.onCalls)
	}
	if len(rep.OffAvoided) != 1 || rep.OffAvoided[0] != "exp-0001" {
		t.Errorf("OffAvoided = %v, want [exp-0001]", rep.OffAvoided)
	}
	if len(rep.ModelHard) != 0 {
		t.Errorf("ModelHard = %v, want empty (nothing bit)", rep.ModelHard)
	}
}

// An OFF-arm hit runs the ON arm; both ON outcomes are distinct, visible classes:
// the card helping (OnAvoided true) and the "on-also-fails" model-hard lead
// (OnAvoided false) — #0114's distinct class.
func TestProspect_OffHitRunsOnArmBothClasses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		onAvoided bool
	}{
		{"card helps", true},
		{"card does not help (on-also-fails)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := mkProspectRecord("exp-0001", "trap", "validated", "horia")
			drafter := &stubDrafter{tasks: map[string]TaskCase{
				"exp-0001": {TrapID: "exp-0001", Prompt: "solve it cleanly", VerifyID: "gobuild"},
			}}
			runner := &prospectStubRunner{}
			verifier := &prospectStubVerifier{
				offAvoided: map[string]bool{"solve it cleanly": false},
				onAvoided:  map[string]bool{"solve it cleanly": tc.onAvoided},
			}

			rep, err := Prospect(context.Background(), ProspectConfig{
				Records: []*record.Record{rec}, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
			})
			if err != nil {
				t.Fatalf("Prospect: %v", err)
			}
			if len(runner.onCalls) != 1 {
				t.Fatalf("want the ON arm run exactly once, got %d", len(runner.onCalls))
			}
			if len(rep.OffAvoided) != 0 {
				t.Errorf("OffAvoided = %v, want empty (the OFF arm hit)", rep.OffAvoided)
			}
			if len(rep.ModelHard) != 1 {
				t.Fatalf("ModelHard len = %d, want 1", len(rep.ModelHard))
			}
			got := rep.ModelHard[0]
			if got.TrapID != "exp-0001" || got.VerifyID != "gobuild" {
				t.Errorf("ModelHard[0] = %+v, want TrapID exp-0001 VerifyID gobuild", got)
			}
			if got.OnAvoided != tc.onAvoided {
				t.Errorf("OnAvoided = %v, want %v", got.OnAvoided, tc.onAvoided)
			}
			if got.TokensOff != 10 || got.TokensOn != 20 {
				t.Errorf("tokens off/on = %d/%d, want 10/20", got.TokensOff, got.TokensOn)
			}
		})
	}
}

func TestProspect_MaxBoundRespected(t *testing.T) {
	recs := []*record.Record{
		mkProspectRecord("exp-0001", "trap", "validated", "horia"),
		mkProspectRecord("exp-0002", "trap", "validated", "horia"),
		mkProspectRecord("exp-0003", "trap", "validated", "horia"),
	}
	drafter := &stubDrafter{tasks: map[string]TaskCase{
		"exp-0001": {TrapID: "exp-0001", Prompt: "p1", VerifyID: "gobuild"},
		"exp-0002": {TrapID: "exp-0002", Prompt: "p2", VerifyID: "gobuild"},
		"exp-0003": {TrapID: "exp-0003", Prompt: "p3", VerifyID: "gobuild"},
	}}
	verifier := &prospectStubVerifier{offAvoided: map[string]bool{"p1": true, "p2": true, "p3": true}}

	rep, err := Prospect(context.Background(), ProspectConfig{
		Records: recs, Runner: &prospectStubRunner{}, Verifier: verifier, Drafter: drafter, Max: 2,
	})
	if err != nil {
		t.Fatalf("Prospect: %v", err)
	}
	if rep.Drafted != 2 {
		t.Errorf("Drafted = %d, want 2 (Max bound)", rep.Drafted)
	}
}

func TestProspectEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  *record.Record
		want bool
	}{
		{"validated trap, non-importer", mkProspectRecord("exp-0001", "trap", "validated", "horia"), true},
		{"validated fix, non-importer", mkProspectRecord("exp-0002", "fix", "validated", "horia"), true},
		{"importer origin (any case)", mkProspectRecord("exp-0003", "trap", "validated", "Twiceshy-Importer"), false},
		{"wrong kind", mkProspectRecord("exp-0004", "convention", "validated", "horia"), false},
		{"wrong status", mkProspectRecord("exp-0005", "trap", "quarantined", "horia"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := prospectEligible(tc.rec); got != tc.want {
				t.Errorf("prospectEligible(%s) = %v, want %v", tc.rec.ID, got, tc.want)
			}
		})
	}
}

func TestProspect_DepsUnavailableSkip(t *testing.T) {
	t.Run("skip deps-unavailable control-verify error, process next", func(t *testing.T) {
		recs := []*record.Record{
			mkProspectRecord("exp-0001", "trap", "validated", "horia"),
			mkProspectRecord("exp-0002", "trap", "validated", "horia"),
		}
		drafter := &stubDrafter{tasks: map[string]TaskCase{
			"exp-0001": {TrapID: "exp-0001", Prompt: "bad deps prompt", VerifyID: "tsc", Control: "control text 1"},
			"exp-0002": {TrapID: "exp-0002", Prompt: "good prompt", VerifyID: "tsc", Control: "control text 2"},
		}}
		runner := &prospectStubRunner{}
		verifier := &prospectStubVerifier{
			controlErr: map[string]error{
				"control text 1": ErrDepsUnavailable,
			},
			offAvoided: map[string]bool{
				"good prompt": true,
			},
		}

		rep, err := Prospect(context.Background(), ProspectConfig{
			Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
		})
		if err != nil {
			t.Fatalf("Prospect: %v", err)
		}
		if rep.Skipped["deps"] != 1 {
			t.Errorf("Skipped[deps] = %d, want 1", rep.Skipped["deps"])
		}
		if rep.Drafted != 1 {
			t.Errorf("Drafted = %d, want 1", rep.Drafted)
		}
		if len(rep.OffAvoided) != 1 || rep.OffAvoided[0] != "exp-0002" {
			t.Errorf("OffAvoided = %v, want [exp-0002]", rep.OffAvoided)
		}
	})

	t.Run("non-404 prepare failure control-verify error aborts", func(t *testing.T) {
		recs := []*record.Record{
			mkProspectRecord("exp-0001", "trap", "validated", "horia"),
		}
		drafter := &stubDrafter{tasks: map[string]TaskCase{
			"exp-0001": {TrapID: "exp-0001", Prompt: "prompt 1", VerifyID: "tsc", Control: "control text 1"},
		}}
		runner := &prospectStubRunner{}
		verifier := &prospectStubVerifier{
			controlErr: map[string]error{
				"control text 1": errors.New("some other error"),
			},
		}

		_, err := Prospect(context.Background(), ProspectConfig{
			Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
		})
		if err == nil {
			t.Fatal("expected run-aborting error, got nil")
		}
		if errors.Is(err, ErrDepsUnavailable) {
			t.Error("expected non-404 error, got ErrDepsUnavailable")
		}
	})

	t.Run("skip deps-unavailable OFF-arm verify error preserves counters", func(t *testing.T) {
		recs := []*record.Record{
			mkProspectRecord("exp-0001", "trap", "validated", "horia"),
		}
		drafter := &stubDrafter{tasks: map[string]TaskCase{
			"exp-0001": {TrapID: "exp-0001", Prompt: "bad deps prompt", VerifyID: "tsc", Control: "control text 1"},
		}}
		runner := &prospectStubRunner{}
		verifier := &prospectStubVerifier{
			controlAvoided: map[string]bool{"control text 1": true},
			offErr: map[string]error{
				"bad deps prompt": ErrDepsUnavailable,
			},
		}

		rep, err := Prospect(context.Background(), ProspectConfig{
			Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
		})
		if err != nil {
			t.Fatalf("Prospect: %v", err)
		}
		if rep.Skipped["deps"] != 1 {
			t.Errorf("Skipped[deps] = %d, want 1", rep.Skipped["deps"])
		}
		if rep.Drafted != 0 {
			t.Errorf("Drafted = %d, want 0", rep.Drafted)
		}
		if len(rep.OffAvoided) != 0 {
			t.Errorf("OffAvoided = %v, want empty", rep.OffAvoided)
		}
		if len(rep.ModelHard) != 0 {
			t.Errorf("ModelHard = %v, want empty", rep.ModelHard)
		}
	})

	t.Run("skip deps-unavailable ON-arm verify error preserves counters", func(t *testing.T) {
		recs := []*record.Record{
			mkProspectRecord("exp-0001", "trap", "validated", "horia"),
		}
		drafter := &stubDrafter{tasks: map[string]TaskCase{
			"exp-0001": {TrapID: "exp-0001", Prompt: "bad deps prompt", VerifyID: "tsc", Control: "control text 1"},
		}}
		runner := &prospectStubRunner{}
		verifier := &prospectStubVerifier{
			controlAvoided: map[string]bool{"control text 1": true},
			offAvoided:     map[string]bool{"bad deps prompt": false},
			onErr: map[string]error{
				"bad deps prompt": ErrDepsUnavailable,
			},
		}

		rep, err := Prospect(context.Background(), ProspectConfig{
			Records: recs, Runner: runner, Verifier: verifier, Drafter: drafter, Max: 10,
		})
		if err != nil {
			t.Fatalf("Prospect: %v", err)
		}
		if rep.Skipped["deps"] != 1 {
			t.Errorf("Skipped[deps] = %d, want 1", rep.Skipped["deps"])
		}
		if rep.Drafted != 0 {
			t.Errorf("Drafted = %d, want 0", rep.Drafted)
		}
		if len(rep.OffAvoided) != 0 {
			t.Errorf("OffAvoided = %v, want empty", rep.OffAvoided)
		}
		if len(rep.ModelHard) != 0 {
			t.Errorf("ModelHard = %v, want empty", rep.ModelHard)
		}
	})
}
