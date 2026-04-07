package back

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// FaissMonitor 负责监控 Faiss 服务的健康状态并在必要时重启服务
type FaissMonitor struct {
	process        *os.Process   // 当前运行的 Faiss 进程
	serviceDir     string        // Faiss 服务目录
	checkInterval  time.Duration // 检查间隔时间
	maxRetries     int           // 重启前的最大重试次数
	retryInterval  time.Duration // 重试间隔时间
	healthEndpoint string        // 健康检查端点
	running        bool          // 监控器是否在运行
	mu             sync.Mutex    // 互斥锁，保护进程重启操作
	stopChan       chan struct{} // 停止监控的通道
}

// NewFaissMonitor 创建一个新的 Faiss 服务监控器
func NewFaissMonitor(process *os.Process, serviceDir string) *FaissMonitor {
	return &FaissMonitor{
		process:        process,
		serviceDir:     serviceDir,
		checkInterval:  5 * time.Second, // 默认每15秒检查一次
		maxRetries:     2,               // 默认重试2次
		retryInterval:  2 * time.Second, // 默认重试间隔3秒
		healthEndpoint: index.DefaultFaissServerURL + "/health",
		running:        false,
		stopChan:       make(chan struct{}),
	}
}

// SetCheckInterval 设置健康检查间隔
func (fm *FaissMonitor) SetCheckInterval(interval time.Duration) {
	fm.checkInterval = interval
}

// SetMaxRetries 设置最大重试次数
func (fm *FaissMonitor) SetMaxRetries(retries int) {
	fm.maxRetries = retries
}

// SetRetryInterval 设置重试间隔
func (fm *FaissMonitor) SetRetryInterval(interval time.Duration) {
	fm.retryInterval = interval
}

// Start 启动 Faiss 服务监控
func (fm *FaissMonitor) Start() {
	fm.mu.Lock()
	if fm.running {
		fm.mu.Unlock()
		return
	}
	fm.running = true
	fm.mu.Unlock()

	go fm.monitorLoop()
	logs.Infof("Faiss service monitoring has been started, check interval: %v", fm.checkInterval)
}

// Stop 停止 Faiss 服务监控
func (fm *FaissMonitor) Stop() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if !fm.running {
		return
	}

	close(fm.stopChan)
	fm.running = false
	logs.Infof("Faiss service monitoring has stopped")
}

// UpdateProcess 更新当前监控的进程
func (fm *FaissMonitor) UpdateProcess(process *os.Process) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.process = process
}

// monitorLoop 监控循环，定期检查 Faiss 服务健康状态
func (fm *FaissMonitor) monitorLoop() {
	ticker := time.NewTicker(fm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !fm.checkHealth() {
				fm.restartService()
			}
		case <-fm.stopChan:
			return
		}
	}
}

// checkHealth 检查 Faiss 服务是否健康
func (fm *FaissMonitor) checkHealth() bool {
	for i := 0; i < fm.maxRetries; i++ {
		resp, err := http.Get(fm.healthEndpoint)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true
		}

		if err != nil {
			logs.Warnf("Faiss service health check failed (attempt %d/%d): %v", i+1, fm.maxRetries, err)
		} else {
			logs.Warnf("Faiss service returned abnormal status code (try %d/%d): %d", i+1, fm.maxRetries, resp.StatusCode)
			resp.Body.Close()
		}

		if i < fm.maxRetries-1 {
			time.Sleep(fm.retryInterval)
		}
	}

	return false
}

// restartService 重启 Faiss 服务
func (fm *FaissMonitor) restartService() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	logs.Warnf("Faiss service is unavailable, try restarting...")

	// 如果有旧进程，先尝试停止
	if fm.process != nil {
		if err := utils.StopFaissService(fm.process); err != nil {
			logs.Warnf("Failed to stop old Faiss process: %v", err)
		}
	}

	// 启动新的 Faiss 服务
	newProcess, err := fm.startFaissService()
	if err != nil {
		logs.Errorf("Failed to restart Faiss service: %v", err)
		return
	}

	fm.process = newProcess
	logs.Infof("Faiss service has been successfully restarted")
}

// startFaissService 启动 Faiss 服务并等待其就绪
func (fm *FaissMonitor) startFaissService() (*os.Process, error) {
	// 检查 Python 环境
	if err := utils.CheckPythonEnvironment("cpu", fm.serviceDir); err != nil {
		return nil, fmt.Errorf("Python environment check failed: %w", err)
	}

	// 启动 Faiss 服务
	process, err := utils.StartFaissService(fm.serviceDir)
	if err != nil {
		return nil, fmt.Errorf("Failed to start Faiss service: %w", err)
	}

	logs.Infof("Starting Faiss service...")

	// 等待服务就绪
	maxRetries := 60
	retryInterval := time.Second
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(fm.healthEndpoint)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			logs.Infof("Faiss service has been started successfully")
			return process, nil
		}

		if i == maxRetries-1 {
			if process != nil {
				utils.StopFaissService(process)
			}
			return nil, fmt.Errorf("Faiss service startup timed out and has not responded for more than %d seconds.", maxRetries)
		}

		logs.Infof("Waiting for the Faiss service to start... (try %d/%d)", i+1, maxRetries)
		time.Sleep(retryInterval)
	}

	return process, nil
}
