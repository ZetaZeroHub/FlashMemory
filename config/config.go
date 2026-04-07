package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

var (
	// 通过 -config 或 -c 指定的配置文件路径
	GlobalConfigPath string

	// 全局唯一的配置对象
	GlobalOllamaConfig *Config

	// 保证只执行一次加载
	loadConfigOnce sync.Once

	// 第一次加载时的错误
	loadConfigErr error
)

var GitManage string

func Init() string {
	// 通过命令行参数 -path 获取配置文件路径
	flag.StringVar(&GlobalConfigPath, "config", "", "配置文件路径")
	flag.StringVar(&GlobalConfigPath, "c", "", "配置文件路径")
	flag.Parse()
	logs.Infof("当前配置文件路径: %s", GlobalConfigPath)
	// 可选：对传入的路径做校验或转换为绝对路径
	//if GlobalConfigPath == "" {
	//	log.Fatal("必须通过 -path 参数传入配置文件路径")
	//}
	// 例如：将路径转换为绝对路径
	// absPath, err := filepath.Abs(GlobalConfigPath)
	// if err != nil {
	//     log.Fatalf("获取配置文件绝对路径失败: %v", err)
	// }
	// GlobalConfigPath = absPath
	return GlobalConfigPath
}

type CloudModel struct {
	Api         string  `mapstructure:"api" json:"api" yaml:"api"`
	Model       string  `mapstructure:"model" json:"model" yaml:"model"`
	Url         string  `mapstructure:"url" json:"url" yaml:"url"`
	Enabled     bool    `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	Type        string  `mapstructure:"type" json:"type" yaml:"type"`
	MaxPrompts  int     `mapstructure:"max_prompts" json:"max_prompts" yaml:"max_prompts"`
	Temperature float32 `mapstructure:"temperature" yaml:"temperature" json:"temperature,omitempty"`
}

type ModelConfig struct {
	Name             string     `mapstructure:"name" yaml:"name" json:"name,omitempty"`
	MaxTokens        int        `mapstructure:"max_tokens" yaml:"max_tokens" json:"max_tokens,omitempty"`
	NumCtx           int        `mapstructure:"num_ctx" yaml:"num_ctx" json:"num_ctx,omitempty"`
	NumKeep          int        `mapstructure:"num_keep" yaml:"num_keep" json:"num_keep,omitempty"`
	NumPredict       int        `mapstructure:"num_predict" yaml:"num_predict" json:"num_predict,omitempty"`
	RepeatLastN      int        `mapstructure:"repeat_last_n" yaml:"repeat_last_n" json:"repeat_last_n,omitempty"`
	PresencePenalty  int        `mapstructure:"presence_penalty" yaml:"presence_penalty" json:"presence_penalty,omitempty"`
	FrequencyPenalty int        `mapstructure:"frequency_penalty" yaml:"frequency_penalty" json:"frequency_penalty,omitempty"`
	Format           string     `mapstructure:"format" yaml:"format" json:"format,omitempty"`
	PromptLength     int        `mapstructure:"prompt_length" yaml:"prompt_length" json:"prompt_length,omitempty"`
	Size             string     `mapstructure:"size" yaml:"size" json:"size,omitempty"`
	Temperature      float32    `mapstructure:"temperature" yaml:"temperature" json:"temperature,omitempty"`
	CloudModel       CloudModel `mapstructure:"cloud_model" yaml:"cloud_model" json:"cloud_model,omitempty"`
}
type AnaPrompts struct {
	Role         string `mapstructure:"role" yaml:"role" json:"role,omitempty"`
	Route        string `mapstructure:"route" yaml:"route" json:"route,omitempty"`
	Imports      string `mapstructure:"imports" yaml:"imports" json:"imports,omitempty"`
	InternalDeps string `mapstructure:"internal_deps" yaml:"internal_deps" json:"internal_deps,omitempty"`
	ExternalDeps string `mapstructure:"external_deps" yaml:"external_deps" json:"external_deps,omitempty"`
	Main         string `mapstructure:"main" yaml:"main" json:"main,omitempty"`
}

type KeywordPrompts struct {
	System string `mapstructure:"system" yaml:"system" json:"system,omitempty"`
	User   string `mapstructure:"user" yaml:"user" json:"user,omitempty"`
}

type ModuleAnalyzerPrompts struct {
	Header          string `mapstructure:"header" yaml:"header" json:"header,omitempty"`
	Footer          string `mapstructure:"footer" yaml:"footer" json:"footer,omitempty"`
	SubModuleHeader string `mapstructure:"sub_module_header" yaml:"sub_module_header" json:"sub_module_header,omitempty"`
	SubModuleFile   string `mapstructure:"sub_module_file" yaml:"sub_module_file" json:"sub_module_file,omitempty"`
	SubModuleDir    string `mapstructure:"sub_module_dir" yaml:"sub_module_dir" json:"sub_module_dir,omitempty"`
	SubModuleDesc   string `mapstructure:"sub_module_desc" yaml:"sub_module_desc" json:"sub_module_desc,omitempty"`
}

type RepoAnalyzerPrompts struct {
	Header          string `mapstructure:"header" yaml:"header" json:"header,omitempty"`
	Footer          string `mapstructure:"footer" yaml:"footer" json:"footer,omitempty"`
	SubModuleHeader string `mapstructure:"sub_module_header" yaml:"sub_module_header" json:"sub_module_header,omitempty"`
	SubModuleFile   string `mapstructure:"sub_module_file" yaml:"sub_module_file" json:"sub_module_file,omitempty"`
	SubModuleDir    string `mapstructure:"sub_module_dir" yaml:"sub_module_dir" json:"sub_module_dir,omitempty"`
	SubModuleDesc   string `mapstructure:"sub_module_desc" yaml:"sub_module_desc" json:"sub_module_desc,omitempty"`
}

type FileAnalyzerPrompts struct {
	Header          string `mapstructure:"header" yaml:"header" json:"header,omitempty"`
	Footer          string `mapstructure:"footer" yaml:"footer" json:"footer,omitempty"`
	SubModuleHeader string `mapstructure:"sub_module_header" yaml:"sub_module_header" json:"sub_module_header,omitempty"`
}

type Config struct {
	PipPath               string                `mapstructure:"pip_path" yaml:"pip_path" json:"pip_path,omitempty"`
	LlmParserPrompts      string                `mapstructure:"llm_parser_prompts" yaml:"llm_parser_prompts" json:"llm_parser_prompts,omitempty"`
	AnaPrompts            AnaPrompts            `mapstructure:"ana_prompts" yaml:"ana_prompts" json:"ana_prompts,omitempty"`
	KeywordPrompts        KeywordPrompts        `mapstructure:"keyword_prompts" yaml:"keyword_prompts" json:"keyword_prompts,omitempty"`
	ModuleAnalyzerPrompts ModuleAnalyzerPrompts `mapstructure:"module_analyzer_prompts" yaml:"module_analyzer_prompts" json:"module_analyzer_prompts,omitempty"`
	RepoAnalyzerPrompts   RepoAnalyzerPrompts   `mapstructure:"repo_analyzer_prompts" yaml:"repo_analyzer_prompts" json:"repo_analyzer_prompts,omitempty"`
	FileAnalyzerPrompts   FileAnalyzerPrompts   `mapstructure:"file_analyzer_prompts" yaml:"file_analyzer_prompts" json:"file_analyzer_prompts,omitempty"`
	ApiBaseUrl            string                `mapstructure:"api_base_url" yaml:"api_base_url" json:"api_base_url,omitempty"`
	CompletionApi         string                `mapstructure:"completion_api" yaml:"completion_api" json:"completion_api,omitempty"`
	EmbeddingApi          string                `mapstructure:"embedding_api" yaml:"embedding_api" json:"embedding_api,omitempty"`
	DefaultModel          string                `mapstructure:"default_model" yaml:"default_model" json:"default_model,omitempty"`
	DefaultFormat         string                `mapstructure:"default_format" yaml:"default_format" json:"default_format,omitempty"`
	DefaultTemp           float64               `mapstructure:"default_temperature" yaml:"default_temperature" json:"default_temp,omitempty"`
	DefaultLowVram        bool                  `mapstructure:"default_low_vram" yaml:"default_low_vram" json:"default_low_vram,omitempty"`
	DefaultMaxWorker      int                   `mapstructure:"default_max_worker" yaml:"default_max_worker" json:"default_max_worker,omitempty"`
	NormalizeModel        string                `mapstructure:"normalize_model" yaml:"normalize_model" json:"normalize_model,omitempty"`
	EmbeddingModel        string                `mapstructure:"embedding_model" yaml:"embedding_model" json:"embedding_model,omitempty"`
	EmbeddingMaxBatch     int                   `mapstructure:"embedding_max_batch" yaml:"embedding_max_batch" json:"embedding_max_batch,omitempty"`
	EmbeddingMaxWorker    int                   `mapstructure:"embedding_max_worker" yaml:"embedding_max_worker" json:"embedding_max_worker,omitempty"`
	EmbeddingCloudModel   CloudModel            `mapstructure:"embedding_cloud_model" yaml:"embedding_cloud_model" json:"embedding_cloud_model,omitempty"`
	DefaultCloudModel     CloudModel            `mapstructure:"default_cloud_model" yaml:"default_cloud_model" json:"default_cloud_model,omitempty"`
	ModelConfigs          []ModelConfig         `mapstructure:"model_configs" yaml:"model_configs" json:"model_configs,omitempty"`
	CodeLimit             int                   `mapstructure:"code_limit" yaml:"code_limit" json:"code_limit,omitempty"`
	PromptLimit           int                   `mapstructure:"prompt_limit" yaml:"prompt_limit" json:"prompt_limit,omitempty"`
	ParserCodeLineLimit   int                   `mapstructure:"parser_code_line_limit" yaml:"parser_code_line_limit" json:"parser_code_line_limit,omitempty"`
	ParserCodeChunkLimit  int                   `mapstructure:"parser_code_chunk_limit" yaml:"parser_code_chunk_limit" json:"parser_code_chunk_limit,omitempty"`
	DbWriterQueueSize     int                   `mapstructure:"db_writer_queue_size" yaml:"db_writer_queue_size" json:"db_writer_queue_size,omitempty"`
	DbWriterMaxRetries    int                   `mapstructure:"db_writer_max_retries" yaml:"db_writer_max_retries" json:"db_writer_max_retries,omitempty"`
	DbWriterRetryInterval int                   `mapstructure:"db_writer_retry_interval_ms" yaml:"db_writer_retry_interval_ms" json:"db_writer_retry_interval_ms,omitempty"`
	LLMLocalTimeoutSec    int                   `mapstructure:"llm_local_timeout_sec" yaml:"llm_local_timeout_sec" json:"llm_local_timeout_sec,omitempty"`
	LLMCloudTimeoutSec    int                   `mapstructure:"llm_cloud_timeout_sec" yaml:"llm_cloud_timeout_sec" json:"llm_cloud_timeout_sec,omitempty"`
	// API配置字段
	ApiToken     string `mapstructure:"api_token" yaml:"api_token" json:"api_token,omitempty"`
	ApiUrl       string `mapstructure:"api_url" yaml:"api_url" json:"api_url,omitempty"`
	ApiModel     string `mapstructure:"api_model" yaml:"api_model" json:"api_model,omitempty"`
	AuthBaseUrl  string `mapstructure:"auth_base_url" yaml:"auth_base_url" json:"auth_base_url,omitempty"`
	ApiUrlSimple string `mapstructure:"api_url_simple" yaml:"api_url_simple" json:"api_url_simple,omitempty"`
}

// LoadConfig 加载配置（优先文件，其次环境变量），并保证仅加载一次
func LoadConfig() (*Config, error) {
	loadConfigOnce.Do(func() {
		vp := viper.New()
		vp.AutomaticEnv()

		// 优先使用命令行参数或环境变量指定路径
		configPath := os.Getenv("BOTGO_CONFIG_PATH")
		if GlobalConfigPath != "" {
			configPath = GlobalConfigPath
		}
		if configPath != "" {
			if strings.HasSuffix(configPath, ".yaml") ||
				strings.HasSuffix(configPath, ".yml") ||
				strings.HasSuffix(configPath, ".json") {
				vp.SetConfigFile(configPath)
			} else {
				vp.SetConfigName("fm")
				vp.AddConfigPath(configPath)
			}
		} else {
			// 默认在可执行文件目录和当前目录查找
			vp.SetConfigName("fm")
			if exePath, err := os.Executable(); err == nil {
				vp.AddConfigPath(filepath.Dir(exePath))
			}
			vp.AddConfigPath(".")
		}

		// 读取配置文件（可选）
		if err := vp.ReadInConfig(); err != nil {
			fmt.Println("Warn: no config file found or parse error, fallback to env or default. Err:", err)
		}

		// 反序列化到 Config 结构体
		var cfg Config
		if err := vp.Unmarshal(&cfg); err != nil {
			loadConfigErr = fmt.Errorf("unmarshal config file error: %v", err)
			return
		}
		GlobalOllamaConfig = &cfg
	})

	return GlobalOllamaConfig, loadConfigErr
}

// InitWithPath 直接通过传入路径设置全局配置路径，不调用 flag.Parse()
// 用于 fm_http 等自行管理参数的入口
func InitWithPath(path string) string {
	GlobalConfigPath = path
	logs.Infof("当前配置文件路径: %s", GlobalConfigPath)
	return GlobalConfigPath
}

// DefaultConfigDir 返回用户主目录下的默认配置目录 ~/.flashmemory/
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flashmemory")
}

// ResolveConfigPath 按优先级查找配置文件路径
// 优先级: 参数指定 > 环境变量 > 当前目录 > 可执行文件目录 > ~/.flashmemory/
func ResolveConfigPath(explicit string) string {
	// 1. 显式指定的路径
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
		// 如果指定了但不存在，也返回（后续加载会报警告）
		return explicit
	}

	// 2. 环境变量
	if envPath := os.Getenv("BOTGO_CONFIG_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// 3. 当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "fm.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 4. 可执行文件同目录
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "fm.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 5. 用户主目录 ~/.flashmemory/fm.yaml
	if defDir := DefaultConfigDir(); defDir != "" {
		candidate := filepath.Join(defDir, "fm.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 6. 未找到任何配置文件
	return ""
}

// GenerateDefaultConfig 在指定路径生成带注释的默认 fm.yaml 配置模板
func GenerateDefaultConfig(configPath string) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}

	// 如果文件已存在则跳过
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("配置文件已存在: %s", configPath)
	}

	template := `# FlashMemory 配置文件
# 生成时间: ` + time.Now().Format("2006-01-02 15:04:05") + `
# 详细说明: https://github.com/ZetaZeroHub/FlashMemory

# ─── 代码解析限制 ──────────────────────────────────
parser_code_chunk_limit: 50
parser_code_line_limit: 200
code_limit: 23000
prompt_limit: 30000

# ─── Python pip 镜像 ──────────────────────────────
pip_path: "https://mirrors.aliyun.com/pypi/simple"

# ─── 本地 LLM (Ollama) 配置 ──────────────────────
api_base_url: "http://127.0.0.1:11434"
completion_api: "/api/generate"
embedding_api: "/api/embed"
default_model: "qwen2.5-coder:1.5b"
default_format: "json"
default_temp: 0.1
default_max_worker: 1

# ─── 数据库写入配置 ───────────────────────────────
db_writer_queue_size: 300
db_writer_max_retries: 7
db_writer_retry_interval_ms: 30

# ─── LLM 超时配置 (秒) ───────────────────────────
llm_local_timeout_sec: 300
llm_cloud_timeout_sec: 300

# ─── 向量模型配置 ─────────────────────────────────
normalize_model: "qwen2.5-coder:0.5b"
embedding_model: "quentinz/bge-large-zh-v1.5:latest"
embedding_max_batch: 30
embedding_max_worker: 3

# ─── 云端向量模型 (可选，取消注释并填入你的 API Key) ─
# embedding_cloud_model:
#   api: "your-api-key-here"
#   model: "BAAI/bge-large-zh-v1.5"
#   url: "https://api.siliconflow.cn/v1/"
#   enabled: false
#   type: "openai"
#   max_prompts: 30000

# ─── 云端分析模型 (可选，取消注释并填入你的 API Key) ─
# default_cloud_model:
#   api: "your-api-key-here"
#   model: "Qwen/Qwen2.5-Coder-7B-Instruct"
#   url: "https://api.siliconflow.cn/v1/"
#   enabled: false
#   type: "openai"
#   max_prompts: 30000

# ─── 模型配置梯度 (按代码长度自动选择) ────────────
model_configs:
  - name: "qwen2.5-coder:1.5b"
    size: "S"
    format: "json"
    temperature: 0.1
    max_tokens: 2048
    num_ctx: 1024
    num_keep: 2048
    num_predict: 512
    repeat_last_n: 128
    # cloud_model:
    #   api: "your-api-key-here"
    #   model: "auto"
    #   url: "https://api.siliconflow.cn/v1/"
    #   enabled: false
    #   type: "openai"
    #   max_prompts: 30000
    #   temperature: 0.1
  - name: "qwen2.5-coder:3b"
    size: "M"
    format: "json"
    temperature: 0.1
    max_tokens: 5000
    num_ctx: 3000
    num_keep: 5000
    num_predict: 512
    prompt_length: 1501
    repeat_last_n: 128
  - name: "qwen2.5-coder:7b"
    size: "L"
    format: "json"
    temperature: 0.1
    max_tokens: 8000
    num_ctx: 6000
    num_keep: 8000
    num_predict: 700
    prompt_length: 6001
    repeat_last_n: 128
  - name: "qwen2.5-coder:7b"
    size: "XL"
    format: "json"
    temperature: 0.1
    max_tokens: 18000
    num_ctx: 12000
    num_keep: 18000
    num_predict: 1000
    prompt_length: 11001
    repeat_last_n: 128
  - name: "qwen2.5-coder:7b"
    size: "XXL"
    format: "json"
    temperature: 0.1
    max_tokens: 30000
    num_ctx: 18000
    num_keep: 18000
    num_predict: 1200
    prompt_length: 20001
    repeat_last_n: 128

# ─── 代码分析提示词 ───────────────────────────────
ana_prompts:
  role: |
    你是一个专业的架构师，请仔细阅读以下函数代码：
  route: |
    该函数的所处路径和包名：
  imports: |
    该函数使用了以下导入包：
  internal_deps: |
    该函数调用了以下内部函数：
  external_deps: |
    并使用了外部库:
  main: |
    请用几句话为以上代码生成该实现的<功能描述>，并说明它的<执行流程>。
    （输出必须为一个合法的 JSON 对象，<功能描述>的Key是"description"，Value是字符串类型；<执行流程>的Key是"process"，Value是字符串数组；所有Value均使用中文描述。）

# ─── 关键词提示词 ─────────────────────────────────
keyword_prompts:
  system: |
    #### 1．角色
      你是一名**关键词预测智能体**，负责根据用户输入的搜索词或句子，提炼并预测用户可能想进一步查询的多个关键词，并提供中英对照。
    #### 2．目标
      接收用户提供的搜索词/句，理解其意图与核心信息，生成一组关键词。
    #### 3．输出规范
    - 输出格式为**纯 JSON 数组**，仅包含字符串元素，如 ` + "`" + `["狗", "dog", "猫", "cat"]` + "`" + `。
    - 建议生成 5–10 个关键词
    - 不得添加其他文本、注释或 Markdown。
  user: |
    {search_query}

# ─── 模块分析提示词 ───────────────────────────────
module_analyzer_prompts:
  header: "请为以下目录生成一个全面的模块级描述。目录路径: \n"
  sub_module_header: |
    该目录包含以下子模块:
  sub_module_file: "- 文件: \n"
  sub_module_dir: "- 目录: \n"
  sub_module_desc: "描述: \n"
  footer: |
    请基于以上子模块的功能，生成一个简洁但全面的目录级描述，包括：
    1. 该目录的主要功能和目的
    2. 目录中实现的核心功能和关键组件
    3. 该目录在项目中的作用
    4. 目录的设计模式或架构特点（如果明显）

    请直接给出描述，不要包含标题或前缀。

# ─── 文件分析提示词 ───────────────────────────────
file_analyzer_prompts:
  header: "请为以下文件生成一个全面的模块级描述。文件路径: \n"
  sub_module_header: "文件中包含的函数/方法及其描述:"
  footer: |
    请基于以上函数的功能，生成一个简洁但全面的文件级描述，包括：
    1. 该文件的主要功能和目的
    2. 文件中实现的核心功能和关键组件
    3. 该文件在项目中的作用
    4. 文件的设计模式或架构特点（如果明显）

    请直接给出描述，不要包含标题或前缀。

# ─── 项目分析提示词 ───────────────────────────────
repo_analyzer_prompts:
  header: |
    请为以下项目生成一个全面的项目级描述。项目路径:
  sub_module_header: |
    该项目包含以下主要模块或目录:
  sub_module_file: |
    - 文件:
  sub_module_dir: |
    - 目录:
  sub_module_desc: |
    描述:
  footer: |
    请基于以上所有子模块的功能和描述，生成一份完整、详细且具有可读性的项目总览介绍。

# ─── API 认证 (可选) ──────────────────────────────
# api_token: ""
# api_url: ""
# api_model: ""
# api_url_simple: ""
# auth_base_url: ""
`

	return os.WriteFile(configPath, []byte(template), 0644)
}
