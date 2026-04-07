package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

var (
	indexFaissType  string
	indexSearchMode string
	indexForce      bool
	indexCommit     string
	indexBranch     string
	indexFile       string
	indexUseFaiss   bool
	indexFaissPath  string
)

func init() {
	i18n := common.I18n

	indexCmd.Flags().StringVar(&indexFaissType, "faiss", "cpu", i18n("Faiss 版本: 'cpu' 或 'gpu'", "Faiss version: 'cpu' or 'gpu'"))
	indexCmd.Flags().StringVar(&indexSearchMode, "search-mode", "semantic", i18n("搜索模式: semantic, keyword, hybrid", "Search mode: semantic, keyword, hybrid"))
	indexCmd.Flags().BoolVar(&indexForce, "force-full", false, i18n("强制进行全量索引", "Force full indexing"))
	indexCmd.Flags().StringVar(&indexCommit, "commit", "", i18n("指定 commit hash", "Specify commit hash"))
	indexCmd.Flags().StringVar(&indexBranch, "branch", "master", i18n("指定分支名称", "Specify branch name"))
	indexCmd.Flags().StringVar(&indexFile, "file", "", i18n("定量更新的文件或文件夹", "File or dir for partial update"))
	indexCmd.Flags().BoolVar(&indexUseFaiss, "use-faiss", false, i18n("使用 Faiss 原生索引存储", "Use Faiss native index storage"))
	indexCmd.Flags().StringVar(&indexFaissPath, "faiss-path", "", i18n("指定 FAISSService 目录", "Specify FAISSService directory"))

	rootCmd.AddCommand(indexCmd)
}

var indexCmd = &cobra.Command{
	Use:   "index [dir]",
	Short: common.I18n("索引项目代码", "Index project code"),
	Long: common.I18n(
		"索引指定目录的代码文件，构建语义搜索索引。默认索引当前目录。",
		"Index code files in the specified directory. Defaults to current directory.",
	),
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := EnsureInit(); err != nil {
			return err
		}

		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		return runIndex(dir)
	},
}

func runIndex(dir string) error {
	// Delegate to the original fm binary (fm_core) with translated flags
	fmBin := findFmCoreBinary()
	if fmBin == "" {
		return fmt.Errorf(common.I18n(
			"找不到 fm 核心二进制文件，请确保安装完整",
			"Cannot find fm core binary. Please ensure proper installation",
		))
	}

	// Build args for the legacy binary
	var coreArgs []string
	coreArgs = append(coreArgs, "-dir", dir)
	coreArgs = append(coreArgs, "-faiss", indexFaissType)
	coreArgs = append(coreArgs, "-search_mode", indexSearchMode)
	coreArgs = append(coreArgs, "-branch", indexBranch)

	if indexForce {
		coreArgs = append(coreArgs, "-force_full")
	}
	if indexCommit != "" {
		coreArgs = append(coreArgs, "-commit", indexCommit)
	}
	if indexFile != "" {
		coreArgs = append(coreArgs, "-file", indexFile)
	}
	if indexUseFaiss {
		coreArgs = append(coreArgs, "-use_faiss")
	}
	if indexFaissPath != "" {
		coreArgs = append(coreArgs, "-faiss_path", indexFaissPath)
	}
	if langFlag != "" {
		coreArgs = append(coreArgs, "-lang", langFlag)
	}

	cmd := exec.Command(fmBin, coreArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set FAISS_SERVICE_PATH if not already set
	cmd.Env = os.Environ()
	if faissDir := resolveFaissServiceDir(indexFaissPath); faissDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("FAISS_SERVICE_PATH=%s", faissDir))
	}

	if err := cmd.Run(); err != nil {
		// Don't wrap the error — the original binary already printed messages
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	// Show next-step guidance after successful indexing
	fmt.Println()
	if common.IsZH() {
		fmt.Println("  下一步:")
		fmt.Println("    fm query \"关键词\"    搜索代码")
		fmt.Println("    fm serve            启动 HTTP API 服务")
	} else {
		fmt.Println("  Next steps:")
		fmt.Println("    fm query \"keyword\"  Search codebase")
		fmt.Println("    fm serve            Start HTTP API server")
	}
	fmt.Println()

	return nil
}

// resolveFaissServiceDir finds the FAISSService directory
func resolveFaissServiceDir(explicitPath string) string {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err == nil {
			return explicitPath
		}
	}
	if envPath := os.Getenv("FAISS_SERVICE_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}
	// ~/.flashmemory/bin/FAISSService
	homeDir := filepath.Join(fmHome, "bin", "FAISSService")
	if _, err := os.Stat(homeDir); err == nil {
		return homeDir
	}
	// Next to executable
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "FAISSService")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// findFmCoreBinary locates the fm_core (original fm) binary
func findFmCoreBinary() string {
	// Check alongside current executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		// Try fm_core first (renamed original)
		for _, name := range []string{"fm_core", "fm_index"} {
			candidate := filepath.Join(execDir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// Check in ~/.flashmemory/bin/
	for _, name := range []string{"fm_core", "fm_index"} {
		candidate := filepath.Join(fmHome, "bin", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check in PATH
	for _, name := range []string{"fm_core", "fm_index"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	return ""
}
