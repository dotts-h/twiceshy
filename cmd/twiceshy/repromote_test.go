// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRepromote_RequiresID(t *testing.T) {
	var buf bytes.Buffer
	err := runRepromote(context.Background(), []string{"-corpus", corpus}, &buf,
		func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "-id") {
		t.Fatalf("repromote without -id must fail; got %v", err)
	}
}

func TestRunRepromote_RejectsInvalidID(t *testing.T) {
	var buf bytes.Buffer
	err := runRepromote(context.Background(), []string{"-corpus", corpus, "-id", "bad-id"}, &buf,
		func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "exp-NNNN") {
		t.Fatalf("invalid record id must be rejected; got %v", err)
	}
}

func TestRunRepromote_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runRepromote(context.Background(), []string{"-corpus", corpus, "-id", "exp-0001"}, &buf,
		func(string) string { return "" }) // no TWICESHY_JUDGE_URL
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("re-promotion without a judge must fail safe; got %v", err)
	}
}

func TestRunRepromote_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runRepromote(context.Background(), []string{"-corpus", corpus, "-id", "exp-0001", "-dry-run"}, &buf,
		func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}
