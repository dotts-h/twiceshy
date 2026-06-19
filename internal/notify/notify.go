// SPDX-License-Identifier: AGPL-3.0-only

// Package notify is the guardrail alert seam (ADR-0013 §B3): it POSTs an alert to
// a configured channel (ntfy, which the brain already runs) when a promote/adapt
// guardrail trips, so an unattended run's halt is visible off the cron box.
// Env-gated — with no URL it is a silent no-op. Alerting must never break the
// loop it watches, so a failed post is logged, never returned.
package notify

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Alerter posts a guardrail alert. Implementations must be safe to call when no
// channel is configured (a no-op) and must never block the loop on a slow or
// dead endpoint beyond a short timeout.
type Alerter interface {
	Alert(ctx context.Context, event, message string)
}

// NopAlerter discards alerts — returned when no channel is configured, and used
// in tests.
type NopAlerter struct{}

// Alert does nothing.
func (NopAlerter) Alert(context.Context, string, string) {}

// HTTPNotifier POSTs alerts to an ntfy topic URL (message body + a Title header
// naming the event). A non-2xx response or transport error is logged at Warn and
// swallowed.
type HTTPNotifier struct {
	url    string
	client *http.Client
	logger *slog.Logger
}

// New returns an Alerter posting to url, or a NopAlerter when url is empty. The
// logger records post failures; a nil logger discards them.
func New(url string, logger *slog.Logger) Alerter {
	if url == "" {
		return NopAlerter{}
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	// 5s: long enough for a LAN ntfy POST, short enough never to stall a nightly run.
	return &HTTPNotifier{url: url, client: &http.Client{Timeout: 5 * time.Second}, logger: logger}
}

// Alert POSTs message to the ntfy topic with the event as the notification
// title. It never returns an error — alerting is best-effort and must not abort
// the guardrail it reports on.
func (n *HTTPNotifier) Alert(ctx context.Context, event, message string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewBufferString(message))
	if err != nil {
		n.logger.Warn("alert post failed", "event", event, "error", err.Error())
		return
	}
	req.Header.Set("Title", "twiceshy: "+event)
	req.Header.Set("Tags", "warning")
	resp, err := n.client.Do(req)
	if err != nil {
		n.logger.Warn("alert post failed", "event", event, "error", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		n.logger.Warn("alert post returned non-2xx", "event", event, "status", resp.StatusCode)
	}
}
