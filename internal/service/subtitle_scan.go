package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"javboss/internal/db"
	"javboss/internal/models"
	"javboss/internal/util"
)

// RefreshExternalSubtitles rescans sidecar subtitle files for one video
// location and replaces its external subtitle records.
func RefreshExternalSubtitles(ctx context.Context, root, videoPath string, locationID int64) ([]models.VideoSubtitle, error) {
	items, err := externalSubtitleRecords(root, videoPath, subtitleDirectoryCache{})
	if err != nil {
		return nil, err
	}
	if err := db.ReplaceVideoLocationSubtitles(
		ctx,
		locationID,
		[]string{models.SubtitleKindExternal},
		items,
		false,
	); err != nil {
		return nil, err
	}
	return items, nil
}

var subtitleExtensions = map[string]struct{}{
	".ass": {},
	".ssa": {},
	".srt": {},
	".vtt": {},
}

type subtitleDirectoryCache map[string][]os.DirEntry

func embeddedSubtitleRecords(streams []util.SubtitleStream) []models.VideoSubtitle {
	items := make([]models.VideoSubtitle, 0, len(streams))
	for _, stream := range streams {
		identity := "embedded:" + strconv.Itoa(stream.Index)
		items = append(items, models.VideoSubtitle{
			Kind:        models.SubtitleKindEmbedded,
			Identity:    identity,
			StreamIndex: stream.Index,
			Codec:       strings.ToLower(strings.TrimSpace(stream.Codec)),
			Language:    normalizeSubtitleLanguage(stream.Language),
			Title:       strings.TrimSpace(stream.Title),
			IsDefault:   stream.Default,
			IsForced:    stream.Forced,
		})
	}
	return items
}

func externalSubtitleRecords(root, videoPath string, cache subtitleDirectoryCache) ([]models.VideoSubtitle, error) {
	root = filepath.Clean(root)
	videoPath = filepath.Clean(videoPath)
	directory := filepath.Dir(videoPath)
	entries, ok := cache[directory]
	if !ok {
		var err error
		entries, err = os.ReadDir(directory)
		if err != nil {
			return nil, fmt.Errorf("read subtitle directory: %w", err)
		}
		cache[directory] = entries
	}

	videoName := filepath.Base(videoPath)
	stem := strings.TrimSuffix(videoName, filepath.Ext(videoName))
	lowerStem := strings.ToLower(stem)
	items := make([]models.VideoSubtitle, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == videoName {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if _, supported := subtitleExtensions[extension]; !supported {
			continue
		}
		lowerName := strings.ToLower(entry.Name())
		if !strings.HasPrefix(lowerName, lowerStem+".") && !strings.HasPrefix(lowerName, lowerStem+"-") {
			continue
		}
		absolutePath := filepath.Join(directory, entry.Name())
		relativePath, err := filepath.Rel(root, absolutePath)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		relativePath = filepath.ToSlash(relativePath)
		items = append(items, models.VideoSubtitle{
			Kind:         models.SubtitleKindExternal,
			Identity:     "external:" + relativePath,
			StreamIndex:  -1,
			Codec:        strings.TrimPrefix(extension, "."),
			Language:     inferExternalSubtitleLanguage(stem, entry.Name()),
			Title:        strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			RelativePath: relativePath,
		})
	}
	return items, nil
}

func inferExternalSubtitleLanguage(videoStem, subtitleName string) string {
	name := strings.TrimSuffix(subtitleName, filepath.Ext(subtitleName))
	lowerName := strings.ToLower(name)
	lowerStem := strings.ToLower(videoStem)
	suffix := strings.TrimPrefix(lowerName, lowerStem)
	value := strings.TrimLeft(strings.TrimSpace(suffix), ".-_ []()")
	switch {
	case strings.Contains(value, "zh-tw"), strings.Contains(value, "zh_tw"),
		strings.Contains(value, "cht"), strings.Contains(value, "繁中"), strings.Contains(value, "繁体"):
		return "zh-tw"
	case strings.Contains(value, "zh-cn"), strings.Contains(value, "zh_cn"),
		strings.Contains(value, "chs"), strings.Contains(value, "简中"), strings.Contains(value, "简体"):
		return "zh-cn"
	case value == "zh", value == "chi", value == "zho", strings.Contains(value, "中文"):
		return "zh"
	case value == "ja", value == "jp", value == "jpn", strings.Contains(value, "日文"), strings.Contains(value, "日语"):
		return "ja"
	case value == "en", value == "eng", strings.Contains(value, "英文"), strings.Contains(value, "英语"):
		return "en"
	default:
		return ""
	}
}

func normalizeSubtitleLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "chi", "zho", "cmn":
		return "zh"
	case "chs", "zh-hans", "zh_cn", "zh-cn":
		return "zh-cn"
	case "cht", "zh-hant", "zh_tw", "zh-tw":
		return "zh-tw"
	case "jpn", "jp":
		return "ja"
	case "eng":
		return "en"
	default:
		return value
	}
}
