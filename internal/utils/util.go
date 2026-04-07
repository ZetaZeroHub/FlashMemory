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
		return fmt.Errorf("Failed to create pip configuration directory: %v", err)
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
		return fmt.Errorf("Failed to write pip configuration file: %v", err)
	}

	log.Printf("pip configuration file created: %s", pipConfigFile)
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
		return buf.Bytes(), fmt.Errorf("Command %s %v failed: %w, output:\n%s", name, args, err, buf.String())
	}
	return buf.Bytes(), nil
}

// 检查Python环境和必要的库是否已安装，如果缺少则自动创建.env虚拟环境并安装
func CheckPythonEnvironment(faissType, faissServiceDir string) error {
	// 加载配置以获取pip镜像源路径
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: Unable to load configuration file, default pip source will be used: %v", err)
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
		log.Printf("Use the configured pip mirror source: %s", currentMirror)
		pipInstallArgs = []string{"-i", currentMirror}
	} else {
		currentMirror = mirrorSources[0] // 默认使用官方源
		log.Println("The pip mirror source is not configured, use the official source")
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
			return fmt.Errorf("Python is not installed or cannot be accessed: %v", err)
		}
		pythonCmd = "python"
		pipCmd = "pip"
	}
	log.Printf("Python version detected: %s", strings.TrimSpace(string(output)))

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
			log.Println(".env virtual environment already exists")
		}
	}

	// 如果虚拟环境不存在，创建它
	if !envExists {
		log.Println("Creating .env virtual environment... (max 1 minute)")
		if _, err := runCmdContext(faissServiceDir, pythonCmd, []string{"-m", "venv", ".env"}, time.Minute); err != nil {
			return fmt.Errorf("Failed to create .env virtual environment: %v", err)
		}
		log.Println(".env virtual environment created successfully")
	}

	// 配置pip镜像源
	if cfg != nil && cfg.PipPath != "" {
		log.Printf("Configuring pip mirror source: %s", cfg.PipPath)
		if err := configurePipMirror(envDir, cfg.PipPath); err != nil {
			log.Printf("Warning: Failed to configure pip mirror source: %v", err)
			log.Println("Will continue to use command line parameters to specify the image source")
		} else {
			log.Println("pip mirror source configuration successful")
			// 清空pipInstallArgs，因为已经通过配置文件设置了镜像源
			pipInstallArgs = []string{}
		}
	}

	// 升级pip到最新版本
	log.Println("Upgrading pip to the latest version...")
	upgradeArgs := []string{"-m", pipCmd, "install"}
	upgradeArgs = append(upgradeArgs, pipInstallArgs...)
	upgradeArgs = append(upgradeArgs, "--upgrade", "pip")
	out, err := runCmdContext(faissServiceDir, envPython, upgradeArgs, 5*time.Minute)
	if err != nil {
		log.Printf("Warning: Failed to upgrade pip: %v\n%s", err, string(out))
		log.Println("Keep using the current pip version...")
	} else {
		log.Println("pip upgrade successful")
	}

	// 需要安装的基础库和Faiss库
	requiredLibs := []string{"flask", "numpy", "flask_cors", "psutil", "apscheduler"}

	// 根据Python版本选择合适的faiss-cpu版本
	var faissLib string
	if faissType == "gpu" {
		faissLib = "faiss-gpu"
		log.Println("The GPU version of Faiss (faiss-gpu) will be installed")
		log.Println("Note: faiss-gpu requires CUDA environment support, please ensure that CUDA is installed on your system")
		log.Println("If the installation fails, it will automatically try to install the CPU version")
	} else {
		// 检查Python版本并选择合适的faiss-cpu版本
		pythonVersion := strings.TrimSpace(string(output))
		if strings.Contains(pythonVersion, "3.6") {
			faissLib = "faiss-cpu==1.7.2" // Python 3.6最高支持版本
			log.Println("Python 3.6 detected, faiss-cpu==1.7.2 will be installed")
		} else if strings.Contains(pythonVersion, "3.7") {
			faissLib = "faiss-cpu==1.7.4" // Python 3.7支持版本
			log.Println("Python 3.7 detected, faiss-cpu==1.7.4 will be installed")
		} else {
			faissLib = "faiss-cpu" // 最新版本，适用于Python 3.8+
			log.Println("Python 3.8+ detected, latest version of faiss-cpu will be installed")
		}
		log.Println("The CPU version of Faiss (faiss-cpu) will be installed")
		log.Println("CPU version works on all systems, but performance may be lower than GPU version")
		log.Println("If your system has an NVIDIA GPU and CUDA environment, you can use the -faiss=gpu parameter for better performance")
	}
	requiredLibs = append(requiredLibs, faissLib)

	// 安装基础库
	for _, lib := range requiredLibs {
		// 使用改进的包检查逻辑
		if !isPackageInstalledWithAliases(envPython, lib) {
			log.Printf("Installing %s...", lib)
			// 构建完整的pip install命令参数
			installArgs := []string{"-m", pipCmd, "install"}
			installArgs = append(installArgs, pipInstallArgs...)
			installArgs = append(installArgs, lib)
			out, err := runCmdContext(faissServiceDir, envPython, installArgs, 10*time.Minute)
			if err != nil {
				return fmt.Errorf("Installation of %s failed: %v\n%s", lib, err, string(out))
			}
			log.Printf("%s installed successfully", lib)
		} else {
			// 提取包名用于日志显示
			packageName := strings.Split(lib, "==")[0]
			packageName = strings.Split(packageName, ">=")[0]
			log.Printf("%s is already installed in the virtual environment", packageName)
		}
	}

	// 安装Faiss库
	lib := faissLib
	// 使用改进的包检查逻辑检查Faiss是否已安装
	if !isPackageInstalledWithAliases(envPython, lib) {
		log.Printf("Installing %s...", lib)

		// 尝试安装faiss库，如果失败则尝试多个镜像源
		err := tryInstallWithMultipleMirrors(faissServiceDir, envPython, pipCmd, lib, mirrorSources, currentMirror, time.Minute)
		if err != nil && lib == "faiss-gpu" {
			log.Printf("Installation of %s failed: %v", lib, err)
			log.Println("Try installing the CPU version (faiss-cpu)...")

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
				return fmt.Errorf("Installation of %s also failed: %v", lib, err)
			}
		} else if err != nil {
			return fmt.Errorf("Installation of %s failed: %v", lib, err)
		}
		log.Printf("%s installed successfully", lib)
	} else {
		log.Println("faiss is installed in the virtual environment")
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
		return "", fmt.Errorf("Unable to get current source file path")
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
		logs.Warnf("Intercept JSON content~")
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
		log.Printf("Trying to install %s using the current image source: %s", lib, currentMirror)
		installArgs := []string{"-m", pipCmd, "install", "-i", currentMirror, lib}
		out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
		if err == nil {
			log.Printf("Successfully installed %s using mirror source %s", currentMirror, lib)
			return nil
		}
		log.Printf("Image source %s failed to install: %v\n%s", currentMirror, err, string(out))
	}

	// 尝试其他镜像源
	for i, mirror := range mirrorSources {
		if mirror == currentMirror {
			continue // 跳过已经尝试过的镜像源
		}

		log.Printf("Trying to install %s using mirror source %d/%d: %s", i+1, len(mirrorSources), lib, mirror)
		installArgs := []string{"-m", pipCmd, "install", "-i", mirror, lib}
		out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
		if err == nil {
			log.Printf("Successfully installed %s using mirror source %s", mirror, lib)
			return nil
		}
		log.Printf("Image source %s failed to install: %v\n%s", mirror, err, string(out))
	}

	// 最后尝试官方源（不使用-i参数）
	log.Printf("Try to install %s using official sources", lib)
	installArgs := []string{"-m", pipCmd, "install", lib}
	out, err := runCmdContext(faissServiceDir, envPython, installArgs, timeout)
	if err == nil {
		log.Printf("Successfully installed %s using official sources", lib)
		return nil
	}

	return fmt.Errorf("Unable to install %s from all image sources, final error: %v\n%s", lib, err, string(out))
}
