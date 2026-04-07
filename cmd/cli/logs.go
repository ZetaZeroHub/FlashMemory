package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

var logsLines int

func init() {
	i18n := common.I18n
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, i18n("显示最近 N 行", "Show last N lines"))
	rootCmd.AddCommand(logsCmd)
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: common.I18n("查看服务日志", "View service logs"),
	Long: common.I18n(
		"实时查看 HTTP 服务日志。按 Ctrl+C 退出。",
		"View HTTP service logs in real-time. Press Ctrl+C to exit.",
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs()
	},
}

func runLogs() error {
	logFile := filepath.Join(fmHome, "logs", "fm_http.log")

	if !fileExists(logFile) {
		if common.IsZH() {
			fmt.Println("  ℹ️  日志文件不存在。请先启动服务: fm serve")
		} else {
			fmt.Println("  ℹ️  Log file not found. Start the service first: fm serve")
		}
		return nil
	}

	// Use tail -f for real-time log viewing
	tailCmd := exec.Command("tail", "-n", fmt.Sprintf("%d", logsLines), "-f", logFile)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr

	if common.IsZH() {
		fmt.Printf("  📋 日志文件: %s (按 Ctrl+C 退出)\n\n", logFile)
	} else {
		fmt.Printf("  📋 Log file: %s (Ctrl+C to exit)\n\n", logFile)
	}

	return tailCmd.Run()
}
