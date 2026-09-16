package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

const deadlockTail = "; refusing to mount to avoid deadlocking the FUSE server"

// CheckMountpointOverlap returns an error if the local repository at repoPath
// and the mountpoint overlap: equal paths, mountpoint nested inside the repo,
// or the repo nested inside the mountpoint. Any overlap deadlocks the FUSE server (GH #5234).
func CheckMountpointOverlap(repoPath, mountpoint string) error {
	rp, err := resolvePath(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}
	mp, err := resolvePath(mountpoint)
	if err != nil {
		return fmt.Errorf("resolving mountpoint path: %w", err)
	}

	// On Windows, paths are case-insensitive, so compare case-insensitively.
	rpLower := strings.ToLower(rp)
	mpLower := strings.ToLower(mp)

	switch {
	case rpLower == mpLower:
		return errors.Fatal(fmt.Sprintf("mountpoint %s is the local repository directory%s", mountpoint, deadlockTail))
	case fs.HasPathPrefix(rpLower, mpLower):
		return errors.Fatal(fmt.Sprintf("mountpoint %s is inside the local repository directory %s%s", mountpoint, repoPath, deadlockTail))
	case fs.HasPathPrefix(mpLower, rpLower):
		return errors.Fatal(fmt.Sprintf("local repository directory %s is inside the mountpoint %s%s", repoPath, mountpoint, deadlockTail))
	}
	return nil
}

// resolvePath returns p as an absolute, symlink-resolved path. If EvalSymlinks
// fails (e.g. the path does not fully exist, such as a new drive X:), it falls back
// to the absolute form: overlap detection is best-effort and we'd rather refuse a clear
// overlap than abort on an unrelated stat error.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("getting absolute path of %q: %w", p, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil
	}
	return resolved, nil
}

// OpenWithReadLock opens the restic repository and acquires a non-exclusive shared lock,
// unless noLock is true. It returns the repository and an unlock cleanup function.
func OpenWithReadLock(ctx context.Context, gopts global.Options, noLock bool, printer restic.Printer) (context.Context, *repository.Repository, func(), error) {
	repo, err := global.OpenRepository(ctx, gopts, printer)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening repository: %w", err)
	}

	unlock := func() {}
	if !noLock {
		var lockErr error
		unlock, ctx, lockErr = repository.LockRepo(ctx, repo, false, gopts.RetryLock, func(msg string) {
			if !gopts.JSON {
				printer.P("%s", msg)
			}
		}, printer.E)
		if lockErr != nil {
			return nil, nil, nil, fmt.Errorf("locking repository: %w", lockErr)
		}
	}

	return ctx, repo, unlock, nil
}
