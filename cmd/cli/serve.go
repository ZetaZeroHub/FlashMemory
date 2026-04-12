package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

var (
	servePort       string
	serveForeground bool
)

func init() {
	i18n := common.I18n

	serveCmd.Flags().StringVar(&servePort, "port", "5532", i18n("HTTP 服务端口", "HTTP service port"))
	serveCmd.Flags().BoolVar(&serveForeground, "foreground", false, i18n("前台运行（默认后台）", "Run in foreground (default: background)"))

	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: common.I18n("启动 HTTP API 服务", "Start HTTP API server"),
	Long: common.I18n(
		"启动 FlashMemory HTTP API 服务器。默认后台运行。使用 --foreground 前台运行。",
		"Start FlashMemory HTTP API server. Runs in background by default. Use --foreground for foreground.",
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := EnsureInit(); err != nil {
			return err
		}
		return runServe()
	},
}

func runServe() error {
	pidFile := filepath.Join(fmHome, "fm_http.pid")

	// Check if already running
	if isServiceRunning(pidFile) {
		pid := readPID(pidFile)
		if common.IsZH() {
			fmt.Printf("  ⚠️  HTTP 服务已在运行中 (PID %d)\n", pid)
			fmt.Println("  使用 'fm stop' 停止服务，或 'fm status' 查看状态")
		} else {
			fmt.Printf("  ⚠️  HTTP service is already running (PID %d)\n", pid)
			fmt.Println("  Use 'fm stop' to stop, or 'fm status' to check status")
		}
		return nil
	}

	configPath := ConfigPath()

	if serveForeground {
		// Foreground mode: exec fm_http directly
		return runServeForeground(configPath)
	}

	// Background (daemon) mode
	return runServeDaemon(configPath, pidFile)
}

func runServeForeground(configPath string) error {
	if common.IsZH() {
		fmt.Printf("  🚀 启动 HTTP 服务 (前台模式, 端口 %s)...\n", servePort)
		fmt.Println("  按 Ctrl+C 停止服务")
	} else {
		fmt.Printf("  🚀 Starting HTTP service (foreground, port %s)...\n", servePort)
		fmt.Println("  Press Ctrl+C to stop")
	}

	// Find the fm_http binary or use the current binary's directory
	fmHTTPBin := findFmHTTPBinary()
	if fmHTTPBin == "" {
		return fmt.Errorf(common.I18n(
			"找不到 fm_http 二进制文件。请确保安装完整",
			"Cannot find fm_http binary. Please ensure proper installation",
		))
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("FM_PORT=%s", servePort))
	if engineFlag != "" {
		env = append(env, fmt.Sprintf("FM_ENGINE=%s", engineFlag))
	}
	if faissDir := resolveFaissServiceDir(""); faissDir != "" {
		env = append(env, fmt.Sprintf("FAISS_SERVICE_PATH=%s", faissDir))
	}

	cmd := exec.Command(fmHTTPBin, "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = env

	return cmd.Run()
}

func runServeDaemon(configPath string, pidFile string) error {
	fmHTTPBin := findFmHTTPBinary()
	if fmHTTPBin == "" {
		return fmt.Errorf(common.I18n(
			"找不到 fm_http 二进制文件。请确保安装完整",
			"Cannot find fm_http binary. Please ensure proper installation",
		))
	}

	logFile := filepath.Join(fmHome, "logs", "fm_http.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)

	// Open log file
	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf(common.I18n("创建日志文件失败: %v", "Failed to create log file: %v"), err)
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("FM_PORT=%s", servePort))
	if engineFlag != "" {
		env = append(env, fmt.Sprintf("FM_ENGINE=%s", engineFlag))
	}
	if faissDir := resolveFaissServiceDir(""); faissDir != "" {
		env = append(env, fmt.Sprintf("FAISS_SERVICE_PATH=%s", faissDir))
	}

	cmd := exec.Command(fmHTTPBin, "-c", configPath)
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.Env = env
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		lf.Close()
		return fmt.Errorf(common.I18n("启动服务失败: %v", "Failed to start service: %v"), err)
	}

	// Write PID file
	pid := cmd.Process.Pid
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)

	// Release the process (don't wait)
	cmd.Process.Release()
	lf.Close()

	fmt.Println()
	if common.IsZH() {
		fmt.Printf("  🚀 HTTP 服务已启动 (后台模式)\n")
		fmt.Printf("     PID:  %d\n", pid)
		fmt.Printf("     端口: %s\n", servePort)
		fmt.Printf("     日志: %s\n", logFile)
		fmt.Println()
		fmt.Println("  管理命令:")
		fmt.Println("    fm status    查看服务状态")
		fmt.Println("    fm logs      查看服务日志")
		fmt.Println("    fm stop      停止服务")
	} else {
		fmt.Printf("  🚀 HTTP service started (background)\n")
		fmt.Printf("     PID:  %d\n", pid)
		fmt.Printf("     Port: %s\n", servePort)
		fmt.Printf("     Logs: %s\n", logFile)
		fmt.Println()
		fmt.Println("  Management commands:")
		fmt.Println("    fm status    Check service status")
		fmt.Println("    fm logs      View service logs")
		fmt.Println("    fm stop      Stop service")
	}
	fmt.Println()

	return nil
}

// findFmHTTPBinary locates the fm_http binary
func findFmHTTPBinary() string {
	// Check alongside current executable
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		candidate := filepath.Join(execDir, "fm_http")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check in ~/.flashmemory/bin/
	candidate := filepath.Join(fmHome, "bin", "fm_http")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Check in PATH
	if p, err := exec.LookPath("fm_http"); err == nil {
		return p
	}

	return ""
}

func isServiceRunning(pidFile string) bool {
	pid := readPID(pidFile)
	if pid == 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check process existence
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func readPID(pidFile string) int {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0
	}
	return pid
}
