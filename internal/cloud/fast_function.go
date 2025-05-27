package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/ollama/ollama/api"
	"net/http"
)

// 字典枚举
var (
	OPENAI = "openai"
	OLLAMA = "ollama"
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
	switch config.Type {
	case OPENAI:
		modelConfig := openai.ChatModelConfig{
			APIKey:  config.Api,
			Model:   config.Model,
			BaseURL: config.Url,
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
	result, err := llm.Generate(ctx, in)
	if err != nil {
		logs.Errorf("llm generate failed: %v", err)
		return nil, err
	}
	return result, nil
}

func Stream(ctx context.Context, llm model.ChatModel, in []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	result, err := llm.Stream(ctx, in)
	if err != nil {
		logs.Errorf("llm generate failed: %v", err)
		return nil, err
	}
	return result, nil
}

func ANAInvoke(config *config.CloudModel, ask string) (string, error) {
	ctx := context.Background()
	messages := []*schema.Message{
		schema.UserMessage(ask),
	}
	cm, err := CreateChatModel(ctx, config)
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

func KeywordInvoke(config *config.CloudModel, prompts *config.KeywordPrompts, ask string) (string, error) {
	ctx := context.Background()
	template := CreateTemplate(prompts.System, prompts.User)
	m := map[string]any{}
	m["search_query"] = ask
	// 使用模板生成消息
	messages, err := template.Format(ctx, m)
	if err != nil {
		logs.Errorf("format template failed: %v\n", err)
		return "", err
	}
	cm, err := CreateChatModel(ctx, config)
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
	apiKey := config.Api

	// 批量输入
	req := api.EmbedRequest{
		Model: config.Model,
		Input: ask,
	}
	logs.Infof("EmbeddingInvoke req: %v", req)

	ctx := context.Background()
	resp, err := GetEmbeddings(ctx, apiKey, req, config)
	if err != nil {
		logs.Errorf("get embeddings failed: %v", err)
		return nil, fmt.Errorf("get embeddings failed: %w", err)
	}
	fmt.Printf("Model: %s, 数据条数: %d\n", resp.Model, len(resp.Data))

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

		fmt.Printf("  index=%d, processed embedding 长度=%d\n", d.Index, len(vec))
		allEmbeddings = append(allEmbeddings, vec)
	}

	return allEmbeddings, nil
}

// GetEmbeddings 调用 OpenAI Embeddings API，返回解析后的响应
func GetEmbeddings(ctx context.Context, apiKey string, req api.EmbedRequest, config *config.CloudModel) (*EmbedResponse, error) {
	// 1. 序列化请求体
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 2. 构造 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Url+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create http request failed: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	// 3. 发送请求
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
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
		return nil, fmt.Errorf("decode response failed: %w", err)
	}
	return &embedResp, nil
}

func ParserInvoke(ctx context.Context, config *config.Config, code string) (string, error) {
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
	return selected
}
