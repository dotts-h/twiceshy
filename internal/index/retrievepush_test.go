// SPDX-License-Identifier: AGPL-3.0-only

// TestRetrievePushPrecisionRecall, TestRetrievePushTraced, and
// TestRetrievePushExcludesQuarantined have been moved to
// retrievepush_livecorpus_test.go (build tag: livecorpus) because the push
// gate's BM25 floor (pushFloor=3.0) is corpus-scale-dependent (ADR-0021 phase 1).

package index_test
