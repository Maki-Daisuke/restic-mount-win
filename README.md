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
- **Honest Filename Handling**: Adopts Git's `core.protectNTFS`-style "refuse" strategy. Invalid Windows characters/reserved names (`CON`, `NUL`, etc.) are safely hidden from Explorer rather than dangerously renamed/mangled, and case-only collisions are resolved cleanly without scanning the whole repository upfront. See [Windows Filesystem & Name Projection](doc/windows-filesystem.md) for the full specification.
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

## Technical Details

This tool makes several non-obvious engineering decisions, each documented in depth under [`doc/`](doc/) and organized according to Google's **Open Knowledge Format (OKF)** specification (see the [Documentation Catalog](doc/index.md)).

| Document | Covers |
| :--- | :--- |
| [Architecture & Design Rationale](doc/rationale.md) | Why upstream `restic mount` cannot support Windows, why this ships as a standalone binary, and how Go's `internal/` restriction is bypassed with directory junctions and `-modfile` — without touching upstream. |
| [Windows Filesystem & Name Projection](doc/windows-filesystem.md) | The complete "refuse" strategy: reserved device names, forbidden characters, case-only collision resolution, and lazy browse-time evaluation. |
| [Mount Safety & Locking](doc/safety.md) | Deadlock prevention via mountpoint/repository overlap detection, and the shared repository lock lifecycle. |
| [Development & Build Guide](doc/development.md) | Environment setup, `Taskfile.yml` workflows, test coverage, and updating the upstream restic submodule. |

### Quick Build Reference

Building requires **[Go](https://go.dev/) (1.27+)** and **[Task](https://taskfile.dev/)**. No C compiler or WinFsp SDK headers are needed (`CGO_ENABLED=0`).

```powershell
git clone --recursive https://github.com/Maki-Daisuke/restic-mount-win.git
cd restic-mount-win

task build   # -> bin/restic-mount.exe
task test    # run unit tests
task clean   # remove build artifacts
```

See the [Development & Build Guide](doc/development.md) for the full contributor workflow, including submodule updates and `task sync-mod`.

---

## License

This project is licensed under the [BSD 2-Clause License](LICENSE).  
The upstream restic submodule is licensed under its own [BSD 2-Clause License](https://github.com/restic/restic/blob/master/LICENSE).
