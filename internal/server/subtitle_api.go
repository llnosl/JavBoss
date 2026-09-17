package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"

	"github.com/gin-gonic/gin"
)

func getVideoSubtitles(c *gin.Context) {
	videoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || videoID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return
	}

	var locationID int64
	if raw := strings.TrimSpace(c.Query("location_id")); raw != "" {
		locationID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || locationID <= 0 {
			respondLocalizedError(c, http.StatusBadRequest, "视频位置 ID 无效", "Invalid video location ID")
			return
		}
	}

	var locationFound bool
	var scanned bool
	if locationID > 0 {
		location, loadErr := dbpkg.GetActiveVideoLocation(c.Request.Context(), videoID, locationID)
		if loadErr != nil {
			logging.Error("get video location subtitles target: %v", loadErr)
			respondLocalizedError(c, http.StatusInternalServerError, "加载字幕信息失败", "Failed to load subtitle information")
			return
		}
		if location != nil {
			locationFound = true
			scanned = location.SubtitlesScannedAt != nil
		}
	} else {
		location, loadErr := dbpkg.GetPrimaryVideoLocation(c.Request.Context(), videoID)
		if loadErr != nil {
			logging.Error("get primary video location subtitles target: %v", loadErr)
			respondLocalizedError(c, http.StatusInternalServerError, "加载字幕信息失败", "Failed to load subtitle information")
			return
		}
		if location != nil {
			locationFound = true
			locationID = location.ID
			scanned = location.SubtitlesScannedAt != nil
		}
	}
	if !locationFound {
		respondLocalizedError(c, http.StatusNotFound, "视频或视频位置不存在", "Video or video location does not exist")
		return
	}

	items, err := dbpkg.ListVideoLocationSubtitles(c.Request.Context(), locationID)
	if err != nil {
		logging.Error("list video subtitles: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "加载字幕信息失败", "Failed to load subtitle information")
		return
	}
	languageSet := make(map[string]struct{})
	for _, item := range items {
		if language := strings.TrimSpace(item.Language); language != "" {
			languageSet[language] = struct{}{}
		}
	}
	languages := make([]string, 0, len(languageSet))
	for language := range languageSet {
		languages = append(languages, language)
	}
	sort.Strings(languages)

	c.JSON(http.StatusOK, gin.H{
		"video_id":      videoID,
		"location_id":   locationID,
		"scanned":       scanned,
		"has_subtitles": len(items) > 0,
		"languages":     languages,
		"items":         items,
	})
}

type updateVideoSubtitlePathRequest struct {
	LocationID int64  `json:"location_id"`
	Path       string `json:"path"`
}

func updateVideoSubtitlePath(c *gin.Context) {
	videoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || videoID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return
	}
	subtitleID, err := strconv.ParseInt(strings.TrimSpace(c.Param("subtitle_id")), 10, 64)
	if err != nil || subtitleID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "字幕 ID 无效", "Invalid subtitle ID")
		return
	}
	var req updateVideoSubtitlePathRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.LocationID <= 0 || strings.TrimSpace(req.Path) == "" {
		respondLocalizedError(c, http.StatusBadRequest, "字幕路径请求无效", "Invalid subtitle path request")
		return
	}

	location, err := dbpkg.GetActiveVideoLocation(c.Request.Context(), videoID, req.LocationID)
	if err != nil {
		logging.Error("get subtitle path update target: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "加载视频位置失败", "Failed to load the video location")
		return
	}
	if location == nil {
		respondLocalizedError(c, http.StatusNotFound, "视频或视频位置不存在", "Video or video location does not exist")
		return
	}

	relativePath, fullPath, err := normalizeSubtitlePathWithinRoot(req.Path, location.DirectoryRef.Path)
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "字幕路径必须位于当前媒体目录内", "The subtitle path must be inside the current media directory")
		return
	}
	extension := strings.ToLower(filepath.Ext(fullPath))
	if _, supported := subtitleUploadExtensions[extension]; !supported {
		respondLocalizedError(c, http.StatusUnsupportedMediaType, "仅支持 SRT、ASS、SSA 和 VTT 字幕", "Only SRT, ASS, SSA, and VTT subtitles are supported")
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondLocalizedError(c, http.StatusNotFound, "指定的字幕文件不存在", "The selected subtitle file does not exist")
			return
		}
		logging.Error("stat updated subtitle path: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取字幕文件失败", "Failed to inspect the subtitle file")
		return
	}
	if !info.Mode().IsRegular() {
		respondLocalizedError(c, http.StatusBadRequest, "字幕路径不是文件", "The subtitle path is not a file")
		return
	}

	updated, err := dbpkg.UpdateExternalVideoSubtitlePath(c.Request.Context(), location.ID, subtitleID, relativePath)
	if err != nil {
		logging.Error("update subtitle path: %v", err)
		respondLocalizedError(c, http.StatusBadRequest, "更新字幕路径失败", "Failed to update the subtitle path")
		return
	}
	if updated == nil {
		respondLocalizedError(c, http.StatusNotFound, "外挂字幕记录不存在", "The external subtitle record does not exist")
		return
	}
	items, err := dbpkg.ListVideoLocationSubtitles(c.Request.Context(), location.ID)
	if err != nil {
		logging.Error("reload updated subtitle path: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "重新加载字幕失败", "Failed to reload subtitles")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":        "path_updated",
		"video_id":      videoID,
		"location_id":   location.ID,
		"subtitle":      updated,
		"absolute_path": fullPath,
		"subtitles":     items,
	})
}

func normalizeSubtitlePathWithinRoot(rawPath, root string) (string, string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	path := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rawPath)))
	if root == "." || !filepath.IsAbs(root) || path == "." {
		return "", "", errors.New("invalid subtitle path")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	relativePath, err := filepath.Rel(root, path)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", "", errors.New("subtitle path escapes media root")
	}
	return filepath.ToSlash(relativePath), filepath.Clean(path), nil
}
