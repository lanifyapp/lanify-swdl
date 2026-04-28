package fileutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "removes reserved characters", in: `bad<>:"/\|?*name`, want: "badname"},
		{name: "trims spaces and dots", in: "  example. ", want: "example"},
		{name: "keeps normal unicode", in: "مرحله 1", want: "مرحله 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeFileName(tc.in); got != tc.want {
				t.Fatalf("SanitizeFileName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSafeWorkshopFolderName(t *testing.T) {
	tests := []struct {
		name string
		id   int
		in   string
		want string
	}{
		{name: "uses sanitized title", id: 123, in: "Cool: Map", want: "123_Cool Map"},
		{name: "falls back to id", id: 456, in: `<>:"/\|?*`, want: "456"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeWorkshopFolderName(tc.id, tc.in); got != tc.want {
				t.Fatalf("SafeWorkshopFolderName(%d, %q) = %q, want %q", tc.id, tc.in, got, tc.want)
			}
		})
	}
}

func TestMetaFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &ContentMeta{
		WorkshopID:  111,
		AppID:       730,
		Name:        "Dust",
		Hash:        "abc",
		HashMode:    "paths+size",
		FileCount:   2,
		TotalSize:   42,
		ValidatedAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}

	if err := WriteMetaFile(dir, want); err != nil {
		t.Fatalf("WriteMetaFile() error = %v", err)
	}

	got, err := ReadMetaFile(dir)
	if err != nil {
		t.Fatalf("ReadMetaFile() error = %v", err)
	}

	if *got != *want {
		t.Fatalf("ReadMetaFile() = %+v, want %+v", got, want)
	}
}

func TestValidateWorkshopContentIgnoresHiddenAndEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	mustWriteFile(t, filepath.Join(dir, ".meta"), "ignored")
	mustWriteFile(t, filepath.Join(dir, "empty.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "models", "item.mdl"), "payload")

	validation, err := ValidateWorkshopContent(dir)
	if err != nil {
		t.Fatalf("ValidateWorkshopContent() error = %v", err)
	}

	if validation.FileCount != 2 {
		t.Fatalf("FileCount = %d, want 2", validation.FileCount)
	}

	if validation.TotalSize != int64(len("payload")) {
		t.Fatalf("TotalSize = %d, want %d", validation.TotalSize, len("payload"))
	}

	if validation.Hash == "" || validation.HashMode != "paths+size" {
		t.Fatalf("validation = %+v, want hash with paths+size mode", validation)
	}
}

func TestPathHasUsableFiles(t *testing.T) {
	dir := t.TempDir()

	if PathHasUsableFiles(dir) {
		t.Fatal("PathHasUsableFiles(empty dir) = true, want false")
	}

	mustWriteFile(t, filepath.Join(dir, ".meta"), "hidden")
	if PathHasUsableFiles(dir) {
		t.Fatal("PathHasUsableFiles(hidden-only dir) = true, want false")
	}

	mustWriteFile(t, filepath.Join(dir, "payload.bin"), "x")
	if !PathHasUsableFiles(dir) {
		t.Fatal("PathHasUsableFiles(dir with payload) = false, want true")
	}
}

func TestRemoveFileIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.zip")

	mustWriteFile(t, path, "payload")

	if err := RemoveFileIfExists(path); err != nil {
		t.Fatalf("RemoveFileIfExists(existing file) error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) error = %v, want not exist", path, err)
	}

	if err := RemoveFileIfExists(path); err != nil {
		t.Fatalf("RemoveFileIfExists(missing file) error = %v", err)
	}
}

func TestFileIsUpToDate(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	target := filepath.Join(root, "download.zip")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	mustWriteFile(t, filepath.Join(sourceDir, "mod.txt"), "v1")
	mustWriteFile(t, target, "zip")

	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)

	if err := os.Chtimes(filepath.Join(sourceDir, "mod.txt"), oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(source) error = %v", err)
	}

	if err := os.Chtimes(sourceDir, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(sourceDir) error = %v", err)
	}

	if err := os.Chtimes(target, newTime, newTime); err != nil {
		t.Fatalf("Chtimes(target) error = %v", err)
	}

	if !FileIsUpToDate(target, sourceDir) {
		t.Fatal("FileIsUpToDate(newer target) = false, want true")
	}

	newerSourceTime := time.Now()

	if err := os.Chtimes(filepath.Join(sourceDir, "mod.txt"), newerSourceTime, newerSourceTime); err != nil {
		t.Fatalf("Chtimes(newer source) error = %v", err)
	}

	if FileIsUpToDate(target, sourceDir) {
		t.Fatal("FileIsUpToDate(older target) = true, want false")
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
