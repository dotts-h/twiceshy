// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"strings"
	"testing"
)

func TestConfirmHelpful_IncrementsCounter(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200

	_, res, err := h.confirmHelpful(context.Background(), nil, ConfirmArgs{
		RecordID: "exp-0200", Author: "tester",
	})
	if err != nil {
		t.Fatalf("confirmHelpful: %v", err)
	}
	if !strings.Contains(res.Message, "exp-0200") {
		t.Fatalf("success message must mention the record id: %q", res.Message)
	}

	u, err := ix.Usage(context.Background(), "exp-0200")
	if err != nil {
		t.Fatal(err)
	}
	if u.ConfirmedHelpful != 1 {
		t.Fatalf("confirmed_helpful = %d, want 1", u.ConfirmedHelpful)
	}

	if _, _, err := h.confirmHelpful(context.Background(), nil, ConfirmArgs{
		RecordID: "exp-0200", Author: "tester",
	}); err != nil {
		t.Fatalf("second confirmHelpful: %v", err)
	}
	u, err = ix.Usage(context.Background(), "exp-0200")
	if err != nil {
		t.Fatal(err)
	}
	if u.ConfirmedHelpful != 2 {
		t.Fatalf("confirmed_helpful after second call = %d, want 2", u.ConfirmedHelpful)
	}
}

func TestConfirmHelpful_RejectsBadAndMissing(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())

	if _, _, err := h.confirmHelpful(context.Background(), nil, ConfirmArgs{
		RecordID: "nope", Author: "tester",
	}); err == nil {
		t.Fatal("a malformed record_id must be rejected before any work")
	}
	u, _ := ix.Usage(context.Background(), "exp-0200")
	if u.ConfirmedHelpful != 0 {
		t.Fatalf("a rejected call must not increment; confirmed_helpful = %d", u.ConfirmedHelpful)
	}

	if _, _, err := h.confirmHelpful(context.Background(), nil, ConfirmArgs{
		RecordID: "exp-9999", Author: "tester",
	}); err == nil {
		t.Fatal("a confirm against a non-existent record must be rejected")
	}
	u, _ = ix.Usage(context.Background(), "exp-0200")
	if u.ConfirmedHelpful != 0 {
		t.Fatalf("a rejected call must not increment; confirmed_helpful = %d", u.ConfirmedHelpful)
	}
}
