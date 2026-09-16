package main

import (
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	v := formatVersion()

	tests := []struct {
		name     string
		contains string
	}{
		{name: "starts with binary name", contains: "restic-mount"},
		{name: "contains version string", contains: Version},
		{name: "contains restic commit", contains: ResticCommit},
		{name: "contains based on restic", contains: "based on restic"},
		{name: "contains os arch", contains: "windows/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(v, tt.contains) {
				t.Errorf("formatVersion() = %q; expected to contain %q", v, tt.contains)
			}
		})
	}
}
