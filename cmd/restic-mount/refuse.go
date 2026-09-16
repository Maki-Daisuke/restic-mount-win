package main

import (
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/restic/restic/internal/data"
)

// reservedWindowsNames lists the reserved MS-DOS device names on Windows.
var reservedWindowsNames = map[string]struct{}{
	"CON":  {},
	"PRN":  {},
	"AUX":  {},
	"NUL":  {},
	"COM1": {},
	"COM2": {},
	"COM3": {},
	"COM4": {},
	"COM5": {},
	"COM6": {},
	"COM7": {},
	"COM8": {},
	"COM9": {},
	"LPT1": {},
	"LPT2": {},
	"LPT3": {},
	"LPT4": {},
	"LPT5": {},
	"LPT6": {},
	"LPT7": {},
	"LPT8": {},
	"LPT9": {},
}

// IsInvalidWindowsName checks if a POSIX file/directory name is invalid on Windows
// according to NTFS and Win32 naming rules.
// Returns true and a descriptive reason if invalid, or false and "" if valid.
func IsInvalidWindowsName(name string) (bool, string) {
	if name == "" {
		return true, "empty name"
	}

	// Rule 1: Length limit (255 characters/runes)
	if utf8.RuneCountInString(name) > 255 {
		return true, "name exceeds 255 characters"
	}

	// Rule 2: Trailing dot or space
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return true, "trailing dot or space"
	}

	// Rule 3: Forbidden characters: < > : " / \ | ? * and control chars (0x00 - 0x1F)
	for _, r := range name {
		if r < 32 {
			return true, fmt.Sprintf("contains control character 0x%02x", r)
		}
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return true, fmt.Sprintf("contains forbidden character '%c'", r)
		}
	}

	// Rule 4: Reserved DOS device names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	// On Windows, the device check applies to the base name before the first dot,
	// or the whole name. E.g., "NUL.txt" or "nul" or "aux.tar.gz" are reserved.
	base := name
	if dotIdx := strings.IndexByte(name, '.'); dotIdx != -1 {
		base = name[:dotIdx]
	}
	baseUpper := strings.ToUpper(base)
	if _, isReserved := reservedWindowsNames[baseUpper]; isReserved {
		return true, fmt.Sprintf("reserved Windows device name '%s'", baseUpper)
	}

	return false, ""
}

// ShadowedEntry records a file/dir shadowed by another due to a case-only collision.
type ShadowedEntry struct {
	ShadowedName string
	KeptName     string
}

// FilterResult holds the outcome of projecting directory entries for Windows.
type FilterResult struct {
	VisibleEntries []*data.Node
	HiddenNames    []string
	Shadowed       []ShadowedEntry
	// LookupMap maps case-folded entry names to the visible Node for fast case-insensitive resolution.
	LookupMap map[string]*data.Node
}

// FilterDirectoryEntries applies the "refuse" strategy to a list of restic nodes:
// 1. Omits entries with invalid Windows names.
// 2. Omits case-only colliding entries (keeping the first one according to restic's order).
// Returns a FilterResult.
func FilterDirectoryEntries(nodes []*data.Node) *FilterResult {
	result := &FilterResult{
		VisibleEntries: make([]*data.Node, 0, len(nodes)),
		HiddenNames:    make([]string, 0),
		Shadowed:       make([]ShadowedEntry, 0),
		LookupMap:      make(map[string]*data.Node, len(nodes)),
	}

	for _, node := range nodes {
		if invalid, _ := IsInvalidWindowsName(node.Name); invalid {
			result.HiddenNames = append(result.HiddenNames, node.Name)
			continue
		}

		// Check for case-only collision with already visible entries
		var collisionWith *data.Node
		for _, visible := range result.VisibleEntries {
			if strings.EqualFold(node.Name, visible.Name) {
				collisionWith = visible
				break
			}
		}

		if collisionWith != nil {
			result.Shadowed = append(result.Shadowed, ShadowedEntry{
				ShadowedName: node.Name,
				KeptName:     collisionWith.Name,
			})
			continue
		}

		result.VisibleEntries = append(result.VisibleEntries, node)
		// Register in lookup map using lowercase for case-insensitive lookup
		result.LookupMap[strings.ToLower(node.Name)] = node
	}

	return result
}

// PrintFilterWarnings outputs warnings to the provided writer (typically os.Stderr)
// when entries are hidden or shadowed in the given directory.
func PrintFilterWarnings(w io.Writer, dirPath string, result *FilterResult) {
	if len(result.HiddenNames) > 0 {
		entryWord := "entry"
		if len(result.HiddenNames) > 1 {
			entryWord = "entries"
		}
		quotedNames := make([]string, len(result.HiddenNames))
		for i, name := range result.HiddenNames {
			quotedNames[i] = fmt.Sprintf("'%s'", name)
		}
		formattedPath := dirPath
		if formattedPath == "" || formattedPath == "." {
			formattedPath = "/"
		}
		fmt.Fprintf(w, "warning: hiding %d %s in '%s' (invalid on Windows): %s\n",
			len(result.HiddenNames), entryWord, formattedPath, strings.Join(quotedNames, ", "))
	}

	for _, shadow := range result.Shadowed {
		formattedPath := dirPath
		if formattedPath == "" || formattedPath == "." {
			formattedPath = "/"
		}
		fmt.Fprintf(w, "warning: case-only collision in '%s': '%s' is shadowed by '%s'\n",
			formattedPath, shadow.ShadowedName, shadow.KeptName)
	}
}

// ResolveEntry looks up a requested name in the directory's visible entries.
// It handles exact match and case-insensitive match (mirroring NTFS behavior where
// accessing 'config.go' resolves to 'Config.go').
// Returns nil if not found or if the name is invalid on Windows.
func (r *FilterResult) ResolveEntry(name string) *data.Node {
	cleanName := path.Base(name)
	if invalid, _ := IsInvalidWindowsName(cleanName); invalid {
		return nil
	}

	// 1. Exact match
	for _, node := range r.VisibleEntries {
		if node.Name == cleanName {
			return node
		}
	}

	// 2. Case-insensitive match using strings.EqualFold
	for _, node := range r.VisibleEntries {
		if strings.EqualFold(node.Name, cleanName) {
			return node
		}
	}

	return nil
}
