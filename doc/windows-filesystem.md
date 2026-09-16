---
type: specification
title: Windows Filesystem and Filename Projection
description: Technical specification for Windows path projection, filename validation, refuse strategy, and case-only collision handling in restic-mount.
tags:
  - specification
  - windows
  - filesystem
  - ntfs
  - refuse-strategy
timestamp: 2026-09-16T18:47:00+09:00
---

# Windows Filesystem and Filename Projection

This document specifies how `restic-mount for Windows` projects POSIX paths stored in restic repositories onto Windows NTFS semantics.

---

## 1. The Challenge: POSIX vs Windows Path Rules

Restic repositories store POSIX-compliant path components created on diverse operating systems (Linux, macOS, BSD). Many names valid on POSIX are strictly prohibited on Windows:

- **Reserved device names**: `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9` (case-insensitive, with or without arbitrary extensions such as `NUL.txt` or `com1.tar.gz`).
- **Forbidden characters**: `<` `>` `:` `"` `/` `\` `|` `?` `*` and ASCII control characters (0x00–0x1F).
- **Trailing characters**: Names ending in a period (`.`) or space (` `).
- **Length limits**: Path components exceeding 255 characters.

---

## 2. The "Refuse" Strategy

Rather than mutating filenames (which archive tools like 7-Zip or WinRAR do), `restic-mount` adopts the **refuse** strategy, directly modeled after Git's `core.protectNTFS`:

- **Directory listings (`Readdir`)**: Entries with names invalid on Windows are **silently omitted from directory listings**. The containing folder remains browsable; only the illegal entries are hidden.
- **Direct lookups (`Lookup` / `Open`)**: Attempting to open or inspect an invalid name directly (e.g., typing the path manually) immediately returns `-fuse.EINVAL`.
- **Zero Mangling**: The original restic path is never falsified. A file stored as `NUL.txt` in the repository is **never** presented as `NUL (1).txt` or `_NUL.txt`.
- **Design Rationale**: This tool is read-only and intended for preview and extraction. Hiding a tiny fraction of pathological files guarantees that file paths on the virtual volume strictly reflect the repository contents without lossy mutation. Users needing those files can still restore them using standard `restic restore` or `restic cat`.

---

## 3. Lazy Evaluation and Browse-Time Warnings

- **Zero Mount-Time Tree Scanning**: Filtering occurs lazily at the moment a directory's tree blob is decoded during `Readdir`. There is no upfront walk of repository snapshots.
- **Browse-Time Warnings**: When `Readdir` hides an invalid entry, a warning is printed to `os.Stderr`:
  ```text
  warning: hiding 1 entry in 'R:\snapshots\latest\src' (invalid on Windows): 'NUL.txt'
  ```
- Warnings are printed on each directory traversal, ensuring immediate visibility without incurring the memory footprint of a global deduplication cache.

---

## 4. Case-Only Collisions (NTFS Invariants)

NTFS guarantees that a single directory cannot contain two entries differing only in case (e.g., `Config.go` and `config.go`). WinFsp's case-insensitive name cache would conflate them if both were exposed.

- **Collision Detection**: Collisions are identified using Go's `strings.EqualFold`, performing full Unicode case-folding (correctly handling non-ASCII collisions such as `straße` / `STRASSE` or `İ` / `i`).
- **Deterministic Resolution**: `Readdir` lists **only one** entry per collision group: deterministically, the first one in restic's tree order (which is pre-sorted in the repository index).
- **Shadow Resolution**: `Lookup` for the shadowed name resolves to the listed entry—identical to accessing `config.go` on a real NTFS filesystem that contains `Config.go`.
- **Console Warning**:
  ```text
  warning: case-only collision in 'R:\snapshots\latest\src': 'config.go' is shadowed by 'Config.go'
  ```

---

## 5. Virtual Snapshot Hierarchy

When mounted without `--snapshot`, the virtual filesystem presents a standardized, intuitive multi-snapshot tree:

```text
R:\
├── snapshots/                  # All snapshots by timestamp
│   ├── latest                  # Most recent snapshot across all hosts
│   ├── 2026-09-16T12-00-00_a1b2c3d4/
│   └── ...
├── hosts/                      # Grouped by hostname
│   └── <hostname>/
│       ├── latest              # Most recent snapshot for this host
│       └── 2026-09-16T12-00-00_a1b2c3d4/
├── tags/                       # Grouped by tag
│   └── <tagname>/
│       ├── latest              # Most recent snapshot with this tag
│       └── 2026-09-16T12-00-00_a1b2c3d4/
└── ids/                        # Grouped by snapshot ID
    ├── a1b2c3d4/               # Short ID
    └── ...
```

Within `hosts/<hostname>/` and `tags/<tagname>/`, paths can be addressed by full timestamp name, short ID, or `latest`.
