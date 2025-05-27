//go:build !windows
// +build !windows

package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// 启动Faiss服务
func StartFaissService(faissServiceDir string) (*os.Process, error) {
	// 构建faiss_server.py的完整路径
	faissServerPath := filepath.Join(faissServiceDir, "faiss_server.py")

	// 检查文件是否存在
	if _, err := os.Stat(faissServerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Faiss服务脚本不存在: %s", faissServerPath)
	}

	// 获取虚拟环境Python路径
	envPython, err := GetEnvPythonPath(faissServiceDir)
	if err != nil {
		return nil, err
	}

	// 检查虚拟环境Python是否存在
	if _, err = os.Stat(envPython); os.IsNotExist(err) {
		return nil, fmt.Errorf(".env虚拟环境中的Python解释器不存在: %s", envPython)
	}

	// 检查并清理默认端口(5533)上的占用进程
	if err := CheckAndKillPort(5533); err != nil {
		return nil, fmt.Errorf("端口清理失败: %v", err)
	}

	// 启动Faiss服务
	cmd := exec.Command(envPython, faissServerPath)
	// 设置进程组ID，以便后续可以终止整个进程组
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 将输出重定向到标准输出和标准错误
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 启动进程（后台运行）
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动Faiss服务失败: %v", err)
	}

	// 立即释放进程资源，使进程在后台运行
	go func() {
		_ = cmd.Wait()
	}()

	log.Printf("Faiss服务已启动，PID: %d", cmd.Process.Pid)
	return cmd.Process, nil
}

// 检查端口是否被占用并终止占用进程
func CheckAndKillPort(port int) error {
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果端口未被占用，直接返回
		return nil
	}

	pids := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, pid := range pids {
		if pid == "" {
			continue
		}
		log.Printf("端口 %d 被进程 %s 占用，正在终止...", port, pid)
		killCmd := exec.Command("kill", "-9", pid)
		if err := killCmd.Run(); err != nil {
			log.Printf("终止进程 %s 失败: %v", pid, err)
			// 尝试终止整个进程组
			pgidCmd := exec.Command("ps", "-o", "pgid=", "-p", pid)
			pgidOut, pgidErr := pgidCmd.CombinedOutput()
			if pgidErr == nil {
				pgid := strings.TrimSpace(string(pgidOut))
				if pgid != "" {
					killPgCmd := exec.Command("kill", "-9", "--", "-"+pgid)
					if pgidErr := killPgCmd.Run(); pgidErr != nil {
						return fmt.Errorf("无法终止进程组 %s: %v", pgid, pgidErr)
					}
					log.Printf("已成功终止进程组 %s", pgid)
					continue
				}
			}
			return fmt.Errorf("无法终止进程 %s: %v", pid, err)
		}
		log.Printf("已成功终止进程 %s", pid)
	}
	return nil
}

// 停止Faiss服务
func StopFaissService(process *os.Process) error {
	if process == nil {
		return nil
	}

	// 获取进程组ID
	pgid, err := syscall.Getpgid(process.Pid)
	if err != nil {
		return fmt.Errorf("获取进程组ID失败: %v", err)
	}

	// 终止整个进程组
	err = syscall.Kill(-pgid, syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("终止Faiss服务失败: %v", err)
	}

	log.Println("Faiss服务已停止")
	return nil
}
