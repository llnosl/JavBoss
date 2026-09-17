package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalSubtitleRecordsMatchVideoStemAndInferLanguage(t *testing.T) {
	root := t.TempDir()
	videoPath := filepath.Join(root, "IPX-001.mkv")
	for _, name := range []string{
		"IPX-001.mkv",
		"ipx-001.zh-CN.ass",
		"IPX-001-英文.srt",
		"IPX-001.jpg",
		"IPX-002.zh-CN.srt",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	items, err := externalSubtitleRecords(root, videoPath, subtitleDirectoryCache{})
	if err != nil {
		t.Fatalf("scan external subtitles: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("subtitles = %#v, want two", items)
	}
	byPath := make(map[string]int, len(items))
	for index, item := range items {
		byPath[item.RelativePath] = index
	}
	chinese := items[byPath["ipx-001.zh-CN.ass"]]
	if chinese.Language != "zh-cn" || chinese.Codec != "ass" {
		t.Fatalf("Chinese subtitle = %#v", chinese)
	}
	english := items[byPath["IPX-001-英文.srt"]]
	if english.Language != "en" || english.Codec != "srt" {
		t.Fatalf("English subtitle = %#v", english)
	}
}

func TestNormalizeSubtitleLanguage(t *testing.T) {
	for input, want := range map[string]string{
		"chi": "zh",
		"zho": "zh",
		"CHS": "zh-cn",
		"cht": "zh-tw",
		"jpn": "ja",
		"eng": "en",
		"fra": "fra",
	} {
		if got := normalizeSubtitleLanguage(input); got != want {
			t.Errorf("normalizeSubtitleLanguage(%q) = %q, want %q", input, got, want)
		}
	}
}
