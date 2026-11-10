package pipeline

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxScreenshotBytes = 4 << 20

var screenshotID = regexp.MustCompile("^[a-f0-9]{64}\\.(png|jpg)$")

func (e *Engine) screenshotPath(id string) (string, error) {
	if !screenshotID.MatchString(id) {
		return "", ErrNotFound
	}
	return filepath.Join(filepath.Dir(e.journal.Name()), "screenshots", id), nil
}

func (e *Engine) SaveScreenshot(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxScreenshotBytes {
		return "", fmt.Errorf("%w: screenshot must be at most 4 MB", ErrValidation)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") ||
		cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 ||
		int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return "", fmt.Errorf("%w: use a PNG or JPEG screenshot up to 16 megapixels", ErrValidation)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("%w: the screenshot could not be decoded", ErrValidation)
	}
	extension := format
	if format == "jpeg" {
		extension = "jpg"
	}
	id := fmt.Sprintf("%x.%s", sha256.Sum256(data), extension)
	path, _ := e.screenshotPath(id)
	if _, err := os.Stat(path); err == nil {
		return id, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		if _, existing := os.Stat(path); existing != nil {
			return "", err
		}
	}
	return id, nil
}

func (e *Engine) Screenshot(id string) ([]byte, string, error) {
	path, err := e.screenshotPath(id)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	contentType := "image/png"
	if strings.HasSuffix(id, ".jpg") {
		contentType = "image/jpeg"
	}
	return data, contentType, nil
}
