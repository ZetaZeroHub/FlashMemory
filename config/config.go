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
	"gopkg.in/yaml.v3"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/kinglegendzzh/flashmemory/resource"
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
	// Zvec 引擎配置
	ZvecConfig ZvecConfig `mapstructure:"zvec_config" yaml:"zvec_config" json:"zvec_config,omitempty"`
	// Language preference (persisted via --lang toggle)
	Lang string `mapstructure:"lang" yaml:"lang" json:"lang,omitempty"`
}

// ZvecConfig Zvec 向量引擎配置
type ZvecConfig struct {
	Engine        string `mapstructure:"engine" yaml:"engine" json:"engine,omitempty"`                   // zvec | faiss | memory
	CollectionDir string `mapstructure:"collection_dir" yaml:"collection_dir" json:"collection_dir,omitempty"` // 默认 .gitgo/zvec_collections
	Dimension     int    `mapstructure:"dimension" yaml:"dimension" json:"dimension,omitempty"`           // 向量维度, 默认 384
	PythonPath    string `mapstructure:"python_path" yaml:"python_path" json:"python_path,omitempty"`     // Python 路径, 默认 python3
	MetricType    string `mapstructure:"metric_type" yaml:"metric_type" json:"metric_type,omitempty"`     // cosine | l2 | ip
	QueryThreads  int    `mapstructure:"query_threads" yaml:"query_threads" json:"query_threads,omitempty"` // 查询线程数
	OptimizeOnBuild bool `mapstructure:"optimize_on_build" yaml:"optimize_on_build" json:"optimize_on_build,omitempty"` // 构建后自动优化
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
		
		// If FM_ENGINE is set, it overrides the config parameter
		if envEngine := os.Getenv("FM_ENGINE"); envEngine != "" {
			cfg.ZvecConfig.Engine = envEngine
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

// GenerateDefaultConfig creates or incrementally upgrades the config file at configPath.
// If the file doesn't exist, it generates a fresh default config.
// If the file exists, it merges any new default keys into it without overwriting user values.
func GenerateDefaultConfig(configPath string) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	template := `# FlashMemory Configuration
# Generated: ` + time.Now().Format("2006-01-02 15:04:05") + `
# Reference: https://github.com/ZetaZeroHub/FlashMemory

` + string(resource.DefaultConfigYAML)

	return os.WriteFile(configPath, []byte(template), 0644)
}

// MergeDefaultsIntoConfig incrementally merges new default keys from the embedded
// resource/fm.yaml into an existing user config file, without overwriting user values.
// Returns the number of new keys added.
func MergeDefaultsIntoConfig(configPath string) (int, error) {
	existing, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse existing config as map
	var userMap map[string]interface{}
	if err := yaml.Unmarshal(existing, &userMap); err != nil {
		return 0, fmt.Errorf("failed to parse existing config: %w", err)
	}
	if userMap == nil {
		userMap = make(map[string]interface{})
	}

	// Parse default config as map
	var defaultMap map[string]interface{}
	if err := yaml.Unmarshal(resource.DefaultConfigYAML, &defaultMap); err != nil {
		return 0, fmt.Errorf("failed to parse default config: %w", err)
	}

	// Merge: only add keys that don't exist in user config (top-level)
	added := 0
	for key, val := range defaultMap {
		if _, exists := userMap[key]; !exists {
			userMap[key] = val
			added++
			logs.Infof("[config-merge] Added new key: %s", key)
		}
	}

	if added == 0 {
		return 0, nil
	}

	// Write back
	merged, err := yaml.Marshal(userMap)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal merged config: %w", err)
	}

	header := fmt.Sprintf("# FlashMemory Configuration\n# Updated: %s\n# Reference: https://github.com/ZetaZeroHub/FlashMemory\n\n",
		time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(configPath, append([]byte(header), merged...), 0644); err != nil {
		return 0, fmt.Errorf("failed to write merged config: %w", err)
	}

	return added, nil
}
