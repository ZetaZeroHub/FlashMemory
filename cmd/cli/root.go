package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

var (
	Version = "0.4.4"

	// Global flags
	langFlag   string
	configFlag string
	engineFlag string

	// Home directory for FlashMemory
	fmHome string
)

func init() {
	home, _ := os.UserHomeDir()
	fmHome = filepath.Join(home, ".flashmemory")
}

// FMHome returns the FlashMemory home directory
func FMHome() string {
	return fmHome
}

// ConfigPath returns the default config file path
func ConfigPath() string {
	if configFlag != "" {
		return configFlag
	}
	return filepath.Join(fmHome, "config.yaml")
}

var rootCmd = &cobra.Command{
	Use:   "fm",
	Short: "FlashMemory — 跨语言代码分析与语义搜索系统",
	Long: `
  ⚡ FlashMemory — 跨语言代码分析与语义搜索系统

  支持 Go, Python, JavaScript, Java, C++ 等语言的代码索引，
  结合 LLM 驱动的语义分析与 FAISS 向量检索。`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Apply language setting globally
		if langFlag != "" {
			common.SetLang(langFlag)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		showWelcome()
	},
}

func showWelcome() {
	configExists := fileExists(ConfigPath())

	if common.IsZH() {
		if !configExists {
			fmt.Println()
			fmt.Println("╔══════════════════════════════════════════════════════╗")
			fmt.Println("║                                                      ║")
			fmt.Printf("║   ⚡ FlashMemory v%-36s ║\n", Version)
			fmt.Println("║   跨语言代码分析与语义搜索系统                        ║")
			fmt.Println("║                                                      ║")
			fmt.Println("╠══════════════════════════════════════════════════════╣")
			fmt.Println("║                                                      ║")
			fmt.Println("║   👋 欢迎！检测到首次运行。                           ║")
			fmt.Println("║   请先执行初始化:  fm init                            ║")
			fmt.Println("║                                                      ║")
			fmt.Println("║   🚀 快速开始:                                        ║")
			fmt.Println("║     fm init           初始化配置                      ║")
			fmt.Println("║     fm index .        索引当前目录的代码               ║")
			fmt.Println("║     fm query \"登录\"    搜索代码                       ║")
			fmt.Println("║     fm serve          启动 HTTP API 服务              ║")
			fmt.Println("║     fm status         查看服务状态                    ║")
			fmt.Println("║                                                      ║")
			fmt.Println("║   📖 更多帮助: fm --help                              ║")
			fmt.Println("║                                                      ║")
			fmt.Println("╚══════════════════════════════════════════════════════╝")
			fmt.Println()
		} else {
			fmt.Println()
			fmt.Printf("  ⚡ FlashMemory v%s\n", Version)
			fmt.Println()
			showStatusBrief()
			fmt.Println()
			fmt.Println("  快速命令:")
			fmt.Println("    fm index .          索引当前目录")
			fmt.Println("    fm query \"关键词\"    搜索代码")
			fmt.Println("    fm serve / fm stop  管理 HTTP 服务")
			fmt.Println("    fm --help           完整帮助")
			fmt.Println()
		}
	} else {
		if !configExists {
			fmt.Println()
			fmt.Println("╔══════════════════════════════════════════════════════╗")
			fmt.Println("║                                                      ║")
			fmt.Printf("║   ⚡ FlashMemory v%-36s ║\n", Version)
			fmt.Println("║   Cross-language Code Analysis & Semantic Search     ║")
			fmt.Println("║                                                      ║")
			fmt.Println("╠══════════════════════════════════════════════════════╣")
			fmt.Println("║                                                      ║")
			fmt.Println("║   👋 Welcome! First run detected.                    ║")
			fmt.Println("║   Please initialize first:  fm init                  ║")
			fmt.Println("║                                                      ║")
			fmt.Println("║   🚀 Quick start:                                    ║")
			fmt.Println("║     fm init           Initialize configuration       ║")
			fmt.Println("║     fm index .        Index code in current dir      ║")
			fmt.Println("║     fm query \"login\"   Search codebase               ║")
			fmt.Println("║     fm serve          Start HTTP API server          ║")
			fmt.Println("║     fm status         Check service status           ║")
			fmt.Println("║                                                      ║")
			fmt.Println("║   📖 More help: fm --help                            ║")
			fmt.Println("║                                                      ║")
			fmt.Println("╚══════════════════════════════════════════════════════╝")
			fmt.Println()
		} else {
			fmt.Println()
			fmt.Printf("  ⚡ FlashMemory v%s\n", Version)
			fmt.Println()
			showStatusBrief()
			fmt.Println()
			fmt.Println("  Quick commands:")
			fmt.Println("    fm index .          Index current directory")
			fmt.Println("    fm query \"keyword\"  Search codebase")
			fmt.Println("    fm serve / fm stop  Manage HTTP service")
			fmt.Println("    fm --help           Full help")
			fmt.Println()
		}
	}
}

func showStatusBrief() {
	pidFile := filepath.Join(fmHome, "fm_http.pid")
	if pid, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pid))
		// Check if process is running
		if _, err := os.FindProcess(0); err == nil {
			if common.IsZH() {
				fmt.Printf("  状态: HTTP 服务运行中 (PID %s)\n", pidStr)
			} else {
				fmt.Printf("  Status: HTTP service running (PID %s)\n", pidStr)
			}
		}
	} else {
		if common.IsZH() {
			fmt.Println("  状态: HTTP 服务未运行")
		} else {
			fmt.Println("  Status: HTTP service not running")
		}
	}
	if common.IsZH() {
		fmt.Printf("  配置: %s\n", ConfigPath())
	} else {
		fmt.Printf("  Config: %s\n", ConfigPath())
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Execute runs the root command
func Execute() {
	// Early lang sniffing before cobra parses flags
	for i, arg := range os.Args {
		if arg == "-lang" || arg == "--lang" {
			if i+1 < len(os.Args) {
				common.SetLang(os.Args[i+1])
			}
		} else if strings.HasPrefix(arg, "-lang=") {
			common.SetLang(strings.TrimPrefix(arg, "-lang="))
		} else if strings.HasPrefix(arg, "--lang=") {
			common.SetLang(strings.TrimPrefix(arg, "--lang="))
		}
	}

	i18n := func(zh, en string) string {
		if common.IsZH() {
			return zh
		}
		return en
	}

	rootCmd.PersistentFlags().StringVar(&langFlag, "lang", "", i18n("指定语言 (zh/en)", "Target language (zh/en)"))
	rootCmd.PersistentFlags().StringVarP(&configFlag, "config", "c", "", i18n("配置文件路径", "Config file path"))
	rootCmd.PersistentFlags().StringVar(&engineFlag, "engine", "", i18n("指定向量引擎 (zvec/faiss)", "Specify vector engine (zvec/faiss)"))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
