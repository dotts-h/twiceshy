// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"strings"
	"time"
)

// Reaper removes orphaned sandbox containers and volumes — the backstop that
// guarantees "no leaked containers, ever" (#0018) even if the broker process
// crashed mid-run. Every container and volume the broker creates carries the
// `twiceshy.repro` label, so a single labelled sweep finds them all.
type Reaper struct {
	runner commandRunner
}

// NewReaper returns a Reaper driving the real docker CLI.
func NewReaper() *Reaper { return &Reaper{runner: dockerRunner{}} }

// Reap force-removes every twiceshy.repro container (running or stopped) and
// then every twiceshy.repro volume. It is idempotent and safe to run on a
// schedule or at broker start-up. It returns the count removed.
func (r *Reaper) Reap(ctx context.Context) (containers, volumes int, err error) {
	const op = 30 * time.Second

	cres, cerr := r.runner.run(ctx, nil, op, "docker", "ps", "-aq", "--filter", "label="+labelKey)
	if cerr != nil {
		return 0, 0, cerr
	}
	for _, id := range strings.Fields(cres.stdout) {
		if _, e := r.runner.run(ctx, nil, op, "docker", "rm", "-f", id); e == nil {
			containers++
		}
	}

	// Volumes can only be removed after their containers are gone (above).
	vres, verr := r.runner.run(ctx, nil, op, "docker", "volume", "ls", "-q", "--filter", "label="+labelKey)
	if verr != nil {
		return containers, 0, verr
	}
	for _, name := range strings.Fields(vres.stdout) {
		if _, e := r.runner.run(ctx, nil, op, "docker", "volume", "rm", "-f", name); e == nil {
			volumes++
		}
	}
	return containers, volumes, nil
}
