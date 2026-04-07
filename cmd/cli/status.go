package cli

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: common.I18n("查看服务运行状态", "Check service status"),
	Run: func(cmd *cobra.Command, args []string) {
		runStatus()
	},
}

func runStatus() {
	pidFile := filepath.Join(fmHome, "fm_http.pid")
	configPath := ConfigPath()

	fmt.Println()
	fmt.Printf("  ⚡ FlashMemory v%s\n", Version)
	fmt.Println()

	// HTTP Service status
	if isServiceRunning(pidFile) {
		pid := readPID(pidFile)
		if common.IsZH() {
			fmt.Printf("  HTTP 服务:  🟢 运行中 (PID %d)\n", pid)
		} else {
			fmt.Printf("  HTTP Service:  🟢 Running (PID %d)\n", pid)
		}

		// Check port
		if isPortOpen("5532") {
			if common.IsZH() {
				fmt.Println("  端口:       5532 ✅")
			} else {
				fmt.Println("  Port:          5532 ✅")
			}
		}
	} else {
		if common.IsZH() {
			fmt.Println("  HTTP 服务:  🔴 未运行")
		} else {
			fmt.Println("  HTTP Service:  🔴 Not running")
		}
	}

	// Config
	if fileExists(configPath) {
		if common.IsZH() {
			fmt.Printf("  配置文件:   ✅ %s\n", configPath)
		} else {
			fmt.Printf("  Config:        ✅ %s\n", configPath)
		}
	} else {
		if common.IsZH() {
			fmt.Println("  配置文件:   ❌ 未初始化 (运行 fm init)")
		} else {
			fmt.Println("  Config:        ❌ Not initialized (run fm init)")
		}
	}

	// Log file
	logFile := filepath.Join(fmHome, "logs", "fm_http.log")
	if fileExists(logFile) {
		if common.IsZH() {
			fmt.Printf("  日志文件:   %s\n", logFile)
		} else {
			fmt.Printf("  Log file:      %s\n", logFile)
		}
	}

	fmt.Println()
}

func isPortOpen(port string) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
