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

// TestConfirmHelpful_AlphaTenantQuotaBoundary is ADR-0031's confirm_helpful
// invariant (#0136): 50 calls per UTC day per token, the 51st rejected —
// driven through a real boundary via countingContributionQuota (tenant_usage_test.go),
// independent of any live index quota store.
func TestConfirmHelpful_AlphaTenantQuotaBoundary(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &countingContributionQuota{}
	ctx := withTenant(context.Background(), "tok_alpha0001")

	for i := 0; i < 50; i++ {
		if _, _, err := h.confirmHelpful(ctx, nil, ConfirmArgs{RecordID: "exp-0200", Author: "tester"}); err != nil {
			t.Fatalf("call %d/50 must be admitted: %v", i+1, err)
		}
	}
	if _, _, err := h.confirmHelpful(ctx, nil, ConfirmArgs{RecordID: "exp-0200", Author: "tester"}); err == nil {
		t.Fatal("the 51st confirm_helpful call today must be rejected (daily quota is 50)")
	} else if !strings.Contains(err.Error(), "confirm_helpful daily contribution quota exceeded") {
		t.Errorf("error = %q, want a confirm_helpful quota-exceeded message", err.Error())
	}
}

// TestConfirmHelpful_OperatorExemptFromQuota guards that the operator tenant
// is never gated by the confirm_helpful contribution quota, even with a
// failing/nil quota store.
func TestConfirmHelpful_OperatorExemptFromQuota(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture()) // h.contribQuota left nil
	ctx := withTenant(context.Background(), "operator")

	if _, _, err := h.confirmHelpful(ctx, nil, ConfirmArgs{RecordID: "exp-0200", Author: "tester"}); err != nil {
		t.Fatalf("operator must be exempt from the contribution quota even with a nil store: %v", err)
	}
	u, err := ix.Usage(context.Background(), "exp-0200")
	if err != nil {
		t.Fatal(err)
	}
	if u.ConfirmedHelpful != 1 {
		t.Fatalf("confirmed_helpful = %d, want 1", u.ConfirmedHelpful)
	}
}

// TestConfirmHelpful_FailsClosedOnUndeclaredTool is checkContributionQuota's
// other fail-closed half (ADR-0031): a limit of 0 (what an undeclared write
// tool would read from alphaContributionQuotas) must reject an alpha call
// outright rather than admitting it as "unlimited".
func TestConfirmHelpful_FailsClosedOnUndeclaredTool(t *testing.T) {
	h := &handlers{logger: quietLogger(), contribQuota: &fakeContributionQuota{}}
	ctx := withTenant(context.Background(), "tok_alpha0001")

	if err := h.checkContributionQuota(ctx, "confirm_helpful", 0); err == nil {
		t.Fatal("a non-positive limit must fail closed, not admit an unlimited alpha call")
	} else if !strings.Contains(err.Error(), "no contribution quota declared for confirm_helpful") {
		t.Errorf("error = %q, want the no-declared-quota message", err.Error())
	}
}
