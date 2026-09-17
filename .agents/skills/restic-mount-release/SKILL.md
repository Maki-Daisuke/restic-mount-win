---
name: restic-mount-release
description: End-to-end restic-mount release workflow. Use when releasing a new restic-mount version (e.g. v0.1.0): bump release metadata in cmd/restic-mount/version.go, commit, create and push the git tag, wait for GitHub Actions to publish the release ZIP, capture the installer URL and SHA256 from the GitHub Release API (never by downloading), update / validate / submit the WinGet manifest under manifests/y/Yanother/restic-mount/, and publish GitHub release notes.
---

# restic-mount Release

You are the release operator for `restic-mount`. Execute the end-to-end release workflow safely and deterministically.

## Scope

- Inputs from user:
  - Target version tag like `v0.1.0` (or `0.1.0`)
- Responsibilities:
  - Update version metadata in `cmd/restic-mount/version.go`.
  - Verify and update upstream restic submodule commit metadata if changed.
  - Commit version changes.
  - Create and push the git tag.
  - Wait for GitHub Actions to publish the release ZIP.
  - Resolve ZIP URL and SHA256 **from the GitHub Release API only** (never by downloading and hashing locally).
  - Create/update WinGet manifest files under `manifests/y/Yanother/restic-mount/` for the same version, **strictly after** the release asset and its SHA256 are available.
  - Run WinGet manifest validation (`winget validate`) and submission (`wingetcreate submit`).
  - Draft and **publish** GitHub release notes after explicit user confirmation.

## Required Guardrails

Group the rules below by category. Each category lists the only constraints that apply to it; you do not need to remember rules across categories.

### Version handling

- Never guess a version. Parse and validate semver from user input (`vX.Y.Z` or `X.Y.Z`).

### Working tree & git operations

- Refuse to continue if the working tree has unrelated dirty changes that affect release safety.
- Use non-interactive commands only.
- Never use destructive git commands like `git reset --hard`.
- Never execute `git push` or publish releases without explicit user approval.

### WinGet manifest ordering

- Do not rename, edit, or create any file under `manifests/y/Yanother/restic-mount/` until Step 5 has produced both the `InstallerUrl` and the `InstallerSha256` for the new tag. The Step 6 `git mv` rename (or template instantiation for initial release) and edits are then permitted as the _first_ manifest changes, never before. If you feel tempted to "prepare manifests ahead of time," stop.

### Installer SHA256 sourcing

- The agent itself must never compute the installer SHA256 by downloading the ZIP and hashing it. The hash MUST come from the GitHub Release asset metadata via Step 5's polling script. Local downloads can race with re-uploads and produce stale hashes.
- If the user explicitly instructs the agent to fall back to local hashing (because the script cannot recover for some external reason), that user-issued override takes precedence; record it in the conversation and proceed. Without such an explicit user instruction, do not local-hash.

### Confirmations before irreversible steps

Ask the user for an explicit confirmation **separately for each** of the following actions; one confirmation does not cover the others:

- Pushing the release commit
- Pushing the tag
- Submitting the WinGet manifest (`wingetcreate submit`)
- Publishing release notes (overwrites the GitHub Release body)

---

## Workflow

### 1. Validate input and normalize versions

- Accept `vX.Y.Z` or `X.Y.Z`.
- Derive:
  - `TAG=vX.Y.Z`
  - `VERSION=X.Y.Z`

### 2. Preflight checks

- Confirm current branch (`main`) and clean working tree.
- Ensure `gh`, `git`, and `winget` (or `wingetcreate`) are available.
- Check current restic submodule commit:

  ```powershell
  git -C restic rev-parse --short HEAD
  ```

### 3. Update local release metadata

- Update [cmd/restic-mount/version.go](cmd/restic-mount/version.go):
  - Set `Version = "X.Y.Z"`
  - Ensure `ResticCommit` matches the current submodule short commit.
- Run tests and build to ensure integrity:

  ```powershell
  $mod = (Resolve-Path 'restic-mount.mod').Path
  go -C restic test -modfile="$mod" -v ./cmd/restic-mount
  ```

- Show exact file diffs to the user before committing.

### 4. Commit and tag

- Commit message format:

  ```text
  chore: bump version to vX.Y.Z
  ```

- Create annotated tag `vX.Y.Z`:

  ```powershell
  git tag -a vX.Y.Z -m "Release vX.Y.Z"
  ```

- **Ask user confirmation** before pushing commit and tag:

  ```powershell
  git push origin main
  git push origin vX.Y.Z
  ```

### 5. Wait for GitHub Release asset and capture URL + SHA256

- Do not start this step until the tag has been pushed in Step 4.
- **Always use the bundled polling script** [`scripts/wait-release.ps1`](scripts/wait-release.ps1):

  ```powershell
  pwsh -NoProfile -File .agents/skills/restic-mount-release/scripts/wait-release.ps1 -Tag vX.Y.Z
  ```

- The script polls the GitHub Release API until the asset exists **and** its `digest` field is published, then outputs JSON:

  ```json
  {
    "tag": "vX.Y.Z",
    "assetName": "restic-mount-vX.Y.Z-windows-amd64.zip",
    "url": "https://github.com/Maki-Daisuke/restic-mount-win/releases/download/vX.Y.Z/restic-mount-vX.Y.Z-windows-amd64.zip",
    "sha256": "<UPPERCASE_HEX>"
  }
  ```

- Parse `url` (`InstallerUrl`) and `sha256` (`InstallerSha256`).
- **Do NOT download the ZIP to compute hash locally.**

### 6. Update WinGet manifest files (only after Step 5 succeeded)

- Preconditions (verify before any file change):
  - Tag `vX.Y.Z` is pushed.
  - GitHub Release for `vX.Y.Z` exists and contains the windows-amd64 ZIP asset.
  - `InstallerUrl` and `InstallerSha256` from Step 5 are both in hand.
- If any precondition fails, stop. Do not pre-rename directories or pre-edit manifests "to save time."
- Reuse previous manifest directory by renaming it with git history preserved:
  - Find previous version directory under `manifests/y/Yanother/restic-mount/`.
  - Rename with `git mv` from previous version to `X.Y.Z`.
  - Example: `git mv manifests/y/Yanother/restic-mount/<PREV_VERSION> manifests/y/Yanother/restic-mount/X.Y.Z`
- Ensure all 3 files exist and are updated consistently:
  - `Yanother.restic-mount.yaml`
  - `Yanother.restic-mount.installer.yaml`
  - `Yanother.restic-mount.locale.en-US.yaml`
- Update fields:
  - `PackageVersion: X.Y.Z` in all files
  - `InstallerUrl` with `vX.Y.Z` path and matching ZIP file name
  - `InstallerSha256` with the SHA256 value from GitHub Release
- Show exact diffs of manifest files.

### 7. Validate and submit WinGet manifest

- Validate manifests locally:

  ```powershell
  winget validate --manifest manifests/y/Yanother/restic-mount/X.Y.Z
  ```

- **Ask user confirmation** before submitting to `microsoft/winget-pkgs`:

  ```powershell
  wingetcreate submit manifests/y/Yanother/restic-mount/X.Y.Z
  ```

- If `wingetcreate` is not installed, inform user (`winget install Microsoft.WingetCreate`).

### 8. Draft and publish release notes

- Determine previous tag (e.g. `git tag --sort=-v:refname`).
- Collect commit logs with `git log <prev>..vX.Y.Z`.
- Draft GitHub release notes:
  - Highlights / New Features
  - Improvements & Fixes
  - Upstream restic commit metadata
  - Full changelog link: `https://github.com/Maki-Daisuke/restic-mount-win/compare/<prev>...vX.Y.Z`
- **Show draft to user and ask for confirmation.**
- Publish with `gh`:

  ```powershell
  gh release edit vX.Y.Z --notes-file <temp-file>
  ```

### 9. Report results

Use this exact structure:

- Release: `vX.Y.Z`
- Commit: `<sha>`
- Tag: `<tag>`
- Asset URL: `<url>`
- Asset SHA256: `<sha256>`
- Updated files:
  - `<path>`
- Validation: `<pass/fail + key output>`
- Submission: `<submitted/not submitted + PR URL>`
- Release notes: `<published / draft-only + URL>`
- Next action: `<one concrete next step if blocked>`
