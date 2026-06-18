// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeRemoteAccess] = newRemoteAccessFolder
}

// RemoteStager is implemented by the model to support staging files for
// RemoteAccess folder uploads. The API layer can type-assert to this
// interface to trigger remote file staging.
type RemoteStager interface {
	StageRemoteFile(folderID, srcPath, dstName string) error
}

// remoteAccessFolder allows browsing and uploading to a remote folder
// without syncing all files locally. It receives the remote index (so file
// listings are available in the database) but never pulls files.
//
// Uploads are handled by staging files in the folder path, scanning them so
// Syncthing's existing send machinery picks them up, then cleaning up after
// transfer completes.
type remoteAccessFolder struct {
	*folder
}

func newRemoteAccessFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, _ versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	if err := os.MkdirAll(cfg.Path, 0o700); err != nil {
		slog.Warn("Failed to create staging directory", "path", cfg.Path, "error", err)
	}

	f := &remoteAccessFolder{
		folder: newFolder(model, ignores, cfg, evLogger, ioLimiter, nil),
	}
	f.puller = f
	return f
}

// PullErrors returns nil — we don't pull files, so there are no pull errors.
func (*remoteAccessFolder) PullErrors() []FileError {
	return nil
}

// pull is a no-op. The remote index is already received and stored in the
// database via index exchange, making file listings available for browsing.
// We intentionally do not download any file data.
func (f *remoteAccessFolder) pull(ctx context.Context) (bool, error) {
	return true, nil
}

// Override is not applicable for RemoteAccess folders.
func (f *remoteAccessFolder) Override() {
	// no-op
}

// StageFile copies a local file into the folder's staging area and triggers a
// scan so Syncthing picks it up, hashes it, and sends it to remote peers via
// the existing index exchange and block transfer machinery.
func (f *remoteAccessFolder) StageFile(srcPath, dstName string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer src.Close()

	dstPath := filepath.Join(f.Path, dstName)
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating staging file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("copying file: %w", err)
	}

	// Trigger a scan so Syncthing's scanner picks up the new file,
	// hashes it, and sends it to remote peers.
	f.ScheduleScan()

	return nil
}

// StageData writes data into the folder's staging area under the given name
// and triggers a scan.
func (f *remoteAccessFolder) StageData(name string, reader io.Reader) error {
	dstPath := filepath.Join(f.Path, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating staging file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, reader); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("writing staging file: %w", err)
	}

	f.ScheduleScan()
	return nil
}

// CleanupStaging removes staged files that have been successfully synced to
// all remote peers known to this folder.
func (f *remoteAccessFolder) CleanupStaging() error {
	folderFS := f.Filesystem()
	names, err := folderFS.DirNames(".")
	if err != nil {
		return fmt.Errorf("reading staging dir: %w", err)
	}

	for _, name := range names {
		// Skip directories
		fi, err := folderFS.Lstat(name)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			continue
		}

		// Check if this file has been synced globally
		gf, ok, err := f.db.GetGlobalFile(f.folderID, name)
		if err != nil {
			f.sl.Warn("CleanupStaging: error checking global file", "name", name, "error", err)
			continue
		}
		if !ok {
			// Not in global yet — might still be sending
			continue
		}

		// Check if our local version matches the global version
		lf, ok, err := f.db.GetDeviceFile(f.folderID, protocol.LocalDeviceID, name)
		if err != nil || !ok {
			continue
		}

		if lf.Version.GreaterEqual(gf.Version) {
			if err := folderFS.Remove(name); err != nil {
				f.sl.Warn("CleanupStaging: error removing file", "name", name, "error", err)
			}
		}
	}

	return nil
}
