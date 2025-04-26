package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

var GlobalConfigPath string
var GitManage string

func Init(f flag.FlagSet) string {
	// 通过命令行参数 -path 获取配置文件路径
	f.StringVar(&GlobalConfigPath, "config", "", "配置文件路径")
	f.StringVar(&GlobalConfigPath, "c", "", "配置文件路径")
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

type ModelConfig struct {
	Name             string `mapstructure:"name" yaml:"name" json:"name,omitempty"`
	MaxTokens        int    `mapstructure:"max_tokens" yaml:"max_tokens" json:"max_tokens,omitempty"`
	NumCtx           int    `mapstructure:"num_ctx" yaml:"num_ctx" json:"num_ctx,omitempty"`
	NumKeep          int    `mapstructure:"num_keep" yaml:"num_keep" json:"num_keep,omitempty"`
	NumPredict       int    `mapstructure:"num_predict" yaml:"num_predict" json:"num_predict,omitempty"`
	RepeatLastN      int    `mapstructure:"repeat_last_n" yaml:"repeat_last_n" json:"repeat_last_n,omitempty"`
	PresencePenalty  int    `mapstructure:"presence_penalty" yaml:"presence_penalty" json:"presence_penalty,omitempty"`
	FrequencyPenalty int    `mapstructure:"frequency_penalty" yaml:"frequency_penalty" json:"frequency_penalty,omitempty"`
	Format           string `mapstructure:"format" yaml:"format" json:"format,omitempty"`
	PromptLength     int    `mapstructure:"prompt_length" yaml:"prompt_length" json:"prompt_length,omitempty"`
}

type Config struct {
	ApiBaseUrl     string        `mapstructure:"api_base_url" yaml:"api_base_url" json:"api_base_url,omitempty"`
	CompletionApi  string        `mapstructure:"completion_api" yaml:"completion_api" json:"completion_api,omitempty"`
	EmbeddingApi   string        `mapstructure:"embedding_api" yaml:"embedding_api" json:"embedding_api,omitempty"`
	DefaultModel   string        `mapstructure:"default_model" yaml:"default_model" json:"default_model,omitempty"`
	DefaultFormat  string        `mapstructure:"default_format" yaml:"default_format" json:"default_format,omitempty"`
	DefaultTemp    float64       `mapstructure:"default_temperature" yaml:"default_temperature" json:"default_temp,omitempty"`
	DefaultLowVram bool          `mapstructure:"default_low_vram" yaml:"default_low_vram" json:"default_low_vram,omitempty"`
	NormalizeModel string        `mapstructure:"normalize_model" yaml:"normalize_model" json:"normalize_model,omitempty"`
	EmbeddingModel string        `mapstructure:"embedding_model" yaml:"embedding_model" json:"embedding_model,omitempty"`
	ModelConfigs   []ModelConfig `mapstructure:"model_configs" yaml:"model_configs" json:"model_configs,omitempty"`
}

var GlobalOllamaConfig *Config

// LoadConfig 优先从文件加载，如果失败则回退到环境变量或默认值
func LoadConfig() (*Config, error) {
	vp := viper.New()
	vp.AutomaticEnv()

	configPath := os.Getenv("BOTGO_CONFIG_PATH")
	if GlobalConfigPath != "" {
		configPath = GlobalConfigPath
	}
	if configPath != "" {
		if strings.HasSuffix(configPath, ".yaml") || strings.HasSuffix(configPath, ".yml") || strings.HasSuffix(configPath, ".json") {
			vp.SetConfigFile(configPath)
		} else {
			// 如果传入的是目录，则在该目录下查找配置文件
			vp.SetConfigName("config")
			vp.AddConfigPath(configPath)
		}
	} else {
		// 默认逻辑：先尝试从可执行文件所在目录加载配置，再从当前工作目录加载
		vp.SetConfigName("config")
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			vp.AddConfigPath(exeDir)
		}
		vp.AddConfigPath(".")
	}

	err := vp.ReadInConfig()
	if err != nil {
		fmt.Println("Warn: no config file found or parse error, fallback to env or default. Err:", err)
	}

	var cfg Config
	if err == nil {
		if err = vp.Unmarshal(&cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config file error: %v", err)
		}
	}

	return &cfg, nil
}
