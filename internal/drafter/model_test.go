// SPDX-License-Identifier: AGPL-3.0-only

package drafter_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// ollamaStub serves a canned Ollama /api/chat response whose message content is
// the given draft text (what the model would emit). A non-200 status lets a test
// exercise the transport-failure path.
func ollamaStub(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"role": "assistant", "content": content},
			"done":    true,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestModelDrafter_DraftsFromModelOutput(t *testing.T) {
	root := t.TempDir()
	draftJSON := `{"check":"SA1019",` +
		`"trap":"package main\nimport \"os\"\nfunc main(){ _ = os.SEEK_SET }\n",` +
		`"fix":"package main\nimport \"io\"\nfunc main(){ _ = io.SeekStart }\n"}`
	srv := ollamaStub(t, draftJSON, 200)
	d := drafter.NewModelDrafter(srv.URL, "qwen2.5-coder:14b")

	rec := goDeprecationRecord("exp-7000", "os", "SA1019: os.SEEK_SET is deprecated: Use io.SeekStart.")
	dir, err := d.Draft(context.Background(), root, rec)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	abs := filepath.Join(root, filepath.FromSlash(dir))
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}
	// The model's trap/fix are written verbatim.
	if !strings.Contains(read("trap/main.go"), "os.SEEK_SET") {
		t.Errorf("trap not written from model output:\n%s", read("trap/main.go"))
	}
	if !strings.Contains(read("fix/main.go"), "io.SeekStart") {
		t.Errorf("fix not written from model output:\n%s", read("fix/main.go"))
	}
	// The proven scaffolding is REUSED (model never writes the fragile scripts):
	// repro keys on the model-supplied check; scripts pin /work + install staticcheck.
	if !strings.Contains(read("repro.sh"), "SA1019") {
		t.Errorf("repro.sh should key on the model-supplied check:\n%s", read("repro.sh"))
	}
	if !strings.Contains(read("prepare.sh"), "staticcheck") || !strings.Contains(read("repro.sh"), "/work") {
		t.Errorf("model drafter must reuse the proven script scaffolding")
	}
}

// TestModelDrafter_RequestContract pins the Ollama request the drafter builds:
// POST to /api/chat with Content-Type application/json, the configured model,
// stream:false, format:"json", and — the load-bearing reproducibility guarantee —
// options.temperature == 0. The other model tests use a stub that ignores the
// request, so without this a regression (wrong path/method, or a dropped
// temperature:0 yielding non-deterministic drafts) would go uncaught.
func TestModelDrafter_RequestContract(t *testing.T) {
	root := t.TempDir()
	var gotMethod, gotPath, gotCT string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotCT = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"role": "assistant", "content": `{"check":"SA1019","trap":"package main\nfunc main(){}\n","fix":"package main\nfunc main(){}\n"}`},
			"done":    true,
		})
	}))
	t.Cleanup(srv.Close)
	d := drafter.NewModelDrafter(srv.URL, "qwen2.5-coder:14b")
	rec := goDeprecationRecord("exp-7099", "os", "SA1019: os.SEEK_SET deprecated")
	if _, err := d.Draft(context.Background(), root, rec); err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if body["model"] != "qwen2.5-coder:14b" {
		t.Errorf("model = %v", body["model"])
	}
	if body["stream"] != false {
		t.Errorf("stream = %v, want false", body["stream"])
	}
	if body["format"] != "json" {
		t.Errorf("format = %v, want json", body["format"])
	}
	// reproducibility contract: temperature MUST be 0 (JSON number decodes as float64).
	opts, _ := body["options"].(map[string]any)
	if temp, ok := opts["temperature"].(float64); !ok || temp != 0 {
		t.Errorf("options.temperature = %v, want 0", opts["temperature"])
	}
}

func TestModelDrafter_ThirdPartyRequireWarmed(t *testing.T) {
	root := t.TempDir()
	draftJSON := `{"check":"SA1019",` +
		`"trap":"package main\nimport \"strings\"\nfunc main(){ _ = strings.Title(\"x\") }\n",` +
		`"fix":"package main\nimport \"golang.org/x/text/cases\"\nvar _ = cases.Title\nfunc main(){}\n",` +
		`"fix_requires":[{"path":"golang.org/x/text","version":"v0.21.0"}]}`
	srv := ollamaStub(t, draftJSON, 200)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7001", "strings", "SA1019: strings.Title deprecated")

	dir, err := d.Draft(context.Background(), root, rec)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	abs := filepath.Join(root, filepath.FromSlash(dir))
	gomod, _ := os.ReadFile(filepath.Join(abs, "fix", "go.mod"))
	prep, _ := os.ReadFile(filepath.Join(abs, "prepare.sh"))
	if !strings.Contains(string(gomod), "require golang.org/x/text") {
		t.Errorf("fix/go.mod missing 3rd-party require:\n%s", gomod)
	}
	if !strings.Contains(string(prep), "go mod") {
		t.Errorf("prepare.sh should warm the 3rd-party module:\n%s", prep)
	}
}

// A fix_require for a bare stdlib name (no dot in the path) must be rejected as a
// skip: emitting `require io vX` would make `go mod tidy` fail in prepare and burn a
// broker run. An incomplete require (missing version) is likewise rejected. Both are
// memory-poisoning-adjacent — the model output is untrusted.
func TestModelDrafter_StdlibFixRequireRejected(t *testing.T) {
	cases := []struct {
		name    string
		require string // the fix_requires JSON array element
	}{
		{
			// path "io" has no dot → not a module path.
			name:    "stdlib path without a dot",
			require: `{"path":"io","version":"v1.0.0"}`,
		},
		{
			// version "" → incomplete require (model.go:177-179).
			name:    "incomplete require missing version",
			require: `{"path":"golang.org/x/text","version":""}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			draftJSON := `{"check":"SA1019",` +
				`"trap":"package main\nfunc main(){}\n",` +
				`"fix":"package main\nfunc main(){}\n",` +
				`"fix_requires":[` + tc.require + `]}`
			srv := ollamaStub(t, draftJSON, 200)
			d := drafter.NewModelDrafter(srv.URL, "m")
			rec := goDeprecationRecord("exp-7012", "strings", "SA1019: deprecated")

			if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
				t.Fatalf("a poisoned fix_require must be ErrUnsupported, got %v", err)
			}
			// The validation short-circuits before any write.
			if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
				t.Errorf("nothing should be written for a rejected fix_require; got %d entries", len(entries))
			}
		})
	}
}

func TestModelDrafter_UnparseableIsUnsupported(t *testing.T) {
	root := t.TempDir()
	srv := ollamaStub(t, "I cannot help with that. There is no JSON here.", 200)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7002", "os", "SA1019: deprecated")

	_, err := d.Draft(context.Background(), root, rec)
	if !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("unparseable model output must be ErrUnsupported (skip), got %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
		t.Errorf("nothing should be written for an unparseable draft; got %d entries", len(entries))
	}
}

func TestModelDrafter_MissingFieldsIsUnsupported(t *testing.T) {
	root := t.TempDir()
	srv := ollamaStub(t, `{"check":"SA1019","trap":"package main"}`, 200) // no fix
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7003", "os", "SA1019: deprecated")

	if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("a draft missing trap/fix/check must be ErrUnsupported, got %v", err)
	}
}

// The model-supplied `check` is interpolated into the executed repro.sh. A check
// that is not a bare staticcheck code (e.g. shell metacharacters) must be rejected
// as a skip, never run.
func TestModelDrafter_InjectingCheckIsUnsupported(t *testing.T) {
	root := t.TempDir()
	draftJSON := `{"check":"SA1019; echo OK; exit 0 #",` +
		`"trap":"package main\nfunc main(){}\n",` +
		`"fix":"package main\nfunc main(){}\n"}`
	srv := ollamaStub(t, draftJSON, 200)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7010", "os", "SA1019: deprecated")

	if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("an injecting check must be ErrUnsupported, got %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
		t.Errorf("nothing should be written for a rejected check; got %d entries", len(entries))
	}
}

// A fix_require path/version is interpolated into a generated go.mod. An embedded
// newline could inject a second go.mod directive (e.g. a replace => /local/path),
// so a path/version with whitespace/newline is rejected as a skip.
func TestModelDrafter_FixRequireNewlineIsUnsupported(t *testing.T) {
	root := t.TempDir()
	draftJSON := `{"check":"SA1019",` +
		`"trap":"package main\nfunc main(){}\n",` +
		`"fix":"package main\nfunc main(){}\n",` +
		`"fix_requires":[{"path":"golang.org/x/text\nreplace evil => /tmp","version":"v0.21.0"}]}`
	srv := ollamaStub(t, draftJSON, 200)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7011", "strings", "SA1019: deprecated")

	if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("a fix_require with an embedded newline must be ErrUnsupported, got %v", err)
	}
}

func TestModelDrafter_NonGoRecordSkipsWithoutCallingModel(t *testing.T) {
	root := t.TempDir()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "{}"}})
	}))
	t.Cleanup(srv.Close)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := &record.Record{
		ID: "exp-7004", Status: "quarantined",
		Symptom:   &record.Symptom{ErrorSignatures: []string{"GHSA-xxxx"}},
		AppliesTo: []record.AppliesTo{{Ecosystem: "PyPI", Package: "numpy"}},
	}

	if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("a non-Go record must be ErrUnsupported, got %v", err)
	}
	if called {
		t.Error("the model must NOT be called for an out-of-class record (waste + safety)")
	}
}

func TestModelDrafter_AdvisoryRecordSkippedWithoutCallingModel(t *testing.T) {
	root := t.TempDir()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "{}"}})
	}))
	t.Cleanup(srv.Close)
	d := drafter.NewModelDrafter(srv.URL, "m")
	// A Go-ecosystem GHSA advisory HAS a Go package but is a vuln, not a deprecation;
	// it must be skipped without a model call (#0026 scope boundary, ADR-0016).
	rec := &record.Record{
		ID: "exp-7006", Status: "quarantined",
		Symptom:   &record.Symptom{ErrorSignatures: []string{"GHSA-xxxx-yyyy-zzzz: vulnerability in github.com/foo/bar"}},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "github.com/foo/bar"}},
	}
	if _, err := d.Draft(context.Background(), root, rec); !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("a Go advisory must be ErrUnsupported, got %v", err)
	}
	if called {
		t.Error("the model must NOT be called for an advisory record (waste + scope boundary)")
	}
}

func TestModelDrafter_TransportErrorSkipsNotAborts(t *testing.T) {
	root := t.TempDir()
	srv := ollamaStub(t, "", 500)
	d := drafter.NewModelDrafter(srv.URL, "m")
	rec := goDeprecationRecord("exp-7005", "os", "SA1019: deprecated")

	// The model drafter is an optional fallback (VM 101 is parked by default); if it
	// is unavailable it must DECLINE (ErrUnsupported) so the batch walk continues and
	// the deterministic class still drafts — not abort the whole run.
	_, err := d.Draft(context.Background(), root, rec)
	if !errors.Is(err, drafter.ErrUnsupported) {
		t.Fatalf("a model endpoint error must be a skip (ErrUnsupported), got %v", err)
	}
}

// TestPipeline_FallsBackToModelDrafter proves the chain: the deterministic drafter
// declines an uncataloged package, the model drafter covers it, and the gate
// attaches the result with a model-origin label.
func TestPipeline_FallsBackToModelDrafter(t *testing.T) {
	root := t.TempDir()
	draftJSON := `{"check":"SA1019",` +
		`"trap":"package main\nimport \"os\"\nfunc main(){ _ = os.SEEK_SET }\n",` +
		`"fix":"package main\nimport \"io\"\nfunc main(){ _ = io.SeekStart }\n"}`
	srv := ollamaStub(t, draftJSON, 200)
	b := &fakeBroker{result: passing()}
	rv := repro.NewRevalidator(b, root)
	p := drafter.NewPipeline(rv, root,
		drafter.NewGoDeprecationDrafter(), // declines "os" (uncataloged)
		drafter.NewModelDrafter(srv.URL, "qwen"))
	rec := goDeprecationRecord("exp-7010", "os", "SA1019: os.SEEK_SET is deprecated")

	out, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Attached {
		t.Fatalf("model-drafted repro should attach via the gate; got %+v", out)
	}
	if rec.Guard == nil || len(rec.Guard.Repros) != 1 {
		t.Fatalf("want one attached repro; guard=%+v", rec.Guard)
	}
	if !strings.Contains(rec.Guard.Repros[0].Label, "model-drafter") {
		t.Errorf("label should mark the model origin for audit; got %q", rec.Guard.Repros[0].Label)
	}
}
