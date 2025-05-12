package utils

import (
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
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

// OllamaEmbedding 调用 Ollama 的 embedding API 获取单条查询的向量
func OllamaEmbedding(query string, dim int) ([]float32, error) {
	cfg, err := getConfig()
	if cfg == nil || err != nil {
		return nil, err
	}

	url := cfg.ApiBaseUrl + cfg.EmbeddingApi
	payload := map[string]interface{}{
		"model": cfg.EmbeddingModel,
		"input": query,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
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

	// 保持长度与 dim 一致
	if len(embedding) > dim {
		embedding = embedding[:dim]
	} else if len(embedding) < dim {
		for i := len(embedding); i < dim; i++ {
			embedding = append(embedding, 0)
		}
	}
	return embedding, nil
}

// OllamaEmbeddingsList 调用 Ollama 的 embedding API 获取多条查询的向量
func OllamaEmbeddingsList(queries []string, dim int) ([][]float32, error) {
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

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return nil, err
	}

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return nil, err
	}

	raw, ok := result["embeddings"]
	if !ok {
		return nil, fmt.Errorf("no embeddings field in response")
	}
	rawList, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("embeddings field is not a list")
	}

	embeddings := make([][]float32, len(rawList))
	for i, item := range rawList {
		sliceRaw, ok := item.([]interface{})
		if !ok {
			return nil, fmt.Errorf("embeddings[%d] is not a list", i)
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
