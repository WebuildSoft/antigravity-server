package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// ExpandHome expande com segurança "~" ou caminhos com prefixo "~/" ou "~\"
// para o diretório pessoal do usuário de forma agnóstica de SO (Linux, macOS e Windows).
func ExpandHome(pathStr string) (string, error) {
	clean := strings.TrimSpace(pathStr)
	if clean == "" {
		return "", nil
	}
	if clean == "~" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return "", errors.New("unable to determine user home directory")
		}
		return filepath.Clean(home), nil
	}
	if strings.HasPrefix(clean, "~/") || strings.HasPrefix(clean, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return "", errors.New("unable to determine user home directory")
		}
		return filepath.Clean(filepath.Join(home, clean[2:])), nil
	}
	return filepath.Clean(clean), nil
}

func resolveSafePath(targetPath, workspaceRoot string) (string, string, error) {
	// 1. Sanitização defensiva contra aspas, apóstrofos, crases ou espaços residuais gerados por LLMs
	workspaceRoot = strings.Trim(strings.TrimSpace(workspaceRoot), " \t\r\n\"'`")
	targetPath = strings.Trim(strings.TrimSpace(targetPath), " \t\r\n\"'`")

	// 2. Remove esquema file:// se presente
	if strings.HasPrefix(targetPath, "file://") {
		targetPath = strings.TrimPrefix(targetPath, "file://")
	}

	// 3. Remove sufixo de âncora/linha (#L10-20 ou #10)
	if idx := strings.Index(targetPath, "#L"); idx != -1 {
		targetPath = targetPath[:idx]
	} else if idx := strings.Index(targetPath, "#"); idx != -1 {
		targetPath = targetPath[:idx]
	}

	// Se o workspace vier vazio, "~" ou ".", utiliza o DefaultWorkspace
	if workspaceRoot == "" || workspaceRoot == "~" || workspaceRoot == "." {
		workspaceRoot = config.AppConfig.DefaultWorkspace
	} else {
		// Tenta expandir o til se o usuário passou algo como "~/projetos"
		expanded, expErr := ExpandHome(workspaceRoot)
		if expErr == nil && expanded != "" {
			workspaceRoot = expanded
		}
	}

	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", fmt.Errorf("invalid workspace root: %w", err)
	}
	root = filepath.Clean(root)

	// Security: validate that workspaceRoot is within allowed default workspace boundary
	defaultRoot, err := filepath.Abs(config.AppConfig.DefaultWorkspace)
	if err == nil && defaultRoot != "" && defaultRoot != "/" {
		dComp := defaultRoot
		rComp := root
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			dComp = strings.ToLower(dComp)
			rComp = strings.ToLower(rComp)
		}
		if rComp != dComp && !strings.HasPrefix(rComp, dComp+string(filepath.Separator)) {
			return "", "", errors.New("access denied: workspace is outside allowed boundary")
		}
	}

	// Tratar targetPath se vier como "~", "." ou com prefixo de til
	if targetPath == "~" || targetPath == "." {
		targetPath = ""
	} else if strings.HasPrefix(targetPath, "~/") || strings.HasPrefix(targetPath, `~\`) {
		targetPath = targetPath[2:]
	}

	// Tratar caminho Windows com barra inicial (ex: "/C:/Users/..." -> "C:/Users/...")
	cleanTarget := strings.Trim(targetPath, " \t\r\n\"'`")
	if len(cleanTarget) >= 3 && (cleanTarget[0] == '/' || cleanTarget[0] == '\\') && cleanTarget[2] == ':' {
		cleanTarget = cleanTarget[1:]
	}

	// Sanitiza targetPath com filepath.Clean
	cleanRel := filepath.Clean(filepath.FromSlash(cleanTarget))
	cleanRel = strings.Trim(cleanRel, " \t\r\n\"'`")
	if cleanRel == "." || cleanRel == "/" || cleanRel == `\` {
		cleanRel = ""
	}

	var resolved string
	isAbs := filepath.IsAbs(cleanRel)
	// No Windows, caminhos como "C:\..." têm drive letter
	if !isAbs && len(cleanRel) >= 2 && cleanRel[1] == ':' {
		isAbs = true
	}

	if isAbs {
		resolved = cleanRel
	} else {
		resolved = filepath.Join(root, cleanRel)
	}

	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", "", err
	}
	resolved = filepath.Clean(resolved)

	// Proteção contra duplicação acidental de raiz (ex: /workspace/workspace/subpath)
	dupRoot := filepath.Join(root, root)
	if strings.HasPrefix(resolved, dupRoot) {
		resolved = filepath.Join(root, strings.TrimPrefix(resolved, dupRoot))
	}

	// Validação de fronteira (boundary check) compatível com qualquer SO
	rootComp := root
	resComp := resolved
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		rootComp = strings.ToLower(rootComp)
		resComp = strings.ToLower(resComp)
	}

	sep := string(filepath.Separator)
	if resComp != rootComp && !strings.HasPrefix(resComp, rootComp+sep) {
		return "", "", errors.New("access denied: path is outside workspace boundary")
	}
	return resolved, root, nil
}

func ListDirectory(targetPath, workspaceRoot string) (*DirectoryResult, error) {
	safePath, rootAbs, err := resolveSafePath(targetPath, workspaceRoot)
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
	safePath, rootAbs, err := resolveSafePath(targetPath, workspaceRoot)
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
	safePath, rootAbs, err := resolveSafePath(targetPath, workspaceRoot)
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
