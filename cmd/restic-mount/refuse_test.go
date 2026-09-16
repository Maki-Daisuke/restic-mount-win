package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
)

func TestIsInvalidWindowsName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantInvalid bool
	}{
		// Valid names
		{name: "simple valid file", input: "hello.txt", wantInvalid: false},
		{name: "valid with unicode", input: "日本語ファイル.txt", wantInvalid: false},
		{name: "valid hidden posix file", input: ".gitignore", wantInvalid: false},
		{name: "valid name with dot in middle", input: "archive.tar.gz", wantInvalid: false},
		{name: "valid name containing con as substring", input: "contact.txt", wantInvalid: false},
		{name: "valid name containing nul as substring", input: "nullify.go", wantInvalid: false},
		{name: "valid name ending with valid chars", input: "file123", wantInvalid: false},

		// Empty name
		{name: "empty name", input: "", wantInvalid: true},

		// Reserved device names (exact and with extension, case-insensitive)
		{name: "reserved CON", input: "CON", wantInvalid: true},
		{name: "reserved con lowercase", input: "con", wantInvalid: true},
		{name: "reserved CON.txt", input: "CON.txt", wantInvalid: true},
		{name: "reserved con.tar.gz", input: "con.tar.gz", wantInvalid: true},
		{name: "reserved PRN", input: "PRN", wantInvalid: true},
		{name: "reserved prn.doc", input: "prn.doc", wantInvalid: true},
		{name: "reserved AUX", input: "AUX", wantInvalid: true},
		{name: "reserved aux.c", input: "aux.c", wantInvalid: true},
		{name: "reserved NUL", input: "NUL", wantInvalid: true},
		{name: "reserved nul.txt", input: "nul.txt", wantInvalid: true},
		{name: "reserved COM1", input: "COM1", wantInvalid: true},
		{name: "reserved com9.dat", input: "com9.dat", wantInvalid: true},
		{name: "reserved LPT1", input: "LPT1", wantInvalid: true},
		{name: "reserved lpt5.log", input: "lpt5.log", wantInvalid: true},

		// Forbidden characters
		{name: "forbidden less than", input: "file<name", wantInvalid: true},
		{name: "forbidden greater than", input: "file>name", wantInvalid: true},
		{name: "forbidden colon", input: "file:stream", wantInvalid: true},
		{name: "forbidden double quote", input: "file\"name", wantInvalid: true},
		{name: "forbidden slash", input: "file/name", wantInvalid: true},
		{name: "forbidden backslash", input: "file\\name", wantInvalid: true},
		{name: "forbidden pipe", input: "file|name", wantInvalid: true},
		{name: "forbidden question", input: "file?name", wantInvalid: true},
		{name: "forbidden asterisk", input: "file*name", wantInvalid: true},
		{name: "forbidden control char newline", input: "file\nname", wantInvalid: true},
		{name: "forbidden control char tab", input: "file\tname", wantInvalid: true},
		{name: "forbidden control char null byte", input: "file\x00name", wantInvalid: true},

		// Trailing dot or space
		{name: "trailing dot", input: "file.", wantInvalid: true},
		{name: "trailing space", input: "file ", wantInvalid: true},
		{name: "trailing dot and space", input: "file. ", wantInvalid: true},
		{name: "trailing multiple dots", input: "file...", wantInvalid: true},

		// Length limit (255 chars)
		{name: "exact 255 chars", input: strings.Repeat("a", 255), wantInvalid: false},
		{name: "256 chars", input: strings.Repeat("a", 256), wantInvalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotInvalid, _ := IsInvalidWindowsName(tt.input)
			if gotInvalid != tt.wantInvalid {
				t.Errorf("IsInvalidWindowsName(%q) = %v, want %v", tt.input, gotInvalid, tt.wantInvalid)
			}
		})
	}
}

func TestFilterDirectoryEntries(t *testing.T) {
	nodes := []*data.Node{
		{Name: "Config.go"},
		{Name: "config.go"}, // Case collision with Config.go -> should be shadowed
		{Name: "NUL.txt"},   // Invalid name -> should be hidden
		{Name: "valid.txt"},
		{Name: "bad:name"},  // Invalid char -> should be hidden
		{Name: "README.md"},
	}

	result := FilterDirectoryEntries(nodes)

	if len(result.VisibleEntries) != 3 {
		t.Fatalf("expected 3 visible entries, got %d", len(result.VisibleEntries))
	}
	expectedVisible := []string{"Config.go", "valid.txt", "README.md"}
	for i, want := range expectedVisible {
		if result.VisibleEntries[i].Name != want {
			t.Errorf("visible[%d] = %s, want %s", i, result.VisibleEntries[i].Name, want)
		}
	}

	if len(result.HiddenNames) != 2 {
		t.Fatalf("expected 2 hidden names, got %d", len(result.HiddenNames))
	}
	expectedHidden := []string{"NUL.txt", "bad:name"}
	for i, want := range expectedHidden {
		if result.HiddenNames[i] != want {
			t.Errorf("hidden[%d] = %s, want %s", i, result.HiddenNames[i], want)
		}
	}

	if len(result.Shadowed) != 1 {
		t.Fatalf("expected 1 shadowed entry, got %d", len(result.Shadowed))
	}
	if result.Shadowed[0].ShadowedName != "config.go" || result.Shadowed[0].KeptName != "Config.go" {
		t.Errorf("unexpected shadowed entry: %+v", result.Shadowed[0])
	}

	// Test ResolveEntry
	t.Run("ResolveEntry exact match", func(t *testing.T) {
		node := result.ResolveEntry("Config.go")
		if node == nil || node.Name != "Config.go" {
			t.Errorf("expected to resolve Config.go, got %v", node)
		}
	})

	t.Run("ResolveEntry case-insensitive resolving to kept entry", func(t *testing.T) {
		node := result.ResolveEntry("config.go")
		if node == nil || node.Name != "Config.go" {
			t.Errorf("expected config.go to resolve to Config.go, got %v", node)
		}
	})

	t.Run("ResolveEntry invalid name returns nil", func(t *testing.T) {
		node := result.ResolveEntry("NUL.txt")
		if node != nil {
			t.Errorf("expected nil for invalid name, got %v", node)
		}
	})

	t.Run("ResolveEntry non-existent name returns nil", func(t *testing.T) {
		node := result.ResolveEntry("nonexistent.txt")
		if node != nil {
			t.Errorf("expected nil for nonexistent name, got %v", node)
		}
	})
}

func TestPrintFilterWarnings(t *testing.T) {
	nodes := []*data.Node{
		{Name: "Config.go"},
		{Name: "config.go"},
		{Name: "NUL.txt"},
	}

	result := FilterDirectoryEntries(nodes)
	var buf bytes.Buffer
	PrintFilterWarnings(&buf, `C:\mnt\restic\src`, result)

	output := buf.String()
	expectedHiddenWarn := "warning: hiding 1 entry in 'C:\\mnt\\restic\\src' (invalid on Windows): 'NUL.txt'\n"
	expectedCollisionWarn := "warning: case-only collision in 'C:\\mnt\\restic\\src': 'config.go' is shadowed by 'Config.go'\n"

	if !strings.Contains(output, expectedHiddenWarn) {
		t.Errorf("output does not contain expected hidden warning.\nGot:\n%s\nWant substring:\n%s", output, expectedHiddenWarn)
	}
	if !strings.Contains(output, expectedCollisionWarn) {
		t.Errorf("output does not contain expected collision warning.\nGot:\n%s\nWant substring:\n%s", output, expectedCollisionWarn)
	}
}
