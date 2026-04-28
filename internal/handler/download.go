package handler

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lanifyapp/lanify-swdl/internal/fileutil"
	"github.com/lanifyapp/lanify-swdl/internal/steam"
	"github.com/lanifyapp/lanify-swdl/internal/steamcmd"
	"github.com/schollz/progressbar/v3"
	"github.com/valyala/fasthttp"
)

func newCollectionProgressBar(collectionID int, totalItems int) *progressbar.ProgressBar {
	description := fmt.Sprintf("Syncing collection %d", collectionID)

	return progressbar.NewOptions(
		totalItems,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionThrottle(100*time.Millisecond),
	)
}

func intPathParam(ctx *fasthttp.RequestCtx, name string) (int, error) {
	value, ok := ctx.UserValue(name).(string)
	if !ok {
		return 0, fmt.Errorf("missing path parameter %q", name)
	}

	return strconv.Atoi(value)
}

func sendText(ctx *fasthttp.RequestCtx, statusCode int, format string, args ...any) {
	ctx.SetStatusCode(statusCode)
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetBodyString(fmt.Sprintf(format, args...))
}

func sendFileAttachment(ctx *fasthttp.RequestCtx, path string, filename string) {
	ctx.Response.Header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.SendFile(path)
}

func (h *App) DownloadWorkshopHandler(ctx *fasthttp.RequestCtx) {
	appID, err := intPathParam(ctx, "app_id")
	if err != nil {
		sendText(ctx, fasthttp.StatusBadRequest, "Invalid App ID.")
		return
	}

	workshopID, err := intPathParam(ctx, "workshop_id")
	if err != nil {
		sendText(ctx, fasthttp.StatusBadRequest, "Invalid Workshop ID.")
		return
	}

	sourcePath := h.steamcmd.GetWorkshopContentPath(appID, workshopID)

	if _, err = h.steamcmd.SyncWorkshopItem(appID, workshopID); err != nil {
		sendText(ctx, fasthttp.StatusInternalServerError, "Failed to download or validate item: %v", err)
		return
	}

	if !fileutil.PathHasUsableFiles(sourcePath) {
		sendText(ctx, fasthttp.StatusInternalServerError, "Workshop content not found after sync.")
		return
	}

	workshopName := h.steamcmd.ResolveWorkshopName(workshopID, sourcePath, "")
	zipFileName := fileutil.SafeWorkshopFolderName(workshopID, workshopName) + ".zip"
	zipFilePath := filepath.Join(h.saveDirectory, zipFileName)

	if fileutil.FileIsUpToDate(zipFilePath, sourcePath) {
		sendFileAttachment(ctx, zipFilePath, zipFileName)
		return
	}

	if err = fileutil.RemoveFileIfExists(zipFilePath); err != nil {
		sendText(ctx, fasthttp.StatusInternalServerError, "Failed to replace zip archive: %v", err)
		return
	}

	if err = fileutil.ZipDirectory(sourcePath, zipFilePath); err != nil {
		sendText(ctx, fasthttp.StatusInternalServerError, "Failed to create zip archive: %v", err)
		return
	}

	sendFileAttachment(ctx, zipFilePath, zipFileName)
}

func chunkCollectionItems(items []steam.WorkshopItem, size int) [][]steam.WorkshopItem {
	if size <= 0 {
		size = 1
	}

	var chunks [][]steam.WorkshopItem

	for start := 0; start < len(items); start += size {
		end := start + size

		if end > len(items) {
			end = len(items)
		}

		chunks = append(chunks, items[start:end])
	}

	return chunks
}

func (h *App) DownloadCollectionHandler(ctx *fasthttp.RequestCtx) {
	appID, err := intPathParam(ctx, "app_id")
	if err != nil {
		sendText(ctx, fasthttp.StatusBadRequest, "Invalid App ID.")
		return
	}

	collectionID, err := intPathParam(ctx, "collection_id")
	if err != nil {
		sendText(ctx, fasthttp.StatusBadRequest, "Invalid Collection ID.")
		return
	}

	collectionTitle, items, err := steam.GetCollectionItems(collectionID)
	if err != nil {
		sendText(ctx, fasthttp.StatusNotFound, "Could not get collection items: %v", err)
		return
	}

	if len(items) == 0 {
		sendText(ctx, fasthttp.StatusNotFound, "Collection is empty or could not be found.")
		return
	}

	collectionSafeName := fileutil.SanitizeFileName(collectionTitle)
	if collectionSafeName == "" {
		collectionSafeName = fmt.Sprintf("%d", collectionID)
	}

	zipFileName := fmt.Sprintf("%d_%s_collection.zip", collectionID, collectionSafeName)
	zipFilePath := filepath.Join(h.saveDirectory, zipFileName)

	const batchSize = steamcmd.MaxBatchDownloads

	var sources []fileutil.ZipSource
	var sourceDirs []string

	batches := chunkCollectionItems(items, batchSize)
	progress := newCollectionProgressBar(collectionID, len(items))

	for batchIndex, batch := range batches {
		progress.Describe(fmt.Sprintf(
			"Syncing collection %d batch %d/%d",
			collectionID,
			batchIndex+1,
			len(batches),
		))

		results := h.steamcmd.SyncWorkshopItems(appID, batch)

		for _, item := range batch {
			res, ok := results[item.ID]
			if !ok {
				slog.Warn("missing sync result", "workshop_id", item.ID, "title", item.Title)
				continue
			}

			if res.Err != nil {
				slog.Warn(
					"failed to sync workshop item",
					"workshop_id",
					item.ID,
					"title",
					item.Title,
					"error",
					res.Err,
				)
				continue
			}

			itemPath := h.steamcmd.GetWorkshopContentPath(appID, item.ID)
			if !fileutil.PathHasUsableFiles(itemPath) {
				slog.Warn("synced item has no usable files", "workshop_id", item.ID)
				continue
			}

			itemName := h.steamcmd.ResolveWorkshopName(item.ID, itemPath, item.Title)

			sources = append(sources, fileutil.ZipSource{
				Path:  itemPath,
				Alias: fileutil.SafeWorkshopFolderName(item.ID, itemName),
			})
			sourceDirs = append(sourceDirs, itemPath)
		}

		if err = progress.Add(len(batch)); err != nil {
			slog.Warn(
				"failed to advance collection progress",
				"collection_id",
				collectionID,
				"batch",
				batchIndex+1,
				"error",
				err,
			)
		}
	}

	if err = progress.Finish(); err != nil {
		slog.Warn("failed to finalize collection progress", "collection_id", collectionID, "error", err)
	}

	if len(sources) == 0 {
		sendText(ctx, fasthttp.StatusInternalServerError, "No collection items were available after sync.")
		return
	}

	if fileutil.FileIsUpToDate(zipFilePath, sourceDirs...) {
		sendFileAttachment(ctx, zipFilePath, zipFileName)
		return
	}

	if err = fileutil.RemoveFileIfExists(zipFilePath); err != nil {
		sendText(ctx, fasthttp.StatusInternalServerError, "Failed to replace collection zip: %v", err)
		return
	}

	if err = fileutil.ZipMultipleDirectories(sources, zipFilePath); err != nil {
		sendText(ctx, fasthttp.StatusInternalServerError, "Failed to create collection zip: %v", err)
		return
	}

	sendFileAttachment(ctx, zipFilePath, zipFileName)
}
