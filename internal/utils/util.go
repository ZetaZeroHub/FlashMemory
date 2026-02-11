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
	"runtime"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// configurePipMirror 配置pip镜像源到虚拟环境
func configurePipMirror(envDir, pipMirrorURL string) error {
	// 创建pip配置目录
	pipConfigDir := filepath.Join(envDir, "pip")
	if err := os.MkdirAll(pipConfigDir, 0755); err != nil {
		return fmt.Errorf("创建pip配置目录失败: %v", err)
	}

	// 创建pip.conf配置文件
	pipConfigFile := filepath.Join(pipConfigDir, "pip.conf")
	if runtime.GOOS == "windows" {
		pipConfigFile = filepath.Join(pipConfigDir, "pip.ini")
	}

	// 提取主机名用于trusted-host配置
	hostname := pipMirrorURL
	if strings.HasPrefix(hostname, "https://") {
		hostname = strings.TrimPrefix(hostname, "https://")
	} else if strings.HasPrefix(hostname, "http://") {
		hostname = strings.TrimPrefix(hostname, "http://")
	}
	// 移除路径部分，只保留主机名
	if idx := strings.Index(hostname, "/"); idx != -1 {
		hostname = hostname[:idx]
	}

	// 配置文件内容
	configContent := fmt.Sprintf(`[global]
index-url = %s
trusted-host = %s
`, pipMirrorURL, hostname)

	// 写入配置文件
	if err := os.WriteFile(pipConfigFile, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("写入pip配置文件失败: %v", err)
	}

	log.Printf("pip配置文件已创建: %s", pipConfigFile)
	return nil
}

// runCmdContext 执行命令，最多等待 timeout，实时打印输出并返回最终输出或错误
func runCmdContext(dir, name string, args []string, timeout time.Duration) ([]byte, error) {
	// 在Windows下使用特定的函数来隐藏cmd窗口
	if runtime.GOOS == "windows" {
		return runCmdContextWindows(dir, name, args, timeout)
	}

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
	// 加载配置以获取pip镜像源路径
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("警告: 无法加载配置文件，将使用默认pip源: %v", err)
	}

	// 定义多个镜像源作为备选方案
	mirrorSources := []string{
		"https://pypi.org/simple",                       // 官方源
		"https://mirrors.aliyun.com/pypi/simple",        // 阿里云
		"https://pypi.tuna.tsinghua.edu.cn/simple",      // 清华大学
		"https://mirrors.cloud.tencent.com/pypi/simple", // 腾讯云
		"https://mirrors.163.com/pypi/simple",           // 网易
	}

	// 构建pip安装参数，优先使用配置的镜像源
	var pipInstallArgs []string
	var currentMirror string
	if cfg != nil && cfg.PipPath != "" {
		currentMirror = cfg.PipPath
		log.Printf("使用配置的pip镜像源: %s", currentMirror)
		pipInstallArgs = []string{"-i", currentMirror}
	} else {
		currentMirror = mirrorSources[0] // 默认使用官方源
		log.Println("未配置pip镜像源，使用官方源")
		pipInstallArgs = []string{}
	}
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
			return fmt.Errorf("python未安装或无法访问: %v", err)
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

	// 配置pip镜像源
	if cfg != nil && cfg.PipPath != "" {
		log.Printf("正在配置pip镜像源为: %s", cfg.PipPath)
		if err := configurePipMirror(envDir, cfg.PipPath); err != nil {
			log.Printf("警告: 配置pip镜像源失败: %v", err)
			log.Println("将继续使用命令行参数指定镜像源")
		} else {
			log.Println("pip镜像源配置成功")
			// 清空pipInstallArgs，因为已经通过配置文件设置了镜像源
			pipInstallArgs = []string{}
		}
	}

	// 升级pip到最新版本
	log.Println("正在升级pip到最新版本...")
	upgradeArgs := []string{"-m", pipCmd, "install"}
	upgradeArgs = append(upgradeArgs, pipInstallArgs...)
	upgradeArgs = append(upgradeArgs, "--upgrade", "pip")
	out, err := runCmdContext(faissServiceDir, envPython, upgradeArgs, 5*time.Minute)
	if err != nil {
		log.Printf("警告: 升级pip失败: %v\n%s", err, string(out))
		log.Println("继续使用当前pip版本...")
	} else {
		log.Println("pip升级成功")
	}

	// 需要安装的基础库和Faiss库
	requiredLibs := []string{"flask", "numpy", "flask_cors", "psutil", "apscheduler"}

	// 根据Python版本选择合适的faiss-cpu版本
	var faissLib string
	if faissType == "gpu" {
		faissLib = "faiss-gpu"
		log.Println("将安装GPU版本的Faiss (faiss-gpu)")
		log.Println("注意: faiss-gpu需要CUDA环境支持，请确保您的系统已安装CUDA")
		log.Println("如果安装失败，将自动尝试安装CPU版本")
	} else {
		// 检查Python版本并选择合适的faiss-cpu版本
		pythonVersion := strings.TrimSpace(string(output))
		if strings.Contains(pythonVersion, "3.6") {
			faissLib = "faiss-cpu==1.7.2" // Python 3.6最高支持版本
			log.Println("检测到Python 3.6，将安装faiss-cpu==1.7.2")
		} else if strings.Contains(pythonVersion, "3.7") {
			faissLib = "faiss-cpu==1.7.4" // Python 3.7支持版本
			log.Println("检测到Python 3.7，将安装faiss-cpu==1.7.4")
		} else {
			faissLib = "faiss-cpu" // 最新版本，适用于Python 3.8+
			log.Println("检测到Python 3.8+，将安装最新版本的faiss-cpu")
		}
		log.Println("将安装CPU版本的Faiss (faiss-cpu)")
		log.Println("CPU版本适用于所有系统，但性能可能低于GPU版本")
		log.Println("如果您的系统有NVIDIA GPU和CUDA环境，可以使用-faiss=gpu参数获得更好性能")
	}
	requiredLibs = append(requiredLibs, faissLib)

	// 安装基础库
	for _, lib := range requiredLibs {
		// 使用改进的包检查逻辑
		if !isPackageInstalledWithAliases(envPython, lib) {
			log.Printf("正在安装 %s...", lib)
			// 构建完整的pip install命令参数
			installArgs := []string{"-m", pipCmd, "install"}
			installArgs = append(installArgs, pipInstallArgs...)
			installArgs = append(installArgs, lib)
			out, err := runCmdContext(faissServiceDir, envPython, installArgs, 10*time.Minute)
			if err != nil {
				return fmt.Errorf("安装 %s 失败: %v\n%s", lib, err, string(out))
			}
			log.Printf("%s 安装成功", lib)
		} else {
			// 提取包名用于日志显示
			packageName := strings.Split(lib, "==")[0]
			packageName = strings.Split(packageName, ">=")[0]
			log.Printf("%s 已安装在虚拟环境中", packageName)
		}
	}

	// 安装Faiss库
	lib := faissLib
	// 使用改进的包检查逻辑检查Faiss是否已安装
	if !isPackageInstalledWithAliases(envPython, lib) {
		log.Printf("正在安装 %s...", lib)

		// 尝试安装faiss库，如果失败则尝试多个镜像源
		err := tryInstallWithMultipleMirrors(faissServiceDir, envPython, pipCmd, lib, mirrorSources, currentMirror, time.Minute)
		if err != nil && lib == "faiss-gpu" {
			log.Printf("安装 %s 失败: %v", lib, err)
			log.Println("尝试安装CPU版本 (faiss-cpu)...")

			// 根据Python版本选择合适的faiss-cpu版本
			pythonVersion := strings.TrimSpace(string(output))
			if strings.Contains(pythonVersion, "3.6") {
				lib = "faiss-cpu==1.7.2" // Python 3.6最高支持版本
			} else if strings.Contains(pythonVersion, "3.7") {
				lib = "faiss-cpu==1.7.4" // Python 3.7支持版本
			} else {
				lib = "faiss-cpu" // 最新版本，适用于Python 3.8+
			}

			// 尝试安装CPU版本
			err = tryInstallWithMultipleMirrors(faissServiceDir, envPython, pipCmd, lib, mirrorSources, currentMirror, time.Minute)
			if err != nil {
				return fmt.Errorf("安装 %s 也失败: %v", lib, err)
			}
		} else if err != nil {
			return fmt.Errorf("安装 %s 失败: %v", lib, err)
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

// 包名别名映射表，用于处理包名大小写和连字符差异
var packageAliases = map[string][]string{
	"flask_cors":  {"flask-cors", "Flask-Cors"},
	"apscheduler": {"APScheduler"},
	"faiss-cpu":   {"faiss"},
	"faiss-gpu":   {"faiss"},
}

// isPackageInstalled 使用pip show命令精确检查包是否已安装
func isPackageInstalled(envPython, packageName string) bool {
	// 在Windows下使用特定的函数来隐藏cmd窗口
	if runtime.GOOS == "windows" {
		return isPackageInstalledWindows(envPython, packageName)
	}

	cmd := exec.Command(envPython, "-m", "pip", "show", packageName)
	err := cmd.Run()
	return err == nil
}

// isPackageInstalledWithAliases 检查包是否已安装，包括检查别名
func isPackageInstalledWithAliases(envPython, packageName string) bool {
	// 提取包名（去除版本号）
	cleanPackageName := strings.Split(packageName, "==")[0]
	cleanPackageName = strings.Split(cleanPackageName, ">=")[0]

	// 首先检查原始包名
	if isPackageInstalled(envPython, cleanPackageName) {
		return true
	}

	// 检查别名
	if aliases, exists := packageAliases[cleanPackageName]; exists {
		for _, alias := range aliases {
			if isPackageInstalled(envPython, alias) {
				return true
			}
		}
	}

	return false
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
	// 跳过一些常见的不需要分析的目录
	skipDirs := []string{"/node_modules/", "mini.js", "/out/", "/dist/", "/build/", "/public/", "/.git/", "/.gitgo/"}
	for _, skipDir := range skipDirs {
		if strings.Contains(path, skipDir) {
			logs.Warnf("exclude path: %s, skipDir: %s", path, skipDir)
			return true
		}
	}
	if len(exclude) != 0 {
		//读取excludeJson中key=exclude的字符串数组判断path是否包含在内
		for _, exc := range exclude {
			if strings.Contains(path, exc.(string)) {
				logs.Warnf("exclude path: %s, exc: %s", path, exc)
				return true
			}
		}
	}
	return false
}

// tryInstallWithMultipleMirrors 尝试使用多个镜像源安装包
func tryInstallWithMultipleMirrors(faissServiceDir, envPython, pipCmd, lib string, mirrorSources []string, currentMirror string, timeout time.Duration) error {
	// 首先尝试当前配置的镜像源
	if currentMirror != "" {
		log.Printf("尝试使用当前镜像源安装 %s: %s", lib, currentMirror)
		installArgs := []string{"-m", pipCmd, "install", "-i", currentMirror, lib}
		out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
		if err == nil {
			log.Printf("使用镜像源 %s 成功安装 %s", currentMirror, lib)
			return nil
		}
		log.Printf("镜像源 %s 安装失败: %v\n%s", currentMirror, err, string(out))
	}

	// 尝试其他镜像源
	for i, mirror := range mirrorSources {
		if mirror == currentMirror {
			continue // 跳过已经尝试过的镜像源
		}

		log.Printf("尝试使用镜像源 %d/%d 安装 %s: %s", i+1, len(mirrorSources), lib, mirror)
		installArgs := []string{"-m", pipCmd, "install", "-i", mirror, lib}
		out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
		if err == nil {
			log.Printf("使用镜像源 %s 成功安装 %s", mirror, lib)
			return nil
		}
		log.Printf("镜像源 %s 安装失败: %v\n%s", mirror, err, string(out))
	}

	// 最后尝试官方源（不使用-i参数）
	log.Printf("尝试使用官方源安装 %s", lib)
	installArgs := []string{"-m", pipCmd, "install", lib}
	out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
	if err == nil {
		log.Printf("使用官方源成功安装 %s", lib)
		return nil
	}

	return fmt.Errorf("所有镜像源都无法安装 %s，最后错误: %v\n%s", lib, err, string(out))
}
