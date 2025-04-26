package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// 检查Python环境和必要的库是否已安装，如果缺少则自动创建.env虚拟环境并安装
func CheckPythonEnvironment(faissType string) error {
	// 检查Python是否已安装
	cmd := exec.Command("python", "--version")
	output, err := cmd.CombinedOutput()
	pythonCmd := "python"
	pipCmd := "pip"
	if err != nil {
		// 尝试python3命令
		cmd = exec.Command("python3", "--version")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Python未安装或无法访问: %v", err)
		}
		pythonCmd = "python3"
		pipCmd = "pip3"
	}
	log.Printf("检测到Python版本: %s", strings.TrimSpace(string(output)))

	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("无法获取当前工作目录: %v", err)
	}

	// 构建虚拟环境目录路径

	// 检查.env虚拟环境是否存在
	envDir := filepath.Join(cwd, ".env")
	envPython := filepath.Join(envDir, "bin", "python")

	// 在Windows系统上路径会不同
	if runtime.GOOS == "windows" {
		envPython = filepath.Join(envDir, "Scripts", "python.exe")
	}

	// 检查虚拟环境是否已存在
	envExists := false
	if _, err := os.Stat(envDir); err == nil {
		if _, err := os.Stat(envPython); err == nil {
			envExists = true
			log.Println(".env虚拟环境已存在")
		}
	}

	// 如果虚拟环境不存在，创建它
	if !envExists {
		log.Println("正在创建.env虚拟环境...")
		cmd = exec.Command(pythonCmd, "-m", "venv", ".env")
		cmd.Dir = cwd
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("创建.env虚拟环境失败: %v\n%s", err, string(output))
		}
		log.Println(".env虚拟环境创建成功")
	}

	// 使用虚拟环境中的pip安装必要的库
	requiredLibs := []string{"flask", "numpy"}

	// 根据用户选择确定安装的faiss版本
	faissLib := "faiss-cpu"
	if faissType == "gpu" {
		faissLib = "faiss-gpu"
		log.Println("将安装GPU版本的Faiss (faiss-gpu)")
		log.Println("注意: faiss-gpu需要CUDA环境支持，请确保您的系统已安装CUDA")
		log.Println("如果安装失败，将自动尝试安装CPU版本")
	} else {
		log.Println("将安装CPU版本的Faiss (faiss-cpu)")
		log.Println("CPU版本适用于所有系统，但性能可能低于GPU版本")
		log.Println("如果您的系统有NVIDIA GPU和CUDA环境，可以使用-faiss=gpu参数获得更好性能")
	}
	requiredLibs = append(requiredLibs, faissLib)

	// 先安装基础库（flask和numpy）
	for _, lib := range requiredLibs[:2] {
		// 检查库是否已安装在虚拟环境中
		cmd = exec.Command(envPython, "-m", pipCmd, "list")
		cmd.Dir = cwd
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("无法检查虚拟环境中的Python库: %v", err)
		}

		// 如果库未安装，则安装它
		if !strings.Contains(string(output), lib) {
			log.Printf("正在安装 %s...", lib)
			cmd = exec.Command(envPython, "-m", pipCmd, "install", lib)
			cmd.Dir = cwd
			output, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("安装 %s 失败: %v\n%s", lib, err, string(output))
			}
			log.Printf("%s 安装成功", lib)
		} else {
			log.Printf("%s 已安装在虚拟环境中", lib)
		}
	}

	// 安装faiss库（可能是CPU或GPU版本）
	lib := faissLib
	// 检查faiss是否已安装
	cmd = exec.Command(envPython, "-m", pipCmd, "list")
	cmd.Dir = cwd
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("无法检查虚拟环境中的Python库: %v", err)
	}

	// 检查是否已安装任何版本的faiss
	faissInstalled := strings.Contains(string(output), "faiss")

	// 如果未安装，则尝试安装指定版本
	if !faissInstalled {
		log.Printf("正在安装 %s...", lib)
		cmd = exec.Command(envPython, "-m", pipCmd, "install", lib)
		cmd.Dir = cwd
		output, err = cmd.CombinedOutput()

		// 如果安装GPU版本失败，尝试安装CPU版本
		if err != nil && lib == "faiss-gpu" {
			log.Printf("安装 %s 失败: %v\n%s", lib, err, string(output))
			log.Println("尝试安装CPU版本 (faiss-cpu)...")

			lib = "faiss-cpu"
			cmd = exec.Command(envPython, "-m", pipCmd, "install", lib)
			cmd.Dir = cwd
			output, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("安装 %s 也失败: %v\n%s", lib, err, string(output))
			}
		} else if err != nil {
			return fmt.Errorf("安装 %s 失败: %v\n%s", lib, err, string(output))
		}

		log.Printf("%s 安装成功", lib)
	} else {
		log.Println("faiss 已安装在虚拟环境中")
	}

	return nil
}

// 获取虚拟环境Python解释器路径
func GetEnvPythonPath() (string, error) {
	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("无法获取当前工作目录: %v", err)
	}

	// 构建虚拟环境Python路径
	envDir := filepath.Join(cwd, ".env")
	envPython := filepath.Join(envDir, "bin", "python")

	// 在Windows系统上路径会不同
	if runtime.GOOS == "windows" {
		envPython = filepath.Join(envDir, "Scripts", "python.exe")
	}

	return envPython, nil
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

// 启动Faiss服务
func StartFaissService(faissServiceDir string) (*os.Process, error) {
	// 构建faiss_server.py的完整路径
	faissServerPath := filepath.Join(faissServiceDir, "faiss_server.py")

	// 检查文件是否存在
	if _, err := os.Stat(faissServerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Faiss服务脚本不存在: %s", faissServerPath)
	}

	// 获取虚拟环境Python路径
	envPython, err := GetEnvPythonPath()
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

// 获取源文件所在目录路径
func GetSourceFileDir() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法获取当前源文件路径")
	}
	return filepath.Dir(filename), nil
}

// 辅助函数：检查字符串是否在字符串切片中
func Contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

func IsHiddenDir(name string) bool {
	// 排除当前和上级目录
	if name == "." || name == ".." {
		return false
	}
	return strings.HasPrefix(name, ".")
}
