// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildChain_Validation(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	nopWrap := func(next http.Handler) http.Handler { return next }

	t.Run("requires violation (declared after provider)", func(t *testing.T) {
		stages := []stage{
			{name: "consumer", requires: []string{"tenant"}, wrap: nopWrap},
			{name: "provider", provides: []string{"tenant"}, wrap: nopWrap},
		}
		_, err := buildChain(stages, dummyHandler)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "consumer") || !strings.Contains(err.Error(), "provider") {
			t.Errorf("expected error naming both stages, got: %v", err)
		}
	})

	t.Run("after violation", func(t *testing.T) {
		stages := []stage{
			{name: "second", after: []string{"first"}, wrap: nopWrap},
			{name: "first", wrap: nopWrap},
		}
		_, err := buildChain(stages, dummyHandler)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "second") || !strings.Contains(err.Error(), "first") {
			t.Errorf("expected error naming both stages, got: %v", err)
		}
	})

	t.Run("after names absent stage (no error)", func(t *testing.T) {
		stages := []stage{
			{name: "first", after: []string{"absent-stage"}, wrap: nopWrap},
		}
		_, err := buildChain(stages, dummyHandler)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("compose order: stages execute in declaration order", func(t *testing.T) {
		var order []string
		wrap := func(name string) func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		stages := []stage{
			{name: "s1", wrap: wrap("s1")},
			{name: "s2", wrap: wrap("s2")},
			{name: "s3", wrap: wrap("s3")},
		}

		chain, err := buildChain(stages, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "final")
		}))
		if err != nil {
			t.Fatalf("unexpected buildChain error: %v", err)
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		chain.ServeHTTP(rec, req)

		expected := []string{"s1", "s2", "s3", "final"}
		if len(order) != len(expected) {
			t.Fatalf("expected order %v, got %v", expected, order)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("at index %d: expected %q, got %q", i, v, order[i])
			}
		}
	})
}
