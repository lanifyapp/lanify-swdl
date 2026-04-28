package steamcmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lanifyapp/lanify-swdl/internal/fileutil"
	"github.com/lanifyapp/lanify-swdl/internal/steam"
)

const MaxBatchDownloads = 10

type SteamCMD struct {
	InstallPath string
	ExePath     string
	username    string
	password    string
	mu          sync.Mutex
}

type SyncResult struct {
	AlreadyExisted bool
	Err            error
}

func New(installPath string, username string, password string) (*SteamCMD, error) {
	resolvedInstallPath, err := resolveInstallPath(installPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve steamcmd path: %w", err)
	}

	absInstallPath, err := filepath.Abs(resolvedInstallPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute install path: %w", err)
	}

	exeName := "steamcmd"
	switch runtime.GOOS {
	case "windows":
		exeName += ".exe"
	case "linux":
		exeName += ".sh"
	}

	return &SteamCMD{
		InstallPath: absInstallPath,
		ExePath:     filepath.Join(absInstallPath, exeName),
		username:    strings.TrimSpace(username),
		password:    password,
	}, nil
}

func resolveInstallPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "steamcmd"
	}

	path = expandHome(path)

	if runtime.GOOS == "linux" && path == "steamcmd" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Steam"), nil
	}

	return path, nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}

	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}

func (s *SteamCMD) Install() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.ExePath); err == nil {
		slog.Info("steamcmd already installed", "path", s.ExePath)
		return nil
	}

	slog.Info("installing steamcmd", "path", s.InstallPath)

	switch runtime.GOOS {
	case "windows":
		return s.installWindows()
	default:
		return s.installLinux()
	}
}

func (s *SteamCMD) installWindows() error {
	url := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"
	archivePath := filepath.Join(s.InstallPath, "steamcmd.zip")

	defer os.Remove(archivePath)

	if err := fileutil.DownloadFile(url, archivePath); err != nil {
		return fmt.Errorf("failed to download steamcmd for Windows: %w", err)
	}

	if err := fileutil.Unzip(archivePath, s.InstallPath); err != nil {
		return fmt.Errorf("failed to unzip steamcmd: %w", err)
	}

	return s.finalizeInstallation()
}

func (s *SteamCMD) installLinux() error {
	url := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"
	archivePath := filepath.Join(s.InstallPath, "steamcmd.tar.gz")

	defer os.Remove(archivePath)

	if err := fileutil.DownloadFile(url, archivePath); err != nil {
		return fmt.Errorf("failed to download steamcmd for Linux: %w", err)
	}

	if err := fileutil.UntarGz(archivePath, s.InstallPath); err != nil {
		return fmt.Errorf("failed to extract steamcmd: %w", err)
	}

	if err := os.Chmod(s.ExePath, 0755); err != nil {
		return fmt.Errorf("failed to make steamcmd executable: %w", err)
	}

	return s.finalizeInstallation()
}

func (s *SteamCMD) finalizeInstallation() error {
	cmd := s.command("+quit")
	cmd.Dir = s.InstallPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		slog.Warn(
			"steamcmd finalization returned non-zero exit status",
			"error",
			err,
		)
	}

	return nil
}

func (s *SteamCMD) command(args ...string) *exec.Cmd {
	if runtime.GOOS == "linux" {
		cmdArgs := append([]string{s.ExePath}, args...)
		return exec.Command("bash", cmdArgs...)
	}

	return exec.Command(s.ExePath, args...)
}

func (s *SteamCMD) loginArgs() []string {
	if s.username != "" && s.password != "" {
		return []string{"+login", s.username, s.password}
	}
	return []string{"+login", "anonymous"}
}

func (s *SteamCMD) downloadWorkshopItemsLocked(appID int, workshopIDs []int, validate bool) error {
	if len(workshopIDs) == 0 {
		return nil
	}

	args := []string{
		"+force_install_dir", s.InstallPath,
	}

	args = append(args, s.loginArgs()...)

	for _, workshopID := range workshopIDs {
		args = append(args, "+workshop_download_item", fmt.Sprint(appID), fmt.Sprint(workshopID))
		if validate {
			args = append(args, "validate")
		}
	}

	args = append(args, "+quit")

	run := func() error {
		cmd := s.command(args...)
		cmd.Dir = s.InstallPath
		cmd.Stdout = io.Discard
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if err := run(); err != nil {
		time.Sleep(2 * time.Second)
		if retryErr := run(); retryErr != nil {
			return fmt.Errorf("steamcmd execution failed after retry: %w", retryErr)
		}
	}

	return nil
}

func (s *SteamCMD) DownloadWorkshopItem(appID, workshopID int, validate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.downloadWorkshopItemsLocked(appID, []int{workshopID}, validate)
}

func (s *SteamCMD) DownloadWorkshopItems(appID int, workshopIDs []int, validate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for start := 0; start < len(workshopIDs); start += MaxBatchDownloads {
		end := start + MaxBatchDownloads
		if end > len(workshopIDs) {
			end = len(workshopIDs)
		}

		if err := s.downloadWorkshopItemsLocked(appID, workshopIDs[start:end], validate); err != nil {
			return err
		}
	}

	return nil
}

func (s *SteamCMD) ResolveWorkshopName(workshopID int, contentPath string, fallback string) string {
	if meta, err := fileutil.ReadMetaFile(contentPath); err == nil && strings.TrimSpace(meta.Name) != "" {
		return strings.TrimSpace(meta.Name)
	}

	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}

	name, err := steam.GetWorkshopName(workshopID)
	if err != nil {
		slog.Warn("failed to scrape workshop title", "workshop_id", workshopID, "error", err)
		return ""
	}

	return strings.TrimSpace(name)
}

func (s *SteamCMD) writeMeta(appID, workshopID int, contentPath string, preferredName string) error {
	validation, err := fileutil.ValidateWorkshopContent(contentPath)
	if err != nil {
		return err
	}

	meta := &fileutil.ContentMeta{
		WorkshopID:  workshopID,
		AppID:       appID,
		Name:        s.ResolveWorkshopName(workshopID, contentPath, preferredName),
		Hash:        validation.Hash,
		HashMode:    validation.HashMode,
		FileCount:   validation.FileCount,
		TotalSize:   validation.TotalSize,
		ValidatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err = fileutil.WriteMetaFile(contentPath, meta); err != nil {
		return fmt.Errorf("failed to write .meta: %w", err)
	}

	return nil
}

func (s *SteamCMD) ValidateWorkshopItem(appID, workshopID int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	contentPath := s.GetWorkshopContentPath(appID, workshopID)

	validation, err := fileutil.ValidateWorkshopContent(contentPath)
	if err != nil {
		return false, err
	}

	meta, err := fileutil.ReadMetaFile(contentPath)
	if err != nil {
		return false, nil
	}

	if meta.WorkshopID != workshopID || meta.AppID != appID {
		return false, nil
	}

	if meta.Hash != validation.Hash {
		return false, nil
	}

	if meta.FileCount != validation.FileCount {
		return false, nil
	}

	if meta.TotalSize != validation.TotalSize {
		return false, nil
	}

	return true, nil
}

func (s *SteamCMD) isItemCachedValidFast(appID, workshopID int, contentPath string) bool {
	if !fileutil.PathHasUsableFiles(contentPath) {
		return false
	}

	meta, err := fileutil.ReadMetaFile(contentPath)
	if err != nil {
		return false
	}

	if meta.WorkshopID != workshopID || meta.AppID != appID {
		return false
	}

	if strings.TrimSpace(meta.Name) == "" {
		return false
	}

	if meta.FileCount <= 0 || meta.TotalSize <= 0 {
		return false
	}

	return true
}

func (s *SteamCMD) repairWorkshopMeta(appID int, item steam.WorkshopItem, contentPath string) (bool, error) {
	validation, err := fileutil.ValidateWorkshopContent(contentPath)
	if err != nil {
		return false, fmt.Errorf("content validation failed: %w", err)
	}

	meta, metaErr := fileutil.ReadMetaFile(contentPath)
	if metaErr == nil &&
		meta.WorkshopID == item.ID &&
		meta.AppID == appID &&
		meta.Hash == validation.Hash &&
		meta.FileCount == validation.FileCount &&
		meta.TotalSize == validation.TotalSize &&
		strings.TrimSpace(meta.Name) != "" {
		return true, nil
	}

	if err = s.writeMeta(appID, item.ID, contentPath, item.Title); err != nil {
		return true, fmt.Errorf("failed to repair .meta for %d: %w", item.ID, err)
	}

	return true, nil
}

func detectCachedItemUpdateReason(appID int, item steam.WorkshopItem, contentPath string) string {
	validation, err := fileutil.ValidateWorkshopContent(contentPath)
	if err != nil {
		return ""
	}

	meta, err := fileutil.ReadMetaFile(contentPath)
	if err != nil {
		return ""
	}

	switch {
	case meta.WorkshopID != item.ID || meta.AppID != appID:
		return fmt.Sprintf("metadata targets app=%d workshop=%d", meta.AppID, meta.WorkshopID)
	case meta.Hash != validation.Hash:
		return "content hash changed"
	case meta.FileCount != validation.FileCount:
		return fmt.Sprintf("file count changed from %d to %d", meta.FileCount, validation.FileCount)
	case meta.TotalSize != validation.TotalSize:
		return fmt.Sprintf("total size changed from %d to %d bytes", meta.TotalSize, validation.TotalSize)
	default:
		return ""
	}
}

func (s *SteamCMD) shouldReuseCachedItem(appID int, item steam.WorkshopItem, contentPath string) bool {
	if s.isItemCachedValidFast(appID, item.ID, contentPath) {
		return true
	}

	if !fileutil.PathHasUsableFiles(contentPath) {
		return false
	}

	valid, err := s.repairWorkshopMeta(appID, item, contentPath)
	if err != nil {
		slog.Warn(
			"cached workshop item appears corrupted; redownloading",
			"workshop_id",
			item.ID,
			"title",
			item.Title,
			"error",
			err,
		)
		return false
	}

	return valid
}

func (s *SteamCMD) prepareWorkshopItem(appID int, item steam.WorkshopItem, results map[int]SyncResult) bool {
	contentPath := s.GetWorkshopContentPath(appID, item.ID)
	alreadyExisted := fileutil.PathHasUsableFiles(contentPath)
	results[item.ID] = SyncResult{AlreadyExisted: alreadyExisted}

	if s.shouldReuseCachedItem(appID, item, contentPath) {
		return false
	}

	if !alreadyExisted {
		return true
	}

	if reason := detectCachedItemUpdateReason(appID, item, contentPath); reason != "" {
		slog.Info(
			"cached workshop item is outdated; redownloading",
			"workshop_id",
			item.ID,
			"title",
			item.Title,
			"reason",
			reason,
		)
	}

	if err := fileutil.RemovePathIfExists(contentPath); err != nil {
		result := results[item.ID]
		result.Err = fmt.Errorf("failed to remove invalid workshop path: %w", err)
		results[item.ID] = result
		return false
	}

	return true
}

func (s *SteamCMD) finalizeDownloadedWorkshopItem(appID int, item steam.WorkshopItem, results map[int]SyncResult) {
	contentPath := s.GetWorkshopContentPath(appID, item.ID)

	if _, err := fileutil.ValidateWorkshopContent(contentPath); err != nil {
		slog.Warn(
			"downloaded workshop item is invalid after sync; retrying once",
			"workshop_id",
			item.ID,
			"title",
			item.Title,
			"error",
			err,
		)

		_ = fileutil.RemovePathIfExists(contentPath)

		if retryErr := s.downloadWorkshopItemsLocked(appID, []int{item.ID}, true); retryErr != nil {
			result := results[item.ID]
			result.Err = fmt.Errorf("steamcmd execution failed for %d after retry: %w", item.ID, retryErr)
			results[item.ID] = result
			return
		}

		if _, err := fileutil.ValidateWorkshopContent(contentPath); err != nil {
			result := results[item.ID]
			result.Err = fmt.Errorf("download completed but workshop content is invalid for %d: %w", item.ID, err)
			results[item.ID] = result
			return
		}
	}

	if err := s.writeMeta(appID, item.ID, contentPath, item.Title); err != nil {
		result := results[item.ID]
		result.Err = err
		results[item.ID] = result
	}
}

func (s *SteamCMD) syncWorkshopItemsLocked(appID int, workshopItems []steam.WorkshopItem) map[int]SyncResult {
	results := make(map[int]SyncResult, len(workshopItems))
	seen := make(map[int]struct{})
	var toDownload []steam.WorkshopItem

	for _, item := range workshopItems {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}

		if s.prepareWorkshopItem(appID, item, results) {
			toDownload = append(toDownload, item)
		}
	}

	for start := 0; start < len(toDownload); start += MaxBatchDownloads {
		end := start + MaxBatchDownloads
		if end > len(toDownload) {
			end = len(toDownload)
		}

		batch := toDownload[start:end]
		batchIDs := make([]int, 0, len(batch))
		for _, item := range batch {
			batchIDs = append(batchIDs, item.ID)
		}

		if err := s.downloadWorkshopItemsLocked(appID, batchIDs, true); err != nil {
			slog.Warn("batch download returned error", "workshop_ids", batchIDs, "error", err)
		}

		for _, item := range batch {
			s.finalizeDownloadedWorkshopItem(appID, item, results)
		}
	}

	return results
}

func (s *SteamCMD) SyncWorkshopItem(appID, workshopID int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := s.syncWorkshopItemsLocked(appID, []steam.WorkshopItem{
		{ID: workshopID},
	})
	result := results[workshopID]
	return result.AlreadyExisted, result.Err
}

func (s *SteamCMD) SyncWorkshopItems(appID int, workshopItems []steam.WorkshopItem) map[int]SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.syncWorkshopItemsLocked(appID, workshopItems)
}

func (s *SteamCMD) WorkshopItemExists(appID, workshopID int) bool {
	path := s.GetWorkshopContentPath(appID, workshopID)
	return fileutil.PathHasUsableFiles(path)
}

func (s *SteamCMD) GetWorkshopContentPath(appID, workshopID int) string {
	return filepath.Join(s.InstallPath, "steamapps", "workshop", "content", fmt.Sprint(appID), fmt.Sprint(workshopID))
}
