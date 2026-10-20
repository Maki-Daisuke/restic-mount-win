# restic-mount for Windows

A standalone CLI tool to natively mount [restic](https://github.com/restic/restic) repositories as virtual drives (or folders) in Windows Explorer.

> **A true drop-in replacement:** If you know `restic mount`, you already know `restic-mount`. Just swap the space for a hyphen (`restic mount` ➡️ `restic-mount`).

---

## TL;DR

```powershell
# 1. Install via winget (automatically installs the WinFsp driver)
❯ winget install --id Yanother.restic-mount

# 2. Mount your repository just like upstream restic
❯ restic-mount -r D:\Backup\restic-repo R:
Now serving the repository at R:
Use another terminal or File Explorer to browse the contents.
When finished, quit with Ctrl-c here to unmount R:.
The service restic-mount has been started.
```

---

## Key Features

- **True Drop-In CLI**: Accepts the exact same repository flags, environment variables (`RESTIC_REPOSITORY`, `RESTIC_PASSWORD`), and syntax as upstream `restic mount`.
- **Pure Go & CGO-Free**: Built with `CGO_ENABLED=0` dynamically loading WinFsp at runtime. No MinGW or C toolchain required.
- **Native File Explorer Integration**: Browse snapshots, inspect directory trees, preview files, and restore specific items directly in Windows File Explorer.
- **Safe by Default**:
  - Automatically refuses to mount inside or over local repositories to prevent kernel deadlocks (matching upstream GH #5234).
  - Acquires and holds a shared read-lock on the repository while mounted, supporting concurrent mounts while blocking destructive operations.
- **Honest Filename Handling**: Adopts Git's `core.protectNTFS`-style "refuse" strategy. Invalid Windows characters/reserved names (`CON`, `NUL`, etc.) are safely hidden from Explorer rather than dangerously renamed/mangled, and case-only collisions are resolved cleanly without scanning the whole repository upfront.
- **One-Command Setup**: Managed via `winget`, which automatically resolves and installs the required WinFsp kernel driver.

---

## Installation

### Using winget (Recommended)

Installs `restic-mount` and automatically handles the WinFsp kernel driver dependency:

```powershell
winget install --id Yanother.restic-mount
```

_(Note: Windows will prompt once for administrator approval to install WinFsp if it isn't already present.)_

### Direct Binary Download

If you prefer a manual setup:

1. Download and install [WinFsp](https://winfsp.dev/) (`winget install WinFsp.WinFsp`).
2. Download `restic-mount-vX.X.X-windows-amd64.zip` from [GitHub Releases](https://github.com/Maki-Daisuke/restic-mount-win/releases).
3. Extract `restic-mount.exe` and place it anywhere in your `PATH`.

---

## Usage

Mount any restic repository by specifying the repository location and a target drive letter or folder:

```powershell
# Mount a local repository as drive X:
restic-mount -r D:\Backup\restic-repo X:

# Mount to an existing empty folder
restic-mount -r D:\Backup\restic-repo C:\mnt\restic

# Mount an S3-compatible backend (supports standard restic env vars)
$env:AWS_ACCESS_KEY_ID="your-key-id"
$env:AWS_SECRET_ACCESS_KEY="your-secret-key"
$env:RESTIC_PASSWORD="repo-password"
restic-mount -r s3:s3.amazonaws.com/my-bucket X:

# Mount a specific snapshot only
restic-mount -r D:\Backup\restic-repo --snapshot <SNAPSHOT_ID> X:

# Mount without acquiring a repository lock
restic-mount -r D:\Backup\restic-repo --no-lock X:
```

### Navigating Mounted Repositories

Once mounted (e.g. at `X:`), File Explorer provides the standard restic virtual directory views:

- `X:\snapshots\`: Snapshots sorted by timestamp.
- `X:\hosts\<hostname>\`: Snapshots filtered by host.
- `X:\tags\<tagname>\`: Snapshots filtered by tag.
- `X:\ids\<id>\`: Snapshots accessed by snapshot ID.

_(Note: When mounting with `--snapshot <ID>`, the specified snapshot's filesystem tree is presented directly at the root of `X:`.)_

### Unmounting

Press `Ctrl + C` in the running terminal. The mount and repository lock will be cleanly released.

_(Note: Because WinFsp registers the mount as a virtual disk, File Explorer does not display an "Eject" option in the right-click context menu. Always terminate the process via `Ctrl + C` in the terminal to unmount cleanly.)_

---

## Windows Filename Projection: The "Refuse" Strategy

Restic repositories store POSIX paths from Linux/macOS that may be illegal on Windows (e.g. reserved device names like `CON`, `NUL`, `AUX`, `PRN`, `COM1`–`COM9`, `LPT1`–`LPT9`; forbidden characters `\ / : * ? " < > |`; trailing dots or spaces; names exceeding 255 characters).

Following Git's `core.protectNTFS` approach, this tool uses a **strict refusal strategy**:

- **Omit invalid names**: Directory listings (`Readdir`) omit entries whose names are illegal on Windows, printing a one-time console warning. The rest of the directory remains completely browsable.
- **Lookup / Open**: Explicitly requesting an invalid path returns `EINVAL`.
- **No falsified names**: We never rewrite/mangle names (e.g. turning `NUL.txt` into `NUL (1).txt`). Preserving data integrity is paramount; users can always retrieve problematic files using `restic restore` or `restic cat`.
- **Zero startup penalty (Lazy evaluation)**: Filtering happens only when a directory is listed in Explorer. There is no upfront tree traversal.
- **Case-only collisions**: For directories with multiple files differing only by case (e.g., `config.go` vs `Config.go`), only the first entry in sorted tree order is listed to protect NTFS invariants, and a console warning is emitted.

---

## Architecture & Engineering Decisions

<details>
<summary>Click to expand technical design details</summary>

### 1. Why does official `restic mount` lack Windows support?

The upstream mount implementation relies on Linux/macOS FUSE (`anacrolix/fuse`). On Linux, exchanging byte streams with `/dev/fuse` works without CGO. Windows lacks native FUSE, requiring an external kernel driver like WinFsp. Merging WinFsp or CGO dependencies upstream conflicts with restic's goal of a lean, CGO-free, cross-compilable core binary.

### 2. Standalone Binary with No upstream `go.mod` Pollution

`restic-mount-win` is completely decoupled. By utilizing `winfsp/cgofuse` (which dynamically loads WinFsp DLLs without MinGW headers) and keeping it in a separate executable, we avoid polluting upstream `go.mod` with Windows-specific dependencies.

### 3. Bypassing Go's `internal` Restriction via Junctions

To reuse restic's robust repository, crypto, and pack-parsing logic without duplicating code, restic is tracked as a git submodule. At build time, a Windows **directory junction** links `cmd/restic-mount` into `restic/cmd/restic-mount`, allowing it to consume `restic/internal/...` packages natively:

```text
restic-mount-win/
├── Taskfile.yml
├── restic-mount.mod            # restic/go.mod + cgofuse require
├── cmd/
│   └── restic-mount/           # Project source code
└── restic/                     # git submodule
    └── cmd/
        └── restic-mount  ======> Junction to ../../cmd/restic-mount
```

### 4. Zero Upstream Modification via `-modfile`

To ensure clean submodule tracking, we never modify `restic/go.mod`. The build executes from inside `restic/` using Go 1.14+'s `-modfile=../restic-mount.mod`, which layers the `cgofuse` dependency over restic's module tree without mutating the submodule.

---

For in-depth technical specifications, safety models, and design rationale, see the [doc/](doc/) directory, organized according to Google's **Open Knowledge Format (OKF)** specification (see the [Documentation Catalog](doc/index.md)).

</details>

---

## Development & Building

Building requires **[Go](https://go.dev/) (1.25+)** and **[Task](https://taskfile.dev/)**. No C compiler or WinFsp SDK headers are needed (`CGO_ENABLED=0`).

### 1. Tool Setup

```powershell
winget install Task.Task
```

### 2. Clone the Repository

Clone recursively to fetch submodules:

```powershell
git clone --recursive https://github.com/Maki-Daisuke/restic-mount-win.git
cd restic-mount-win
```

### 3. Build & Test

`Taskfile.yml` automatically verifies/creates the directory junction, checks that `restic-mount.mod` is in sync with `restic/go.mod`, disables CGO, and outputs the binary.

```powershell
# Build binary (outputs to bin/restic-mount.exe)
task build

# Run unit tests
task test

# Clean build artifacts
task clean
```

Under the hood, the build runs (from inside `restic/`):

```powershell
$env:CGO_ENABLED = "0"
go -C restic build -modfile=../restic-mount.mod -trimpath -o ../bin/restic-mount.exe ./cmd/restic-mount
```

### 4. Updating the restic submodule

After bumping the submodule pointer, regenerate the alternate module file:

```powershell
task sync-mod   # copies restic/go.mod -> restic-mount.mod, re-adds cgofuse, runs go mod tidy
```

For detailed contributor workflows and development notes, see the [Development & Build Guide](doc/development.md).

---

## License

This project is licensed under the [BSD 2-Clause License](LICENSE).  
The upstream restic submodule is licensed under its own [BSD 2-Clause License](https://github.com/restic/restic/blob/master/LICENSE).
