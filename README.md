# restic-mount for Windows

A standalone CLI tool to natively mount [restic](https://github.com/restic/restic) repositories as virtual drives (or folders) on Windows.

[![License](https://img.shields.io/badge/License-BSD_2--Clause-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Windows-0078D6?logo=windows)](https://www.microsoft.com/windows)

## Overview

The official `restic mount` command relies on Linux/macOS FUSE and is not available natively on Windows. **restic-mount for Win32** fills this gap by leveraging [WinFsp](https://winfsp.dev/) and [cgofuse](https://github.com/winfsp/cgofuse) to provide a seamless, read-only Windows drive mount experience.

Built with `CGO_ENABLED=0`, `restic-mount` is a pure Go binary that dynamically loads WinFsp at runtime—requiring no C compiler or build-time headers.

## Features

- **Native Windows Mount**: Mount any restic repository directly to a drive letter (e.g. `R:`) or an empty directory path.
- **Pure Go / No CGO**: Built with `CGO_ENABLED=0`. Dynamically links to WinFsp at runtime.
- **Explorable Snapshots**: Browse through snapshots by timestamp, tag, hostname, or snapshot ID directly in Windows File Explorer.
- **Safe by Default**:
  - Automatically rejects overlapping repository and mount paths to avoid kernel deadlocks.
  - Holds a shared read lock (`OpenWithReadLock`) to safely coexist with concurrent reads while preventing destructive write operations.
- **Windows-Friendly Filesystem Projection**:
  - Implements the **refuse strategy** (similar to Git's `core.protectNTFS`): POSIX paths with Windows-reserved device names (like `CON`, `NUL`, `AUX`), invalid characters (`:`, `*`, `?`, etc.), or control characters are safely hidden from directory listings rather than mutated.
  - Automatically resolves Unicode case-folding collisions to maintain NTFS invariants.
  - Emits non-intrusive warnings on the console when problematic paths are detected.
- **Package Manager Support**: Easy installation and automatic dependency management via `winget`.

## Prerequisites

Running `restic-mount` requires the **WinFsp** kernel driver installed on your machine.

To install WinFsp:

```powershell
winget install WinFsp.WinFsp
```

*(Note: WinFsp is installed automatically when using winget to install `restic-mount`.)*

## Installation

### Via winget (Recommended)

```powershell
winget install Yanother.restic-mount
```

### Direct Binary Download

Download the latest `restic-mount-windows-amd64.zip` from the [Releases](https://github.com/Maki-Daisuke/restic-mount-win/releases) page, extract it, and place `restic-mount.exe` in your system `PATH`.

## Usage

The command-line options, flags, and environment variables (such as `RESTIC_REPOSITORY` and `RESTIC_PASSWORD`) are **identical to the upstream `restic mount`** command. If you are already familiar with `restic mount`, you can use `restic-mount` as a drop-in replacement on Windows.

### Examples

```powershell
# 1. Mount a local repository to drive R:
restic-mount -r D:\Backup\restic-repo R:

# 2. Mount using a password file
restic-mount -p path\to\password.txt -r \\server\share\backup R:

# 3. Mount an S3-compatible cloud repository
$env:AWS_ACCESS_KEY_ID="your-key-id"
$env:AWS_SECRET_ACCESS_KEY="your-secret-key"
$env:RESTIC_PASSWORD="repo-password"
restic-mount -r s3:s3.amazonaws.com/my-bucket C:\mnt\restic

# 4. Mount a specific snapshot only
restic-mount -r D:\Backup\restic-repo --snapshot <SNAPSHOT_ID> R:
```

### Navigating Mounted Repositories

Once mounted (e.g. at `R:`), the virtual drive provides the standard restic virtual directory views:

- `R:\snapshots\`: Snapshots sorted by timestamp.
- `R:\hosts\<hostname>\`: Snapshots filtered by host.
- `R:\tags\<tagname>\`: Snapshots filtered by tag.
- `R:\ids\<id>\`: Snapshots accessed by snapshot ID.

### Unmounting

Press `Ctrl + C` in the terminal running `restic-mount`. The tool will gracefully unmount the virtual drive and release the repository read lock.

*(Note: Because WinFsp registers the mount as a virtual disk, File Explorer does not display an "Eject" option in the right-click menu).*

### Command-Line Options (`restic-mount --help`)

```text
Usage:
  restic-mount [flags] mountpoint

Flags:
      --cacert file                      file to load root certificates from (default: use system certificates or $RESTIC_CACERT)
      --cache-dir directory              set the cache directory. (default: use system default cache directory)
      --cleanup-cache                    auto remove old cache directories
      --compression mode                 compression mode (only available for repository format version 2), one of (auto|off|fastest|better|max) (default: $RESTIC_COMPRESSION) (default auto)
  -h, --help                             help for restic-mount
      --http-user-agent string           set a http user agent for outgoing http requests
      --insecure-no-password             use an empty password for the repository, must be passed to every restic command (insecure)
      --insecure-tls                     skip TLS certificate verification when connecting to the repository (insecure)
      --json                             set output mode to JSON for commands that support it
      --key-hint key                     key ID of key to try decrypting first (default: $RESTIC_KEY_HINT)
      --limit-download rate              limits downloads to a maximum rate in KiB/s. (default: unlimited)
      --limit-upload rate                limits uploads to a maximum rate in KiB/s. (default: unlimited)
      --no-cache                         do not use a local cache
      --no-extra-verify                  skip additional verification of data before upload (see documentation)
      --no-lock                          do not lock the repository, this allows some operations on read-only repositories
  -o, --option key=value                 set extended option (key=value, can be specified multiple times)
      --pack-size size                   set target pack size in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)
      --password-command command         shell command to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)
  -p, --password-file file               file to read the repository password from (default: $RESTIC_PASSWORD_FILE)
  -q, --quiet                            do not output comprehensive progress report
  -r, --repo repository                  repository to backup to or restore from (default: $RESTIC_REPOSITORY)
      --repository-file file             file to read the repository location from (default: $RESTIC_REPOSITORY_FILE)
      --retry-lock duration              retry to lock the repository if it is already locked, takes a value like 5m or 2h (default: no retries)
      --snapshot string                  mount only the specified snapshot ID directly at the root
      --stuck-request-timeout duration   duration after which to retry stuck requests (default 5m0s)
      --tls-client-cert file             path to a file containing PEM encoded TLS client certificate and private key (default: $RESTIC_TLS_CLIENT_CERT)
  -v, --verbose                          be verbose (specify multiple times or a level using --verbose=n, max level/times is 2)
```

## Building from Source

Building `restic-mount` produces a standalone, pure Go binary with `CGO_ENABLED=0`. You do not need MinGW-w64, a C compiler, or WinFsp development headers to build the executable.

### 1. Prerequisites

- **Git**: With submodule support
- **Go**: Version 1.25 or higher
- **Task**: The [Task runner](https://taskfile.dev/) (`go-task` or `task`)

  ```powershell
  winget install Task.Task
  # or via Go
  go install github.com/go-task/task/v3/cmd/task@latest
  ```

### 2. Clone the Repository

Clone recursively to fetch the upstream restic submodule:

```powershell
git clone --recurse-submodules https://github.com/Maki-Daisuke/restic-mount-win.git
cd restic-mount-win
```

*(If you already cloned without submodules, run `git submodule update --init --recursive`)*

### 3. Build Using Task

`Taskfile.yml` automatically creates the required directory junction inside `restic/cmd/restic-mount` (to bypass Go's `internal` packaging restriction), verifies module synchronization, disables CGO, and compiles the binary into `bin/`:

```powershell
# Build binary (outputs to bin/restic-mount.exe)
task build

# Run automated tests
task test

# Clean build artifacts
task clean
```

### 4. Updating the Upstream Submodule

When updating to a newer restic release or commit:

```powershell
# 1. Fetch tags and checkout the target release/commit in the submodule
git -C restic fetch --tags
git -C restic checkout <TAG_OR_COMMIT>   # e.g., v0.18.0

# 2. Resynchronize restic-mount.mod with upstream restic/go.mod
task sync-mod

# 3. Verify tests and build
task test
task build

# 4. Commit the updated submodule and modfiles
git add restic restic-mount.mod restic-mount.sum
git commit -m "chore: update restic submodule to <TAG_OR_COMMIT>"
```

For in-depth explanations of the build architecture, directory junctions, and module management, see the [Development & Build Guide](doc/development.md).

## Architecture & Specifications

Detailed technical documentation is organized in the [doc/](doc/) directory according to Google's **Open Knowledge Format (OKF)** specification:

- [**Documentation Catalog**](doc/index.md) (`doc/index.md`)
- [**Architecture & Design Rationale**](doc/rationale.md): Why WinFsp/cgofuse was chosen, standalone philosophy, directory junctions for Go `internal` restriction, and `-modfile` dependency management.
- [**Windows Filesystem & Name Projection**](doc/windows-filesystem.md): Specification of the refuse strategy for reserved names, forbidden characters, and Unicode case-folding collisions.
- [**Mount Safety & Locking**](doc/safety.md): Mountpoint/repository overlap rejection and shared read locking.
- [**Development & Build Guide**](doc/development.md): Detailed development workflows, Taskfile tasks, and submodule maintenance.

## License

This project is licensed under the [BSD 2-Clause License](LICENSE), the same as upstream [restic](https://github.com/restic/restic).
