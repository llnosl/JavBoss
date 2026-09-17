package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
	"javboss/internal/service"
)

const subtitleGenerationOutputLimit = 16 << 10

var subtitleProgressPattern = regexp.MustCompile(`(?m)(\d{1,3}(?:\.\d+)?)%`)
var subtitleModelAssemblyMu sync.Mutex

type subtitleGenerationRequest struct {
	LocationID int64  `json:"location_id"`
	Model      string `json:"model"`
	Device     string `json:"device"`
	Language   string `json:"language"`
}

type subtitleGenerationJob struct {
	VideoID      int64      `json:"video_id"`
	LocationID   int64      `json:"location_id"`
	Filename     string     `json:"filename"`
	Model        string     `json:"model"`
	Device       string     `json:"device"`
	Language     string     `json:"language"`
	OutputLang   string     `json:"output_language"`
	Phase        string     `json:"phase"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	Output       string     `json:"output,omitempty"`
	Error        string     `json:"error,omitempty"`
	Subtitle     string     `json:"subtitle,omitempty"`
	SubtitlePath string     `json:"subtitle_path,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	PausedAt     *time.Time `json:"paused_at,omitempty"`

	videoPath    string
	root         string
	toolDir      string
	exePath      string
	process      *os.Process
	workerActive bool
}

type subtitleGenerationStore struct {
	sync.RWMutex
	jobs map[string]*subtitleGenerationJob
	gate chan struct{}
}

var subtitleGenerations = subtitleGenerationStore{
	jobs: make(map[string]*subtitleGenerationJob),
	gate: make(chan struct{}, 1),
}

func subtitleGenerationKey(videoID, locationID int64) string {
	return strconv.FormatInt(videoID, 10) + ":" + strconv.FormatInt(locationID, 10)
}

func generateVideoSubtitle(c *gin.Context) {
	videoID, ok := parseSubtitleGenerationVideoID(c)
	if !ok {
		return
	}

	var request subtitleGenerationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "生成字幕请求无效", "Invalid subtitle generation request")
		return
	}
	request.Model = strings.ToLower(strings.TrimSpace(request.Model))
	request.Device = strings.ToLower(strings.TrimSpace(request.Device))
	request.Language = strings.ToLower(strings.TrimSpace(request.Language))
	if request.LocationID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频位置 ID 无效", "Invalid video location ID")
		return
	}
	if request.Model == "" {
		request.Model = "medium"
	}
	if request.Model != "medium" && request.Model != "large-v3" {
		respondLocalizedError(c, http.StatusBadRequest, "仅支持 medium 和 large-v3 模型", "Only medium and large-v3 models are supported")
		return
	}
	if request.Device == "" {
		request.Device = "cuda"
	}
	if request.Device != "cuda" && request.Device != "cpu" {
		respondLocalizedError(c, http.StatusBadRequest, "仅支持 CUDA GPU 或 CPU", "Only CUDA GPU or CPU is supported")
		return
	}
	if request.Language != "" && request.Language != "ja" && request.Language != "zh" && request.Language != "en" {
		respondLocalizedError(c, http.StatusBadRequest, "字幕语言无效", "Invalid subtitle language")
		return
	}
	if request.Language != "zh" && strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_API_KEY")) == "" {
		respondLocalizedError(c, http.StatusServiceUnavailable, "未配置 DeepSeek API Key，请先设置 JAVBOSS_DEEPSEEK_API_KEY", "DeepSeek API key is not configured; set JAVBOSS_DEEPSEEK_API_KEY first")
		return
	}

	location, err := dbpkg.GetActiveVideoLocation(c.Request.Context(), videoID, request.LocationID)
	if err != nil {
		logging.Error("get subtitle generation target: %v", err)
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

	toolDir, exePath, err := findFasterWhisperXXL()
	if err != nil {
		respondLocalizedError(c, http.StatusServiceUnavailable, "未找到 Faster-Whisper-XXL 工具", "Faster-Whisper-XXL tool was not found")
		return
	}
	modelPath := filepath.Join(toolDir, "_models", "faster-whisper-"+request.Model, "model.bin")
	if err := ensureFasterWhisperModel(modelPath); err != nil {
		respondLocalizedError(c, http.StatusServiceUnavailable, "未找到所选 Whisper 模型", "The selected Whisper model was not found")
		return
	}

	job := &subtitleGenerationJob{
		VideoID:      videoID,
		LocationID:   location.ID,
		Filename:     filepath.Base(videoPath),
		Model:        request.Model,
		Device:       request.Device,
		Language:     request.Language,
		OutputLang:   "zh-cn",
		Phase:        "queued",
		Status:       "queued",
		CreatedAt:    time.Now(),
		videoPath:    videoPath,
		root:         root,
		toolDir:      toolDir,
		exePath:      exePath,
		workerActive: true,
	}
	key := subtitleGenerationKey(videoID, location.ID)
	subtitleGenerations.Lock()
	if existing := subtitleGenerations.jobs[key]; existing != nil && (existing.Status == "queued" || existing.Status == "running" || existing.Status == "paused") {
		snapshot := cloneSubtitleGenerationJob(existing)
		subtitleGenerations.Unlock()
		c.JSON(http.StatusConflict, snapshot)
		return
	}
	subtitleGenerations.jobs[key] = job
	snapshot := cloneSubtitleGenerationJob(job)
	subtitleGenerations.Unlock()

	go runSubtitleGeneration(key)
	c.JSON(http.StatusAccepted, snapshot)
}

func getVideoSubtitleGeneration(c *gin.Context) {
	videoID, ok := parseSubtitleGenerationVideoID(c)
	if !ok {
		return
	}
	locationID, err := strconv.ParseInt(strings.TrimSpace(c.Query("location_id")), 10, 64)
	if err != nil || locationID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频位置 ID 无效", "Invalid video location ID")
		return
	}
	key := subtitleGenerationKey(videoID, locationID)
	subtitleGenerations.RLock()
	job := cloneSubtitleGenerationJob(subtitleGenerations.jobs[key])
	subtitleGenerations.RUnlock()
	if job == nil {
		respondLocalizedError(c, http.StatusNotFound, "没有找到字幕生成任务", "Subtitle generation job was not found")
		return
	}
	c.JSON(http.StatusOK, job)
}

func listSubtitleGenerations(c *gin.Context) {
	subtitleGenerations.RLock()
	jobs := make([]*subtitleGenerationJob, 0, len(subtitleGenerations.jobs))
	for _, job := range subtitleGenerations.jobs {
		jobs = append(jobs, cloneSubtitleGenerationJob(job))
	}
	subtitleGenerations.RUnlock()
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	c.JSON(http.StatusOK, gin.H{"items": jobs})
}

func pauseSubtitleGeneration(c *gin.Context) {
	videoID, locationID, ok := parseSubtitleGenerationTarget(c)
	if !ok {
		return
	}
	key := subtitleGenerationKey(videoID, locationID)
	subtitleGenerations.Lock()
	job := subtitleGenerations.jobs[key]
	if job == nil {
		subtitleGenerations.Unlock()
		respondLocalizedError(c, http.StatusNotFound, "没有找到字幕生成任务", "Subtitle generation job was not found")
		return
	}
	if job.Status == "paused" {
		snapshot := cloneSubtitleGenerationJob(job)
		subtitleGenerations.Unlock()
		c.JSON(http.StatusOK, snapshot)
		return
	}
	if job.Status != "queued" && job.Status != "running" {
		subtitleGenerations.Unlock()
		respondLocalizedError(c, http.StatusConflict, "当前任务无法暂停", "The current job cannot be paused")
		return
	}
	if job.process != nil {
		if err := suspendProcess(job.process); err != nil {
			subtitleGenerations.Unlock()
			logging.Error("pause subtitle generation: video_id=%d location_id=%d err=%v", videoID, locationID, err)
			respondLocalizedError(c, http.StatusInternalServerError, "暂停字幕生成失败", "Failed to pause subtitle generation")
			return
		}
	}
	now := time.Now()
	job.Status = "paused"
	job.PausedAt = &now
	snapshot := cloneSubtitleGenerationJob(job)
	subtitleGenerations.Unlock()
	logging.Info("subtitle generation paused: video_id=%d location_id=%d", videoID, locationID)
	c.JSON(http.StatusOK, snapshot)
}

func resumeSubtitleGeneration(c *gin.Context) {
	videoID, locationID, ok := parseSubtitleGenerationTarget(c)
	if !ok {
		return
	}
	key := subtitleGenerationKey(videoID, locationID)
	startWorker := false
	subtitleGenerations.Lock()
	job := subtitleGenerations.jobs[key]
	if job == nil {
		subtitleGenerations.Unlock()
		respondLocalizedError(c, http.StatusNotFound, "没有找到字幕生成任务", "Subtitle generation job was not found")
		return
	}
	if job.Status != "paused" {
		subtitleGenerations.Unlock()
		respondLocalizedError(c, http.StatusConflict, "当前任务没有暂停", "The current job is not paused")
		return
	}
	if job.process != nil {
		if err := resumeProcess(job.process); err != nil {
			subtitleGenerations.Unlock()
			logging.Error("resume subtitle generation: video_id=%d location_id=%d err=%v", videoID, locationID, err)
			respondLocalizedError(c, http.StatusInternalServerError, "继续字幕生成失败", "Failed to resume subtitle generation")
			return
		}
		job.Status = "running"
	} else {
		if job.workerActive {
			job.Status = "running"
		} else {
			job.Status = "queued"
			job.workerActive = true
			startWorker = true
		}
	}
	job.PausedAt = nil
	snapshot := cloneSubtitleGenerationJob(job)
	subtitleGenerations.Unlock()
	if startWorker {
		go runSubtitleGeneration(key)
	}
	logging.Info("subtitle generation resumed: video_id=%d location_id=%d", videoID, locationID)
	c.JSON(http.StatusOK, snapshot)
}

func parseSubtitleGenerationTarget(c *gin.Context) (int64, int64, bool) {
	videoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("video_id")), 10, 64)
	if err != nil || videoID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return 0, 0, false
	}
	locationID, err := strconv.ParseInt(strings.TrimSpace(c.Param("location_id")), 10, 64)
	if err != nil || locationID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频位置 ID 无效", "Invalid video location ID")
		return 0, 0, false
	}
	return videoID, locationID, true
}

func parseSubtitleGenerationVideoID(c *gin.Context) (int64, bool) {
	videoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || videoID <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return 0, false
	}
	return videoID, true
}

func cloneSubtitleGenerationJob(job *subtitleGenerationJob) *subtitleGenerationJob {
	if job == nil {
		return nil
	}
	clone := *job
	clone.videoPath = ""
	clone.root = ""
	clone.toolDir = ""
	clone.exePath = ""
	clone.process = nil
	clone.workerActive = false
	return &clone
}

func runSubtitleGeneration(key string) {
	subtitleGenerations.gate <- struct{}{}
	defer func() { <-subtitleGenerations.gate }()

	subtitleGenerations.Lock()
	job := subtitleGenerations.jobs[key]
	if job == nil {
		subtitleGenerations.Unlock()
		return
	}
	if job.Status == "paused" {
		job.workerActive = false
		subtitleGenerations.Unlock()
		return
	}
	if job.Status != "queued" {
		job.workerActive = false
		subtitleGenerations.Unlock()
		return
	}
	now := time.Now()
	job.Status = "running"
	job.Phase = "transcribing"
	job.StartedAt = &now
	subtitleGenerations.Unlock()

	err := executeSubtitleGeneration(key, job)
	completedAt := time.Now()
	subtitleGenerations.Lock()
	job = subtitleGenerations.jobs[key]
	if job != nil {
		job.process = nil
		job.workerActive = false
		job.CompletedAt = &completedAt
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			logging.Error("subtitle generation failed: video_id=%d location_id=%d err=%v", job.VideoID, job.LocationID, err)
		} else {
			job.Status = "completed"
			job.Phase = "completed"
			job.Progress = 100
			logging.Info("subtitle generation completed: video_id=%d location_id=%d subtitle=%s", job.VideoID, job.LocationID, job.Subtitle)
		}
	}
	subtitleGenerations.Unlock()
}

func executeSubtitleGeneration(key string, job *subtitleGenerationJob) error {
	tempDir, err := os.MkdirTemp(filepath.Dir(job.videoPath), ".javboss-whisper-*")
	if err != nil {
		return fmt.Errorf("create subtitle output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	args := []string{
		job.videoPath,
		"--model", job.Model,
		"--model_dir", filepath.Join(job.toolDir, "_models"),
		"--device", job.Device,
		"--output_dir", tempDir,
		"--output_format", "srt",
		"--print_progress",
		"--check_files",
		"--batched",
		"--beep_off",
	}
	if job.Language == "en" {
		args = append(args, "--standard")
	} else {
		args = append(args, "--standard_asia")
	}
	if job.Device == "cuda" {
		args = append(args, "--compute_type", "float16", "--batch_size", "8")
	} else {
		args = append(args, "--compute_type", "int8", "--batch_size", "4")
	}
	if job.Language != "" {
		args = append(args, "--language", job.Language)
	}

	writer := &subtitleGenerationWriter{key: key}
	cmd := exec.CommandContext(context.Background(), job.exePath, args...)
	cmd.Dir = job.toolDir
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Faster-Whisper-XXL: %w", err)
	}
	shouldPause := false
	subtitleGenerations.Lock()
	if current := subtitleGenerations.jobs[key]; current != nil {
		current.process = cmd.Process
		shouldPause = current.Status == "paused"
	}
	subtitleGenerations.Unlock()
	if shouldPause {
		if err := suspendProcess(cmd.Process); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("pause Faster-Whisper-XXL after start: %w", err)
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("run Faster-Whisper-XXL: %w", err)
	}
	subtitleGenerations.Lock()
	if current := subtitleGenerations.jobs[key]; current != nil {
		current.process = nil
		current.Phase = "translating"
		current.Progress = 85
		current.Output = appendSubtitleGenerationOutput(current.Output, "语音转写完成，正在翻译为简体中文")
	}
	subtitleGenerations.Unlock()

	generatedPath, err := findGeneratedSRT(tempDir)
	if err != nil {
		return err
	}
	chinesePath := filepath.Join(tempDir, "javboss.zh-cn.srt")
	if err := translateSRTToSimplifiedChinese(
		context.Background(),
		generatedPath,
		chinesePath,
		job.Language,
		func(completed, total int) error {
			if err := waitForSubtitleGenerationResume(context.Background(), key); err != nil {
				return err
			}
			progress := 85.0
			if total > 0 {
				progress += float64(completed) / float64(total) * 14
			}
			subtitleGenerations.Lock()
			if current := subtitleGenerations.jobs[key]; current != nil && progress > current.Progress {
				current.Progress = progress
			}
			subtitleGenerations.Unlock()
			return nil
		},
	); err != nil {
		return fmt.Errorf("translate subtitle to Simplified Chinese: %w", err)
	}
	if err := waitForSubtitleGenerationResume(context.Background(), key); err != nil {
		return err
	}
	subtitleGenerations.Lock()
	if current := subtitleGenerations.jobs[key]; current != nil {
		current.Phase = "installing"
		current.Progress = 99
		current.Output = appendSubtitleGenerationOutput(current.Output, "简体中文字幕校验通过，正在写入视频目录")
	}
	subtitleGenerations.Unlock()
	videoStem := strings.TrimSuffix(filepath.Base(job.videoPath), filepath.Ext(job.videoPath))
	targetName := videoStem + ".zh-cn.generated.srt"
	targetPath := filepath.Join(filepath.Dir(job.videoPath), targetName)
	if err := replaceUploadedScreenshot(chinesePath, targetPath); err != nil {
		return fmt.Errorf("install generated subtitle: %w", err)
	}
	if info, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("verify generated subtitle: %w", err)
	} else if info.IsDir() || info.Size() <= 0 {
		return errors.New("verify generated subtitle: installed subtitle file is empty")
	}
	if _, err := service.RefreshExternalSubtitles(context.Background(), job.root, job.videoPath, job.LocationID); err != nil {
		return fmt.Errorf("refresh generated subtitle: %w", err)
	}
	subtitleGenerations.Lock()
	if current := subtitleGenerations.jobs[key]; current != nil {
		current.Subtitle = targetName
		current.SubtitlePath = targetPath
	}
	subtitleGenerations.Unlock()
	return nil
}

func findGeneratedSRT(directory string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(directory, "*.srt"))
	if err != nil {
		return "", fmt.Errorf("find generated subtitle: %w", err)
	}
	if len(matches) == 0 {
		return "", errors.New("Faster-Whisper-XXL did not create an SRT file")
	}
	sort.Strings(matches)
	return matches[0], nil
}

func ensureFasterWhisperModel(modelPath string) error {
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() && info.Size() > 0 {
		return nil
	}
	subtitleModelAssemblyMu.Lock()
	defer subtitleModelAssemblyMu.Unlock()
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() && info.Size() > 0 {
		return nil
	}
	parts, err := filepath.Glob(filepath.Join(modelPath+".parts", "*.part"))
	if err != nil {
		return fmt.Errorf("find model parts: %w", err)
	}
	if len(parts) == 0 {
		return os.ErrNotExist
	}
	sort.Strings(parts)
	temporary, err := os.CreateTemp(filepath.Dir(modelPath), ".javboss-whisper-model-*")
	if err != nil {
		return fmt.Errorf("create model restore file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	for _, partPath := range parts {
		part, openErr := os.Open(partPath)
		if openErr != nil {
			cleanup()
			return fmt.Errorf("open model part: %w", openErr)
		}
		_, copyErr := io.Copy(temporary, part)
		closeErr := part.Close()
		if copyErr != nil {
			cleanup()
			return fmt.Errorf("restore model part: %w", copyErr)
		}
		if closeErr != nil {
			cleanup()
			return fmt.Errorf("close model part: %w", closeErr)
		}
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync restored model: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close restored model: %w", err)
	}
	if err := os.Rename(temporaryPath, modelPath); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("install restored model: %w", err)
	}
	return nil
}

type subtitleGenerationWriter struct {
	key string
	mu  sync.Mutex
	buf bytes.Buffer
}

func (writer *subtitleGenerationWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	_, _ = writer.buf.Write(data)
	output := writer.buf.String()
	if len(output) > subtitleGenerationOutputLimit {
		output = output[len(output)-subtitleGenerationOutputLimit:]
		writer.buf.Reset()
		_, _ = io.WriteString(&writer.buf, output)
	}
	progress := latestSubtitleProgress(output)
	progress *= 0.85
	subtitleGenerations.Lock()
	if job := subtitleGenerations.jobs[writer.key]; job != nil {
		job.Output = strings.TrimSpace(output)
		if progress > job.Progress {
			job.Progress = progress
		}
	}
	subtitleGenerations.Unlock()
	return len(data), nil
}

func appendSubtitleGenerationOutput(current, message string) string {
	current = strings.TrimSpace(current)
	message = strings.TrimSpace(message)
	if current == "" {
		return message
	}
	if message == "" {
		return current
	}
	return current + "\n" + message
}

func waitForSubtitleGenerationResume(ctx context.Context, key string) error {
	for {
		subtitleGenerations.RLock()
		job := subtitleGenerations.jobs[key]
		paused := job != nil && job.Status == "paused"
		subtitleGenerations.RUnlock()
		if job == nil {
			return errors.New("subtitle generation job no longer exists")
		}
		if !paused {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func latestSubtitleProgress(output string) float64 {
	matches := subtitleProgressPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(matches[len(matches)-1][1], 64)
	if err != nil || value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func findFasterWhisperXXL() (string, string, error) {
	candidates := make([]string, 0, 10)
	if configured := strings.TrimSpace(os.Getenv("JAVBOSS_WHISPER_DIR")); configured != "" {
		candidates = append(candidates, configured)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = appendSubtitleToolCandidates(candidates, workingDirectory)
	}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		for range 4 {
			candidates = appendSubtitleToolCandidates(candidates, directory)
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
			directory = parent
		}
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		executable := filepath.Join(candidate, "faster-whisper-xxl.exe")
		if info, err := os.Stat(executable); err == nil && !info.IsDir() {
			return candidate, executable, nil
		}
	}
	return "", "", errors.New("faster-whisper-xxl.exe not found")
}

func appendSubtitleToolCandidates(candidates []string, root string) []string {
	return append(
		candidates,
		filepath.Join(root, "tools", "faster-whisper-xxl"),
		filepath.Join(root, "tools", "faster-whisper-xxl", "Faster-Whisper-XXL"),
		filepath.Join(root, "tools", "Faster-Whisper-XXL", "Faster-Whisper-XXL"),
	)
}
