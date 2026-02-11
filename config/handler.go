package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// Response API响应结构
type Response struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Handler 配置处理器
type Handler struct {
	configPath string
}

// NewHandler 创建配置处理器实例
func NewHandler(configPath string) *Handler {
	return &Handler{configPath: configPath}
}

// 辅助函数：返回JSON响应
func responseWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// 辅助函数：返回错误响应
func responseWithError(w http.ResponseWriter, code int, message string) {
	responseWithJSON(w, code, Response{
		Status:  code,
		Message: message,
	})
}

// GetConfig 处理 GET /config 请求
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := GetConfig(h.configPath)
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, fmt.Sprintf("加载配置文件失败: %v", err))
		return
	}
	responseWithJSON(w, http.StatusOK, cfg)
}

// UpdateConfig 处理 PUT /config 请求
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	jsonData, err := ioutil.ReadAll(r.Body)
	if err != nil {
		responseWithError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	if err := UpdateConfig(h.configPath, jsonData); err != nil {
		responseWithError(w, http.StatusInternalServerError, fmt.Sprintf("更新配置失败: %v", err))
		return
	}
	responseWithJSON(w, http.StatusOK, Response{
		Status:  http.StatusOK,
		Message: "配置更新成功",
	})
}

// GetConfig 从指定的配置文件中读取 YAML 格式的配置，并解析为 Config 对象。
func GetConfig(filePath string) (*Config, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML config: %v", err)
	}
	return &cfg, nil
}

// UpdateConfig 接收 JSON 格式的配置数据，将其与原始配置进行合并后转换为 YAML 格式写回到配置文件。
func UpdateConfig(filePath string, jsonData []byte) error {
	// 1. 读取原始配置
	origCfg, err := GetConfig(filePath)
	if err != nil {
		return fmt.Errorf("failed to read original config: %v", err)
	}

	// 2. 将原始配置转换为 map[string]interface{}
	origJson, err := json.Marshal(origCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal original config: %v", err)
	}
	var origMap map[string]interface{}
	if err := json.Unmarshal(origJson, &origMap); err != nil {
		return fmt.Errorf("failed to unmarshal original config to map: %v", err)
	}

	// 3. 解析请求的 JSON 数据为 map[string]interface{}
	var updateMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &updateMap); err != nil {
		return fmt.Errorf("failed to unmarshal input JSON: %v", err)
	}

	// 4. 合并更新：对于更新 JSON 中存在的键，替换或递归更新原始配置 map 中对应的值
	mergedMap := mergeMaps(origMap, updateMap)

	// 5. 将合并后的 map 转换为 YAML 格式
	updatedYaml, err := yaml.Marshal(mergedMap)
	if err != nil {
		return fmt.Errorf("failed to marshal merged config to YAML: %v", err)
	}
	logs.Infof("updatedYaml: %s", updatedYaml)

	// 6. 写回到配置文件
	if err := ioutil.WriteFile(filePath, updatedYaml, 0644); err != nil {
		return fmt.Errorf("failed to write merged config to file: %v", err)
	}

	return nil
}

// mergeMaps 以第一层配置为准合并两个 map，跳过 null 值和空字符串的更新
func mergeMaps(baseMap, updateMap map[string]interface{}) map[string]interface{} {
	for k, v := range updateMap {
		// 跳过 null 值或空字符串的更新
		if v == nil {
			continue
		}
		if strVal, ok := v.(string); ok && strVal == "" {
			continue
		}

		// 以第一层配置为准：只更新第一层的键值，不进行递归合并
		baseMap[k] = v
	}
	return baseMap
}
