// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRecordExperienceRedactPIIClearsIncidentalPIIFlags(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Incidental PII in a novel guarding test",
			"summary":          "A novel record contains incidental PII in its guard.",
			"error_signatures": []string{"redact-pii-enabled-unique-signature-0095"},
			"root_cause":       "diagnostic details were copied into the guard",
			"fix":              "redact incidental PII before recording",
			"guarding_test":    "connect to 10.1.2.3 and notify a@b.com",
			"body":             "The author can remove incidental PII while preserving the engineering lesson.",
			"author":           "test",
			"redact_pii":       true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("record_experience error: %s", toolText(res))
	}
	var result server.RecordResult
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Novelty != "similar" && result.Novelty != "novel" {
		t.Fatalf("novelty = %q, want similar or novel", result.Novelty)
	}
	if result.RecordID == "" {
		t.Fatal("record_id is empty")
	}
	for _, absent := range []string{"pii:private-ip", "pii:email", "10.1.2.3", "a@b.com"} {
		if strings.Contains(result.Markdown, absent) {
			t.Errorf("markdown contains %q after redaction", absent)
		}
	}
	for _, present := range []string{"<REDACTED-IP>", "<REDACTED-EMAIL>"} {
		if !strings.Contains(result.Markdown, present) {
			t.Errorf("markdown does not contain %q after redaction", present)
		}
	}
	for _, want := range []string{"pii:email", "pii:private-ip"} {
		if !containsString(result.Redacted, want) {
			t.Errorf("redacted = %v, want %q", result.Redacted, want)
		}
	}
}

func TestRecordExperienceDefaultPreservesIncidentalPII(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Incidental PII remains by default",
			"summary":          "A novel record preserves incidental PII without opt-in.",
			"error_signatures": []string{"redact-pii-default-unique-signature-0095"},
			"root_cause":       "diagnostic details were copied into the guard",
			"fix":              "explicitly request redaction when appropriate",
			"guarding_test":    "connect to 10.1.2.3 and notify a@b.com",
			"body":             "The default recording behavior remains unchanged for compatibility.",
			"author":           "test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("record_experience error: %s", toolText(res))
	}
	var result server.RecordResult
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !strings.Contains(result.Markdown, "pii:private-ip") {
		t.Error("markdown does not contain pii:private-ip")
	}
	if !strings.Contains(result.Markdown, "10.1.2.3") {
		t.Error("markdown does not contain the raw private IP")
	}
	if len(result.Redacted) != 0 {
		t.Errorf("redacted = %v, want empty", result.Redacted)
	}
}

func TestRecordExperienceRedactPIIDoesNotLaunderSecret(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	secret := "AKIA" + strings.Repeat("A", 16)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Secret remains flagged with PII redaction",
			"summary":          "A novel record contains a secret-shaped value.",
			"error_signatures": []string{"redact-pii-secret-unique-signature-0095"},
			"root_cause":       "a credential was copied into the narrative",
			"fix":              "rotate and remove the credential",
			"body":             "The leaked credential is " + secret + " and must remain visible to the safety gate.",
			"author":           "test",
			"redact_pii":       true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("record_experience error: %s", toolText(res))
	}
	var result server.RecordResult
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !strings.Contains(result.Markdown, "secret:aws-access-key") {
		t.Error("markdown does not contain secret:aws-access-key")
	}
	if !strings.Contains(result.Markdown, secret) {
		t.Error("secret was changed by PII redaction")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
