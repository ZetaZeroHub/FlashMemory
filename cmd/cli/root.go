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
	Version = "0.4.5"

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

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cCyan   = "\033[36m"
	cPurple = "\033[35m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
)

func showWelcome() {
	configExists := fileExists(ConfigPath())

	// Top border
	fmt.Println()
	fmt.Printf("%s%s╭─────────────────────────────────────────────────────────────╮%s\n", cDim, cPurple, cReset)
	
	if common.IsZH() {
		fmt.Printf("%s│ %s  ⚡ FlashMemory %s%s v%-36s %s│%s\n", cDim, cCyan, cYellow, cBold, Version, cDim, cReset)
		fmt.Printf("%s│ %s  跨语言代码分析与语义搜索系统                              %s│%s\n", cDim, cReset, cDim, cReset)
		fmt.Printf("%s╰─────────────────────────────────────────────────────────────╯%s\n", cDim, cReset)
		
		if !configExists {
			fmt.Println()
			fmt.Printf("  %s👋 很高兴遇见你！检测到这是您的首次运行。%s\n", cBold, cReset)
			fmt.Printf("  %sFlashMemory 需要进行一次极简配置，请按以下步骤开启探索之旅：%s\n", cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s🚀 快速入门指引 (Onboarding):%s\n", cPurple, cReset)
			fmt.Printf("    %sStep 1.%s %sfm init%s           %s—  初始化基础配置（可灵活切换底层 Zvec/FAISS 向量引擎与 LLM 模型）%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 2.%s %sfm index .%s        %s—  一键解析当前项目，构建代码语义与结构的高速向量索引%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 3.%s %sfm query \"登录\"%s   %s—  体验自然语言搜代码的快感，跟传统正则说拜拜！%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 4.%s %sfm serve%s          %s—  启动常驻 API 服务，作为 IDE 插件或其它应用的智能大脑%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s💡 核心特色功能:%s\n", cYellow, cReset)
			fmt.Printf("    %s• 万物皆可索引：%s 支持包括 Go/Python/JS/Java 等多语言全量/增量解析\n", cBold, cReset)
			fmt.Printf("    %s• AI 语义加持：%s 基于 LLM 提取业务逻辑与摘要，彻底摆脱关键字匹配\n", cBold, cReset)
			fmt.Printf("    %s• 极速本地检索：%s 深度集成 FAISS/Zvec 引擎，离线检索毫秒级响应\n", cBold, cReset)
			fmt.Println()
			fmt.Printf("  %s📖 进阶指引: %sfm --help%s\n", cDim, cBold, cReset)
			fmt.Println()
		} else {
			fmt.Println()
			showStatusBrief(true)
			fmt.Println()
			fmt.Printf("  %s⚡ 常用命令:%s\n", cPurple, cReset)
			fmt.Printf("    %sfm index .%s          %s一键索引当前工作目录%s\n", cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm query \"关键词\"%s   %s自然语言语义化搜索代码库%s\n", cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm serve %s/ %sfm stop%s  %s管理本地 API 后台守护服务%s\n", cCyan, cDim, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm status%s           %s查看系统与向量引擎健康状态%s\n", cCyan, cReset, cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s📖 完整帮助: %sfm --help%s\n", cDim, cBold, cReset)
			fmt.Println()
		}
	} else {
		// English Version
		fmt.Printf("%s│ %s  ⚡ FlashMemory %s%s v%-36s %s│%s\n", cDim, cCyan, cYellow, cBold, Version, cDim, cReset)
		fmt.Printf("%s│ %s  Cross-language Code Analysis & Semantic Search            %s│%s\n", cDim, cReset, cDim, cReset)
		fmt.Printf("%s╰─────────────────────────────────────────────────────────────╯%s\n", cDim, cReset)
		
		if !configExists {
			fmt.Println()
			fmt.Printf("  %s👋 Great to see you! First run detected.%s\n", cBold, cReset)
			fmt.Printf("  %sFlashMemory needs a quick setup. Follow these steps to begin your journey:%s\n", cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s🚀 Quick Start Onboarding:%s\n", cPurple, cReset)
			fmt.Printf("    %sStep 1.%s %sfm init%s           %s—  Initialize config (Switch between Zvec/FAISS engines & LLM models)%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 2.%s %sfm index .%s        %s—  1-click parse & build high-speed vector index for your project%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 3.%s %sfm query \"login\"%s  %s—  Experience natural language code search. Say goodbye to regex!%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sStep 4.%s %sfm serve%s          %s—  Start the API daemon, turning FM into a smart brain for IDEs/apps%s\n", cYellow, cReset, cCyan, cReset, cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s💡 Core Features:%s\n", cYellow, cReset)
			fmt.Printf("    %s• Polyglot Indexing:%s Supports Go/Python/JS/Java full & incremental sync\n", cBold, cReset)
			fmt.Printf("    %s• AI Semantic Tech:%s LLM-driven abstraction, beyond simple keyword matching\n", cBold, cReset)
			fmt.Printf("    %s• Lightning Local Search:%s Integrated FAISS/Zvec for ms-level offline search\n", cBold, cReset)
			fmt.Println()
			fmt.Printf("  %s📖 Advanced Help: %sfm --help%s\n", cDim, cBold, cReset)
			fmt.Println()
		} else {
			fmt.Println()
			showStatusBrief(false)
			fmt.Println()
			fmt.Printf("  %s⚡ Quick Commands:%s\n", cPurple, cReset)
			fmt.Printf("    %sfm index .%s          %sIndex current workspace%s\n", cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm query \"keyword\"%s  %sSemantic codebase search%s\n", cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm serve %s/ %sfm stop%s  %sManage local API service%s\n", cCyan, cDim, cCyan, cReset, cDim, cReset)
			fmt.Printf("    %sfm status%s           %sCheck system health%s\n", cCyan, cReset, cDim, cReset)
			fmt.Println()
			fmt.Printf("  %s📖 Full Help: %sfm --help%s\n", cDim, cBold, cReset)
			fmt.Println()
		}
	}
}

func showStatusBrief(isZH bool) {
	pidFile := filepath.Join(fmHome, "fm_http.pid")
	if pid, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pid))
		// Check if process is running
		if _, err := os.FindProcess(0); err == nil {
			if isZH {
				fmt.Printf("  %s◉%s 状态: %sHTTP 服务运行中%s (PID %s)\n", cGreen, cReset, cBold, cReset, pidStr)
			} else {
				fmt.Printf("  %s◉%s Status: %sHTTP service running%s (PID %s)\n", cGreen, cReset, cBold, cReset, pidStr)
			}
		}
	} else {
		if isZH {
			fmt.Printf("  %s○%s 状态: %sHTTP 服务未运行%s\n", cDim, cReset, cDim, cReset)
		} else {
			fmt.Printf("  %s○%s Status: %sHTTP service not running%s\n", cDim, cReset, cDim, cReset)
		}
	}
	if isZH {
		fmt.Printf("  %s⚙%s 配置: %s%s%s\n", cDim, cReset, cDim, ConfigPath(), cReset)
	} else {
		fmt.Printf("  %s⚙%s Config: %s%s%s\n", cDim, cReset, cDim, ConfigPath(), cReset)
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
