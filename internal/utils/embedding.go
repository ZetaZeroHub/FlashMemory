package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

var (
	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    true,
		},
		Timeout: 60 * time.Second,
	}

	// 配置只加载一次
	cfg     *config.Config
	cfgErr  error
	cfgOnce sync.Once
)

// getConfig 确保配置只加载一次
func getConfig() (*config.Config, error) {
	cfgOnce.Do(func() {
		cfg, cfgErr = config.LoadConfig()
		if cfg == nil || cfgErr != nil {
			logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", cfgErr)
		}
	})
	return cfg, cfgErr
}

func EmbeddingsListOnlyOllama(queries []string, dim int) ([][]float32, error) {
	cfg, err := getConfig()
	if cfg == nil || err != nil {
		return nil, err
	}
	url := cfg.ApiBaseUrl + cfg.EmbeddingApi
	payload := map[string]interface{}{
		"model": cfg.EmbeddingModel,
		"input": queries,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	logs.Infof("EmbeddingsList: %s, %d", url, len(jsonPayload))

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		logs.Errorf("Ollama embedding HTTP request failed: %v", err)
		return nil, common.NewLLMResponseError(err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Errorf("Ollama embedding response read failed: %v", err)
		return nil, common.NewLLMResponseError(err.Error())
	}

	// Log response body size and status for debugging
	logs.Infof("Ollama embedding response: status=%d, body_size=%d bytes", resp.StatusCode, len(body))

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		logs.Errorf("Ollama embedding response JSON parse failed: %v, body=%s", err, string(body[:min(len(body), 200)]))
		return nil, common.NewLLMResponseError(err.Error())
	}

	raw, ok := result["embeddings"]
	if !ok {
		// Log actual response keys and truncated body for debugging
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		logs.Errorf("Ollama response missing 'embeddings' field. Available keys: %v, body_preview=%s",
			keys, string(body[:min(len(body), 300)]))
		return nil, common.NewLLMResponseError("no embeddings field in response")
	}
	rawList, ok := raw.([]interface{})
	if !ok {
		logs.Errorf("Ollama 'embeddings' field is not a list, actual type: %T", raw)
		return nil, common.NewLLMResponseError("embeddings field is not a list")
	}

	embeddings := make([][]float32, len(rawList))
	for i, item := range rawList {
		sliceRaw, ok := item.([]interface{})
		if !ok {
			logs.Errorf("Ollama embeddings[%d] is not a list, actual type: %T", i, item)
			return nil, common.NewLLMResponseError(fmt.Errorf("embeddings[%d] is not a list", i).Error())
		}
		vec := make([]float32, 0, len(sliceRaw))
		for _, v := range sliceRaw {
			if num, ok := v.(float64); ok {
				vec = append(vec, float32(num))
			}
		}
		if len(vec) > dim {
			vec = vec[:dim]
		} else if len(vec) < dim {
			for j := len(vec); j < dim; j++ {
				vec = append(vec, 0)
			}
		}
		embeddings[i] = vec
	}
	return embeddings, nil
}

// EmbeddingsList 调用 Ollama 的 embedding API 获取多条查询的向量
func EmbeddingsList(queries []string, dim int) ([][]float32, error) {
	cfg, err := getConfig()
	if cfg == nil || err != nil {
		return nil, err
	}
	var res [][]float32
	if cfg.EmbeddingCloudModel.Enabled {
		logs.Infof("Use Cloud Model: %s", cfg.EmbeddingCloudModel.Model)
		res, err = cloud.EmbeddingInvoke(&cfg.EmbeddingCloudModel, queries, dim)
		if err != nil {
			logs.Infof("Back To use Ollama Model: %s", cfg.EmbeddingModel)
			res, err = EmbeddingsListOnlyOllama(queries, dim)
		}
	} else {
		logs.Infof("Use Ollama Model: %s", cfg.EmbeddingModel)
		res, err = EmbeddingsListOnlyOllama(queries, dim)
	}
	return res, err
}
