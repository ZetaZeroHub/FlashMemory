//go:build windows
// +build windows

package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// StartFaissService 在 Windows 下启动 Faiss 服务，
// 会先清理端口占用，然后在新的进程组中运行 Python 脚本
func StartFaissService(faissServiceDir string) (*os.Process, error) {
	// 构建 faiss_server.py 的完整路径
	faissServerPath := filepath.Join(faissServiceDir, "faiss_server.py")
	if _, err := os.Stat(faissServerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Faiss 服务脚本不存在: %s", faissServerPath)
	}

	// 获取虚拟环境 Python 路径
	envPython, err := GetEnvPythonPath(faissServiceDir)
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(envPython); os.IsNotExist(err) {
		return nil, fmt.Errorf(".env 虚拟环境中的 Python 解释器不存在: %s", envPython)
	}

	// Windows 下清理端口
	if err := CheckAndKillPort(5533); err != nil {
		return nil, fmt.Errorf("端口清理失败: %v", err)
	}

	// 启动 Faiss 服务，指定在新进程组中运行，以便后续可以统一终止
	cmd := exec.Command(envPython, faissServerPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 Faiss 服务失败: %v", err)
	}

	log.Printf("Faiss 服务已启动 (Windows)，PID: %d", cmd.Process.Pid)
	return cmd.Process, nil
}

// 在 Windows 上，通过 tasklist + taskkill 来替代 lsof/kill
func CheckAndKillPort(port int) error {
	// 1) 找到占用该端口的 PID
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`Get-NetTCPConnection -LocalPort %d | Select-Object -ExpandProperty OwningProcess`, port))
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 如果查不到就跳过
		log.Printf("无法查询端口 %d 的占用: %v", port, err)
		return nil
	}
	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		return nil
	}

	// 解析 PID，确保它是一个有效的整数
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		log.Printf("无法解析 PID: %v", err)
		return nil
	}

	log.Printf("端口 %d 被 PID=%d 占用，正在终止…", port, pid)

	// 终止占用端口的进程
	killCmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
	kout, kerr := killCmd.CombinedOutput()
	if kerr != nil {
		// 输出错误信息时增加字符编码的处理
		log.Printf("taskkill 命令输出: %s", string(kout))
		return fmt.Errorf("taskkill 失败: %v\n%s", kerr, string(kout))
	}

	log.Printf("已成功终止进程 %d", pid)
	return nil
}

func StopFaissService(process *os.Process) error {
	if process == nil {
		return nil
	}
	// Windows 下直接用 Kill
	if err := process.Kill(); err != nil {
		return fmt.Errorf("终止 Faiss 服务失败: %v", err)
	}
	log.Println("Faiss 服务已停止")
	return nil
}

// runCmdContextWindows Windows特定的命令执行函数，隐藏cmd窗口
func runCmdContextWindows(dir, name string, args []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	// Windows下隐藏cmd窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	// 实时打印到主进程的 stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 同时保留完整输出，万一超时或失败，能一并返回
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)

	if err := cmd.Run(); err != nil {
		return buf.Bytes(), fmt.Errorf("命令 %s %v 失败: %w，输出:\n%s", name, args, err, buf.String())
	}
	return buf.Bytes(), nil
}

// isPackageInstalledWindows Windows特定的包检查函数，隐藏cmd窗口
func isPackageInstalledWindows(envPython, packageName string) bool {
	cmd := exec.Command(envPython, "-m", "pip", "show", packageName)

	// Windows下隐藏cmd窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	err := cmd.Run()
	return err == nil
}
