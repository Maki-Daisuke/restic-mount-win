---
type: runbook
title: Development and Build Guide
description: Step-by-step developer guide for building, testing, and managing restic-mount for Windows using Taskfile and Go submodule workflows.
tags:
  - restic
  - windows
  - build
  - taskfile
  - testing
  - runbook
timestamp: 2026-09-16T18:48:00+09:00
---

# Development and Build Guide

This runbook describes how to set up the development environment, build `restic-mount.exe`, run automated tests, and synchronize submodule dependencies.

---

## 1. Prerequisites

Before developing or building `restic-mount`, ensure the following tools are installed and available in your `PATH`:

- **Go**: Version 1.25 or higher
- **Task**: The [Task runner](https://taskfile.dev/) (`go-task` or `task`)
  ```powershell
  winget install Task.Task
  # or via Go
  go install github.com/go-task/task/v3/cmd/task@latest
  ```
- **Git**: For cloning the repository and managing submodules.
- **WinFsp**: Required at **runtime** for mounting virtual filesystems (not needed at build time).
  ```powershell
  winget install WinFsp.WinFsp
  ```

---

## 2. Initial Setup

Clone the repository with submodules:

```powershell
git clone --recurse-submodules https://github.com/Maki-Daisuke/restic-mount-win.git
cd restic-mount-win
```

If already cloned without submodules:

```powershell
git submodule update --init --recursive
```

---

## 3. Taskfile Workflows

All common developer tasks are orchestrated via [Taskfile.yml](../Taskfile.yml).

### Available Tasks

| Command | Description |
| :--- | :--- |
| `task` or `task build` | Builds `bin/restic-mount.exe` with `CGO_ENABLED=0`. Automatically sets up directory junction and validates module sync. |
| `task test` | Runs all unit tests under `cmd/restic-mount/` using `-modfile=../restic-mount.mod`. |
| `task clean` | Deletes the `bin/` directory and generated executables. |
| `task sync-mod` | Synchronizes `restic-mount.mod` and `restic-mount.sum` from `restic/go.mod` and adds the `cgofuse` dependency. |

---

## 4. Under the Hood: Build Architecture

Building `restic-mount` leverages several automated steps orchestrated by `Taskfile.yml`:

### Directory Junction (`junction` task)
Go enforces that packages inside `restic/internal/...` can only be imported by packages within the same subtree. To respect this rule while keeping `cmd/restic-mount` tracked in this repository, Task creates an NTFS directory junction:

```powershell
New-Item -ItemType Junction -Path 'restic/cmd/restic-mount' -Target '..\cmd\restic-mount'
```

### Module Synchronization Check (`check-sync` task)
Ensures that `restic-mount.mod` remains identical to `restic/go.mod` (except for the addition of `github.com/winfsp/cgofuse`). If the upstream submodule was bumped or modified without updating `restic-mount.mod`, the build will halt and instruct you to run `task sync-mod`.

### Compilation (`build` task)
Builds the binary without requiring a C compiler:

```powershell
$env:CGO_ENABLED="0"
go -C restic build -modfile=../restic-mount.mod -o ../bin/restic-mount.exe ./cmd/restic-mount
```

- `-C restic`: Executes the Go toolchain within the `restic/` submodule directory context.
- `-modfile=../restic-mount.mod`: Instructs Go to use the root `restic-mount.mod` rather than `restic/go.mod`, keeping the submodule pristine.
- `-o ../bin/restic-mount.exe`: Places the compiled binary into the workspace's `bin/` folder.

---

## 5. Running Tests

Unit tests are written using standard Go testing conventions and table-driven tests:

```powershell
task test
```

Direct command equivalent:

```powershell
go -C restic test -modfile=../restic-mount.mod -v ./cmd/restic-mount
```

### Key Test Coverage
- **Path Sanitization & Name Projection**: Verifies that reserved Windows names (`CON`, `PRN`, `AUX`, `NUL`, `COM1-9`, `LPT1-9`), forbidden characters (`<`, `>`, `:`, `"`, `/`, `\`, `|`, `?`, `*`), control characters, and trailing dots/spaces are correctly refused with `ERROR_INVALID_NAME` (`ENOENT`).
- **Unicode Case-Insensitive Matching**: Verifies that case-folding collisions in directories are handled and warnings are logged.
- **Path Separation & Directory Structure**: Verifies that virtual top-level hierarchies (`snapshots/`, `hosts/`, `tags/`, `ids/`, `sub/`) correctly navigate without returning `ENOTDIR`.

---

## 6. Updating Upstream Restic Submodule

When updating to a newer restic release or commit:

1. Update the git submodule:
   ```powershell
   git -C restic checkout <tag-or-commit>
   ```
2. Resynchronize `restic-mount.mod`:
   ```powershell
   task sync-mod
   ```
3. Run tests and build:
   ```powershell
   task test
   task build
   ```
4. Commit the updated submodule reference and `restic-mount.mod`/`restic-mount.sum`.
