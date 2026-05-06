package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/internal/ingest"
	"github.com/spf13/cobra"
)

var (
	ingestDir       string
	ingestRecursive bool
	ingestWatch     bool
	ingestInterval  int
)

func init() {
	i18n := common.I18n

	ingestCmd.Flags().StringVar(&ingestDir, "dir", ".", i18n("项目根目录（用于构建相对路径）", "Project root directory for relative indexing path"))
	ingestCmd.Flags().BoolVar(&ingestRecursive, "recursive", true, i18n("递归扫描目录中的文档", "Recursively scan directory for documents"))
	ingestCmd.Flags().BoolVar(&ingestWatch, "watch", false, i18n("监听目录变化并自动 ingest", "Watch directory and auto-ingest"))
	ingestCmd.Flags().IntVar(&ingestInterval, "watch-interval", 5, i18n("watch 轮询间隔（秒）", "Watch polling interval in seconds"))

	rootCmd.AddCommand(ingestCmd)
}

var ingestCmd = &cobra.Command{
	Use:   "ingest <file|dir>",
	Short: common.I18n("摄取文档并纳入统一索引", "Ingest documents into unified index"),
	Long: common.I18n(
		"将 md/markdown/txt/rst/pdf/pptx/docx/image 文档通过统一索引链路纳入 FlashMemory，支持文件或目录输入。",
		"Ingest md/markdown/txt/rst/pdf/pptx/docx/image documents into FlashMemory with the unified indexing pipeline.",
	),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := EnsureInit(); err != nil {
			return err
		}
		return runIngest(args[0])
	},
}

func runIngest(target string) error {
	rootAbs, err := filepath.Abs(ingestDir)
	if err != nil {
		return fmt.Errorf(common.I18n(
			"解析 --dir 失败: %v",
			"failed to resolve --dir: %v",
		), err)
	}

	targetAbs := target
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(rootAbs, target)
	}
	targetInfo, err := os.Stat(targetAbs)
	if err != nil {
		return fmt.Errorf(common.I18n(
			"目标路径不可访问: %v",
			"target path is not accessible: %v",
		), err)
	}

	docs, err := ingest.CollectTextDocuments(targetAbs, ingestRecursive)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf(common.I18n(
			"未发现可摄取文档（支持扩展名: %s）",
			"no ingestible documents found (supported extensions: %s)",
		), strings.Join(ingest.SupportedTextDocumentExtensions(), ", "))
	}

	if !ingestWatch {
		return runIngestOnce(rootAbs, targetAbs, targetInfo, docs)
	}
	return runIngestWatch(rootAbs, targetAbs, targetInfo, docs)
}

func runIngestOnce(rootAbs string, targetAbs string, targetInfo os.FileInfo, docs []string) error {
	relTarget, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || strings.HasPrefix(relTarget, "..") {
		return fmt.Errorf(common.I18n(
			"目标路径必须位于 --dir 目录内。请调整 --dir 或传入相对路径",
			"target must be inside --dir. Adjust --dir or pass a relative path",
		))
	}

	// Reuse existing index pipeline through selective update mode.
	normalizedRelTarget := filepath.ToSlash(relTarget)
	indexFile = normalizedRelTarget

	fmt.Println()
	if common.IsZH() {
		fmt.Printf("  📚 检测到 %d 个文档，准备纳入索引\n", len(docs))
		fmt.Printf("  根目录: %s\n", rootAbs)
		fmt.Printf("  目标路径: %s\n", normalizedRelTarget)
		if targetInfo.IsDir() {
			fmt.Printf("  扫描模式: recursive=%t\n", ingestRecursive)
		}
	} else {
		fmt.Printf("  📚 Found %d documents, preparing ingest index\n", len(docs))
		fmt.Printf("  Root: %s\n", rootAbs)
		fmt.Printf("  Target: %s\n", normalizedRelTarget)
		if targetInfo.IsDir() {
			fmt.Printf("  Scan mode: recursive=%t\n", ingestRecursive)
		}
	}
	fmt.Println()

	return runIndex(rootAbs)
}

func runIngestWatch(rootAbs string, targetAbs string, targetInfo os.FileInfo, initialDocs []string) error {
	if ingestInterval <= 0 {
		ingestInterval = 5
	}

	if err := runIngestOnce(rootAbs, targetAbs, targetInfo, initialDocs); err != nil {
		return err
	}

	snapshot := buildDocSnapshot(initialDocs)
	interval := time.Duration(ingestInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if common.IsZH() {
		fmt.Printf("  👀 watch 模式已启动，轮询间隔 %ds，按 Ctrl+C 退出。\n", ingestInterval)
	} else {
		fmt.Printf("  👀 watch mode started, polling every %ds, press Ctrl+C to stop.\n", ingestInterval)
	}

	for {
		select {
		case <-sigCh:
			fmt.Println()
			if common.IsZH() {
				fmt.Println("  ✅ watch 模式已停止")
			} else {
				fmt.Println("  ✅ watch mode stopped")
			}
			return nil
		case <-ticker.C:
			docs, err := ingest.CollectTextDocuments(targetAbs, ingestRecursive)
			if err != nil {
				fmt.Printf("  [watch] collect failed: %v\n", err)
				continue
			}
			changed, nextSnapshot := detectChangedDocs(snapshot, docs)
			snapshot = nextSnapshot
			if len(changed) == 0 {
				continue
			}

			for _, docPath := range changed {
				rel, err := filepath.Rel(rootAbs, docPath)
				if err != nil || strings.HasPrefix(rel, "..") {
					continue
				}
				indexFile = filepath.ToSlash(rel)
				if common.IsZH() {
					fmt.Printf("  🔄 检测到变更文档: %s\n", indexFile)
				} else {
					fmt.Printf("  🔄 detected changed document: %s\n", indexFile)
				}
				if err := runIndex(rootAbs); err != nil {
					fmt.Printf("  [watch] ingest failed for %s: %v\n", docPath, err)
				}
			}
		}
	}
}

func buildDocSnapshot(paths []string) map[string]int64 {
	snapshot := make(map[string]int64, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		snapshot[p] = info.ModTime().UnixNano()
	}
	return snapshot
}

func detectChangedDocs(previous map[string]int64, current []string) ([]string, map[string]int64) {
	next := buildDocSnapshot(current)
	changed := make([]string, 0, len(current))
	for path, ts := range next {
		prevTs, ok := previous[path]
		if !ok || prevTs != ts {
			changed = append(changed, path)
		}
	}
	return changed, next
}
