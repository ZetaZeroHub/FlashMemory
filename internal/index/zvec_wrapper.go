package index

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// activeZvecWrappers 维护当前进程内所有"已就绪"的 ZvecWrapper 实例。
// 用于在进程退出时(信号处理路径)统一释放，避免 zvec_bridge 子进程残留写
// 半截 RocksDB segment/MANIFEST，导致下次启动时 "segment path not found"。
//
// key 是 *ZvecWrapper，value 用 struct{} 节省空间（sync.Map 不允许 nil value）。
// 双重保险：每个 wrapper 自己的 Free() 也会 Delete，避免悬空引用。
var activeZvecWrappers sync.Map

// FreeAllActiveWrappers 释放当前进程内所有活跃的 Zvec wrapper。
// 设计为可在 SIGINT/SIGTERM 处理路径调用，幂等且 panic-safe。
// 返回实际释放的 wrapper 数量。
func FreeAllActiveWrappers() int {
	count := 0
	activeZvecWrappers.Range(func(key, _ interface{}) bool {
		zw, ok := key.(*ZvecWrapper)
		if !ok || zw == nil {
			activeZvecWrappers.Delete(key)
			return true
		}
		// 单个 wrapper.Free 失败/panic 不能拖垮其他 wrapper 的释放
		func() {
			defer func() {
				if r := recover(); r != nil {
					logs.Warnf("FreeAllActiveWrappers: wrapper.Free panicked: %v", r)
				}
			}()
			zw.Free()
		}()
		count++
		return true
	})
	return count
}

// ZvecWrapper 通过 subprocess stdin/stdout 调用 Python Zvec 引擎
// 实现 FaissWrapper 接口，可无缝替换 HTTPFaissWrapper / MemoryFaissWrapper
type ZvecWrapper struct {
	Dim            int
	CollectionPath string // .gitgo/zvec_collections
	PythonPath     string // Python 可执行文件路径

	Scores map[int]float32 // 搜索结果的分数缓存

	// subprocess 管理
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex // 保护 stdin/stdout 的并发访问

	// 向量缓存
	vectorCache  map[string][]float32
	cacheEnabled bool

	// 状态
	ready     bool
	dirtyFlag bool
}

// zvecRequest 发送给 Python Bridge 的 JSON-line 请求
type zvecRequest struct {
	Action string      `json:"action"`
	Params interface{} `json:"params"`
}

// zvecResponse Python Bridge 返回的 JSON-line 响应
type zvecResponse struct {
	Status  string                 `json:"status"`
	Data    map[string]interface{} `json:"data"`
	Message string                 `json:"message"`
}

// NewZvecWrapper 创建一个新的 Zvec 向量引擎封装
func NewZvecWrapper(dimension int, collectionPath string, pythonPath string) (*ZvecWrapper, error) {
	if pythonPath == "" {
		pythonPath = "python3"
	}

	zw := &ZvecWrapper{
		Dim:            dimension,
		CollectionPath: collectionPath,
		PythonPath:     pythonPath,
		Scores:         make(map[int]float32),
		vectorCache:    make(map[string][]float32),
		cacheEnabled:   true,
		dirtyFlag:      false,
	}

	// Start Python Bridge subprocess
	if err := zw.startBridge(); err != nil {
		return nil, fmt.Errorf("启动 Zvec Bridge 失败: %w", err)
	}

	// Initialize Collection
	if err := zw.initCollection(false); err != nil {
		// Second safety net: if init fails due to missing deps, auto-provision and retry
		if zw.isDepsMissingError(err) {
			logs.Warnf("[bridge] Init failed due to missing deps, attempting auto-provision and retry...")
			zw.Free()

			provisionedPython, provErr := zw.autoProvisionPythonEnv()
			if provErr != nil {
				return nil, fmt.Errorf("初始化失败(依赖缺失)且自动安装也失败: %w (原始错误: %v)", provErr, err)
			}

			// Re-create wrapper with provisioned venv
			zw = &ZvecWrapper{
				Dim:            dimension,
				CollectionPath: collectionPath,
				PythonPath:     provisionedPython,
				Scores:         make(map[int]float32),
				vectorCache:    make(map[string][]float32),
				cacheEnabled:   true,
				dirtyFlag:      false,
			}
			if retryErr := zw.startBridge(); retryErr != nil {
				return nil, fmt.Errorf("自动安装后重启 Bridge 失败: %w", retryErr)
			}
			if retryErr := zw.initCollection(false); retryErr != nil {
				zw.Free()
				return nil, fmt.Errorf("自动安装后初始化仍失败: %w", retryErr)
			}
			logs.Infof("ZvecWrapper 创建成功(自动安装后), dimension=%d, path=%s", dimension, collectionPath)
			activeZvecWrappers.Store(zw, struct{}{})
			return zw, nil
		}

		zw.Free()
		return nil, fmt.Errorf("初始化 Zvec Collection 失败: %w", err)
	}

	logs.Infof("ZvecWrapper 创建成功, dimension=%d, path=%s", dimension, collectionPath)
	activeZvecWrappers.Store(zw, struct{}{})
	return zw, nil
}

// startBridge launches the Python zvec_bridge subprocess using a multi-strategy approach:
//
//	Strategy 1: python3 -m flashmemory.zvec_bridge (pip-installed package)
//	Strategy 2: local zvec_bridge.py script (dev mode)
//	Strategy 2+deps: if Strategy 2 reports missing deps, auto-provision venv and retry
//	Strategy 3: auto-provision a venv with flashmemory[embedding], then retry Strategy 1
func (zw *ZvecWrapper) startBridge() error {
	// Strategy 1: Try module mode (pip installed globally or in active venv)
	logs.Infof("[bridge] Strategy 1: trying module mode (python3 -m flashmemory.zvec_bridge)")
	if err := zw.tryStartBridgeModule(zw.PythonPath); err == nil {
		return nil
	}

	// Strategy 2: Try local script path (dev/source mode)
	if script := zw.findBridgeScript(); script != "" {
		logs.Infof("[bridge] Strategy 2: trying local script: %s", script)
		err := zw.tryStartBridgeScript(zw.PythonPath, script)
		if err == nil {
			return nil
		}
		// Bridge started but dependencies missing -> auto-provision venv
		if errors.Is(err, ErrDepsMissing) {
			logs.Infof("[bridge] Strategy 2 detected missing deps, triggering auto-provision...")
			provisionedPython, provErr := zw.autoProvisionPythonEnv()
			if provErr != nil {
				return fmt.Errorf("auto-provision after deps-missing detection failed: %w", provErr)
			}
			logs.Infof("[bridge] Retrying with provisioned venv: %s", provisionedPython)
			retryErr := zw.tryStartBridgeModule(provisionedPython)
			if retryErr != nil {
				return fmt.Errorf("bridge failed even after auto-provision for missing deps: %w", retryErr)
			}
			return nil
		}
	}

	// Strategy 3: Check managed venv, auto-provision if needed
	venvPython := zw.getManagedVenvPython()
	if venvPython != "" {
		logs.Infof("[bridge] Strategy 3a: trying managed venv: %s", venvPython)
		if err := zw.tryStartBridgeModule(venvPython); err == nil {
			return nil
		}
		logs.Warnf("[bridge] Managed venv exists but bridge failed, will re-provision")
	}

	// Auto-provision: create venv and install flashmemory[embedding]
	logs.Infof("[bridge] Strategy 3b: auto-provisioning Python environment...")
	provisionedPython, err := zw.autoProvisionPythonEnv()
	if err != nil {
		return fmt.Errorf("all bridge strategies failed, auto-provision also failed: %w", err)
	}

	logs.Infof("[bridge] Retrying module mode with provisioned venv: %s", provisionedPython)
	if err := zw.tryStartBridgeModule(provisionedPython); err != nil {
		return fmt.Errorf("bridge failed even after auto-provision: %w", err)
	}

	return nil
}

// tryStartBridgeModule tries to start the bridge via `python -m flashmemory.zvec_bridge`
func (zw *ZvecWrapper) tryStartBridgeModule(pythonPath string) error {
	cmd := exec.Command(pythonPath, "-u", "-m", "flashmemory.zvec_bridge")
	cmd.Stderr = os.Stderr
	return zw.launchAndWaitReady(cmd)
}

// tryStartBridgeScript tries to start the bridge via a local .py script
func (zw *ZvecWrapper) tryStartBridgeScript(pythonPath string, scriptPath string) error {
	cmd := exec.Command(pythonPath, "-u", scriptPath)
	cmd.Stderr = os.Stderr
	return zw.launchAndWaitReady(cmd)
}

// launchAndWaitReady starts the subprocess command and waits for the "ready" JSON-line response.
// Returns nil on full readiness, ErrDepsMissing if bridge started but dependencies are absent,
// or other errors if the bridge fails to start.
var ErrDepsMissing = fmt.Errorf("bridge started but required Python dependencies are missing")

func (zw *ZvecWrapper) launchAndWaitReady(cmd *exec.Cmd) error {
	var err error
	zw.stdin, err = cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	zw.stdout = bufio.NewScanner(stdoutPipe)
	zw.stdout.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Python process: %w", err)
	}
	zw.cmd = cmd

	resp, err := zw.readResponse()
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		zw.cmd = nil
		return fmt.Errorf("bridge did not become ready: %w", err)
	}

	switch {
	case resp.Status == "success" && resp.Message == "ready":
		zw.ready = true
		logs.Infof("[bridge] Zvec Bridge is ready (pid=%d)", cmd.Process.Pid)
		return nil

	case resp.Status == "success" && resp.Message == "ready_with_deps_missing":
		missingPkgs := "unknown"
		if mp, ok := resp.Data["missing_packages"]; ok {
			if arr, ok := mp.([]interface{}); ok {
				parts := make([]string, len(arr))
				for i, v := range arr {
					parts[i] = fmt.Sprintf("%v", v)
				}
				missingPkgs = fmt.Sprintf("%v", parts)
			}
		}
		logs.Warnf("[bridge] Bridge started but dependencies missing: %s", missingPkgs)
		cmd.Process.Kill()
		cmd.Wait()
		zw.cmd = nil
		return ErrDepsMissing

	default:
		cmd.Process.Kill()
		cmd.Wait()
		zw.cmd = nil
		return fmt.Errorf("bridge returned unexpected status: %s", resp.Message)
	}
}

// getManagedVenvDir returns the path to the managed venv directory (~/.flashmemory/pyenv)
func (zw *ZvecWrapper) getManagedVenvDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".flashmemory", "pyenv")
}

// getManagedVenvPython returns the python binary path inside the managed venv, or "" if not exists
func (zw *ZvecWrapper) getManagedVenvPython() string {
	venvDir := zw.getManagedVenvDir()
	if venvDir == "" {
		return ""
	}

	// Platform-aware python path
	var pythonBin string
	if _, err := os.Stat(filepath.Join(venvDir, "bin", "python3")); err == nil {
		pythonBin = filepath.Join(venvDir, "bin", "python3")
	} else if _, err := os.Stat(filepath.Join(venvDir, "Scripts", "python.exe")); err == nil {
		pythonBin = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	return pythonBin
}

// provisionCooldownFile is the marker file for auto-provision cooldown.
// If this file exists and is recent (within 5 minutes), auto-provision is skipped
// to prevent infinite retry loops.
const provisionCooldownFile = ".flashmemory_provision_in_progress"
const provisionCooldownDuration = 5 * time.Minute

// autoProvisionPythonEnv creates a managed venv at ~/.flashmemory/pyenv and installs flashmemory[zvec]
func (zw *ZvecWrapper) autoProvisionPythonEnv() (string, error) {
	venvDir := zw.getManagedVenvDir()
	if venvDir == "" {
		return "", fmt.Errorf("cannot determine home directory for managed venv")
	}

	// Cooldown check: prevent infinite retry loops from repeated HTTP requests
	homeDir, _ := os.UserHomeDir()
	cooldownPath := filepath.Join(homeDir, provisionCooldownFile)
	if info, err := os.Stat(cooldownPath); err == nil {
		if time.Since(info.ModTime()) < provisionCooldownDuration {
			return "", fmt.Errorf("auto-provision cooldown active (last attempt %v ago, retry after %v). If stuck, delete %s or install zvec manually",
				time.Since(info.ModTime()), provisionCooldownDuration, cooldownPath)
		}
	}

	// Set cooldown marker
	os.WriteFile(cooldownPath, []byte(time.Now().Format(time.RFC3339)), 0644)

	logs.Infof("[provision] Creating managed Python environment at: %s", venvDir)
	fmt.Fprintf(os.Stderr, "\n⚡ FlashMemory: Setting up Zvec Python environment (first-time only)...\n")

	// Step 1: Find a working python3
	pythonPath := zw.findSystemPython()
	if pythonPath == "" {
		return "", fmt.Errorf("python3 not found on system, please install Python 3.8+ first, or set FM_PYTHON env var")
	}
	logs.Infof("[provision] Using system Python: %s", pythonPath)

	// Step 2: Create venv (remove old if exists)
	os.RemoveAll(venvDir)
	os.MkdirAll(filepath.Dir(venvDir), 0755)

	createCmd := exec.Command(pythonPath, "-m", "venv", venvDir)
	createCmd.Stderr = os.Stderr
	createCmd.Stdout = os.Stdout
	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create venv with %s: %w (try setting FM_PYTHON to a working python3)", pythonPath, err)
	}
	logs.Infof("[provision] Venv created successfully")

	// Step 3: Get venv python path and verify it actually works
	venvPython := zw.getManagedVenvPython()
	if venvPython == "" {
		return "", fmt.Errorf("venv created but python binary not found in %s", venvDir)
	}
	// Verify the venv python binary is executable
	verifyCmd := exec.Command(venvPython, "-c", "import sys; print(sys.version_info)")
	verifyCmd.Stderr = os.Stderr
	if out, err := verifyCmd.Output(); err != nil {
		return "", fmt.Errorf("venv python at %s is not functional: %w (try setting FM_PYTHON env var)", venvPython, err)
	} else {
		logs.Infof("[provision] Venv python verified: %s", strings.TrimSpace(string(out)))
	}

	// Step 4: Upgrade pip (quiet)
	upgradeCmd := exec.Command(venvPython, "-m", "pip", "install", "--upgrade", "pip", "-q")
	upgradeCmd.Stderr = os.Stderr
	upgradeCmd.Run() // best-effort, don't fail on this

	// Step 5: Install flashmemory[zvec] — prefer local pip-package/ if available (dev mode)
	localPipPackage := zw.findLocalPipPackage()
	if localPipPackage != "" {
		fmt.Fprintf(os.Stderr, "📦 Installing flashmemory[zvec] from local source: %s\n", localPipPackage)
		logs.Infof("[provision] Using local pip-package: %s", localPipPackage)
		// Note: shell quoting needed for extras with -e flag
		installCmd := exec.Command(venvPython, "-m", "pip", "install", "-e",
			localPipPackage+"[zvec]",
			"-q",
		)
		installCmd.Stderr = os.Stderr
		installCmd.Stdout = os.Stdout
		if err := installCmd.Run(); err != nil {
			logs.Warnf("[provision] Local pip-package[zvec] install failed: %v, trying without extras", err)
			// Retry: install base package first, then zvec separately
			baseCmd := exec.Command(venvPython, "-m", "pip", "install", "-e", localPipPackage, "-q")
			baseCmd.Stderr = os.Stderr
			baseCmd.Stdout = os.Stdout
			if baseErr := baseCmd.Run(); baseErr != nil {
				logs.Warnf("[provision] Local pip-package base install also failed: %v, falling back to PyPI", baseErr)
			} else {
				// Install zvec extras separately
				zvecCmd := exec.Command(venvPython, "-m", "pip", "install", "zvec>=0.1.1", "rank_bm25", "jieba", "dashtext", "-q")
				zvecCmd.Stderr = os.Stderr
				zvecCmd.Stdout = os.Stdout
				if zvecErr := zvecCmd.Run(); zvecErr != nil {
					logs.Warnf("[provision] zvec extras install failed: %v", zvecErr)
				}
				fmt.Fprintf(os.Stderr, "✅ Zvec Python environment ready (local dev mode)!\n\n")
				logs.Infof("[provision] flashmemory + zvec installed from local source")
				return venvPython, nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "✅ Zvec Python environment ready (local dev mode)!\n\n")
			logs.Infof("[provision] flashmemory[zvec] installed from local source")
			return venvPython, nil
		}
	}

	// Step 6: Install from PyPI
	fmt.Fprintf(os.Stderr, "📦 Installing flashmemory[zvec] from PyPI...\n")
	installCmd := exec.Command(venvPython, "-m", "pip", "install", "flashmemory[zvec]", "-q")
	installCmd.Stderr = os.Stderr
	installCmd.Stdout = os.Stdout
	if err := installCmd.Run(); err != nil {
		// Fallback: try [embedding] extras (older name)
		logs.Warnf("[provision] flashmemory[zvec] install failed, trying [embedding]")
		fallbackCmd := exec.Command(venvPython, "-m", "pip", "install", "flashmemory[embedding]", "-q")
		fallbackCmd.Stderr = os.Stderr
		fallbackCmd.Stdout = os.Stdout
		if err2 := fallbackCmd.Run(); err2 != nil {
			// Final fallback: base package only
			logs.Warnf("[provision] flashmemory[embedding] also failed, trying base package")
			baseCmd := exec.Command(venvPython, "-m", "pip", "install", "flashmemory", "-q")
			baseCmd.Stderr = os.Stderr
			baseCmd.Stdout = os.Stdout
			if err3 := baseCmd.Run(); err3 != nil {
				return "", fmt.Errorf("pip install flashmemory failed: %w (original: %v)", err3, err)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "✅ Zvec Python environment ready!\n\n")
	logs.Infof("[provision] flashmemory installed successfully in managed venv")

	// Remove cooldown marker on success
	os.Remove(cooldownPath)

	return venvPython, nil
}

// findSystemPython locates a usable python3 binary on the system.
// Priority: FM_PYTHON env > conda > homebrew > system PATH.
// macOS system python (/Library/Developer/CommandLineTools/) is deprioritized
// because it often lacks venv/pip support.
func (zw *ZvecWrapper) findSystemPython() string {
	// Highest priority: FM_PYTHON environment variable
	if fmPython := os.Getenv("FM_PYTHON"); fmPython != "" {
		if _, err := os.Stat(fmPython); err == nil {
			logs.Infof("[provision] Using FM_PYTHON: %s", fmPython)
			return fmPython
		}
		logs.Warnf("[provision] FM_PYTHON=%s not found, falling back to auto-detect", fmPython)
	}

	// Collect all python3 candidates with quality scoring
	type pythonCandidate struct {
		path  string
		score int // higher is better
	}
	var candidates []pythonCandidate

	// Scan PATH for all python3/python binaries
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		absPath, _ := filepath.Abs(path)
		score := 0

		// Score: prefer conda, homebrew; deprioritize macOS system python
		switch {
		case strings.Contains(absPath, "miniconda") || strings.Contains(absPath, "anaconda"):
			score = 100
		case strings.Contains(absPath, "homebrew") || strings.Contains(absPath, "/opt/homebrew"):
			score = 90
		case strings.Contains(absPath, "/usr/local/bin"):
			score = 80
		case strings.Contains(absPath, "CommandLineTools"):
			score = 10 // macOS system python — often broken for venv
		default:
			score = 50
		}

		// Verify it's Python 3.x and can create venvs
		out, err := exec.Command(absPath, "-c", "import sys; print(sys.version_info.major)").Output()
		if err != nil || len(out) == 0 || out[0] != '3' {
			continue
		}

		candidates = append(candidates, pythonCandidate{path: absPath, score: score})
	}

	if len(candidates) == 0 {
		return ""
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	chosen := candidates[0]
	if chosen.score <= 10 {
		logs.Warnf("[provision] Only macOS system python found (%s), venv creation may fail. Consider installing Python via conda or homebrew, or set FM_PYTHON env var.", chosen.path)
	}

	logs.Infof("[provision] Selected Python: %s (score=%d)", chosen.path, chosen.score)
	return chosen.path
}

// findBridgeScript locates the zvec_bridge.py script for dev/source mode
func (zw *ZvecWrapper) findBridgeScript() string {
	candidates := []string{}

	// Next to the executable binary
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exePath), "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// Relative path (source tree)
	candidates = append(candidates,
		filepath.Join("pip-package", "flashmemory", "zvec_bridge.py"),
	)

	for _, p := range candidates {
		if absPath, err := filepath.Abs(p); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				logs.Infof("[bridge] Found zvec_bridge.py: %s", absPath)
				return absPath
			}
		}
	}

	return ""
}

// findLocalPipPackage locates the local pip-package/ directory for dev-mode installation.
// Returns the directory containing pyproject.toml, or "" if not found.
func (zw *ZvecWrapper) findLocalPipPackage() string {
	candidates := []string{}

	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exePath), "pip-package"),
		)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pip-package"),
		)
	}

	candidates = append(candidates, "pip-package")

	for _, p := range candidates {
		if absPath, err := filepath.Abs(p); err == nil {
			pyprojectPath := filepath.Join(absPath, "pyproject.toml")
			if _, err := os.Stat(pyprojectPath); err == nil {
				logs.Infof("[provision] Found local pip-package: %s", absPath)
				return absPath
			}
		}
	}

	return ""
}

// writeShutdownLocked 在已持有 zw.mu 的前提下，把 shutdown 请求写入 stdin。
// 不读响应（桥进程被 SIGTERM 后可能不返回），仅尽力让 Python 端走优雅退出
// 路径触发 atexit 的 flush+close。
// 调用方必须已经 Lock(zw.mu)，否则破坏 sendRequest 的并发不变量。
func (zw *ZvecWrapper) writeShutdownLocked() error {
	if zw.stdin == nil {
		return nil
	}
	req := zvecRequest{Action: "shutdown", Params: map[string]interface{}{}}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = zw.stdin.Write(append(reqBytes, '\n'))
	return err
}

// sendRequest 向 Python Bridge 发送请求并读取响应
func (zw *ZvecWrapper) sendRequest(action string, params interface{}) (*zvecResponse, error) {
	zw.mu.Lock()
	defer zw.mu.Unlock()

	if !zw.ready {
		return nil, fmt.Errorf("Zvec Bridge 未就绪")
	}

	req := zvecRequest{
		Action: action,
		Params: params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 写入 stdin
	if _, err := zw.stdin.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("写入 stdin 失败: %w", err)
	}

	// 读取响应
	return zw.readResponse()
}

// readResponse 从 stdout 读取一行 JSON 响应
func (zw *ZvecWrapper) readResponse() (*zvecResponse, error) {
	// 设置读取超时 (通过 context 或简单轮询)
	done := make(chan bool, 1)
	var resp zvecResponse
	var scanErr error

	go func() {
		if zw.stdout.Scan() {
			line := zw.stdout.Text()
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				scanErr = fmt.Errorf("解析响应 JSON 失败: %w, 原始数据: %s", err, line)
			}
		} else {
			if err := zw.stdout.Err(); err != nil {
				scanErr = fmt.Errorf("读取 stdout 失败: %w", err)
			} else {
				scanErr = fmt.Errorf("Bridge 进程已退出")
			}
		}
		done <- true
	}()

	select {
	case <-done:
		if scanErr != nil {
			return nil, scanErr
		}
		return &resp, nil
	case <-time.After(600 * time.Second):
		return nil, fmt.Errorf("读取响应超时 (600s)")
	}
}

// isDepsMissingError checks if an error is caused by missing Python dependencies (zvec etc.)
func (zw *ZvecWrapper) isDepsMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "zvec 未安装") ||
		strings.Contains(msg, "No module named 'zvec'") ||
		strings.Contains(msg, "No module named zvec") ||
		strings.Contains(msg, "ImportError")
}

// initCollection 初始化 Zvec Collection
func (zw *ZvecWrapper) initCollection(forceNew bool) error {
	resp, err := zw.sendRequest("init", map[string]interface{}{
		"collection_path": zw.CollectionPath,
		"dimension":       zw.Dim,
		"force_new":       forceNew,
		"collection_type": "both", // 初始化函数级和模块级
	})
	if err != nil {
		return err
	}
	if resp.Status != "success" {
		return fmt.Errorf("初始化失败: %s", resp.Message)
	}
	logs.Infof("Zvec Collection 初始化成功: %s", resp.Message)
	return nil
}

// --- FaissWrapper 接口实现 ---

// Dimension 返回向量维度
func (zw *ZvecWrapper) Dimension() int {
	return zw.Dim
}

// GetScore 返回搜索结果中指定函数ID的分数
func (zw *ZvecWrapper) GetScore(funcID int) float32 {
	score, ok := zw.Scores[funcID]
	if !ok {
		return 0
	}
	return score
}

// AddFunctionVector 添加带有完整元数据的单个向量到全量引擎索引中
// Metadata必须包含 func_name 和 description 等字段供引擎建立 Sparse BM25 索引。
func (zw *ZvecWrapper) AddFunctionVector(funcID int, vector []float32, metadata map[string]interface{}) error {
	zw.dirtyFlag = true

	// 截断或填充向量到正确维度
	if len(vector) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, vector)
		vector = resized
	}

	funcIdStr := fmt.Sprintf("func_%d", funcID)

	resp, err := zw.sendRequest("add_vector", map[string]interface{}{
		"func_id":  funcIdStr,
		"vector":   vector,
		"metadata": metadata,
	})
	if err != nil {
		return fmt.Errorf("添加向量失败 (id=%d): %w", funcID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加向量失败: %s", resp.Message)
	}

	return nil
}

// AddVector 添加单个向量到索引
func (zw *ZvecWrapper) AddVector(funcID int, vector []float32) error {
	zw.dirtyFlag = true

	// 截断或填充向量到正确维度
	if len(vector) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, vector)
		vector = resized
	}

	funcIdStr := fmt.Sprintf("func_%d", funcID)

	resp, err := zw.sendRequest("add_vector", map[string]interface{}{
		"func_id":  funcIdStr,
		"vector":   vector,
		"metadata": map[string]interface{}{},
	})
	if err != nil {
		return fmt.Errorf("添加向量失败 (id=%d): %w", funcID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加向量失败: %s", resp.Message)
	}

	return nil
}

// AddModuleVector 添加单个模块的向量到索引
func (zw *ZvecWrapper) AddModuleVector(modID int, vector []float32) error {
	zw.dirtyFlag = true

	// 截断或填充向量到正确维度
	if len(vector) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, vector)
		vector = resized
	}

	modIdStr := fmt.Sprintf("mod_%d", modID)

	resp, err := zw.sendRequest("add_module_vector", map[string]interface{}{
		"module_id": modIdStr,
		"vector":    vector,
		"metadata":  map[string]interface{}{},
	})
	if err != nil {
		return fmt.Errorf("添加模块向量失败 (id=%d): %w", modID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加模块向量失败: %s", resp.Message)
	}

	return nil
}

// AddVectorsBatch 批量添加向量
func (zw *ZvecWrapper) AddVectorsBatch(funcIDs []int, vectors []float32) error {
	zw.dirtyFlag = true

	items := make([]map[string]interface{}, len(funcIDs))
	for i, id := range funcIDs {
		vec := vectors[i*zw.Dim : (i+1)*zw.Dim]
		vecCopy := make([]float64, len(vec))
		for j, v := range vec {
			vecCopy[j] = float64(v)
		}
		items[i] = map[string]interface{}{
			"func_id":  fmt.Sprintf("func_%d", id),
			"vector":   vecCopy,
			"metadata": map[string]interface{}{},
		}
	}

	resp, err := zw.sendRequest("add_vectors_batch", map[string]interface{}{
		"items": items,
	})
	if err != nil {
		return fmt.Errorf("批量添加向量失败: %w", err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("批量添加向量失败: %s", resp.Message)
	}

	return nil
}

// SearchVectors 搜索最相似的 topK 个向量
func (zw *ZvecWrapper) SearchVectors(query []float32, topK int) []int {
	// 确保查询向量维度正确
	if len(query) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, query)
		query = resized
	}

	// 将 float32 转换为 float64 以便 JSON 序列化
	queryF64 := make([]float64, len(query))
	for i, v := range query {
		queryF64[i] = float64(v)
	}

	resp, err := zw.sendRequest("search", map[string]interface{}{
		"query":           queryF64,
		"top_k":           topK,
		"collection_type": "functions",
	})
	if err != nil {
		logs.Errorf("Zvec search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec search error: %s", resp.Message)
		return []int{}
	}

	return zw.parseSearchResults(resp, "func_")
}

// SearchVectorsWithFilter performs vector search with scalar filter expression
func (zw *ZvecWrapper) SearchVectorsWithFilter(query []float32, topK int, filter string) []int {
	if len(query) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, query)
		query = resized
	}

	queryF64 := make([]float64, len(query))
	for i, v := range query {
		queryF64[i] = float64(v)
	}

	params := map[string]interface{}{
		"query":           queryF64,
		"top_k":           topK,
		"collection_type": "functions",
	}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := zw.sendRequest("search", params)
	if err != nil {
		logs.Errorf("Zvec filtered search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec filtered search error: %s", resp.Message)
		return []int{}
	}

	return zw.parseSearchResults(resp, "func_")
}

// SearchModuleVectors finds the topK module vectors closest to the query vector
func (zw *ZvecWrapper) SearchModuleVectors(query []float32, topK int) []int {
	return zw.hybridSearchVectorsInternal(query, nil, topK, "", "modules", "mod_", "", false)
}

// HybridSearchVectors performs Dense + Sparse multi-vector search with RRF fusion.
// queryText is the original search text for auto BM25 sparse generation and reranker.
func (zw *ZvecWrapper) HybridSearchVectors(denseQuery []float32, sparseQuery map[string]float64, topK int, filter string, queryText string, enableReranker bool) []int {
	return zw.hybridSearchVectorsInternal(denseQuery, sparseQuery, topK, filter, "functions", "func_", queryText, enableReranker)
}

func (zw *ZvecWrapper) hybridSearchVectorsInternal(denseQuery []float32, sparseQuery map[string]float64, topK int, filter string, collectionType string, idPrefix string, queryText string, enableReranker bool) []int {
	if len(denseQuery) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, denseQuery)
		denseQuery = resized
	}

	denseF64 := make([]float64, len(denseQuery))
	for i, v := range denseQuery {
		denseF64[i] = float64(v)
	}

	params := map[string]interface{}{
		"dense_query":     denseF64,
		"top_k":           topK,
		"use_rrf":         true,
		"collection_type": collectionType,
		"enable_reranker": enableReranker,
	}
	// Pass original query text for auto BM25 sparse generation and cross-encoder reranking
	if queryText != "" {
		params["query_text"] = queryText
	}
	if sparseQuery != nil && len(sparseQuery) > 0 {
		params["sparse_query"] = sparseQuery
	}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := zw.sendRequest("hybrid_search", params)
	if err != nil {
		logs.Errorf("Zvec hybrid search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec hybrid search error: %s", resp.Message)
		return []int{}
	}

	// Log search type for debugging
	if searchType, ok := resp.Data["search_type"].(string); ok {
		logs.Infof("Hybrid search completed, type=%s, collection=%s, reranker=%v", searchType, collectionType, enableReranker)
	}

	return zw.parseSearchResults(resp, idPrefix)
}

// parseSearchResults extracts IDs from a zvec search response
func (zw *ZvecWrapper) parseSearchResults(resp *zvecResponse, idPrefix string) []int {
	resultsRaw, ok := resp.Data["results"].([]interface{})
	if !ok {
		logs.Errorf("Invalid search result format")
		return []int{}
	}

	type idScorePair struct {
		id    int
		score float32
	}
	var pairs []idScorePair

	for _, item := range resultsRaw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		idStr, _ := itemMap["id"].(string)
		score, _ := itemMap["score"].(float64)
		var objID int
		fmt.Sscanf(idStr, idPrefix+"%d", &objID)

		if objID > 0 {
			zw.Scores[objID] = float32(score)
			pairs = append(pairs, idScorePair{id: objID, score: float32(score)})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	results := make([]int, 0, len(pairs))
	for _, p := range pairs {
		results = append(results, p.id)
	}

	return results
}

// SaveToFile 保存索引 (Zvec 自动持久化，这里触发 optimize)
func (zw *ZvecWrapper) SaveToFile(path string) error {
	if !zw.dirtyFlag {
		logs.Infof("Zvec 索引未修改，跳过保存")
		return nil
	}

	resp, err := zw.sendRequest("optimize", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("优化索引失败: %w", err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("优化索引失败: %s", resp.Message)
	}

	zw.dirtyFlag = false
	logs.Infof("Zvec 索引已优化并持久化")
	return nil
}

// LoadFromFile 加载索引 (Zvec 通过 open 自动加载，这里仅做兼容)
func (zw *ZvecWrapper) LoadFromFile(path string) error {
	logs.Infof("ZvecWrapper: Collection 通过 init_collection 自动加载，LoadFromFile 为兼容接口")
	return nil
}

// SetSimilarityMetric 设置相似度计算方法 (Zvec 使用 HNSW+Cosine，此方法为兼容)
func (zw *ZvecWrapper) SetSimilarityMetric(metric string) {
	logs.Infof("ZvecWrapper: 相似度计算由 HNSW 索引决定，SetSimilarityMetric(%s) 为兼容接口", metric)
}

// EnableCache 启用向量缓存
func (zw *ZvecWrapper) EnableCache() {
	zw.cacheEnabled = true
}

// DisableCache 禁用向量缓存
func (zw *ZvecWrapper) DisableCache() {
	zw.cacheEnabled = false
}

// ClearCache 清除向量缓存
func (zw *ZvecWrapper) ClearCache() {
	zw.vectorCache = make(map[string][]float32)
}

// GetCacheStats 获取缓存统计
func (zw *ZvecWrapper) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":    zw.cacheEnabled,
		"cache_size": len(zw.vectorCache),
		"dirty":      zw.dirtyFlag,
		"engine":     "zvec",
	}
}

// Free 释放资源，关闭 Python 子进程。幂等且并发安全：
// 多次调用 / 多 goroutine 同时调用都会被 mu 串行化，
// 第二次进入时所有字段已清，立即返回。
func (zw *ZvecWrapper) Free() {
	// 一进入就反注册，避免 FreeAllActiveWrappers 与正常 defer 路径双重 Free。
	// Delete 必须在 Lock 之前——signal 处理路径可能与某个慢 sendRequest 抢锁，
	// 我们想让 sendRequest 先完成、再让本 Free 取锁清理；但注册表必须先脱钩，
	// 否则 FreeAllActiveWrappers 与本 Free 都可能等同一把锁。
	activeZvecWrappers.Delete(zw)

	zw.mu.Lock()
	defer zw.mu.Unlock()

	if zw.ready {
		// sendRequest 内部也会取 zw.mu（不可重入），所以这里不能直接调
		// sendRequest——改用直接写 stdin 的简化路径，丢弃响应即可。
		_ = zw.writeShutdownLocked()
		zw.ready = false
	}

	if zw.stdin != nil {
		zw.stdin.Close()
		zw.stdin = nil
	}

	if zw.cmd != nil && zw.cmd.Process != nil {
		// Send SIGTERM first for graceful shutdown (allows zvec to release LOCK files)
		if err := zw.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			logs.Warnf("[bridge] Failed to send SIGTERM: %v", err)
		}

		done := make(chan error, 1)
		go func() {
			done <- zw.cmd.Wait()
		}()

		select {
		case <-done:
			logs.Infof("Zvec Bridge 进程已正常退出")
		case <-time.After(3 * time.Second):
			logs.Warnf("[bridge] Bridge did not exit after SIGTERM within 3s, sending SIGKILL")
			zw.cmd.Process.Kill()
			<-done
		}
		zw.cmd = nil
	}
}
