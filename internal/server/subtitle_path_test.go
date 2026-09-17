package server

import (
	"path/filepath"
	"testing"
)

func TestNormalizeSubtitlePathWithinRoot(t *testing.T) {
	root := t.TempDir()
	wantFullPath := filepath.Join(root, "subtitles", "ABC-001.srt")

	relativePath, fullPath, err := normalizeSubtitlePathWithinRoot(
		filepath.Join("subtitles", "ABC-001.srt"),
		root,
	)
	if err != nil {
		t.Fatalf("normalizeSubtitlePathWithinRoot() error = %v", err)
	}
	if relativePath != "subtitles/ABC-001.srt" {
		t.Fatalf("relative path = %q", relativePath)
	}
	if fullPath != wantFullPath {
		t.Fatalf("full path = %q, want %q", fullPath, wantFullPath)
	}
}

func TestNormalizeSubtitlePathWithinRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, _, err := normalizeSubtitlePathWithinRoot(filepath.Join("..", "outside.srt"), root); err == nil {
		t.Fatal("expected path outside the media root to be rejected")
	}
}
