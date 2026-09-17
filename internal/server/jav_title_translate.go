package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
)

const javTitleTranslationBatchSize = 12

var errDeepSeekInvalidJavTitleOutput = errors.New("DeepSeek returned invalid JAV title translations")

type javTitleTranslationRequest struct {
	Force bool `json:"force"`
}

type javTitleTranslationFailure struct {
	ID    int64  `json:"id"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

type javTitleTranslationJob struct {
	Status      string                       `json:"status"`
	Force       bool                         `json:"force"`
	Total       int                          `json:"total"`
	Completed   int                          `json:"completed"`
	Failed      int                          `json:"failed"`
	CurrentCode string                       `json:"current_code,omitempty"`
	Error       string                       `json:"error,omitempty"`
	Failures    []javTitleTranslationFailure `json:"failures,omitempty"`
	CreatedAt   time.Time                    `json:"created_at"`
	StartedAt   *time.Time                   `json:"started_at,omitempty"`
	CompletedAt *time.Time                   `json:"completed_at,omitempty"`
}

type javTitleTranslationStore struct {
	sync.RWMutex
	job *javTitleTranslationJob
}

var javTitleTranslations javTitleTranslationStore

type deepSeekJavTitleInput struct {
	ID    int64  `json:"id"`
	Code  string `json:"code"`
	Title string `json:"title"`
}

type deepSeekJavTitleOutputItem struct {
	ID              int64  `json:"id"`
	TitleZH         string `json:"title_zh"`
	Title           string `json:"title"`
	Text            string `json:"text"`
	TranslatedTitle string `json:"translated_title"`
	Translation     string `json:"translation"`
}

type deepSeekJavTitleOutput struct {
	Translations []deepSeekJavTitleOutputItem `json:"translations"`
}

func getJavTitleTranslation(c *gin.Context) {
	javTitleTranslations.RLock()
	job := cloneJavTitleTranslationJob(javTitleTranslations.job)
	javTitleTranslations.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"configured": strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_API_KEY")) != "",
		"job":        job,
	})
}

func startJavTitleTranslation(c *gin.Context) {
	if strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_API_KEY")) == "" {
		respondLocalizedError(c, http.StatusBadRequest, "未配置 DeepSeek API Key，请先设置 JAVBOSS_DEEPSEEK_API_KEY", "JAVBOSS_DEEPSEEK_API_KEY is not configured")
		return
	}
	var request javTitleTranslationRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			respondLocalizedError(c, http.StatusBadRequest, "标题翻译请求无效", "Invalid title translation request")
			return
		}
	}

	javTitleTranslations.RLock()
	active := javTitleTranslations.job != nil && (javTitleTranslations.job.Status == "queued" || javTitleTranslations.job.Status == "running")
	javTitleTranslations.RUnlock()
	if active {
		respondLocalizedError(c, http.StatusConflict, "已有中文标题翻译任务正在运行", "A Chinese title translation job is already running")
		return
	}

	items, err := dbpkg.ListJavTitleTranslationItems(c.Request.Context(), request.Force)
	if err != nil {
		logging.Error("list JAV titles for translation: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取待翻译标题失败", "Failed to load titles for translation")
		return
	}
	now := time.Now()
	job := &javTitleTranslationJob{
		Status:    "queued",
		Force:     request.Force,
		Total:     len(items),
		Failures:  []javTitleTranslationFailure{},
		CreatedAt: now,
	}
	javTitleTranslations.Lock()
	if javTitleTranslations.job != nil && (javTitleTranslations.job.Status == "queued" || javTitleTranslations.job.Status == "running") {
		javTitleTranslations.Unlock()
		respondLocalizedError(c, http.StatusConflict, "已有中文标题翻译任务正在运行", "A Chinese title translation job is already running")
		return
	}
	javTitleTranslations.job = job
	javTitleTranslations.Unlock()

	snapshot := cloneJavTitleTranslationJob(job)
	go runJavTitleTranslation(items)
	c.JSON(http.StatusAccepted, snapshot)
}

func runJavTitleTranslation(items []dbpkg.JavTitleTranslationItem) {
	startedAt := time.Now()
	updateJavTitleTranslationJob(func(job *javTitleTranslationJob) {
		job.Status = "running"
		job.StartedAt = &startedAt
	})
	if len(items) == 0 {
		finishJavTitleTranslationJob()
		return
	}

	ctx := context.Background()
	for start := 0; start < len(items); start += javTitleTranslationBatchSize {
		end := start + javTitleTranslationBatchSize
		if end > len(items) {
			end = len(items)
		}
		translateAndSaveJavTitleBatch(ctx, items[start:end])
	}
	finishJavTitleTranslationJob()
}

func translateAndSaveJavTitleBatch(ctx context.Context, items []dbpkg.JavTitleTranslationItem) {
	if len(items) == 0 {
		return
	}
	updateJavTitleTranslationJob(func(job *javTitleTranslationJob) {
		job.CurrentCode = items[0].Code
	})

	translated, requestErr := requestDeepSeekJavTitleTranslation(ctx, items)
	pending := make([]dbpkg.JavTitleTranslationItem, 0, len(items))
	for _, item := range items {
		titleZH := strings.TrimSpace(translated[item.ID])
		if titleZH == "" {
			pending = append(pending, item)
			continue
		}
		if err := dbpkg.SaveJavChineseTitle(ctx, item.ID, item.Title, titleZH); err != nil {
			markJavTitleTranslationFailure(item, err.Error())
			continue
		}
		updateJavTitleTranslationJob(func(job *javTitleTranslationJob) {
			job.Completed++
			job.CurrentCode = item.Code
		})
	}
	if len(pending) == 0 {
		return
	}
	if requestErr == nil {
		requestErr = fmt.Errorf("%w: DeepSeek omitted %d title(s)", errDeepSeekInvalidJavTitleOutput, len(pending))
	}

	if errors.Is(requestErr, errDeepSeekInvalidJavTitleOutput) || errors.Is(requestErr, errDeepSeekOutputLength) {
		logging.Error("retry invalid JAV title translation batch size=%d pending=%d: %v", len(items), len(pending), requestErr)
		if len(pending) < len(items) {
			translateAndSaveJavTitleBatch(ctx, pending)
			return
		}
		if len(pending) > 1 {
			middle := len(pending) / 2
			translateAndSaveJavTitleBatch(ctx, pending[:middle])
			translateAndSaveJavTitleBatch(ctx, pending[middle:])
			return
		}
	}

	message := requestErr.Error()
	logging.Error("translate JAV title batch size=%d: %v", len(items), requestErr)
	for _, item := range pending {
		markJavTitleTranslationFailure(item, message)
	}
}

func finishJavTitleTranslationJob() {
	completedAt := time.Now()
	updateJavTitleTranslationJob(func(job *javTitleTranslationJob) {
		job.CurrentCode = ""
		job.CompletedAt = &completedAt
		if job.Failed > 0 {
			job.Status = "completed_with_errors"
		} else {
			job.Status = "completed"
		}
	})
}

func markJavTitleTranslationFailure(item dbpkg.JavTitleTranslationItem, message string) {
	message = truncateSubtitleText(message, 300)
	updateJavTitleTranslationJob(func(job *javTitleTranslationJob) {
		job.Failed++
		job.Error = message
		job.Failures = append(job.Failures, javTitleTranslationFailure{ID: item.ID, Code: item.Code, Error: message})
	})
}

func updateJavTitleTranslationJob(update func(*javTitleTranslationJob)) {
	javTitleTranslations.Lock()
	defer javTitleTranslations.Unlock()
	if javTitleTranslations.job != nil {
		update(javTitleTranslations.job)
	}
}

func cloneJavTitleTranslationJob(job *javTitleTranslationJob) *javTitleTranslationJob {
	if job == nil {
		return nil
	}
	clone := *job
	clone.Failures = append([]javTitleTranslationFailure(nil), job.Failures...)
	return &clone
}

func requestDeepSeekJavTitleTranslation(ctx context.Context, items []dbpkg.JavTitleTranslationItem) (map[int64]string, error) {
	apiKey := strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("JAVBOSS_DEEPSEEK_API_KEY is not configured")
	}
	endpoint := strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_API_URL"))
	if endpoint == "" {
		endpoint = defaultDeepSeekAPIURL
	}
	model := strings.TrimSpace(os.Getenv("JAVBOSS_DEEPSEEK_MODEL"))
	if model == "" {
		model = defaultDeepSeekModel
	}

	input := make([]deepSeekJavTitleInput, 0, len(items))
	for _, item := range items {
		input = append(input, deepSeekJavTitleInput{ID: item.ID, Code: item.Code, Title: item.Title})
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode JAV titles: %w", err)
	}
	systemPrompt := `You translate Japanese adult-video catalog titles into natural, fluent Simplified Chinese. Produce readable Chinese title-style wording that sounds idiomatic and conversational where appropriate, not stiff word-for-word machine translation. Preserve the full original meaning, tone, relationships, and intensity. Do not omit, summarize, censor, soften, add facts, or explain. Keep catalog codes, numbers, model names, brands, and established performer names unchanged unless there is a well-known Chinese rendering. Return only one JSON object in exactly this shape: {"translations":[{"id":1,"title_zh":"中文标题"}]}. Keep every id unchanged and return exactly one non-empty translation for every input item.`
	userPrompt := "Translate every title in this JSON array into natural Simplified Chinese:\n" + string(inputJSON)
	payload, err := json.Marshal(deepSeekChatRequest{
		Model: model,
		Messages: []deepSeekMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Thinking:       deepSeekThinking{Type: "disabled"},
		Temperature:    0.35,
		MaxTokens:      4096,
		ResponseFormat: deepSeekFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("User-Agent", "JavBoss title translator")
		response, err := subtitleTranslationHTTPClient.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
			closeErr := response.Body.Close()
			if readErr == nil && closeErr != nil {
				readErr = closeErr
			}
			if response.StatusCode >= 200 && response.StatusCode < 300 && readErr == nil {
				translated, parseErr := parseDeepSeekJavTitleTranslation(body, input)
				if parseErr == nil {
					return translated, nil
				}
				logging.Error("parse DeepSeek JAV title response: %v; body=%s", parseErr, truncateSubtitleText(string(body), 800))
				if len(translated) > 0 || errors.Is(parseErr, errDeepSeekOutputLength) || errors.Is(parseErr, errDeepSeekInvalidJavTitleOutput) {
					return translated, parseErr
				}
				lastErr = parseErr
			} else if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = fmt.Errorf("DeepSeek returned HTTP %d: %s", response.StatusCode, truncateSubtitleText(string(body), 240))
			}
		} else {
			lastErr = err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == 4 {
			break
		}
		wait := time.Duration(1<<attempt) * 2 * time.Second
		if response != nil && response.StatusCode == http.StatusTooManyRequests {
			if seconds, parseErr := strconv.Atoi(strings.TrimSpace(response.Header.Get("Retry-After"))); parseErr == nil && seconds > 0 {
				wait = time.Duration(seconds) * time.Second
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func parseDeepSeekJavTitleTranslation(body []byte, expected []deepSeekJavTitleInput) (map[int64]string, error) {
	var response deepSeekChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode DeepSeek response: %w", err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("DeepSeek API error: %s", strings.TrimSpace(response.Error.Message))
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("DeepSeek response contains no choices")
	}
	choice := response.Choices[0]
	if choice.FinishReason == "length" {
		return nil, fmt.Errorf("%w (finish_reason=length)", errDeepSeekOutputLength)
	}
	if choice.FinishReason != "" && choice.FinishReason != "stop" {
		return nil, fmt.Errorf("DeepSeek stopped with finish_reason=%s", choice.FinishReason)
	}
	content := strings.TrimSpace(choice.Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var output deepSeekJavTitleOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("%w: decode title translation JSON: %v", errDeepSeekInvalidJavTitleOutput, err)
	}
	expectedIDs := make(map[int64]struct{}, len(expected))
	for _, item := range expected {
		expectedIDs[item.ID] = struct{}{}
	}
	translations := make(map[int64]string, len(output.Translations))
	issues := make([]string, 0)
	for _, item := range output.Translations {
		if _, ok := expectedIDs[item.ID]; !ok {
			issues = append(issues, fmt.Sprintf("unexpected JAV id %d", item.ID))
			continue
		}
		if _, duplicate := translations[item.ID]; duplicate {
			issues = append(issues, fmt.Sprintf("duplicate JAV id %d", item.ID))
			continue
		}
		titleZH := item.chineseTitle()
		if titleZH == "" {
			issues = append(issues, fmt.Sprintf("empty Chinese title for JAV id %d", item.ID))
			continue
		}
		translations[item.ID] = titleZH
	}
	for _, item := range expected {
		if _, ok := translations[item.ID]; !ok {
			issues = append(issues, fmt.Sprintf("missing JAV id %d", item.ID))
		}
	}
	if len(issues) > 0 {
		return translations, fmt.Errorf("%w: %s", errDeepSeekInvalidJavTitleOutput, strings.Join(issues, "; "))
	}
	return translations, nil
}

func (item deepSeekJavTitleOutputItem) chineseTitle() string {
	for _, value := range []string{item.TitleZH, item.TranslatedTitle, item.Translation, item.Title, item.Text} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
