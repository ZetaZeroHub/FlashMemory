package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

var (
	querySearchMode  string
	queryLimit       int
	queryIncludeCode bool
	queryDir         string
)

func init() {
	i18n := common.I18n

	queryCmd.Flags().StringVar(&querySearchMode, "mode", "semantic", i18n("搜索模式: semantic, keyword, hybrid", "Search mode: semantic, keyword, hybrid"))
	queryCmd.Flags().IntVar(&queryLimit, "limit", 5, i18n("返回结果数量", "Number of results"))
	queryCmd.Flags().BoolVar(&queryIncludeCode, "include-code", false, i18n("结果中包含代码片段", "Include code snippets"))
	queryCmd.Flags().StringVar(&queryDir, "dir", ".", i18n("项目目录", "Project directory"))

	rootCmd.AddCommand(queryCmd)
}

var queryCmd = &cobra.Command{
	Use:   "query <keyword>",
	Short: common.I18n("语义搜索代码库", "Semantic search codebase"),
	Long: common.I18n(
		"使用自然语言查询已索引的代码库，支持语义、关键词和混合搜索模式。",
		"Search indexed codebase with natural language queries.",
	),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := EnsureInit(); err != nil {
			return err
		}
		return runQuery(args[0])
	},
}

func runQuery(queryStr string) error {
	// Delegate to the original fm binary with -query_only flag
	fmBin := findFmCoreBinary()
	if fmBin == "" {
		return fmt.Errorf(common.I18n(
			"找不到 fm 核心二进制文件，请确保安装完整",
			"Cannot find fm core binary. Please ensure proper installation",
		))
	}

	var coreArgs []string
	coreArgs = append(coreArgs, "-dir", queryDir)
	coreArgs = append(coreArgs, "-query", queryStr)
	coreArgs = append(coreArgs, "-query_only")
	coreArgs = append(coreArgs, "-search_mode", querySearchMode)

	if langFlag != "" {
		coreArgs = append(coreArgs, "-lang", langFlag)
	}

	cmd := exec.Command(fmBin, coreArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	cmd.Env = os.Environ()
	if faissDir := resolveFaissServiceDir(""); faissDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("FAISS_SERVICE_PATH=%s", faissDir))
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}
