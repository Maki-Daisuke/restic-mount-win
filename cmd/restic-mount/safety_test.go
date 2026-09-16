package main

import (
	"os"
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
