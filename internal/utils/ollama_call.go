package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// GetModelConfigByPromptLength 根据提示词长度动态选择合适的模型和参数配置
func GetModelConfigByPromptLength(promptLength int) (model string, maxTokens int, numCtx int, numKeep int, numPredict int) {
	// 默认使用最小模型
	model = "qwen2.5-coder:1.5b"
	maxTokens = 2048
	numCtx = 1024
	numKeep = 2048
	numPredict = 512

	// 根据提示词长度动态调整模型大小
	if promptLength > 1500 {
		model = "qwen2.5-coder:3b"
		maxTokens = 2048 * 2
		numCtx = 1024 * 2
		numKeep = 2048 * 2
		numPredict = 512
	}

	if promptLength > 5000 {
		model = "qwen2.5-coder:7b"
		maxTokens = 2048 * 2 * 1.5
		numCtx = 1024 * 2 * 1.5
		numKeep = 2048 * 2 * 1.5
		numPredict = 512
	}

	if promptLength > 10000 {
		model = "qwen2.5-coder:7b"
		maxTokens = 10000
		numCtx = 6000
		numKeep = 10000
		numPredict = 512
	}

	if promptLength > 20000 {
		model = "qwen2.5-coder:7b"
		maxTokens = 10000 * 2
		numCtx = 6000 * 2
		numKeep = 10000 * 2
		numPredict = 512
	}

	return
}

// OllamaCompletion 调用 Ollama 的 completion API 获取完整回答
// NormalizeResponseWithSmallModel 使用小模型规范化大模型的返回结果
func NormalizeResponseWithSmallModel(rawResponse string) (string, error) {
	prompt := fmt.Sprintf(`请将以下文本规范化为一个合法的JSON对象，必须包含"功能描述"字段：
%s`, rawResponse)

	url := "http://127.0.0.1:11434/api/generate"
	payload := map[string]interface{}{
		"model":  "qwen2.5-coder:1.5b",
		"prompt": prompt,
		"stream": false,
		"format": "json",
		"options": map[string]interface{}{
			"temperature": 0.1,
			"low_vram":    true,
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
	l := len(prompt)
	autoModel, maxTokens, numCtx, numKeep, numPredict := GetModelConfigByPromptLength(l)
	logs.Infof("Using model: %s", autoModel)

	url := "http://127.0.0.1:11434/api/generate"
	// 可选参数
	optionLoad := map[string]interface{}{
		"temperature":       0.1,
		"presence_penalty":  0,
		"frequency_penalty": 0,
		"max_tokens":        maxTokens,
		"num_ctx":           numCtx,
		"num_keep":          numKeep,
		"num_predict":       numPredict,
		"repeat_last_n":     128,
		"low_vram":          true,
	}
	payload := map[string]interface{}{
		"model":         autoModel,
		"prompt":        prompt,
		"stream":        false,
		"format":        "json",
		"options":       optionLoad,
		"keep_alive":    "30m",
		"max_tokens":    maxTokens,
		"num_ctx":       numCtx,
		"num_keep":      numKeep,
		"num_predict":   numPredict,
		"repeat_last_n": 128,
		"low_vram":      true,
	}

	var lastErr error
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

		// 尝试解析为JSON
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			logs.Infof("Response is not valid JSON, retrying...")
			lastErr = fmt.Errorf("invalid JSON response: %v", err)
			continue
		}

		// 检查response字段
		response, ok := result["response"]
		if !ok {
			lastErr = fmt.Errorf("no response field in result")
			continue
		}

		// 尝试解析response为JSON并检查功能描述字段
		var responseObj map[string]interface{}
		responseStr := response.(string)
		if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
			logs.Infof("Response content is not valid JSON, retrying..., %s", responseStr)
			lastErr = fmt.Errorf("invalid JSON in response content: %v", err)
			continue
		}

		// 检查功能描述字段
		if _, ok := responseObj["功能描述"]; !ok {
			logs.Infof("Response missing '功能描述' field, retrying...")
			lastErr = fmt.Errorf("missing '功能描述' field in response")
			continue
		}

		// 所有检查都通过，返回结果
		return responseStr, nil
	}

	// 所有重试都失败
	if lastErr != nil {
		logs.Infof("All retries failed with large model, attempting to normalize last response with small model")
		if normalizedResponse, err := NormalizeResponseWithSmallModel(lastErr.Error()); err == nil {
			return normalizedResponse, nil
		}
	}
	return "", fmt.Errorf("all retry attempts failed, last error: %v", lastErr)
}

// OllamaEmbedding 调用 Ollama 的 embedding API 获取查询向量
func OllamaEmbedding(query string, dim int) ([]float32, error) {
	url := "http://127.0.0.1:11434/api/embeddings"
	payload := map[string]interface{}{
		"model":  "nomic-embed-text",
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
	// 如需添加API key可在此设置请求头
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// 解析返回的 JSON 格式，假设返回 {"embedding": [float32 array]}
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
		// 若维度不匹配，则进行截断或填充
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
