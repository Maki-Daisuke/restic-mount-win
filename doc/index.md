---
type: index
title: restic-mount for Windows Documentation Index
description: Entry point and overview of all architectural specifications, runbooks, and design documents for restic-mount-win.
tags:
  - restic
  - windows
  - fuse
  - winfsp
  - documentation
timestamp: 2026-09-16T18:47:00+09:00
---

# restic-mount for Windows Documentation

This directory contains the internal design documents, specifications, and runbooks for **`restic-mount for Windows`**, structured in accordance with Google's **Open Knowledge Format (OKF)** specification.

## Document Catalog

| Document | Type | Summary |
| :--- | :--- | :--- |
| **[Architecture & Design Rationale](rationale.md)** | `architecture` | Why WinFsp/cgofuse was chosen, standalone tool philosophy, 1:1 upstream CLI compatibility, Git submodule integration, and bypassing Go's `internal` restriction via directory junctions and `-modfile`. |
| **[Windows Filesystem & Name Projection](windows-filesystem.md)** | `specification` | The "refuse" strategy for handling POSIX paths invalid on Windows (reserved device names, forbidden characters, length limits, and case-only collision resolution). |
| **[Mount Safety & Locking](safety.md)** | `specification` | Deadlock prevention via mountpoint/repo overlap checks and shared repository locking (`OpenWithReadLock`) matching upstream behavior. |
| **[Development & Build Guide](development.md)** | `runbook` | How to build the project with `Taskfile.yml`, manage dependencies, update the upstream submodule, and run automated tests. |

## Knowledge Bundle Conventions

Each document in this bundle adheres to the following principles:
- **Human and Agent Friendly**: Written in standard GitHub Flavored Markdown with structured YAML frontmatter.
- **Traceable & Version-Controlled**: Maintained directly in the Git repository alongside source code.
- **Categorized Metadata**: Tagged with an explicit `type` field (`architecture`, `specification`, `runbook`, `index`) to assist automated knowledge discovery and navigation.
