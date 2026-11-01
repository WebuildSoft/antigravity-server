package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"antigravity-bridge/config"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	Modified string      `json:"modified"`
	Children []*FileNode `json:"children,omitempty"`
}

type DirectoryResult struct {
	CurrentPath string      `json:"currentPath"`
	Workspace   string      `json:"workspace"`
	Items       []*FileNode `json:"items"`
}

type FileContentResult struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Size        int64  `json:"size"`
	Modified    string `json:"modified"`
	IsEditable  bool   `json:"isEditable"`
	CurrentPath string `json:"currentPath"`
}

var ignoredNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	".dart_tool":   true,
	"build":        true,
	".gradle":      true,
	".idea":        true,
	".vscode":      true,
	"Pods":         true,
	".DS_Store":    true,
}

var textExtensions = map[string]bool{
	".dart": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".jsx": true,
	".json": true, ".yaml": true, ".yml": true, ".md": true, ".txt": true, ".html": true, ".css": true,
	".scss": true, ".xml": true, ".gradle": true, ".properties": true, ".sh": true, ".bash": true,
	".zsh": true, ".env": true, ".gitignore": true, ".c": true, ".cpp": true, ".h": true, ".py": true,
	".java": true, ".kt": true, ".swift": true, ".rb": true, ".php": true, ".sql": true, ".toml": true,
	".go": true, ".mod": true, ".sum": true,
}

func resolveSafePath(targetPath, workspaceRoot string) (string, error) {
	if workspaceRoot == "" {
		workspaceRoot = config.AppConfig.DefaultWorkspace
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}

	// Security: validate that workspaceRoot is within allowed default workspace boundary
	defaultRoot, err := filepath.Abs(config.AppConfig.DefaultWorkspace)
	if err == nil && defaultRoot != "" && defaultRoot != "/" {
		if root != defaultRoot && !strings.HasPrefix(root, defaultRoot+string(filepath.Separator)) {
			return "", errors.New("access denied: workspace is outside allowed boundary")
		}
	}

	resolved, err := filepath.Abs(filepath.Join(root, targetPath))
	if err != nil {
		return "", err
	}

	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", errors.New("access denied: path is outside workspace boundary")
	}
	return resolved, nil
}

func ListDirectory(targetPath, workspaceRoot string) (*DirectoryResult, error) {
	safePath, err := resolveSafePath(targetPath, workspaceRoot)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(safePath)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("target path is not a directory")
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, err
	}

	rootAbs, _ := filepath.Abs(workspaceRoot)
	relPath, err := filepath.Rel(rootAbs, safePath)
	if err != nil || relPath == "." {
		relPath = ""
	}

	var items []*FileNode
	for _, entry := range entries {
		name := entry.Name()
		if ignoredNames[name] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		itemRelPath := filepath.Join(relPath, name)
		items = append(items, &FileNode{
			Name:     name,
			Path:     itemRelPath,
			IsDir:    entry.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	// Sort: Directories first, then alphabetical
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return &DirectoryResult{
		CurrentPath: relPath,
		Workspace:   rootAbs,
		Items:       items,
	}, nil
}

func ReadFileContent(targetPath, workspaceRoot string) (*FileContentResult, error) {
	safePath, err := resolveSafePath(targetPath, workspaceRoot)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(safePath)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, errors.New("path is a directory, not a file")
	}

	const maxEditableSize = 5 * 1024 * 1024
	if fi.Size() > maxEditableSize {
		return nil, fmt.Errorf("file exceeds maximum editable size (5 MB): %d bytes", fi.Size())
	}

	ext := strings.ToLower(filepath.Ext(safePath))
	if !textExtensions[ext] {
		return nil, fmt.Errorf("file type not supported for text editing: %s", ext)
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, err
	}

	rootAbs, _ := filepath.Abs(workspaceRoot)
	relPath, _ := filepath.Rel(rootAbs, safePath)

	return &FileContentResult{
		Path:        relPath,
		Content:     string(data),
		Size:        fi.Size(),
		Modified:    fi.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		IsEditable:  true,
		CurrentPath: relPath,
	}, nil
}

func SaveFileContent(targetPath, content, workspaceRoot string) (*FileContentResult, error) {
	safePath, err := resolveSafePath(targetPath, workspaceRoot)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(safePath))
	if !textExtensions[ext] {
		return nil, fmt.Errorf("file type not supported for text saving: %s", ext)
	}

	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return nil, err
	}

	fi, err := os.Stat(safePath)
	if err != nil {
		return nil, err
	}

	rootAbs, _ := filepath.Abs(workspaceRoot)
	relPath, _ := filepath.Rel(rootAbs, safePath)

	return &FileContentResult{
		Path:        relPath,
		Content:     content,
		Size:        fi.Size(),
		Modified:    fi.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		IsEditable:  true,
		CurrentPath: relPath,
	}, nil
}
