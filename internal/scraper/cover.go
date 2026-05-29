package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func DownloadCover(gameDir, coverURL string) error {
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

	filePath := filepath.Join(gameDir, "cover"+ext)

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

	m := "cover" + ext
	_ = m

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
