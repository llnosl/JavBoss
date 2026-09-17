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
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"javboss/internal/util"
)

const (
	defaultDeepSeekAPIURL = "https://api.deepseek.com/chat/completions"
	defaultDeepSeekModel  = "deepseek-flash"
	deepSeekBatchMaxCues  = 40
	deepSeekBatchMaxRunes = 6000
)

var srtBlockSeparatorPattern = regexp.MustCompile(`\n[\t ]*\n+`)
var subtitleTranslationHTTPClient = util.NewHTTPClient(2 * time.Minute)
var errDeepSeekOutputLength = errors.New("DeepSeek translation output reached the token limit")

type generatedSRTCue struct {
	prefix     string
	source     string
	translated string
}

func translateSRTToSimplifiedChinese(
	ctx context.Context,
	inputPath string,
	outputPath string,
	sourceLanguage string,
	progress func(completed, total int) error,
) error {
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read generated SRT: %w", err)
	}
	cues, err := parseGeneratedSRT(content)
	if err != nil {
		return err
	}
	if len(cues) == 0 {
		return errors.New("generated SRT contains no subtitle cues")
	}

	sourceLanguage = normalizeTranslationSourceLanguage(sourceLanguage)
	if sourceLanguage == "zh-CN" {
		for index := range cues {
			cues[index].translated = cues[index].source
		}
		if progress != nil {
			if err := progress(len(cues), len(cues)); err != nil {
				return err
			}
		}
	} else if err := translateGeneratedSRTCues(ctx, cues, sourceLanguage, progress); err != nil {
		return err
	}

	if err := validateSimplifiedChineseCues(cues, sourceLanguage); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, renderGeneratedSRT(cues), 0o644); err != nil {
		return fmt.Errorf("write Simplified Chinese SRT: %w", err)
	}
	return nil
}

func parseGeneratedSRT(content []byte) ([]generatedSRTCue, error) {
	text := strings.TrimPrefix(string(content), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	blocks := srtBlockSeparatorPattern.Split(strings.TrimSpace(text), -1)
	cues := make([]generatedSRTCue, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		timeline := -1
		for index, line := range lines {
			if strings.Contains(line, "-->") {
				timeline = index
				break
			}
		}
		if timeline < 0 || timeline == len(lines)-1 {
			return nil, fmt.Errorf("invalid generated SRT block: %q", truncateSubtitleText(block, 80))
		}
		source := strings.TrimSpace(strings.Join(lines[timeline+1:], "\n"))
		if source == "" {
			continue
		}
		cues = append(cues, generatedSRTCue{
			prefix: strings.Join(lines[:timeline+1], "\n"),
			source: source,
		})
	}
	return cues, nil
}

func translateGeneratedSRTCues(
	ctx context.Context,
	cues []generatedSRTCue,
	sourceLanguage string,
	progress func(completed, total int) error,
) error {
	for start := 0; start < len(cues); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := subtitleTranslationBatchEnd(cues, start)
		var translated []string
		for {
			var err error
			translated, err = requestDeepSeekSubtitleTranslation(ctx, cues[start:end], sourceLanguage, start)
			if err == nil {
				break
			}
			if errors.Is(err, errDeepSeekOutputLength) && end-start > 1 {
				end = start + (end-start)/2
				continue
			}
			return fmt.Errorf("translate cues %d-%d with DeepSeek: %w", start+1, end, err)
		}
		for index, text := range translated {
			cues[start+index].translated = text
		}
		start = end
		if progress != nil {
			if err := progress(start, len(cues)); err != nil {
				return err
			}
		}
	}
	return nil
}

func subtitleTranslationBatchEnd(cues []generatedSRTCue, start int) int {
	end := start
	runes := 0
	for end < len(cues) && end-start < deepSeekBatchMaxCues {
		next := utf8.RuneCountInString(cues[end].source)
		if end > start && runes+next > deepSeekBatchMaxRunes {
			break
		}
		runes += next
		end++
	}
	return end
}

type deepSeekSubtitleInput struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type deepSeekSubtitleOutput struct {
	Translations []deepSeekSubtitleInput `json:"translations"`
}

type deepSeekChatRequest struct {
	Model          string            `json:"model"`
	Messages       []deepSeekMessage `json:"messages"`
	Thinking       deepSeekThinking  `json:"thinking"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens"`
	ResponseFormat deepSeekFormat    `json:"response_format"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekFormat struct {
	Type string `json:"type"`
}

type deepSeekThinking struct {
	Type string `json:"type"`
}

type deepSeekChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func requestDeepSeekSubtitleTranslation(
	ctx context.Context,
	cues []generatedSRTCue,
	sourceLanguage string,
	offset int,
) ([]string, error) {
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

	items := make([]deepSeekSubtitleInput, 0, len(cues))
	for index, cue := range cues {
		items = append(items, deepSeekSubtitleInput{ID: offset + index + 1, Text: cue.source})
	}
	inputJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode subtitle batch: %w", err)
	}
	systemPrompt := `You are a professional subtitle translator. Translate every input item into natural Simplified Chinese. Preserve meaning, tone, names, punctuation, and line breaks. Do not omit, censor, summarize, merge, split, or explain any text. Return only a JSON object in exactly this shape: {"translations":[{"id":1,"text":"translated text"}]}. Keep every original id unchanged and return exactly one item for every input item.`
	userPrompt := fmt.Sprintf("Source language: %s\nTranslate this JSON array into Simplified Chinese:\n%s", sourceLanguage, inputJSON)
	payload, err := json.Marshal(deepSeekChatRequest{
		Model: model,
		Messages: []deepSeekMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Thinking:       deepSeekThinking{Type: "disabled"},
		Temperature:    0.1,
		MaxTokens:      8192,
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
		request.Header.Set("User-Agent", "JavBoss subtitle translator")
		response, err := subtitleTranslationHTTPClient.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
			closeErr := response.Body.Close()
			if readErr == nil && closeErr != nil {
				readErr = closeErr
			}
			if response.StatusCode >= 200 && response.StatusCode < 300 && readErr == nil {
				translated, parseErr := parseDeepSeekSubtitleTranslation(body, items)
				if parseErr == nil {
					return translated, nil
				}
				if errors.Is(parseErr, errDeepSeekOutputLength) {
					return nil, parseErr
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

func parseDeepSeekSubtitleTranslation(body []byte, expected []deepSeekSubtitleInput) ([]string, error) {
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
	if content == "" {
		return nil, errors.New("DeepSeek response contains empty content")
	}
	var output deepSeekSubtitleOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("decode DeepSeek translation JSON: %w", err)
	}
	if len(output.Translations) != len(expected) {
		return nil, fmt.Errorf("DeepSeek returned %d translations, want %d", len(output.Translations), len(expected))
	}
	byID := make(map[int]string, len(output.Translations))
	for _, item := range output.Translations {
		text := strings.TrimSpace(item.Text)
		if item.ID <= 0 || text == "" {
			return nil, errors.New("DeepSeek returned an invalid or empty translation item")
		}
		if _, duplicate := byID[item.ID]; duplicate {
			return nil, fmt.Errorf("DeepSeek returned duplicate subtitle id %d", item.ID)
		}
		byID[item.ID] = text
	}
	translated := make([]string, len(expected))
	for index, item := range expected {
		text, ok := byID[item.ID]
		if !ok {
			return nil, fmt.Errorf("DeepSeek omitted subtitle id %d", item.ID)
		}
		translated[index] = text
	}
	return translated, nil
}

func validateSimplifiedChineseCues(cues []generatedSRTCue, sourceLanguage string) error {
	var source strings.Builder
	var translated strings.Builder
	for _, cue := range cues {
		if strings.TrimSpace(cue.translated) == "" {
			return errors.New("translated SRT contains an empty cue")
		}
		source.WriteString(cue.source)
		translated.WriteString(cue.translated)
	}
	translatedText := translated.String()
	if sourceLanguage != "zh-CN" && normalizeSubtitleComparison(source.String()) == normalizeSubtitleComparison(translatedText) {
		return errors.New("translation did not change the source subtitle")
	}
	han, kana := countChineseAndJapaneseRunes(translatedText)
	if han == 0 {
		return errors.New("translated SRT contains no Chinese characters")
	}
	if kana > 5 && kana*4 > han {
		return fmt.Errorf("translated SRT still contains too much Japanese text (Chinese=%d, kana=%d)", han, kana)
	}
	return nil
}

func renderGeneratedSRT(cues []generatedSRTCue) []byte {
	var output bytes.Buffer
	for index, cue := range cues {
		if index > 0 {
			output.WriteString("\r\n\r\n")
		}
		output.WriteString(cue.prefix)
		output.WriteString("\r\n")
		output.WriteString(strings.TrimSpace(cue.translated))
	}
	output.WriteString("\r\n")
	return output.Bytes()
}

func normalizeTranslationSourceLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ja":
		return "ja"
	case "zh", "zh-cn":
		return "zh-CN"
	case "en":
		return "en"
	default:
		return "auto"
	}
}

func normalizeSubtitleComparison(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func countChineseAndJapaneseRunes(value string) (int, int) {
	han := 0
	kana := 0
	for _, r := range value {
		switch {
		case unicode.In(r, unicode.Han):
			han++
		case r >= 0x3040 && r <= 0x30ff:
			kana++
		}
	}
	return han, kana
}

func truncateSubtitleText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "…"
}
