package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"antigravity-bridge/config"
)

type UploadResult struct {
	Success  bool   `json:"success"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Path     string `json:"path"`
}

func RandomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func SaveBase64Image(dataURI string) (*UploadResult, error) {
	if dataURI == "" {
		return nil, errors.New("no image provided")
	}

	ext := ".jpg"
	var rawBase64 string

	if strings.HasPrefix(dataURI, "data:") {
		parts := strings.SplitN(dataURI, ",", 2)
		if len(parts) == 2 {
			header := parts[0]
			rawBase64 = parts[1]
			if strings.Contains(header, "png") {
				ext = ".png"
			} else if strings.Contains(header, "webp") {
				ext = ".webp"
			} else if strings.Contains(header, "gif") {
				ext = ".gif"
			}
		} else {
			rawBase64 = dataURI
		}
	} else {
		rawBase64 = dataURI
	}

	const maxUploadSize = 25 * 1024 * 1024 // 25 MB
	if len(rawBase64) > (maxUploadSize * 4 / 3) {
		return nil, fmt.Errorf("upload exceeds maximum allowed size (25 MB)")
	}

	decoded, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %v", err)
	}

	if len(decoded) > maxUploadSize {
		return nil, fmt.Errorf("upload exceeds maximum allowed size (25 MB)")
	}

	safeName := fmt.Sprintf("img_%d_%s%s", time.Now().UnixMilli(), RandomHex(4), ext)
	targetPath := filepath.Join(config.AppConfig.UploadsDir, safeName)

	if err := os.WriteFile(targetPath, decoded, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %v", err)
	}

	return &UploadResult{
		Success:  true,
		Filename: safeName,
		URL:      "/uploads/" + safeName,
		Path:     targetPath,
	}, nil
}

func GetUploadFilePath(filename string) (string, bool) {
	clean := filepath.Base(filename)
	full := filepath.Join(config.AppConfig.UploadsDir, clean)
	fi, err := os.Stat(full)
	if err == nil && !fi.IsDir() {
		return full, true
	}
	return "", false
}

func GetMediaFilePath(rawPath string) (string, bool) {
	if rawPath == "" {
		return "", false
	}
	clean := strings.TrimPrefix(rawPath, "file://")
	absPath, err := filepath.Abs(clean)
	if err != nil {
		return "", false
	}

	// Security: Prevent Directory Traversal / Arbitrary File Read.
	// Only serve media from allowed directories: UploadsDir or BrainDir.
	uploadsAbs, _ := filepath.Abs(config.AppConfig.UploadsDir)
	brainAbs, _ := filepath.Abs(config.AppConfig.BrainDir)

	isUnderUploads := uploadsAbs != "" && (absPath == uploadsAbs || strings.HasPrefix(absPath, uploadsAbs+string(filepath.Separator)))
	isUnderBrain := brainAbs != "" && (absPath == brainAbs || strings.HasPrefix(absPath, brainAbs+string(filepath.Separator)))

	if !isUnderUploads && !isUnderBrain {
		return "", false
	}

	fi, err := os.Stat(absPath)
	if err == nil && !fi.IsDir() {
		return absPath, true
	}
	return "", false
}
