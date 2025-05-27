package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/config"
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
		logs.Infof("提示词内容过长，截取前%d个字符", cfg.PromptLimit)
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
		client := &http.Client{Timeout: 30 * time.Minute}
		req, err := http.NewRequest("POST", cfg.ApiBaseUrl+cfg.CompletionApi, strings.NewReader(string(jsonPayload)))
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
		if modelConfig.Format == "json" {
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
