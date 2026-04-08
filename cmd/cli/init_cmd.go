package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/resource"
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
	template := `# FlashMemory Configuration
# 完整配置参考: https://github.com/ZetaZeroHub/FlashMemory

` + string(resource.DefaultConfigYAML)

	return os.WriteFile(path, []byte(template), 0644)
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
