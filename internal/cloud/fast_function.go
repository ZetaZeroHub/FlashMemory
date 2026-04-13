package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/ollama/ollama/api"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

func deadlineInfo(ctx context.Context) (string, time.Duration) {
	if ctx == nil {
		return "none", 0
	}
	d, ok := ctx.Deadline()
	if !ok {
		return "none", 0
	}
	remaining := time.Until(d)
	if remaining < 0 {
		remaining = 0
	}
	return d.Format(time.RFC3339Nano), remaining
}

// 字典枚举
var (
	OPENAI  = "openai"
	OLLAMA  = "ollama"
	QWEN    = "qwen"
	GITHAVE = "githave"
)

// EmbeddingData 对应接口返回的每一条 embedding
type EmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbedResponse 定义了 /v1/embeddings 接口的响应体
type EmbedResponse struct {
	Data  []EmbeddingData `json:"data"`
	Model string          `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func CreateTemplate(sys, user string) prompt.ChatTemplate {
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage(sys),
		// 插入需要的对话历史（新对话的话这里不填）
		schema.MessagesPlaceholder("chat_history", true),
		// 用户消息模板
		schema.UserMessage(user),
	)
}

func CreateSimpleTemplate(user string) prompt.ChatTemplate {
	return prompt.FromMessages(schema.Jinja2,
		// 插入需要的对话历史（新对话的话这里不填）
		schema.MessagesPlaceholder("chat_history", true),
		// 用户消息模板
		schema.UserMessage(user),
	)
}

func CreateANATemplate(ctx context.Context, prompts string, hasHis bool, history []*schema.Message) ([]*schema.Message, error) {
	template := CreateTemplate("", prompts)
	m := map[string]any{}
	if hasHis {
		m["has_history"] = history
	}
	// 使用模板生成消息
	messages, err := template.Format(ctx, m)
	if err != nil {
		logs.Errorf("format template failed: %v\n", err)
		return nil, err
	}
	return messages, nil
}

func CreateChatModel(ctx context.Context, config *config.CloudModel) (model model.ChatModel, err error) {
	_, remaining := deadlineInfo(ctx)
	switch config.Type {
	case OPENAI:
		modelConfig := openai.ChatModelConfig{
			APIKey:  config.Api,
			Model:   config.Model,
			BaseURL: config.Url,
		}
		if remaining > 0 {
			modelConfig.Timeout = remaining
		} else {
			// 如果没有 remaining (即没有 deadline)，或者 remaining 为 0，给一个保底的 Timeout
			// 防止 HTTP Client 使用默认超时（通常较短或无限等待）
			modelConfig.Timeout = 120 * time.Second
		}
		if config.Temperature != 0 {
			modelConfig.Temperature = &config.Temperature
		}
		cm, err := openai.NewChatModel(ctx, &modelConfig)
		return cm, err
	case OLLAMA:
		modelConfig := ollama.ChatModelConfig{
			BaseURL: config.Url,
			Model:   config.Model,
		}
		if config.Temperature != 0 {
			modelConfig.Options.Temperature = config.Temperature
		}
		cm, err := ollama.NewChatModel(ctx, &modelConfig)
		return cm, err
	case QWEN:
		modelConfig := qwen.ChatModelConfig{
			APIKey:  config.Api,
			Model:   config.Model,
			BaseURL: config.Url,
		}
		if remaining > 0 {
			modelConfig.Timeout = remaining
		} else {
			modelConfig.Timeout = 120 * time.Second
		}
		if config.Temperature != 0 {
			modelConfig.Temperature = &config.Temperature
		}
		cm, err := qwen.NewChatModel(ctx, &modelConfig)
		return cm, err
	case GITHAVE:
		apiKey := config.Api
		if common.HasToken() {
			apiKey = common.GetCurrentToken()
			logs.Infof("githave token: %s", common.GetCurrentToken())
		}
		baseURL := config.Url
		if common.GetURL() != "" {
			baseURL = common.GetURL()
			logs.Infof("override githave url: %s", common.GetURL())
		}
		modelConfig := openai.ChatModelConfig{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: baseURL,
		}
		if remaining > 0 {
			modelConfig.Timeout = remaining
		} else {
			modelConfig.Timeout = 120 * time.Second
		}
		if config.Temperature != 0 {
			modelConfig.Temperature = &config.Temperature
		}
		cm, err := openai.NewChatModel(ctx, &modelConfig)
		return cm, err
	default:
		logs.Errorf("unknown model type: %s", config.Type)
		return nil, err
	}
}

func CreateOllamaModel(ctx context.Context, url string, model string) (model.ChatModel, error) {
	cm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: url,
		Model:   model,
		Options: &api.Options{
			Temperature: 0.1,
		},
	})
	return cm, err
}

func Generate(ctx context.Context, llm model.ChatModel, in []*schema.Message) (*schema.Message, error) {
	start := time.Now()
	result, err := llm.Generate(ctx, in)
	dl, remaining := deadlineInfo(ctx)
	if err != nil {
		logs.Errorf("llm generate failed: %v (duration=%s, deadline=%s, remaining=%s)", err, time.Since(start), dl, remaining)
		errMsg := err.Error()

		// 检测是否为服务端提前超时
		if (strings.Contains(errMsg, "context deadline exceeded") || strings.Contains(errMsg, "Client.Timeout exceeded")) && remaining > 10*time.Second {
			logs.Warnf("Premature server/client timeout detected, local context still has time remaining (%s). This usually means that the gateway or upstream service has a shorter timeout limit, or the HTTP Client is configured with a shorter Timeout.", remaining)
		}

		// 检查错误消息中是否包含 429 状态码或 Too Many Requests 字符串
		if common.IsLLMRateLimit(errMsg) {
			return nil, common.NewLLMRateLimitError(errMsg)
		}
		// 如果是EOF错误，直接返回普通错误而不是LLMResponseError
		if strings.Contains(errMsg, "EOF") {
			logs.Warnf("Generate encountered an EOF error: %v", err)
			return nil, err
		}
		return nil, common.NewLLMResponseError(errMsg)
	}
	logs.Infof("llm generate success (duration=%s, deadline=%s, remaining=%s)", time.Since(start), dl, remaining)
	return result, nil
}

func Stream(ctx context.Context, llm model.ChatModel, in []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	start := time.Now()
	result, err := llm.Stream(ctx, in)
	dl, remaining := deadlineInfo(ctx)
	if err != nil {
		logs.Errorf("llm stream failed: %v (duration=%s, deadline=%s, remaining=%s)", err, time.Since(start), dl, remaining)
		errMsg := err.Error()
		// 检查错误消息中是否包含 429 状态码或 Too Many Requests 字符串
		if common.IsLLMRateLimit(errMsg) {
			return nil, common.NewLLMRateLimitError(errMsg)
		}
		// 如果是EOF错误，直接返回普通错误而不是LLMResponseError
		if strings.Contains(errMsg, "EOF") {
			logs.Warnf("Stream encountered EOF error: %v", err)
			return nil, err
		}
		return nil, common.NewLLMResponseError(errMsg)
	}
	logs.Infof("llm stream started (duration=%s, deadline=%s, remaining=%s)", time.Since(start), dl, remaining)
	return result, nil
}

func ANAInvoke(modelCfg *config.CloudModel, ask string) (string, error) {
	cfg, _ := config.LoadConfig()
	t := 90 * time.Second
	if cfg != nil && cfg.LLMCloudTimeoutSec > 0 {
		t = time.Duration(cfg.LLMCloudTimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), t)
	logs.Infof("cloud llm call timeout: %v", t)
	defer cancel()
	messages := []*schema.Message{
		schema.UserMessage(ask),
	}
	messageLen := len(ask)
	modelConfig := GetModelConfigByPromptLength(messageLen)
	if modelConfig == nil {
		logs.Infof("No model configuration found for prompt length %d", messageLen)
		return "", nil
	}
	if modelConfig.CloudModel.MaxPrompts > 0 && messageLen > modelConfig.CloudModel.MaxPrompts {
		logs.Infof("prompt length %d is greater than model config prompt length %d, truncate the prompt", messageLen, modelConfig.CloudModel.MaxPrompts)
		messages = []*schema.Message{
			schema.UserMessage(ask[:modelConfig.CloudModel.MaxPrompts]),
		}
	}
	cm, err := CreateChatModel(ctx, modelCfg)
	if err != nil {
		logs.Errorf("create chat model failed: %v", err)
		return "", err
	}
	msg, err := Generate(ctx, cm, messages)
	if err != nil {
		logs.Errorf("generate failed: %v", err)
		return "", err
	}
	return msg.Content, nil
}

func KeywordInvoke(modelCfg *config.CloudModel, prompts *config.KeywordPrompts, ask string) (string, error) {
	cfg, _ := config.LoadConfig()
	t := 90 * time.Second
	if cfg != nil && cfg.LLMCloudTimeoutSec > 0 {
		t = time.Duration(cfg.LLMCloudTimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), t)
	logs.Infof("cloud llm call timeout: %v", t)
	defer cancel()
	template := CreateTemplate(prompts.System, prompts.User)
	m := map[string]any{}
	m["search_query"] = ask
	// 使用模板生成消息
	messages, err := template.Format(ctx, m)
	if err != nil {
		logs.Errorf("format template failed: %v\n", err)
		return "", err
	}
	messageLen := len(prompts.System + prompts.User + ask)
	modelConfig := GetModelConfigByPromptLength(messageLen)
	if modelConfig == nil {
		logs.Infof("No model configuration found for prompt length %d", messageLen)
		return "", nil
	}
	if modelConfig.CloudModel.MaxPrompts > 0 && messageLen > modelConfig.CloudModel.MaxPrompts {
		logs.Infof("prompt length %d is greater than model config prompt length %d, truncate the prompt", messageLen, modelConfig.CloudModel.MaxPrompts)
		m["search_query"] = ask[:modelConfig.CloudModel.MaxPrompts-len(prompts.System+prompts.User)]
		messages, err = template.Format(ctx, m)
		if err != nil {
			logs.Errorf("format template failed: %v\n", err)
			return "", err
		}
	}
	cm, err := CreateChatModel(ctx, modelCfg)
	if err != nil {
		logs.Errorf("create chat model failed: %v", err)
		return "", err
	}
	logs.Infof("messages: %v", messages)
	msg, err := Generate(ctx, cm, messages)
	if err != nil {
		logs.Errorf("generate failed: %v", err)
		return "", err
	}
	return msg.Content, nil
}

// EmbeddingInvoke 调用云端模型获取多条查询的 embeddings，
// 并保证每条向量长度为 dim，不足时补零，多余时截断。
func EmbeddingInvoke(config *config.CloudModel, ask []string, dim int) ([][]float32, error) {

	// 批量输入
	req := api.EmbedRequest{
		Model: config.Model,
		Input: ask,
	}

	ctx := context.Background()
	resp, err := GetEmbeddings(ctx, req, config)
	if err != nil {
		logs.Errorf("get embeddings failed: %v", err)
		return nil, fmt.Errorf("get embeddings failed: %w", err)
	}
	fmt.Printf("Model: %s, Number of data items: %d\n", resp.Model, len(resp.Data))

	// 准备返回的切片，容量预分配为 resp.Data 长度
	allEmbeddings := make([][]float32, 0, len(resp.Data))

	for _, d := range resp.Data {
		// 原始 embedding 可能是 []float64 或 []float32，根据实际类型做转换
		raw := d.Embedding
		vec := make([]float32, 0, dim)
		for _, v := range raw {
			// 如果原始是 float32，可直接转换；如果是 float64，也能正确处理
			vec = append(vec, float32(v))
		}

		// 长度大于 dim 时截断
		if len(vec) > dim {
			vec = vec[:dim]
		}
		// 长度小于 dim 时补零
		if len(vec) < dim {
			pad := make([]float32, dim-len(vec))
			vec = append(vec, pad...)
		}

		fmt.Printf("index=%d, processed embedding length=%d\n", d.Index, len(vec))
		allEmbeddings = append(allEmbeddings, vec)
	}

	return allEmbeddings, nil
}

// GetEmbeddings 调用 OpenAI Embeddings API，返回解析后的响应
func GetEmbeddings(ctx context.Context, req api.EmbedRequest, config *config.CloudModel) (*EmbedResponse, error) {
	apiKey := config.Api
	url := config.Url
	if config.Type == GITHAVE {
		if common.HasToken() {
			logs.Infof("switch to GitHave AI, token: %s", common.GetCurrentToken())
			apiKey = common.GetCurrentToken()
		}
		if common.GetURL() != "" {
			logs.Infof("override githave url: %s", common.GetURL())
			url = common.GetURL()
		}

	}
	// 1. 序列化请求体
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 2. 构造 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create http request failed: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	// 打印请求体字节数
	logs.Infof("httpReq size: %d bytes", len(bodyBytes))
	// 3. 发送请求
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		// 如果是EOF错误，直接返回普通错误而不是LLMResponseError
		if strings.Contains(err.Error(), "EOF") {
			logs.Warnf("GetEmbeddings encountered EOF error: %v", err)
			return nil, err
		}
		return nil, common.NewLLMResponseError(err.Error())
	}
	defer httpResp.Body.Close()

	// 4. 错误码处理
	if httpResp.StatusCode != http.StatusOK {
		// 解析错误响应体
		var errBody map[string]interface{}
		_ = json.NewDecoder(httpResp.Body).Decode(&errBody)
		return nil, fmt.Errorf("openai api error [%d]: %v", httpResp.StatusCode, errBody)
	}

	// 5. 解析成功响应
	var embedResp EmbedResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&embedResp); err != nil {
		// 如果是EOF错误，直接返回普通错误而不是LLMResponseError
		if strings.Contains(err.Error(), "EOF") {
			logs.Warnf("GetEmbeddings encountered an EOF error while parsing the response: %v", err)
			return nil, err
		}
		return nil, common.NewLLMResponseError(err.Error())
	}
	return &embedResp, nil
}

func ParserInvoke(ctx context.Context, config *config.Config, code string) (string, error) {
	// 确保上下文有超时控制
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		// 如果传入的 context 没有 deadline，则设置一个默认的超时
		t := 90 * time.Second
		if config != nil && config.LLMCloudTimeoutSec > 0 {
			t = time.Duration(config.LLMCloudTimeoutSec) * time.Second
		}
		ctx, cancel = context.WithTimeout(ctx, t)
		logs.Infof("ParserInvoke create context with timeout: %v", t)
		defer cancel()
	}

	template := CreateSimpleTemplate(config.LlmParserPrompts)
	m := map[string]any{}
	m["code"] = code
	// 使用模板生成消息
	messages, err := template.Format(ctx, m)
	if err != nil {
		logs.Errorf("format template failed: %v\n", err)
		return "", err
	}
	messageLen := len(config.LlmParserPrompts + code)
	modelConfig := GetModelConfigByPromptLength(messageLen)
	if modelConfig == nil {
		logs.Infof("No model configuration found for prompt length %d", messageLen)
		return "", nil
	}
	if modelConfig.CloudModel.MaxPrompts > 0 && messageLen > modelConfig.CloudModel.MaxPrompts {
		logs.Infof("prompt length %d is greater than model config prompt length %d, truncate the prompt", messageLen, modelConfig.CloudModel.MaxPrompts)
		m["code"] = code[:modelConfig.CloudModel.MaxPrompts-len(config.LlmParserPrompts)]
		messages, err = template.Format(ctx, m)
		if err != nil {
			logs.Errorf("format template failed: %v\n", err)
			return "", err
		}
	}
	if modelConfig.CloudModel.Enabled {
		cm, err := CreateChatModel(ctx, &modelConfig.CloudModel)
		if err != nil {
			logs.Errorf("create cloud chat model failed: %v", err)
			return "", err
		}
		msg, err := Generate(ctx, cm, messages)
		if err != nil {
			logs.Errorf("generate failed: %v", err)
			return "", err
		}
		return msg.Content, nil
	} else {
		cm, err := CreateOllamaModel(ctx, config.ApiBaseUrl, modelConfig.Name)
		if err != nil {
			logs.Errorf("create ollama chat model failed: %v", err)
			return "", err
		}
		logs.Infof("messages: %v", messages)
		msg, err := Generate(ctx, cm, messages)
		if err != nil {
			logs.Errorf("generate failed: %v", err)
			return "", err
		}
		return msg.Content, nil
	}
}

// GetModelConfigByPromptLength 根据提示词长度动态选择合适的模型和参数配置
func GetModelConfigByPromptLength(promptLength int) *config.ModelConfig {
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return nil
	}
	var selected *config.ModelConfig
	for i := len(cfg.ModelConfigs) - 1; i >= 0; i-- {
		mc := cfg.ModelConfigs[i]
		if promptLength >= mc.PromptLength {
			selected = &mc
			break
		}
	}
	if selected == nil {
		logs.Errorf("No model configuration found for prompt length %d", promptLength)
		return nil
	}
	logs.Infof("Selected model size: %v", selected.MaxTokens)
	return selected
}

// FastFunction 快速调用云端模型生成文本响应
func FastFunction(cfg *config.Config, prompt string) (string, error) {
	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}
	modelConfig := GetModelConfigByPromptLength(len(prompt))
	if modelConfig == nil {
		logs.Errorf("No model configuration found for prompt length %d", len(prompt))
		return "", nil
	}
	if modelConfig.CloudModel.MaxPrompts > 0 && len(prompt) > modelConfig.CloudModel.MaxPrompts {
		logs.Infof("prompt length %d is greater than model config prompt length %d, truncate the prompt", len(prompt), modelConfig.CloudModel.MaxPrompts)
		messages = []*schema.Message{
			schema.UserMessage(prompt[:modelConfig.CloudModel.MaxPrompts]),
		}
	}
	var t time.Duration
	t = 5 * 60 * time.Second
	if cfg != nil && cfg.LLMLocalTimeoutSec > 0 {
		t = time.Duration(cfg.LLMLocalTimeoutSec) * time.Second
	}
	if modelConfig.CloudModel.Enabled {
		t = 90 * time.Second
		if cfg != nil && cfg.LLMCloudTimeoutSec > 0 {
			t = time.Duration(cfg.LLMCloudTimeoutSec) * time.Second
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), t)
	logs.Infof("create context with timeout: %v", t)
	defer cancel()
	if modelConfig.CloudModel.Enabled {
		cm, err := CreateChatModel(ctx, &modelConfig.CloudModel)
		if err != nil {
			logs.Errorf("create chat model failed: %v", err)
			return "", err
		}
		msg, err := Generate(ctx, cm, messages)
		if err != nil {
			logs.Errorf("generate failed: %v", err)
			return "", err
		}
		return msg.Content, nil
	} else {
		cm, err := CreateOllamaModel(ctx, cfg.ApiBaseUrl, modelConfig.Name)
		if err != nil {
			logs.Errorf("create ollama chat model failed: %v", err)
			return "", err
		}
		logs.Infof("messages: %v", messages)
		msg, err := Generate(ctx, cm, messages)
		if err != nil {
			logs.Errorf("generate failed: %v", err)
			return "", err
		}
		return msg.Content, nil
	}
}
