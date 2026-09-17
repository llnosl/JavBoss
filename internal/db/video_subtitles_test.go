package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"javboss/internal/common"
	"javboss/internal/models"
)

func TestReplaceVideoLocationSubtitlesByKind(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "subtitles.db"))
	if err != nil {
		t.Fatal(err)
	}
	previousDB := common.DB
	common.DB = database
	t.Cleanup(func() {
		common.DB = previousDB
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	directory := models.Directory{Path: t.TempDir()}
	video := models.Video{Fingerprint: "subtitle-test"}
	if err := database.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&video).Error; err != nil {
		t.Fatal(err)
	}
	location, err := UpsertVideoLocation(context.Background(), video.ID, directory.ID, "movie.mkv", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	items := []models.VideoSubtitle{
		{Kind: models.SubtitleKindEmbedded, Identity: "embedded:2", StreamIndex: 2, Codec: "ass", Language: "zh"},
		{Kind: models.SubtitleKindExternal, Identity: "external:movie.en.srt", StreamIndex: -1, Codec: "srt", Language: "en", RelativePath: "movie.en.srt"},
	}
	if err := ReplaceVideoLocationSubtitles(
		context.Background(),
		location.ID,
		[]string{models.SubtitleKindEmbedded, models.SubtitleKindExternal},
		items,
		true,
	); err != nil {
		t.Fatal(err)
	}

	got, err := ListVideoLocationSubtitles(context.Background(), location.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("subtitles = %#v, want two", got)
	}
	if err := ReplaceVideoLocationSubtitles(
		context.Background(),
		location.ID,
		[]string{models.SubtitleKindExternal},
		nil,
		false,
	); err != nil {
		t.Fatal(err)
	}
	got, err = ListVideoLocationSubtitles(context.Background(), location.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != models.SubtitleKindEmbedded {
		t.Fatalf("subtitles after external refresh = %#v", got)
	}

	if err := ReplaceVideoLocationSubtitles(
		context.Background(),
		location.ID,
		[]string{models.SubtitleKindExternal},
		[]models.VideoSubtitle{{
			Kind:         models.SubtitleKindExternal,
			Identity:     "external:movie.zh-cn.srt",
			StreamIndex:  -1,
			Codec:        "srt",
			Language:     "zh-cn",
			RelativePath: "movie.zh-cn.srt",
		}},
		false,
	); err != nil {
		t.Fatal(err)
	}
	got, err = ListVideoLocationSubtitles(context.Background(), location.ID)
	if err != nil {
		t.Fatal(err)
	}
	var externalID int64
	for _, item := range got {
		if item.Kind == models.SubtitleKindExternal {
			externalID = item.ID
		}
	}
	if externalID == 0 {
		t.Fatal("external subtitle was not stored")
	}
	if _, err := UpdateExternalVideoSubtitlePath(
		context.Background(),
		location.ID,
		externalID,
		"linked/movie.zh-cn.srt",
	); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceVideoLocationSubtitles(
		context.Background(),
		location.ID,
		[]string{models.SubtitleKindExternal},
		[]models.VideoSubtitle{
			{Kind: models.SubtitleKindExternal, Identity: "external:linked/movie.zh-cn.srt", StreamIndex: -1, Codec: "srt", RelativePath: "linked/movie.zh-cn.srt"},
			{Kind: models.SubtitleKindExternal, Identity: "external:movie.ja.srt", StreamIndex: -1, Codec: "srt", RelativePath: "movie.ja.srt"},
		},
		false,
	); err != nil {
		t.Fatal(err)
	}
	got, err = ListVideoLocationSubtitles(context.Background(), location.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("linked subtitle should survive external refresh without duplication: %#v", got)
	}

	refreshed, err := GetActiveVideoLocation(context.Background(), video.ID, location.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == nil || refreshed.SubtitlesScannedAt == nil {
		t.Fatalf("subtitle scan timestamp was not stored: %#v", refreshed)
	}
}
