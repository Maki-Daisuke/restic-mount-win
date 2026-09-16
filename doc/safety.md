---
type: specification
title: Mount Safety and Repository Locking
description: Safety mechanisms in restic-mount, including mount point overlap detection to prevent kernel deadlocks and repository lock lifecycle management.
tags:
  - specification
  - safety
  - locking
  - deadlock-prevention
timestamp: 2026-09-16T18:47:00+09:00
---

# Mount Safety and Repository Locking

This document details the safety guarantees and locking behavior implemented by `restic-mount for Windows`, mirroring upstream restic semantics.

---

## 1. Mount Point / Repository Overlap Rejection

### Problem Statement
When using a local filesystem repository (e.g. `D:\Backup\restic-repo`), mounting over the repository directory—or mounting to any folder nested inside the repository—causes an immediate kernel deadlock. The FUSE filesystem driver would attempt to read its own storage backend through the virtual mount point it just created, creating a cyclic dependency in the Windows I/O manager.

### Implementation
`restic-mount` executes `CheckMountpointOverlap` before initializing the FUSE driver:
- **Symlink Resolution**: Both the repository path and the mountpoint path are normalized with `filepath.EvalSymlinks`.
- **Relationship Detection**: The tool rejects the mount if:
  1. The mountpoint and repository directory are identical.
  2. The mountpoint is nested inside the local repository.
  3. The local repository is nested inside the mountpoint.
- **Error Messages**: Exact parity with upstream restic error reporting:
  - `mountpoint <mp> is the local repository directory; refusing to mount to avoid deadlocking the FUSE server`
  - `mountpoint <mp> is inside the local repository directory <rp>; refusing to mount to avoid deadlocking the FUSE server`
  - `local repository directory <rp> is inside the mountpoint <mp>; refusing to mount to avoid deadlocking the FUSE server`
- **Scope**: Applied exclusively to local repositories; remote backends (S3, REST, SFTP) are not subject to local overlap.

---

## 2. Shared Repository Locking

### Upstream Parity
To guarantee data consistency while preserving concurrent accessibility, `restic-mount` acquires a **non-exclusive (shared) read lock** upon repository initialization:

```go
lock, err := repository.LockRepo(ctx, repo, false, "mount")
```

### Lifecycle and Behavior
- **Concurrency**:
  - Multiple concurrent mounts of the same repository coexist peacefully.
  - Read-only commands (`restic ls`, `restic cat`, `restic find`) run seamlessly in parallel with active mounts.
- **Mutual Exclusion**:
  - Operations requiring an **exclusive lock** (e.g., `restic forget`, `restic prune`, `restic check --read-data`) are blocked while a mount is active.
- **Automatic Release**:
  - The lock is released cleanly when the process terminates via `Ctrl + C` or standard unmount signals.
- **`--no-lock` Flag**:
  - Matches upstream `--no-lock` flag to skip lock acquisition entirely when necessary (e.g., inspecting an immutable or read-only storage backend).
