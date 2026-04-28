package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	defaultHTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*]`)
)

type ContentMeta struct {
	WorkshopID  int    `json:"workshop_id"`
	AppID       int    `json:"app_id"`
	Name        string `json:"name"`
	Hash        string `json:"hash"`
	HashMode    string `json:"hash_mode"`
	FileCount   int    `json:"file_count"`
	TotalSize   int64  `json:"total_size"`
	ValidatedAt string `json:"validated_at"`
}

type ContentValidation struct {
	Hash      string
	HashMode  string
	FileCount int
	TotalSize int64
}

func DownloadFile(url, targetPath string) error {
	slog.Info("downloading file", "url", url, "target", targetPath)

	if err := os.MkdirAll(filepath.Dir(targetPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory for download: %w", err)
	}

	resp, err := defaultHTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("http.Get failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("os.Create failed: %w", err)
	}

	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("io.Copy failed: %w", err)
	}

	return nil
}

func SanitizeFileName(name string) string {
	sanitized := invalidFilenameChars.ReplaceAllString(name, "")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.Trim(sanitized, " .")
	return sanitized
}

func SafeWorkshopFolderName(id int, name string) string {
	safe := SanitizeFileName(name)
	if safe == "" {
		return fmt.Sprintf("%d", id)
	}

	return fmt.Sprintf("%d_%s", id, safe)
}

func PathHasUsableFiles(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	found := false

	err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if info.Mode().IsRegular() && info.Size() > 0 {
			found = true
			return filepath.SkipAll
		}

		return nil
	})

	return err == nil && found
}

func RemovePathIfExists(path string) error {
	err := os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func RemoveFileIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func ReadMetaFile(dir string) (*ContentMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".meta"))
	if err != nil {
		return nil, err
	}

	var meta ContentMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func WriteMetaFile(dir string, meta *ContentMeta) error {
	if meta == nil {
		return fmt.Errorf("meta is nil")
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, ".meta"), append(data, '\n'), 0644)
}

func ValidateWorkshopContent(path string) (*ContentValidation, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}

	type fileEntry struct {
		rel  string
		size int64
	}

	var files []fileEntry
	var totalSize int64

	err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(path, current)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		files = append(files, fileEntry{
			rel:  rel,
			size: info.Size(),
		})

		totalSize += info.Size()

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no real files found")
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].rel < files[j].rel
	})

	h := sha256.New()

	for _, f := range files {
		if _, err = io.WriteString(h, f.rel); err != nil {
			return nil, err
		}

		if _, err = io.WriteString(h, "\n"); err != nil {
			return nil, err
		}

		if _, err = io.WriteString(h, fmt.Sprintf("%d", f.size)); err != nil {
			return nil, err
		}

		if _, err = io.WriteString(h, "\n"); err != nil {
			return nil, err
		}
	}

	return &ContentValidation{
		Hash:      hex.EncodeToString(h.Sum(nil)),
		HashMode:  "paths+size",
		FileCount: len(files),
		TotalSize: totalSize,
	}, nil
}

func FileIsUpToDate(filePath string, sourcePaths ...string) bool {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	latestSourceMod := time.Time{}

	for _, sourcePath := range sourcePaths {
		err = filepath.Walk(sourcePath, func(_ string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if info.ModTime().After(latestSourceMod) {
				latestSourceMod = info.ModTime()
			}

			return nil
		})

		if err != nil {
			return false
		}
	}

	if latestSourceMod.IsZero() {
		return false
	}

	return !fileInfo.ModTime().Before(latestSourceMod)
}
