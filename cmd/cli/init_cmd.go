package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: common.I18n("初始化 FlashMemory 配置", "Initialize FlashMemory configuration"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func runInit() error {
	configPath := ConfigPath()
	configDir := filepath.Dir(configPath)
	logsDir := filepath.Join(fmHome, "logs")

	// Create directories
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create config file if not exists
	if !fileExists(configPath) {
		if err := writeDefaultConfig(configPath); err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
		if common.IsZH() {
			fmt.Printf("  ✅ 配置文件已创建: %s\n", configPath)
		} else {
			fmt.Printf("  ✅ Config file created: %s\n", configPath)
		}
	} else {
		if common.IsZH() {
			fmt.Printf("  ℹ️  配置文件已存在: %s\n", configPath)
		} else {
			fmt.Printf("  ℹ️  Config file already exists: %s\n", configPath)
		}
	}

	if common.IsZH() {
		fmt.Println()
		fmt.Println("  ✅ FlashMemory 初始化完成！")
		fmt.Println()
		fmt.Println("  下一步:")
		fmt.Println("    fm index .        索引当前目录的代码")
		fmt.Println("    fm serve          启动 HTTP API 服务")
		fmt.Println("    fm config         查看当前配置")
		fmt.Println()
	} else {
		fmt.Println()
		fmt.Println("  ✅ FlashMemory initialized!")
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println("    fm index .        Index code in current directory")
		fmt.Println("    fm serve          Start HTTP API server")
		fmt.Println("    fm config         View configuration")
		fmt.Println()
	}

	return nil
}

func writeDefaultConfig(path string) error {
	defaultConfig := `# FlashMemory Configuration
# 完整配置参考: https://github.com/ZetaZeroHub/FlashMemory

# LLM API 配置
api_base_url: http://127.0.0.1:11434
api_url: https://api.githave.com/api/v1
api_url_simple: https://api.githave.com
auth_base_url: https://githave.com
completion_api: /api/generate
embedding_api: /api/embed

# 模型配置
default_model: qwen2.5-coder:1.5b
embedding_model: qwen2.5-coder:0.5b
normalize_model: qwen2.5-coder:0.5b
default_format: json
default_temp: 0.1
default_max_worker: 1

# Embedding 配置
embedding_max_batch: 30
embedding_max_worker: 3
embedding_cloud_model:
  api: ""
  enabled: true
  max_prompts: 30000
  model: BAAI/bge-large-zh-v1.5
  type: githave
  url: https://api.githave.com/v1/

default_cloud_model:
  api: ""
  enabled: true
  max_prompts: 30000
  model: auto
  type: githave
  url: https://api.githave.com/v1/

# 数据库配置
db_writer_queue_size: 300
db_writer_max_retries: 7
db_writer_retry_interval_ms: 30

# 解析器配置
code_limit: 23000
prompt_limit: 30000
parser_code_line_limit: 200
parser_code_chunk_limit: 50

# 超时配置 (秒)
llm_local_timeout_sec: 300
llm_cloud_timeout_sec: 300
`

	return os.WriteFile(path, []byte(defaultConfig), 0644)
}

// EnsureInit checks if FlashMemory is initialized, auto-inits if needed
func EnsureInit() error {
	configPath := ConfigPath()
	if !fileExists(configPath) {
		if common.IsZH() {
			fmt.Println("  ⚙️  首次运行，正在自动初始化...")
		} else {
			fmt.Println("  ⚙️  First run, auto-initializing...")
		}
		return runInit()
	}
	return nil
}
