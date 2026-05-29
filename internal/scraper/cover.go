package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DownloadCover(thumbDir, gameID, coverURL string) (string, error) {
	if coverURL == "" {
		return "", fmt.Errorf("no cover URL")
	}

	resp, err := http.Get(coverURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cover download: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", err
	}

	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	filename := fmt.Sprintf("%s_%d%s", gameID, time.Now().Unix(), ext)
	filePath := filepath.Join(thumbDir, filename)

	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		os.Remove(filePath)
		return "", err
	}

	return ToFileURL(filePath), nil
}

func ToFileURL(absPath string) string {
	p := filepath.ToSlash(absPath)
	p = strings.TrimPrefix(p, "/")
	return "file:///" + p
}
