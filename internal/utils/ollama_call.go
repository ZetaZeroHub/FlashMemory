package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// GetModelConfigByPromptLength 根据提示词长度动态选择合适的模型和参数配置
func GetModelConfigByPromptLength(promptLength int) (model string, maxTokens int, numCtx int, numKeep int, numPredict int, repeatLastN int, presencePenalty int, frequencyPenalty int, format string) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", 0, 0, 0, 0, 0, 0, 0, ""
	}
	var selected *config.ModelConfig
	for i := len(cfg.ModelConfigs) - 1; i >= 0; i-- {
		mc := cfg.ModelConfigs[i]
		if promptLength >= mc.PromptLength {
			selected = &mc
			break
		}
	}
	if selected == nil || promptLength > 30000 {
		return "", 0, 0, 0, 0, 0, 0, 0, ""
	}
	return selected.Name, selected.MaxTokens, selected.NumCtx, selected.NumKeep, selected.NumPredict, selected.RepeatLastN, selected.PresencePenalty, selected.FrequencyPenalty, selected.Format
}

// OllamaCompletion 调用 Ollama 的 completion API 获取完整回答
// NormalizeResponseWithSmallModel 使用小模型规范化大模型的返回结果
func NormalizeResponseWithSmallModel(rawResponse string) (string, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", err
	}
	prompt := fmt.Sprintf(`请将以下格式不规范的文本优化成一个合法的JSON对象，必须包含"功能描述"中文字段Key：\n%s`, rawResponse)
	url := cfg.ApiBaseUrl + cfg.CompletionApi
	payload := map[string]interface{}{
		"model":  cfg.NormalizeModel,
		"prompt": prompt,
		"stream": false,
		"format": cfg.DefaultFormat,
		"options": map[string]interface{}{
			"temperature": cfg.DefaultTemp,
			"low_vram":    cfg.DefaultLowVram,
		},
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("invalid JSON response from small model: %v", err)
	}
	response, ok := result["response"]
	if !ok {
		return "", fmt.Errorf("no response field in small model result")
	}
	responseStr := response.(string)
	var responseObj map[string]interface{}
	if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
		return "", fmt.Errorf("invalid JSON in small model response: %v", err)
	}
	if _, ok := responseObj["功能描述"]; !ok {
		return "", fmt.Errorf("missing '功能描述' field in small model response")
	}
	return responseStr, nil
}

func OllamaCompletion(prompt string) (string, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", err
	}
	l := len(prompt)
	autoModel, maxTokens, numCtx, numKeep, numPredict, repeatLastN, presencePenalty, frequencyPenalty, format := GetModelConfigByPromptLength(l)
	if autoModel == "" {
		return "[Warning]代码内容过大，请单独处理", nil
	}
	logs.Infof("Using model: %s", autoModel)
	url := cfg.ApiBaseUrl + cfg.CompletionApi
	optionLoad := map[string]interface{}{
		"temperature":       cfg.DefaultTemp,
		"presence_penalty":  presencePenalty,
		"frequency_penalty": frequencyPenalty,
		"max_tokens":        maxTokens,
		"num_ctx":           numCtx,
		"num_keep":          numKeep,
		"num_predict":       numPredict,
		"repeat_last_n":     repeatLastN,
		"low_vram":          cfg.DefaultLowVram,
	}
	payload := map[string]interface{}{
		"model":         autoModel,
		"prompt":        prompt,
		"stream":        false,
		"format":        format,
		"options":       optionLoad,
		"keep_alive":    "30m",
		"max_tokens":    maxTokens,
		"num_ctx":       numCtx,
		"num_keep":      numKeep,
		"num_predict":   numPredict,
		"repeat_last_n": repeatLastN,
		"low_vram":      cfg.DefaultLowVram,
	}
	var lastErr error
	tempjson := ""
	for retries := 0; retries < 3; retries++ {
		if retries > 0 {
			logs.Infof("Retrying request, attempt %d/3", retries+1)
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			lastErr = err
			continue
		}
		client := &http.Client{Timeout: 30 * time.Minute}
		req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			logs.Infof("Response is not valid JSON, retrying...")
			lastErr = fmt.Errorf("invalid JSON response: %v", err)
			continue
		}
		response, ok := result["response"]
		if !ok {
			lastErr = fmt.Errorf("no response field in result")
			continue
		}
		responseStr := response.(string)
		if strings.Contains(responseStr, "</think>") {
			parts := strings.SplitAfter(responseStr, "</think>")
			if len(parts) > 1 {
				logs.Infof("移除<think>标签")
				responseStr = strings.TrimSpace(parts[1])
			}
		}
		if format == "json" {
			var responseObj map[string]interface{}
			if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
				tempjson = responseStr
				// 尝试使用小模型规范化返回结果
				normalizedResponse, err := NormalizeResponseWithSmallModel(responseStr)
				if err != nil {
					logs.Infof("Normalized Response content is not valid JSON: %s", normalizedResponse)
					continue
				}
				responseStr = normalizedResponse
				logs.Infof("Normalized Response: %s", normalizedResponse)
			}
			if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
				logs.Infof("Response content is not valid JSON, retrying...")
				lastErr = fmt.Errorf("invalid JSON in response content: %v", err)
				continue
			}
			if _, ok := responseObj["description"]; !ok {
				logs.Infof("Response missing 'description' field, retrying...")
				lastErr = fmt.Errorf("missing 'description' field in response")
				continue
			}
		}
		return responseStr, nil
	}
	if lastErr != nil {
		logs.Infof("All retries failed with large model, attempting to normalize last response with small model")
		if normalizedResponse, err := NormalizeResponseWithSmallModel(tempjson); err == nil {
			return normalizedResponse, nil
		}
	}
	return "", fmt.Errorf("all retry attempts failed, last error: %v", lastErr)
}

// OllamaEmbedding 调用 Ollama 的 embedding API 获取查询向量
func OllamaEmbedding(query string, dim int) ([]float32, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return nil, err
	}
	url := cfg.ApiBaseUrl + cfg.EmbeddingApi
	payload := map[string]interface{}{
		"model":  cfg.EmbeddingModel,
		"prompt": query,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	embeddingRaw, ok := result["embedding"]
	if !ok {
		return nil, fmt.Errorf("no embedding field in response")
	}
	embeddingSlice, ok := embeddingRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("embedding field is not a slice")
	}
	embedding := make([]float32, 0, len(embeddingSlice))
	for _, v := range embeddingSlice {
		if num, ok := v.(float64); ok {
			embedding = append(embedding, float32(num))
		}
	}
	if len(embedding) != dim {
		if len(embedding) > dim {
			embedding = embedding[:dim]
		} else {
			for i := len(embedding); i < dim; i++ {
				embedding = append(embedding, 0)
			}
		}
	}
	return embedding, nil
}
