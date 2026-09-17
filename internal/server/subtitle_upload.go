package server

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
	"javboss/internal/service"
)

const maxSubtitleUploadBytes int64 = 10 << 20

var (
	errSubtitleUploadTooLarge = errors.New("subtitle upload is too large")
	errSubtitleUploadType     = errors.New("subtitle upload type is invalid")
)

var subtitleUploadExtensions = map[string]struct{}{
	".ass": {},
	".ssa": {},
	".srt": {},
	".vtt": {},
}

func importVideoSubtitle(c *gin.Context) {
	videoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || videoID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return
	}
	if c.Request.ContentLength > maxSubtitleUploadBytes+(1<<20) {
		respondLocalizedError(c, http.StatusRequestEntityTooLarge, "字幕文件不能超过 10 MB", "The subtitle file cannot exceed 10 MB")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSubtitleUploadBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(maxSubtitleUploadBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			respondLocalizedError(c, http.StatusRequestEntityTooLarge, "字幕文件不能超过 10 MB", "The subtitle file cannot exceed 10 MB")
		} else {
			respondLocalizedError(c, http.StatusBadRequest, "字幕上传请求无效", "Invalid subtitle upload request")
		}
		return
	}

	locationID, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("location_id")), 10, 64)
	if err != nil || locationID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频位置 ID 无效", "Invalid video location ID")
		return
	}
	language, ok := normalizeImportedSubtitleLanguage(c.PostForm("language"))
	if !ok {
		respondLocalizedError(c, http.StatusBadRequest, "字幕语言无效", "Invalid subtitle language")
		return
	}

	location, err := dbpkg.GetActiveVideoLocation(c.Request.Context(), videoID, locationID)
	if err != nil {
		logging.Error("get subtitle import target: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "加载视频位置失败", "Failed to load the video location")
		return
	}
	if location == nil {
		respondLocalizedError(c, http.StatusNotFound, "视频或视频位置不存在", "Video or video location does not exist")
		return
	}
	videoPath, root, err := resolveVideoPath(location.RelativePath, location.DirectoryRef.Path)
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "视频文件路径无效", "Invalid video file path")
		return
	}
	if err := ensureVideoFileExists(c, videoPath); err != nil {
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "请选择字幕文件", "Select a subtitle file")
		return
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(header.Filename)))
	if _, supported := subtitleUploadExtensions[extension]; !supported {
		respondLocalizedError(c, http.StatusUnsupportedMediaType, "仅支持 SRT、ASS、SSA 和 VTT 字幕", "Only SRT, ASS, SSA, and VTT subtitles are supported")
		return
	}

	videoStem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	targetName := videoStem + ".manual" + extension
	if language != "" {
		targetName = videoStem + "." + language + ".manual" + extension
	}
	targetPath := filepath.Join(filepath.Dir(videoPath), targetName)
	info, err := saveUploadedSubtitle(targetPath, header)
	if err != nil {
		switch {
		case errors.Is(err, errSubtitleUploadTooLarge):
			respondLocalizedError(c, http.StatusRequestEntityTooLarge, "字幕文件不能超过 10 MB", "The subtitle file cannot exceed 10 MB")
		case errors.Is(err, errSubtitleUploadType):
			respondLocalizedError(c, http.StatusUnsupportedMediaType, "字幕文件为空或格式无效", "The subtitle file is empty or invalid")
		default:
			logging.Error("save uploaded subtitle: %v", err)
			respondLocalizedError(c, http.StatusInternalServerError, "保存字幕失败，请检查媒体目录写入权限", "Failed to save the subtitle; check media directory write permissions")
		}
		return
	}

	items, err := service.RefreshExternalSubtitles(c.Request.Context(), root, videoPath, location.ID)
	if err != nil {
		logging.Error("refresh imported subtitle records: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "字幕已保存，但刷新字幕状态失败", "The subtitle was saved, but refreshing subtitle status failed")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"video_id":    videoID,
		"location_id": location.ID,
		"name":        targetName,
		"size":        info.Size(),
		"subtitles":   items,
	})
}

func normalizeImportedSubtitleLanguage(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "unknown":
		return "", true
	case "zh", "zh-cn", "zh-tw", "ja", "en":
		return value, true
	default:
		return "", false
	}
}

func saveUploadedSubtitle(targetPath string, header *multipart.FileHeader) (os.FileInfo, error) {
	if header == nil || header.Size <= 0 {
		return nil, errSubtitleUploadType
	}
	if header.Size > maxSubtitleUploadBytes {
		return nil, errSubtitleUploadTooLarge
	}
	input, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open subtitle upload: %w", err)
	}
	defer input.Close()

	directory := filepath.Dir(targetPath)
	temp, err := os.CreateTemp(directory, ".javboss-subtitle-upload-*")
	if err != nil {
		return nil, fmt.Errorf("create subtitle upload temp file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}

	written, err := io.Copy(temp, io.LimitReader(input, maxSubtitleUploadBytes+1))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("write subtitle upload: %w", err)
	}
	if written <= 0 {
		cleanup()
		return nil, errSubtitleUploadType
	}
	if written > maxSubtitleUploadBytes {
		cleanup()
		return nil, errSubtitleUploadTooLarge
	}
	if err := temp.Chmod(0o644); err != nil {
		cleanup()
		return nil, fmt.Errorf("set subtitle upload permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync subtitle upload: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("close subtitle upload: %w", err)
	}
	if err := replaceUploadedScreenshot(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("install subtitle upload: %w", err)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("inspect saved subtitle: %w", err)
	}
	return info, nil
}
