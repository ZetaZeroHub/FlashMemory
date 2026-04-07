package cli

import (
	"fmt"
	"os"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: common.I18n("查看当前配置", "View current configuration"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfig()
	},
}

func runConfig() error {
	configPath := ConfigPath()

	if !fileExists(configPath) {
		if common.IsZH() {
			fmt.Println("  ❌ 配置文件不存在。请先运行: fm init")
		} else {
			fmt.Println("  ❌ Config file not found. Please run: fm init")
		}
		return nil
	}

	if common.IsZH() {
		fmt.Printf("  📄 配置文件: %s\n\n", configPath)
	} else {
		fmt.Printf("  📄 Config file: %s\n\n", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf(common.I18n("读取配置文件失败: %v", "Failed to read config: %v"), err)
	}

	fmt.Println(string(data))
	return nil
}
