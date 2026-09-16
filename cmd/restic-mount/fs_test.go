package main

import (
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
)

func setTestSnapshotID(sn *data.Snapshot, id restic.ID) {
	field, ok := reflect.TypeOf(data.Snapshot{}).FieldByName("id")
	if !ok {
		return
	}
	idCopy := id
	ptr := (**restic.ID)(unsafe.Pointer(uintptr(unsafe.Pointer(sn)) + field.Offset))
	*ptr = &idCopy
}

func TestSnapshotName(t *testing.T) {
	tm := time.Date(2026, 9, 16, 15, 4, 5, 0, time.UTC)
	id := restic.ID{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	sn := &data.Snapshot{
		Time: tm,
	}
	setTestSnapshotID(sn, id)

	name := snapshotName(sn)
	expected := "2026-09-16T15-04-05_aabbccdd"
	if name != expected {
		t.Errorf("snapshotName() = %q, want %q", name, expected)
	}

	// Test without ID set
	snNoID := &data.Snapshot{Time: tm}
	if got := snapshotName(snNoID); got != "2026-09-16T15-04-05" {
		t.Errorf("snapshotName(snNoID) = %q, want %q", got, "2026-09-16T15-04-05")
	}

	// Test nil
	if got := snapshotName(nil); got != "" {
		t.Errorf("snapshotName(nil) = %q, want empty", got)
	}
}

func TestFindSnapshotInList(t *testing.T) {
	tm1 := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	tm2 := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)

	id1 := restic.ID{0x11, 0x22, 0x33, 0x44}
	id2 := restic.ID{0x55, 0x66, 0x77, 0x88}

	sn1 := &data.Snapshot{Time: tm1, Hostname: "LEVEL-D"}
	setTestSnapshotID(sn1, id1)

	sn2 := &data.Snapshot{Time: tm2, Hostname: "LEVEL-D"}
	setTestSnapshotID(sn2, id2)

	snList := []*data.Snapshot{sn1, sn2}

	tests := []struct {
		name     string
		target   string
		expected *data.Snapshot
	}{
		{
			name:     "latest resolves to first snapshot",
			target:   "latest",
			expected: sn1,
		},
		{
			name:     "full name resolves correctly",
			target:   snapshotName(sn1),
			expected: sn1,
		},
		{
			name:     "short ID resolves correctly",
			target:   sn2.ID().Str(),
			expected: sn2,
		},
		{
			name:     "time string resolves correctly",
			target:   sn2.Time.Format("2006-01-02T15-04-05"),
			expected: sn2,
		},
		{
			name:     "non-existent target returns nil",
			target:   "non-existent",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findSnapshotInList(snList, tc.target)
			if got != tc.expected {
				t.Errorf("findSnapshotInList(snList, %q) = %v, want %v", tc.target, got, tc.expected)
			}
		})
	}

	// Test with empty list
	if got := findSnapshotInList(nil, "latest"); got != nil {
		t.Errorf("findSnapshotInList(nil, 'latest') = %v, want nil", got)
	}
}
