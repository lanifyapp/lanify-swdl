package fileutil

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipDirectoryCreatesArchiveAtTargetPath(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	targetDir := filepath.Join(root, "downloads")
	targetZipPath := filepath.Join(targetDir, "sample.zip")
	
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	
	if err := os.WriteFile(filepath.Join(sourceDir, "mod.txt"), []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	
	if err := ZipDirectory(sourceDir, targetZipPath); err != nil {
		t.Fatalf("ZipDirectory() error = %v", err)
	}
	
	if _, err := os.Stat(targetZipPath); err != nil {
		t.Fatalf("Stat(targetZipPath) error = %v", err)
	}
	
	matches, err := filepath.Glob(filepath.Join(targetDir, ".zip-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	
	if len(matches) != 0 {
		t.Fatalf("temporary zip files were not cleaned up: %v", matches)
	}
	
	reader, err := zip.OpenReader(targetZipPath)
	if err != nil {
		t.Fatalf("zip.OpenReader() error = %v", err)
	}
	
	defer reader.Close()
	
	if len(reader.File) == 0 {
		t.Fatal("zip archive is empty")
	}
	
	foundPayload := false
	
	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, "mod.txt") {
			continue
		}
		
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("file.Open() error = %v", err)
		}
		
		buf, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		
		if string(buf) != "payload" {
			t.Fatalf("archived payload = %q, want %q", string(buf), "payload")
		}
		
		foundPayload = true
	}
	
	if !foundPayload {
		t.Fatal("archived payload file not found")
	}
}
