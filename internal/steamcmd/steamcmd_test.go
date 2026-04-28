package steamcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lanifyapp/lanify-swdl/internal/fileutil"
	"github.com/lanifyapp/lanify-swdl/internal/steam"
)

func writeTestWorkshopContent(t *testing.T, root string, appID int, workshopID int) string {
	t.Helper()

	contentPath := filepath.Join(root, "steamapps", "workshop", "content", strconv.Itoa(appID), strconv.Itoa(workshopID))

	if err := os.MkdirAll(contentPath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(contentPath, "mod.txt")
	if err := os.WriteFile(filePath, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return contentPath
}

func TestSyncWorkshopItemsRepairsMissingMetaWithoutRedownload(t *testing.T) {
	root := t.TempDir()
	const appID = 730
	const workshopID = 12345

	contentPath := writeTestWorkshopContent(t, root, appID, workshopID)

	s := &SteamCMD{
		InstallPath: root,
		ExePath:     filepath.Join(root, "does-not-exist"),
	}

	results := s.SyncWorkshopItems(appID, []steam.WorkshopItem{
		{ID: workshopID, Title: "Cached Item"},
	})

	result := results[workshopID]
	if result.Err != nil {
		t.Fatalf("SyncWorkshopItems() err = %v", result.Err)
	}
	if !result.AlreadyExisted {
		t.Fatal("SyncWorkshopItems() AlreadyExisted = false, want true")
	}

	meta, err := fileutil.ReadMetaFile(contentPath)
	if err != nil {
		t.Fatalf("ReadMetaFile() error = %v", err)
	}

	if meta.WorkshopID != workshopID || meta.AppID != appID {
		t.Fatalf("meta ids = (%d, %d), want (%d, %d)", meta.WorkshopID, meta.AppID, workshopID, appID)
	}
	if meta.Name != "Cached Item" {
		t.Fatalf("meta name = %q, want %q", meta.Name, "Cached Item")
	}
}

func TestSyncWorkshopItemsRepairsBlankMetaNameWithoutRedownload(t *testing.T) {
	root := t.TempDir()
	const appID = 730
	const workshopID = 12345

	contentPath := writeTestWorkshopContent(t, root, appID, workshopID)
	validation, err := fileutil.ValidateWorkshopContent(contentPath)
	if err != nil {
		t.Fatalf("ValidateWorkshopContent() error = %v", err)
	}

	if err := fileutil.WriteMetaFile(contentPath, &fileutil.ContentMeta{
		WorkshopID: workshopID,
		AppID:      appID,
		Name:       "",
		Hash:       validation.Hash,
		HashMode:   validation.HashMode,
		FileCount:  validation.FileCount,
		TotalSize:  validation.TotalSize,
	}); err != nil {
		t.Fatalf("WriteMetaFile() error = %v", err)
	}

	s := &SteamCMD{
		InstallPath: root,
		ExePath:     filepath.Join(root, "does-not-exist"),
	}

	results := s.SyncWorkshopItems(appID, []steam.WorkshopItem{
		{ID: workshopID, Title: "Recovered Name"},
	})

	result := results[workshopID]
	if result.Err != nil {
		t.Fatalf("SyncWorkshopItems() err = %v", result.Err)
	}
	if !result.AlreadyExisted {
		t.Fatal("SyncWorkshopItems() AlreadyExisted = false, want true")
	}

	meta, err := fileutil.ReadMetaFile(contentPath)
	if err != nil {
		t.Fatalf("ReadMetaFile() error = %v", err)
	}

	if meta.Name != "Recovered Name" {
		t.Fatalf("meta name = %q, want %q", meta.Name, "Recovered Name")
	}
}
