// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// rejectSymlinkComponents rejects a symlink at the leaf or in any existing
// parent. allowMissing is for a destination that has not been created yet; a
// missing component proves every component below it is missing too.
func rejectSymlinkComponents(path string, allowMissing bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	var components []string
	for current := filepath.Clean(abs); ; current = filepath.Dir(current) {
		components = append(components, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for i := len(components) - 1; i >= 0; i-- {
		info, err := os.Lstat(components[i])
		if err != nil {
			if allowMissing && os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("inspecting path component: %w", err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return errorsPathSymlink
		}
	}
	return nil
}

var errorsPathSymlink = errors.New("refusing path containing a symbolic link")

func requireEmptyOutput(path string) error {
	if err := rejectSymlinkComponents(path, true); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("pack output exists and is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("pack output directory must be new or empty")
	}
	return nil
}

// writePackFile creates one artifact without following links or replacing a
// pre-existing file. Combined with an initially empty output root, this prevents
// stale overlays and leaf-symlink writes.
func writePackFile(root, rel string, body []byte) error {
	dst, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	parent := filepath.Dir(dst)
	if err := rejectSymlinkComponents(parent, true); err != nil {
		return err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = rootFS.Close() }()
	cleanRel := filepath.FromSlash(rel)
	if err := rootFS.MkdirAll(filepath.Dir(cleanRel), 0o755); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(parent, false); err != nil {
		return err
	}
	f, err := rootFS.OpenFile(cleanRel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
