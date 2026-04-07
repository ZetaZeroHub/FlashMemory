package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// Completion 调用 Ollama 的 completion API 获取完整回答
// NormalizeResponseWithSmallModel 使用小模型规范化大模型的返回结果
func NormalizeResponseWithSmallModel(rawResponse string) (string, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", err
	}
	prompt := fmt.Sprintf(`Please optimize the following non-standard format text into a legal JSON object, which must contain the "Function Description" Chinese field Key:\n%s`, rawResponse)
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
		// 如果是EOF错误，直接返回普通错误而不是LLMResponseError
		if strings.Contains(err.Error(), "EOF") {
			logs.Warnf("NormalizeResponseWithSmallModel encountered EOF error: %v", err)
			return "", err
		}
		return "", common.NewLLMResponseError(err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", common.NewLLMResponseError(err.Error())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", common.NewLLMResponseError(fmt.Errorf("invalid JSON response from small model: %v", err).Error())
	}
	response, ok := result["response"]
	if !ok {
		return "", common.NewLLMResponseError("no response field in small model result")
	}
	responseStr := response.(string)
	var responseObj map[string]interface{}
	if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
		return "", common.NewLLMResponseError(fmt.Errorf("invalid JSON in small model response: %v", err).Error())
	}
	if _, ok := responseObj["功能描述"]; !ok {
		return "", common.NewLLMResponseError("missing '功能描述' field in small model response")
	}
	return responseStr, nil
}

func DefaultModelCompletion(rawResponse string) (string, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", err
	}
	logs.Infof("Keyword search for query: %s", rawResponse)
	if cfg.DefaultCloudModel.Enabled {
		invoke, err := cloud.KeywordInvoke(&cfg.DefaultCloudModel, &cfg.KeywordPrompts, rawResponse)
		if err != nil {
			logs.Errorf("Failed to invoke keyword model: %v", err)
			return "", err
		}
		return FilterJSONContent(invoke), nil
	} else {
		logs.Infof("Using model: %s/%s", cfg.ApiBaseUrl+cfg.CompletionApi, cfg.DefaultModel)
		ctx := context.Background()
		template := cloud.CreateTemplate(cfg.KeywordPrompts.System, cfg.KeywordPrompts.User)
		m := map[string]any{}
		m["search_query"] = rawResponse
		// 使用模板生成消息
		messages, err := template.Format(ctx, m)
		if err != nil {
			logs.Errorf("format template failed: %v\n", err)
			return "", err
		}
		cm, err := cloud.CreateOllamaModel(ctx, cfg.ApiBaseUrl, cfg.DefaultModel)
		if err != nil {
			logs.Errorf("create chat model failed: %v", err)
			return "", err
		}
		logs.Infof("messages: %v", messages)
		msg, err := cloud.Generate(ctx, cm, messages)
		if err != nil {
			logs.Errorf("generate failed: %v", err)
			return "", err
		}
		return FilterJSONContent(msg.Content), nil
	}
}

func Completion(prompt string) (string, error) {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return "", err
	}
	l := len(prompt)
	//截取
	if l > cfg.PromptLimit {
		logs.Infof("The content of the prompt word is too long, the first %d characters are intercepted", cfg.PromptLimit)
		prompt = prompt[:cfg.PromptLimit]
	}
	modelConfig := cloud.GetModelConfigByPromptLength(l)
	if modelConfig == nil {
		logs.Infof("No model configuration found for prompt length %d", l)
		return "", nil
	}
	if modelConfig.CloudModel.Enabled {
		logs.Infof("Using cloud model: %s", modelConfig.CloudModel.Model)
		return cloud.ANAInvoke(&modelConfig.CloudModel, prompt)
	}

	logs.Infof("Using model: %s", modelConfig.Name)
	optionLoad := map[string]interface{}{
		"temperature":       cfg.DefaultTemp,
		"presence_penalty":  modelConfig.PresencePenalty,
		"frequency_penalty": modelConfig.FrequencyPenalty,
		"max_tokens":        modelConfig.MaxTokens,
		"num_ctx":           modelConfig.NumCtx,
		"num_keep":          modelConfig.NumKeep,
		"num_predict":       modelConfig.NumPredict,
		"repeat_last_n":     modelConfig.RepeatLastN,
		"low_vram":          cfg.DefaultLowVram,
	}
	payload := map[string]interface{}{
		"model":         modelConfig.Name,
		"prompt":        prompt,
		"stream":        false,
		"format":        modelConfig.Format,
		"options":       optionLoad,
		"keep_alive":    "30m",
		"max_tokens":    modelConfig.MaxTokens,
		"num_ctx":       modelConfig.NumCtx,
		"num_keep":      modelConfig.NumKeep,
		"num_predict":   modelConfig.NumPredict,
		"repeat_last_n": modelConfig.RepeatLastN,
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
		// 改进HTTP客户端配置，禁用连接复用避免EOF错误
		transport := &http.Transport{
			DisableKeepAlives:   true, // 禁用连接复用
			DisableCompression:  false,
			MaxIdleConns:        0, // 不保持空闲连接
			MaxIdleConnsPerHost: 0, // 不保持每个主机的空闲连接
			IdleConnTimeout:     0, // 立即关闭空闲连接
		}
		client := &http.Client{
			Timeout:   30 * time.Minute,
			Transport: transport,
		}

		req, err := http.NewRequest("POST", cfg.ApiBaseUrl+cfg.CompletionApi, strings.NewReader(string(jsonPayload)))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connection", "close") // 明确要求关闭连接
		resp, err := client.Do(req)
		if err != nil {
			// 添加详细的网络错误日志
			logs.Errorf("HTTP request failed (retry %d/3): %v", retries+1, err)
			logs.Errorf("Request details - URL: %s, prompt length: %d", cfg.ApiBaseUrl+cfg.CompletionApi, len(prompt))
			if strings.Contains(err.Error(), "EOF") {
				logs.Warnf("An EOF error was detected. It may be that the server closed the connection prematurely.")
				// EOF错误直接返回普通错误，不包装为LLMResponseError
				lastErr = err
			} else if strings.Contains(err.Error(), "connection reset") {
				logs.Warnf("Connection reset error detected, may be due to network instability")
				lastErr = common.NewLLMResponseError(err.Error())
			} else if strings.Contains(err.Error(), "timeout") {
				logs.Warnf("Timeout error detected, possibly slow server response")
				lastErr = common.NewLLMResponseError(err.Error())
			} else {
				lastErr = common.NewLLMResponseError(err.Error())
			}
			continue
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = common.NewLLMResponseError(err.Error())
			continue
		}
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			logs.Infof("Response is not valid JSON, retrying...")
			lastErr = common.NewLLMResponseError(fmt.Errorf("invalid JSON response: %v", err).Error())
			continue
		}
		response, ok := result["response"]
		if !ok {
			lastErr = common.NewLLMResponseError("no response field in result")
			continue
		}
		responseStr := response.(string)
		if strings.Contains(responseStr, "</think>") {
			parts := strings.SplitAfter(responseStr, "</think>")
			if len(parts) > 1 {
				logs.Infof("Remove <think> tag")
				responseStr = strings.TrimSpace(parts[1])
			}
		}
		if modelConfig.Format == "json" {
			var responseObj map[string]interface{}
			if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
				tempjson = responseStr
				normalizedResponse, normErr := NormalizeResponseWithSmallModel(responseStr)
				if normErr != nil {
					logs.Errorf("NormalizeResponseWithSmallModel failed (attempt %d/3), raw response: %.200s, err: %v", retries+1, responseStr, normErr)
					lastErr = normErr
					continue
				}
				responseStr = normalizedResponse
				logs.Infof("Normalized Response: %.200s", normalizedResponse)
			}
			if err := json.Unmarshal([]byte(responseStr), &responseObj); err != nil {
				logs.Infof("Response content is not valid JSON, retrying...")
				lastErr = common.NewLLMResponseError(fmt.Errorf("invalid JSON in response content: %v", err).Error())
				continue
			}
			if _, ok := responseObj["description"]; !ok {
				logs.Infof("Response missing 'description' field, retrying...")
				lastErr = common.NewLLMResponseError("missing 'description' field in response")
				continue
			}
		}
		return responseStr, nil
	}
	if lastErr != nil {
		if tempjson != "" {
			logs.Infof("All retries failed with large model output, attempting final normalization with small model")
			if normalizedResponse, err := NormalizeResponseWithSmallModel(tempjson); err == nil {
				return normalizedResponse, nil
			} else {
				logs.Errorf("Final NormalizeResponseWithSmallModel failed, raw response: %.200s, err: %v", tempjson, err)
				return "", fmt.Errorf("small model failed to normalize response after retries: %v", err)
			}
		}
		if strings.Contains(lastErr.Error(), "EOF") {
			return "", lastErr
		}
		return "", common.NewLLMResponseError(fmt.Errorf("all retry attempts failed, last error: %v", lastErr).Error())
	}
	return "", fmt.Errorf("all retry attempts failed without specific error")
}
