package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"GameLibrary/internal/logger"
)

func DownloadCover(gameDir, coverURL, filename string) error {
	if coverURL == "" {
		return fmt.Errorf("no cover URL")
	}

	resp, err := http.Get(coverURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover download: HTTP %d", resp.StatusCode)
	}

	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	filePath := filepath.Join(gameDir, filename+ext)

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		os.Remove(filePath)
		return err
	}

	return nil
}

func CoverPath(gameDir string) string {
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(gameDir, "cover"+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func CoverLandscapePath(gameDir string) string {
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(gameDir, "cover_landscape"+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func DownloadCoverWithLog(gameDir, gameID, coverURL, filename string) {
	err := DownloadCover(gameDir, coverURL, filename)
	logger.CoverDownloaded(gameID, filename, coverURL, err)
}
