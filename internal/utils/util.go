package utils

import (
	"bytes"
	"context"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// runCmdContext 执行命令，最多等待 timeout，实时打印输出并返回最终输出或错误
func runCmdContext(dir, name string, args []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
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

// 检查Python环境和必要的库是否已安装，如果缺少则自动创建.env虚拟环境并安装
func CheckPythonEnvironment(faissType, faissServiceDir string) error {
	// 检查Python是否已安装
	cmd := exec.Command("python3", "--version")
	output, err := cmd.CombinedOutput()
	pythonCmd := "python3"
	pipCmd := "pip"
	if err != nil {
		// 尝试python命令
		cmd = exec.Command("python", "--version")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Python未安装或无法访问: %v", err)
		}
		pythonCmd = "python"
		pipCmd = "pip"
	}
	log.Printf("检测到Python版本: %s", strings.TrimSpace(string(output)))

	// 构建虚拟环境目录路径
	envDir := filepath.Join(faissServiceDir, ".env")
	envPython := filepath.Join(envDir, "bin", "python")
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
		log.Println("正在创建 .env 虚拟环境…（最多 1 分钟）")
		if _, err := runCmdContext(faissServiceDir, pythonCmd, []string{"-m", "venv", ".env"}, time.Minute); err != nil {
			return fmt.Errorf("创建 .env 虚拟环境失败: %v", err)
		}
		log.Println(".env 虚拟环境创建成功")
	}

	// 需要安装的基础库和Faiss库
	requiredLibs := []string{"flask", "numpy", "flask_cors"}
	faissLib := "faiss-cpu==1.10.0"
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

	// 安装基础库
	for _, lib := range requiredLibs[:3] {
		// 检查库是否已安装
		cmd = exec.Command(envPython, "-m", pipCmd, "list")
		cmd.Dir = faissServiceDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("无法检查虚拟环境中的Python库: %v", err)
		}

		// 安装库
		if !strings.Contains(string(output), lib) {
			log.Printf("正在安装 %s...", lib)
			out, err := runCmdContext(faissServiceDir, envPython, []string{"-m", pipCmd, "install", lib}, 10*time.Minute)
			if err != nil {
				return fmt.Errorf("安装 %s 失败: %v\n%s", lib, err, string(out))
			}
			log.Printf("%s 安装成功", lib)
		} else {
			log.Printf("%s 已安装在虚拟环境中", lib)
		}
	}

	// 安装Faiss库
	lib := faissLib
	// 检查Faiss是否已安装
	cmd = exec.Command(envPython, "-m", pipCmd, "list")
	cmd.Dir = faissServiceDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("无法检查虚拟环境中的Python库: %v", err)
	}
	faissInstalled := strings.Contains(string(output), "faiss")

	if !faissInstalled {
		log.Printf("正在安装 %s...", lib)
		out, err := runCmdContext(faissServiceDir, envPython, []string{"-m", pipCmd, "install", lib}, time.Minute)
		if err != nil && lib == "faiss-gpu" {
			log.Printf("安装 %s 失败: %v\n%s", lib, err, string(out))
			log.Println("尝试安装CPU版本 (faiss-cpu)...")

			lib = "faiss-cpu"
			out, err = runCmdContext(faissServiceDir, envPython, []string{"-m", pipCmd, "install", lib}, time.Minute)
			if err != nil {
				return fmt.Errorf("安装 %s 也失败: %v\n%s", lib, err, string(out))
			}
		} else if err != nil {
			return fmt.Errorf("安装 %s 失败: %v\n%s", lib, err, string(out))
		}
		log.Printf("%s 安装成功", lib)
	} else {
		log.Println("faiss 已安装在虚拟环境中")
	}

	return nil
}

// 获取虚拟环境Python解释器路径
func GetEnvPythonPath(faissServiceDir string) (string, error) {
	// 构建虚拟环境Python路径
	envDir := filepath.Join(faissServiceDir, ".env")
	envPython := filepath.Join(envDir, "bin", "python")

	// 在Windows系统上路径会不同
	if runtime.GOOS == "windows" {
		envPython = filepath.Join(envDir, "Scripts", "python.exe")
	}

	return envPython, nil
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

func FilterJSONContent(result string) string {
	startIndex := strings.Index(result, "```json") // 查找 ```json 的起始位置
	endIndex := strings.LastIndex(result, "```")   // 查找 ``` 的结束位置
	logs.Warnf("filterJSON startIndex: %d, endIndex: %d", startIndex, endIndex)

	// 检查是否能找到有效的区间
	if startIndex != -1 && endIndex != -1 && startIndex < endIndex {
		// 截取 ```json 和 ``` 之间的内容
		logs.Warnf("截取到JSON内容~")
		return result[startIndex+7 : endIndex]
	}

	// 如果没有找到匹配的 ```json 和 ```，返回原始结果
	return result
}

func IsExcludedPath(path string, exclude []interface{}) bool {
	//读取excludeJson中key=exclude的字符串数组判断path是否包含在内
	for _, exc := range exclude {
		if strings.Contains(path, exc.(string)) {
			logs.Warnf("exclude path: %s", path)
			return true
		}
	}
	return false
}
