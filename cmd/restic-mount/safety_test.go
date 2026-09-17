package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckMountpointOverlap(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "restic-safety-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoDir := filepath.Join(tempDir, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	nestedMount := filepath.Join(repoDir, "mount")
	if err := os.Mkdir(nestedMount, 0755); err != nil {
		t.Fatalf("failed to create nested mount: %v", err)
	}
	siblingMount := filepath.Join(tempDir, "other_mount")
	if err := os.Mkdir(siblingMount, 0755); err != nil {
		t.Fatalf("failed to create sibling mount: %v", err)
	}

	tests := []struct {
		name        string
		repoPath    string
		mountpoint  string
		wantErr     bool
		errContains string
	}{
		{
			name:        "exact match",
			repoPath:    repoDir,
			mountpoint:  repoDir,
			wantErr:     true,
			errContains: "is the local repository directory",
		},
		{
			name:        "exact match with case difference",
			repoPath:    repoDir,
			mountpoint:  strings.ToUpper(repoDir),
			wantErr:     true,
			errContains: "is the local repository directory",
		},
		{
			name:        "mountpoint nested inside repo",
			repoPath:    repoDir,
			mountpoint:  nestedMount,
			wantErr:     true,
			errContains: "is inside the local repository directory",
		},
		{
			name:        "repo nested inside mountpoint",
			repoPath:    nestedMount,
			mountpoint:  repoDir,
			wantErr:     true,
			errContains: "is inside the mountpoint",
		},
		{
			name:       "unrelated sibling directory",
			repoPath:   repoDir,
			mountpoint: siblingMount,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckMountpointOverlap(tt.repoPath, tt.mountpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestCheckMountpointOverlap_Junction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "restic-junction-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoDir := filepath.Join(tempDir, "real_repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	junctionDir := filepath.Join(tempDir, "junction_repo")
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("New-Item -ItemType Junction -Path '%s' -Target '%s'", junctionDir, repoDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping junction test: unable to create junction: %v: %s", err, string(out))
	}

	// 1. Mountpoint is the junction pointing to the repo
	err = CheckMountpointOverlap(repoDir, junctionDir)
	if err == nil {
		t.Errorf("expected error when mountpoint is a junction to repo, got nil")
	} else if !strings.Contains(err.Error(), "is the local repository directory") {
		t.Errorf("expected error to contain repository directory warning, got: %v", err)
	}

	// 2. Repo path is the junction pointing to the mountpoint
	err = CheckMountpointOverlap(junctionDir, repoDir)
	if err == nil {
		t.Errorf("expected error when repo is a junction to mountpoint, got nil")
	} else if !strings.Contains(err.Error(), "is the local repository directory") {
		t.Errorf("expected error to contain repository directory warning, got: %v", err)
	}

	// 3. Mountpoint is nested inside real repo, but accessed via junction
	nestedInReal := filepath.Join(repoDir, "mount")
	if err := os.Mkdir(nestedInReal, 0755); err != nil {
		t.Fatalf("failed to create nested mount: %v", err)
	}
	nestedViaJunc := filepath.Join(junctionDir, "mount")
	err = CheckMountpointOverlap(repoDir, nestedViaJunc)
	if err == nil {
		t.Errorf("expected error when mountpoint is inside repo via junction, got nil")
	} else if !strings.Contains(err.Error(), "is inside the local repository directory") {
		t.Errorf("expected error to contain inside directory warning, got: %v", err)
	}
}

func TestResolvePath_Junction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "restic-resolve-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetDir := filepath.Join(tempDir, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	junctionDir := filepath.Join(tempDir, "junc")
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("New-Item -ItemType Junction -Path '%s' -Target '%s'", junctionDir, targetDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping junction test: unable to create junction: %v: %s", err, string(out))
	}

	resolved, err := resolvePath(junctionDir)
	if err != nil {
		t.Fatalf("resolvePath failed: %v", err)
	}

	expected, err := resolvePath(targetDir)
	if err != nil {
		t.Fatalf("resolvePath on target failed: %v", err)
	}

	if !strings.EqualFold(resolved, expected) {
		t.Errorf("expected resolvePath to return %q, got %q", expected, resolved)
	}
}

