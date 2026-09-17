package models

import "time"

const (
	SubtitleKindEmbedded = "embedded"
	SubtitleKindExternal = "external"
)

// VideoSubtitle describes one subtitle track or sidecar file for a video location.
// Subtitles belong to locations because separate copies of the same video may have
// different embedded tracks or neighboring files.
type VideoSubtitle struct {
	ID              int64     `json:"id" gorm:"primaryKey"`
	VideoLocationID int64     `json:"video_location_id" gorm:"not null;index;uniqueIndex:idx_video_subtitle_location_identity,priority:1"`
	Kind            string    `json:"kind" gorm:"not null;index"`
	Identity        string    `json:"-" gorm:"not null;uniqueIndex:idx_video_subtitle_location_identity,priority:2"`
	StreamIndex     int       `json:"stream_index" gorm:"not null;default:-1"`
	Codec           string    `json:"codec" gorm:"not null;default:''"`
	Language        string    `json:"language" gorm:"not null;default:''"`
	Title           string    `json:"title" gorm:"not null;default:''"`
	IsDefault       bool      `json:"is_default" gorm:"not null;default:false"`
	IsForced        bool      `json:"is_forced" gorm:"not null;default:false"`
	RelativePath    string    `json:"relative_path" gorm:"not null;default:''"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	VideoLocation VideoLocation `json:"-" gorm:"foreignKey:VideoLocationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}
