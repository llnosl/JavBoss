package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"javboss/internal/common"
	"javboss/internal/models"

	"gorm.io/gorm"
)

// ReplaceVideoLocationSubtitles replaces the selected subtitle kinds atomically.
// markScanned records that embedded subtitle probing completed successfully.
func ReplaceVideoLocationSubtitles(
	ctx context.Context,
	locationID int64,
	kinds []string,
	items []models.VideoSubtitle,
	markScanned bool,
) error {
	if locationID <= 0 {
		return errors.New("video location id is required")
	}
	kindSet := make(map[string]struct{}, len(kinds))
	cleanKinds := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		kind = strings.TrimSpace(kind)
		if kind != models.SubtitleKindEmbedded && kind != models.SubtitleKindExternal {
			return fmt.Errorf("unsupported subtitle kind %q", kind)
		}
		if _, exists := kindSet[kind]; exists {
			continue
		}
		kindSet[kind] = struct{}{}
		cleanKinds = append(cleanKinds, kind)
	}
	if len(cleanKinds) == 0 {
		return errors.New("at least one subtitle kind is required")
	}

	cleanItems := make([]models.VideoSubtitle, 0, len(items))
	identities := make(map[string]struct{}, len(items))
	for _, item := range items {
		item.Kind = strings.TrimSpace(item.Kind)
		item.Identity = strings.TrimSpace(item.Identity)
		if _, ok := kindSet[item.Kind]; !ok {
			continue
		}
		if item.Identity == "" {
			return errors.New("subtitle identity is required")
		}
		if _, duplicate := identities[item.Identity]; duplicate {
			continue
		}
		identities[item.Identity] = struct{}{}
		item.ID = 0
		item.VideoLocationID = locationID
		item.Codec = strings.ToLower(strings.TrimSpace(item.Codec))
		item.Language = strings.ToLower(strings.TrimSpace(item.Language))
		item.Title = strings.TrimSpace(item.Title)
		item.RelativePath = strings.TrimSpace(item.RelativePath)
		cleanItems = append(cleanItems, item)
	}
	sort.Slice(cleanItems, func(i, j int) bool { return cleanItems[i].Identity < cleanItems[j].Identity })

	return common.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, refreshesExternal := kindSet[models.SubtitleKindExternal]; refreshesExternal {
			var linked []models.VideoSubtitle
			if err := tx.
				Where("video_location_id = ? AND kind = ? AND identity LIKE ?", locationID, models.SubtitleKindExternal, "external-linked:%").
				Find(&linked).Error; err != nil {
				return fmt.Errorf("load linked video subtitles: %w", err)
			}
			if len(linked) > 0 {
				linkedPaths := make(map[string]struct{}, len(linked))
				for _, item := range linked {
					linkedPaths[strings.TrimSpace(item.RelativePath)] = struct{}{}
				}
				filtered := cleanItems[:0]
				for _, item := range cleanItems {
					if item.Kind == models.SubtitleKindExternal {
						if _, exists := linkedPaths[strings.TrimSpace(item.RelativePath)]; exists {
							continue
						}
					}
					filtered = append(filtered, item)
				}
				cleanItems = filtered
			}
		}

		if err := tx.Where("video_location_id = ? AND kind IN ?", locationID, cleanKinds).
			Where("NOT (kind = ? AND identity LIKE ?)", models.SubtitleKindExternal, "external-linked:%").
			Delete(&models.VideoSubtitle{}).Error; err != nil {
			return fmt.Errorf("delete video location subtitles: %w", err)
		}
		if len(cleanItems) > 0 {
			if err := tx.Create(&cleanItems).Error; err != nil {
				return fmt.Errorf("create video location subtitles: %w", err)
			}
		}
		if markScanned {
			now := time.Now().UTC()
			if err := tx.Model(&models.VideoLocation{}).
				Where("id = ?", locationID).
				Update("subtitles_scanned_at", now).Error; err != nil {
				return fmt.Errorf("mark video location subtitles scanned: %w", err)
			}
		}
		return nil
	})
}

func ListVideoLocationSubtitles(ctx context.Context, locationID int64) ([]models.VideoSubtitle, error) {
	if locationID <= 0 {
		return nil, errors.New("video location id is required")
	}
	items := make([]models.VideoSubtitle, 0)
	if err := common.DB.WithContext(ctx).
		Where("video_location_id = ?", locationID).
		Order("kind, stream_index, relative_path, id").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list video location subtitles: %w", err)
	}
	return items, nil
}

// UpdateExternalVideoSubtitlePath changes the file associated with one external subtitle.
func UpdateExternalVideoSubtitlePath(
	ctx context.Context,
	locationID int64,
	subtitleID int64,
	relativePath string,
) (*models.VideoSubtitle, error) {
	if locationID <= 0 || subtitleID <= 0 {
		return nil, errors.New("video location id and subtitle id are required")
	}
	relativePath = cleanRelativePathForDB(relativePath)
	if relativePath == "" {
		return nil, errors.New("subtitle relative path is required")
	}

	var item models.VideoSubtitle
	err := common.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("id = ? AND video_location_id = ?", subtitleID, locationID).
			First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("load video subtitle: %w", err)
		}
		if item.Kind != models.SubtitleKindExternal {
			return errors.New("only external subtitle paths can be updated")
		}

		extension := strings.ToLower(filepath.Ext(filepath.FromSlash(relativePath)))
		title := strings.TrimSuffix(filepath.Base(filepath.FromSlash(relativePath)), extension)
		updates := map[string]any{
			"identity":      "external-linked:" + relativePath,
			"relative_path": relativePath,
			"codec":         strings.TrimPrefix(extension, "."),
			"title":         title,
		}
		if err := tx.Model(&item).Updates(updates).Error; err != nil {
			return fmt.Errorf("update video subtitle path: %w", err)
		}
		return tx.First(&item, subtitleID).Error
	})
	if err != nil {
		return nil, err
	}
	if item.ID == 0 {
		return nil, nil
	}
	return &item, nil
}
