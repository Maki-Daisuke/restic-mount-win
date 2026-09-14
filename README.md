# restic-mount

A standalone CLI tool to natively mount [restic](https://github.com/restic/restic) repositories as virtual drives (or folders) on Windows.

---

## Background & Architecture

### 1. Why doesn't the official `restic mount` work on Windows?

The upstream restic mount implementation relies on Linux/macOS FUSE (Filesystem in Userspace) via `anacrolix/fuse`.
On Linux, interacting with the `/dev/fuse` character device to exchange byte streams allows mounting using standard system calls without requiring CGO (an external C compiler).

Windows, on the other hand, lacks a native FUSE interface. Implementing a user-mode filesystem on Windows requires interfacing with a kernel driver like **WinFsp** (Windows File System Proxy) through C APIs (DLLs). This fundamentally requires CGO (e.g., MinGW-w64).

### 2. Why a standalone tool instead of merging into upstream?

A core design principle of restic is **"a single static binary that anyone can cross-compile anywhere with `CGO_ENABLED=0`"**—an exceptionally sound and pragmatic engineering decision.
Adding Windows mount capabilities directly to the official binary would force a C toolchain (CGO) dependency exclusively for Windows builds. This would complicate build pipelines, undermine cross-compilation simplicity, and significantly increase maintenance overhead—the primary reason Windows support has not been merged upstream for years.

Therefore, this project respects upstream's philosophy: rather than compromising that design, it isolates the Windows-specific mounting functionality into a dedicated external command (`restic-mount.exe`) without polluting the upstream codebase.

### 3. Module structure and bypassing Go's `internal` restriction

To reuse restic's encryption, decryption, index parsing, and tree traversal logic, this project includes the upstream repository as a Git submodule.

However, Go's compiler prevents external modules from importing packages under another module's `internal/` directory.
This project maintains its own source code directly in the project tree while creating a **directory junction** under the submodule's `cmd/` directory at build time. This bypasses Go's `internal` packaging restriction without modifying any upstream source code.

```text
my-restic-mount/
├── Taskfile.yml
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

---

## Features

- **Read-Only Mount**: Optimized for browsing backup contents, previewing files in File Explorer, and restoring individual files.
- **WinFsp + cgofuse**: High compatibility with Windows File Explorer powered by a stable virtual filesystem framework.
- **winget Support**: Automatically resolves the required WinFsp driver as a dependency.

---

## Prerequisites

Running this tool requires the **WinFsp** Windows kernel driver.

To install manually:

```powershell
winget install WinFsp.WinFsp
```

_(Note: WinFsp is installed automatically when installing this tool via winget.)_

---

## Installation

### Using winget (Recommended)

Automatically detects and installs the WinFsp dependency.

```powershell
winget install YANOTHER.restic-mount
```

### Direct Binary Download

Download the latest `restic-mount-vX.X.X-windows-amd64.zip` from the [Releases](../../releases) page, extract it, and place `restic-mount.exe` anywhere in your `PATH`.

---

## Usage

Provides a command-line interface and option flags similar to the upstream `restic mount`.

```powershell
# Mount a local repository as drive X:
restic-mount -r D:\Backup\restic-repo X:

# Mount an S3-compatible backend to a folder
$env:AWS_ACCESS_KEY_ID="your-key-id"
$env:AWS_SECRET_ACCESS_KEY="your-secret-key"
$env:RESTIC_PASSWORD="repo-password"
restic-mount -r s3:s3.amazonaws.com/my-bucket C:\mnt\restic

# Mount a specific snapshot only
restic-mount -r D:\Backup\restic-repo --snapshot <SNAPSHOT_ID> X:
```

### Unmounting

Press `Ctrl + C` in the running terminal, or eject the drive directly from File Explorer.

---

## Development

Building the project requires **Go**, **MinGW-w64** (for CGO), and the **[Task](https://taskfile.dev/)** task runner.

### 1. Tool Setup

```powershell
winget install Task.Task
# If you don't have MinGW-w64 (e.g., via MSYS2 or w64devkit)
winget install skeeto.w64devkit
```

### 2. Clone the Repository

Clone recursively to fetch submodules.

```powershell
git clone --recursive https://github.com/Maki-Daisuke/restic-mount.git
cd restic-mount
```

### 3. Build

`Taskfile.yml` automatically verifies/creates the directory junction, enables CGO, and outputs the binary.

```powershell
# Build binary (outputs to bin/restic-mount.exe)
task build

# Clean build artifacts
task clean
```

---

## License

This project is licensed under the [BSD 2-Clause License](LICENSE).
The upstream restic submodule is licensed under its own [BSD 2-Clause License](https://github.com/restic/restic/blob/master/LICENSE).
