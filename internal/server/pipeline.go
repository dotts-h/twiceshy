// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"fmt"
	"log/slog"
	"net/http"
)

// stage represents a single step in the HTTP middleware pipeline.
// It is designed to implement the declared middleware pipeline from ADR-0033
// and prevent ordering bugs like the daily-quota misordering (#0131 finding 1).
type stage struct {
	name     string
	requires []string
	after    []string
	provides []string
	wrap     func(http.Handler) http.Handler
}

// buildChain validates and composes the middleware stages in declared order.
// The first stage (stages[0]) is the outermost wrapper and executes first per request.
// It verifies that:
// 1. Every entry in `requires` has been provided by an earlier stage.
// 2. Every entry in `after` is either absent from the slice or refers to a stage declared earlier.
// If any violation is found, it returns a descriptive error naming both stages.
// Cited from ADR-0033 and #0131 finding 1.
func buildChain(stages []stage, final http.Handler) (http.Handler, error) {
	stageIndices := make(map[string]int)
	for i, stg := range stages {
		stageIndices[stg.name] = i
	}

	provided := make(map[string]bool)

	for i, stg := range stages {
		for _, req := range stg.requires {
			if !provided[req] {
				// Find if a later stage provides it
				var providerStage string
				for j, other := range stages {
					if j > i {
						for _, p := range other.provides {
							if p == req {
								providerStage = other.name
								break
							}
						}
					}
					if providerStage != "" {
						break
					}
				}
				if providerStage != "" {
					return nil, fmt.Errorf("stage %q requires %q, which is provided by later stage %q", stg.name, req, providerStage)
				}
				return nil, fmt.Errorf("stage %q requires %q, which is not provided by any earlier stage", stg.name, req)
			}
		}

		for _, aft := range stg.after {
			if otherIdx, exists := stageIndices[aft]; exists {
				if otherIdx >= i {
					return nil, fmt.Errorf("stage %q must run after stage %q, but %q is declared later or at the same position", stg.name, aft, aft)
				}
			}
		}

		for _, prov := range stg.provides {
			provided[prov] = true
		}
	}

	chain := final
	for i := len(stages) - 1; i >= 0; i-- {
		chain = stages[i].wrap(chain)
	}
	return chain, nil
}

// authedStages returns the canonical middleware stages for the authenticated endpoint pipeline.
// this is THE canonical chain declaration; the chain-contract test composes it directly, so a change here is exercised by TestChainContract.
func authedStages(logger *slog.Logger, limiter *tokenBucket, operatorToken string, store TokenStore) []stage {
	requestLog := func(next http.Handler) http.Handler {
		return withRequestLog(logger, next)
	}
	auth := func(next http.Handler) http.Handler {
		return tenantAuth(logger, operatorToken, store, next)
	}
	rateLimit := func(next http.Handler) http.Handler {
		return withRateLimit(logger, limiter, next)
	}
	quota := func(next http.Handler) http.Handler {
		return withDailyQuota(logger, store, next)
	}
	timeout := func(next http.Handler) http.Handler {
		return withTimeout(requestTimeout, next)
	}
	maxBytes := func(next http.Handler) http.Handler {
		return withMaxBytes(maxRequestBytes, next)
	}

	return []stage{
		{name: "request-log", provides: []string{"state"}, wrap: requestLog},
		{name: "tenant-auth", requires: []string{"state"}, provides: []string{"tenant"}, wrap: auth},
		{name: "global-rate-limit", after: []string{"tenant-auth"}, provides: []string{"global-rate-limit"}, wrap: rateLimit},
		{name: "daily-quota", requires: []string{"tenant"}, after: []string{"global-rate-limit"}, wrap: quota},
		{name: "timeout", wrap: timeout},
		{name: "max-bytes", wrap: maxBytes},
	}
}

// signupStages returns the canonical middleware stages for the signup endpoint pipeline.
// this is THE canonical chain declaration; the chain-contract test composes it directly, so a change here is exercised by TestChainContract.
func signupStages(logger *slog.Logger, limiter *tokenBucket) []stage {
	rateLimit := func(next http.Handler) http.Handler {
		return withRateLimit(logger, limiter, next)
	}
	timeout := func(next http.Handler) http.Handler {
		return withTimeout(requestTimeout, next)
	}
	maxBytes := func(next http.Handler) http.Handler {
		return withMaxBytes(maxRequestBytes, next)
	}

	return []stage{
		{name: "global-rate-limit", provides: []string{"global-rate-limit"}, wrap: rateLimit},
		{name: "timeout", wrap: timeout},
		{name: "max-bytes", wrap: maxBytes},
	}
}
