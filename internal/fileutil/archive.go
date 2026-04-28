package fileutil

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type ZipSource struct {
	Path  string
	Alias string
}

func ZipDirectory(sourcePath, targetZipPath string) error {
	return ZipMultipleDirectories([]ZipSource{
		{
			Path:  sourcePath,
			Alias: filepath.Base(sourcePath),
		},
	}, targetZipPath)
}

func ZipMultipleDirectories(sources []ZipSource, targetZipPath string) error {
	slog.Info("creating zip archive", "target", targetZipPath)

	targetAbs, err := filepath.Abs(targetZipPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target zip path: %w", err)
	}

	targetDir := filepath.Dir(targetAbs)
	if err = os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(targetDir, ".zip-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp zip file: %w", err)
	}

	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	defer cleanup()

	tmpAbs, err := filepath.Abs(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to resolve temp zip path: %w", err)
	}

	archive := zip.NewWriter(tmpFile)

	for _, source := range sources {
		if source.Alias == "" {
			source.Alias = filepath.Base(source.Path)
		}

		info, statErr := os.Stat(source.Path)
		if os.IsNotExist(statErr) {
			slog.Warn("skipping missing source path", "path", source.Path)
			continue
		}

		if statErr != nil {
			return fmt.Errorf("failed to stat source path %s: %w", source.Path, statErr)
		}
		if !info.IsDir() {
			return fmt.Errorf("source path is not a directory: %s", source.Path)
		}

		base := filepath.Clean(source.Path)

		err = filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			pathAbs, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("failed to resolve path %s: %w", path, err)
			}

			cleanPathAbs := filepath.Clean(pathAbs)

			if cleanPathAbs == filepath.Clean(targetAbs) || cleanPathAbs == filepath.Clean(tmpAbs) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return fmt.Errorf("failed to get relative path for %s: %w", path, err)
			}

			name := source.Alias
			if relPath != "." {
				name = filepath.ToSlash(filepath.Join(source.Alias, relPath))
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return fmt.Errorf("failed to create zip header for %s: %w", path, err)
			}

			header.Name = name
			if info.IsDir() {
				header.Name += "/"
			} else {
				header.Method = zip.Deflate
			}

			writer, err := archive.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("failed to create zip entry for %s: %w", path, err)
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}

			_, err = io.Copy(writer, file)
			closeErr := file.Close()

			if err != nil {
				return fmt.Errorf("failed to write file %s to zip: %w", path, err)
			}

			if closeErr != nil {
				return fmt.Errorf("failed to close source file %s: %w", path, closeErr)
			}

			return nil
		})

		if err != nil {
			_ = archive.Close()
			return err
		}
	}

	if err = archive.Close(); err != nil {
		return fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp zip file: %w", err)
	}

	if err = os.Rename(tmpPath, targetZipPath); err != nil {
		return fmt.Errorf("failed to move temp zip into place: %w", err)
	}

	return nil
}

func Unzip(sourceZipPath, destination string) error {
	slog.Info("extracting zip", "source", sourceZipPath, "destination", destination)

	r, err := zip.OpenReader(sourceZipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}

	defer r.Close()

	base := filepath.Clean(destination) + string(os.PathSeparator)

	for _, f := range r.File {
		targetPath := filepath.Join(destination, f.Name)

		if !strings.HasPrefix(filepath.Clean(targetPath), base) {
			return fmt.Errorf("illegal file path: %s", targetPath)
		}

		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(targetPath), os.ModePerm); err != nil {
			return err
		}

		dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		src, err := f.Open()
		if err != nil {
			_ = dst.Close()
			return err
		}

		_, copyErr := io.Copy(dst, src)
		closeErr1 := src.Close()
		closeErr2 := dst.Close()

		if copyErr != nil {
			return copyErr
		}
		if closeErr1 != nil {
			return closeErr1
		}
		if closeErr2 != nil {
			return closeErr2
		}
	}

	return nil
}

func UntarGz(sourceTarballPath, destination string) error {
	slog.Info("extracting tarball", "source", sourceTarballPath, "destination", destination)

	file, err := os.Open(sourceTarballPath)
	if err != nil {
		return fmt.Errorf("failed to open tarball: %w", err)
	}

	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}

	defer gzr.Close()

	tr := tar.NewReader(gzr)
	base := filepath.Clean(destination) + string(os.PathSeparator)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(destination, header.Name)

		if !strings.HasPrefix(filepath.Clean(target), base) {
			return fmt.Errorf("illegal file path in tarball: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()

			if copyErr != nil {
				return fmt.Errorf("failed to write file: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}

	return nil
}
