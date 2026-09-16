---
type: architecture
title: Architecture and Design Rationale
description: Core architectural motivations, design constraints, rationale for standalone implementation, and packaging strategies for restic-mount on Windows.
tags:
  - architecture
  - rationale
  - go
  - restic
  - winfsp
  - cgofuse
  - modules
timestamp: 2026-09-16T18:47:00+09:00
---

# Architecture and Design Rationale

This document details the core architectural motivations, trade-offs, and technical design rationale behind `restic-mount for Windows`.

---

## 1. Why doesn't the official `restic mount` work on Windows?

The upstream restic mount implementation relies on Linux/macOS FUSE (Filesystem in Userspace) via `anacrolix/fuse`.
On Linux, interacting with the `/dev/fuse` character device to exchange byte streams allows mounting using standard system calls without requiring CGO (an external C compiler).

Windows, on the other hand, lacks a native FUSE kernel interface. To mount virtual filesystems on Windows, a kernel driver is necessary. This project uses **WinFsp** (Windows File System Proxy) as the kernel driver and the Windows no-CGO implementation of **cgofuse** to load the WinFsp DLL dynamically at runtime.

This design preserves restic's core build model:

- **Pure Go build**: Cross-compilation remains straightforward with `CGO_ENABLED=0`.
- **No C compiler or header requirements**: Developers and CI systems do not need MinGW, MSVC, or WinFsp development headers to build the binary.
- **Runtime-only dependency**: The end user only needs the standard WinFsp runtime installed.

## 2. Why a standalone tool instead of merging into upstream?

A core design principle of restic is **"a single static binary that anyone can cross-compile anywhere with `CGO_ENABLED=0`"**—an exceptionally sound and pragmatic engineering decision.

Although cgofuse can provide Windows mounting without CGO, adding it directly to upstream restic would introduce:

- A Windows-only filesystem implementation.
- An external WinFsp runtime driver dependency.
- Windows-specific path semantics and error-handling branches.
- Additional release, integration testing, and maintenance overhead for upstream maintainers.

Therefore, this project keeps the Windows-specific mounting functionality in a dedicated external command (`restic-mount.exe`) while preserving restic's no-CGO philosophy.

## 3. Why full CLI and environment compatibility with upstream?

While `restic-mount` is a standalone executable, its command-line interface and option flags intentionally mirror the upstream `restic mount` command 1:1:

- **Zero learning curve**: Users already familiar with restic do not need to learn custom flags or argument conventions. Replaying an existing mount command simply involves running `restic-mount` instead of `restic mount`.
- **Drop-in script and environment reusability**: All standard restic environment variables (`RESTIC_REPOSITORY`, `RESTIC_PASSWORD`, `RESTIC_PASSWORD_FILE`, `RESTIC_KEY_HINT`, etc.) and cloud provider credentials (`AWS_ACCESS_KEY_ID`, `AZURE_ACCOUNT_NAME`, `GOOGLE_APPLICATION_CREDENTIALS`, etc.) work natively without modification. Existing backup automation, credential wrappers, and shell scripts work out of the box.
- **Identical operational semantics**: Flags controlling concurrency and safety (such as `--no-lock` and `--retry-lock`) behave with the exact same locking guarantees as upstream restic, preventing operational surprises in multi-user or automated environments.

## 4. Module structure and bypassing Go's `internal` restriction

A core architectural philosophy of this project is to **reuse upstream restic code as much as possible to maximize compatibility**. Beyond merely avoiding duplicate implementation, executing restic's own battle-tested cryptographic primitives, repository locking, index parsing, cache management, and snapshot tree traversal directly guarantees 100% behavioral consistency, complete data integrity, and effortless alignment with upstream restic releases.

To accomplish this, this project includes the upstream restic repository directly as a Git submodule.

However, the Go compiler strictly enforces that packages located under an `internal/` directory can only be imported by code rooted at the parent of that `internal/` directory within the same module.

This project maintains its own source code cleanly in its own project tree while creating a **directory junction** under the submodule's `cmd/` directory at build time:

```text
my-restic-mount/
├── Taskfile.yml
├── restic-mount.mod            # restic/go.mod + cgofuse require (used via -modfile)
├── restic-mount.sum            # Checksums for the above (generated)
├── bin/
│   └── restic-mount.exe        # Generated binary
├── cmd/
│   └── restic-mount/           # Project source code
│       ├── main.go
│       └── fs.go
└── restic/                     # git submodule (github.com/restic/restic)
    └── cmd/
        └── restic-mount  ======> (Junction pointing to ../../cmd/restic-mount)
```

Because Go resolves `internal/` packages based on filesystem hierarchy relative to the nearest `go.mod`, placing the junction under `restic/cmd/` allows our code to import restic's `internal/data`, `internal/restic`, `internal/repository`, etc., without modifying any upstream source files.

## 5. Declaring the cgofuse dependency without touching upstream

Because the junction makes `cmd/restic-mount` part of the restic module, any imports of third-party packages (specifically `github.com/winfsp/cgofuse`) must be declared in restic's module dependency graph.

We do not want to modify the submodule's `go.mod` (it must stay pristine for clean upstream submodule updates). To achieve this, we leverage Go's `-modfile` flag (introduced in Go 1.14+):

- **`restic-mount.mod`**: A replica of `restic/go.mod` with an added `require` directive for `github.com/winfsp/cgofuse`.
- **`restic-mount.sum`**: Holds the cryptographic checksums for cgofuse and its transitive dependencies.
- **Build execution**: Builds are executed with `go -C restic build "-modfile=../restic-mount.mod" -o ../bin/restic-mount.exe ./cmd/restic-mount`. Module resolution uses our alternate file while the module root and import paths remain restic's.
- **Drift verification**: `Taskfile.yml` includes an automatic `check-sync` task that verifies `restic-mount.mod` has not drifted from `restic/go.mod` beyond the expected cgofuse dependency.
