package services

import (
	"os"
	"path/filepath"
	"testing"

	"antigravity-bridge/config"
)

func TestResolveSafePathAbsoluteAndRelative(t *testing.T) {
	config.InitConfig()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get user home dir")
	}

	workspace := home
	relPath := "Documents/test.txt"
	absPath := filepath.Join(home, relPath)

	// Test relative path
	safe1, root1, err1 := resolveSafePath(relPath, workspace)
	if err1 != nil {
		t.Fatalf("resolveSafePath relative failed: %v", err1)
	}
	if safe1 != absPath {
		t.Errorf("Expected safe1 %q, got %q", absPath, safe1)
	}
	if root1 != workspace {
		t.Errorf("Expected root1 %q, got %q", workspace, root1)
	}

	// Test absolute path (should NOT duplicate the root directory!)
	safe2, root2, err2 := resolveSafePath(absPath, workspace)
	if err2 != nil {
		t.Fatalf("resolveSafePath absolute failed: %v", err2)
	}
	if safe2 != absPath {
		t.Errorf("Expected safe2 %q, got %q (path duplication detected!)", absPath, safe2)
	}
	if root2 != workspace {
		t.Errorf("Expected root2 %q, got %q", workspace, root2)
	}

	// Test unauthorized outside path
	_, _, err3 := resolveSafePath("/etc/hosts", workspace)
	if err3 == nil {
		t.Errorf("Expected error for path outside workspace, got nil")
	}

	// Test duplicated root path (e.g. joined by client)
	dupPath := filepath.Join(workspace, workspace, relPath)
	safe4, _, err4 := resolveSafePath(dupPath, workspace)
	if err4 != nil {
		t.Fatalf("resolveSafePath duplicated failed: %v", err4)
	}
	if safe4 != absPath {
		t.Errorf("Expected deduplicated safe4 %q, got %q", absPath, safe4)
	}

	// Test quoted path (real-world LLM output: escaped quotes)
	quotedPath := "\"" + absPath + "\""
	safe5, _, err5 := resolveSafePath(quotedPath, workspace)
	if err5 != nil {
		t.Fatalf("resolveSafePath with quotes failed: %v", err5)
	}
	if safe5 != absPath {
		t.Errorf("Expected unquoted safe5 %q, got %q", absPath, safe5)
	}

	// Test file:// URI and anchor fragment
	uriPath := "file://" + absPath + "#L50-100"
	safe6, _, err6 := resolveSafePath(uriPath, workspace)
	if err6 != nil {
		t.Fatalf("resolveSafePath with URI and fragment failed: %v", err6)
	}
	if safe6 != absPath {
		t.Errorf("Expected cleaned safe6 %q, got %q", absPath, safe6)
	}
}
