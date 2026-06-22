// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/dotts-h/twiceshy/internal/screen"
)

// runScreen reads text on stdin and runs the ingestion content screen over it,
// printing any "category:rule" flags and exiting non-zero when a SECRET is present
// (harmful-code / pii flag but do not block — they are expected in a coding
// transcript, mirroring the /retro endpoint policy). It exposes the same tested
// screen (internal/screen) the importer and the /retro endpoint use, so the
// SessionEnd hook can screen a transcript client-side — before anything leaves the
// machine — without a bash re-implementation that would drift from the rules (#0065).
func runScreen(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("screen", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("screen: reading input: %w", err)
	}
	findings := screen.Scan(string(data))
	for _, fl := range screen.Flags(findings) {
		_, _ = fmt.Fprintln(out, fl)
	}
	if screen.HasSecret(findings) {
		return errors.New("screen: a secret was detected")
	}
	return nil
}
