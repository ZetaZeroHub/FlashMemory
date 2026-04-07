package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: common.I18n("停止 HTTP API 服务", "Stop HTTP API server"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStop()
	},
}

func runStop() error {
	pidFile := filepath.Join(fmHome, "fm_http.pid")

	if !isServiceRunning(pidFile) {
		if common.IsZH() {
			fmt.Println("  ℹ️  HTTP 服务未在运行")
		} else {
			fmt.Println("  ℹ️  HTTP service is not running")
		}
		return nil
	}

	pid := readPID(pidFile)
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf(common.I18n("找不到进程: %v", "Process not found: %v"), err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf(common.I18n("发送停止信号失败: %v", "Failed to send stop signal: %v"), err)
	}

	// Clean up PID file
	os.Remove(pidFile)

	if common.IsZH() {
		fmt.Printf("  ✅ HTTP 服务已停止 (PID %d)\n", pid)
	} else {
		fmt.Printf("  ✅ HTTP service stopped (PID %d)\n", pid)
	}

	return nil
}
