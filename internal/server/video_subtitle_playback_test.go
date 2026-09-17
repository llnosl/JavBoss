package server

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"javboss/internal/models"
)

func TestExternalSubtitlePlaybackFilesReturnsExistingExternalTracks(t *testing.T) {
	root := t.TempDir()
	videoPath := filepath.Join(root, "ABC-001.mp4")
	externalPath := filepath.Join(root, "ABC-001.zh-cn.manual.srt")
	japanesePath := filepath.Join(root, "ABC-001.ja.generated.srt")
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalPath, []byte("subtitle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(japanesePath, []byte("subtitle"), 0o600); err != nil {
		t.Fatal(err)
	}

	location := &models.VideoLocation{
		RelativePath: "ABC-001.mp4",
		DirectoryRef: models.Directory{Path: root},
		Subtitles: []models.VideoSubtitle{
			{Kind: models.SubtitleKindEmbedded, StreamIndex: 2},
			{Kind: models.SubtitleKindExternal, Language: "ja", RelativePath: "ABC-001.ja.generated.srt"},
			{Kind: models.SubtitleKindExternal, Language: "zh-cn", RelativePath: "ABC-001.zh-cn.manual.srt"},
			{Kind: models.SubtitleKindExternal, RelativePath: "ABC-001.missing.srt"},
		},
	}

	got := externalSubtitlePlaybackFiles(location, videoPath)
	want := []string{externalPath, japanesePath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("externalSubtitlePlaybackFiles() = %v, want %v", got, want)
	}
}

func TestExternalSubtitlePlaybackFilesRejectsMismatchedVideo(t *testing.T) {
	root := t.TempDir()
	location := &models.VideoLocation{
		RelativePath: "ABC-001.mp4",
		DirectoryRef: models.Directory{Path: root},
		Subtitles: []models.VideoSubtitle{
			{Kind: models.SubtitleKindExternal, RelativePath: "ABC-001.srt"},
		},
	}

	if got := externalSubtitlePlaybackFiles(location, filepath.Join(root, "ABC-002.mp4")); got != nil {
		t.Fatalf("expected no subtitles for mismatched video, got %v", got)
	}
}
