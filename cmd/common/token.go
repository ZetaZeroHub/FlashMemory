package common

import (
	"sync"
)

// TokenManager 管理应用程序中的token、url和model参数
type TokenManager struct {
	mu           sync.RWMutex
	currentToken string // 当前请求的token
	envToken     string // 环境变量中的token
	url          string // API URL
	model        string // 模型名称
}

// 全局token管理器实例
var tokenManager = &TokenManager{}

// SetEnvToken 设置环境变量中的token
func SetEnvToken(token string) {
	tokenManager.mu.Lock()
	defer tokenManager.mu.Unlock()
	tokenManager.envToken = token
}

// SetCurrentToken 设置当前请求的token
func SetCurrentToken(token string) {
	tokenManager.mu.Lock()
	defer tokenManager.mu.Unlock()
	tokenManager.currentToken = token
}

// GetCurrentToken 获取当前请求的token
// 优先返回请求头中的token，如果没有则返回环境变量中的token
func GetCurrentToken() string {
	tokenManager.mu.RLock()
	defer tokenManager.mu.RUnlock()

	if tokenManager.currentToken != "" {
		return tokenManager.currentToken
	}
	return tokenManager.envToken
}

// GetEnvToken 获取环境变量中的token
func GetEnvToken() string {
	tokenManager.mu.RLock()
	defer tokenManager.mu.RUnlock()
	return tokenManager.envToken
}

// ClearCurrentToken 清除当前请求的token（通常在请求结束时调用）
func ClearCurrentToken() {
	tokenManager.mu.Lock()
	defer tokenManager.mu.Unlock()
	tokenManager.currentToken = ""
}

// HasToken 检查是否有可用的token
func HasToken() bool {
	tokenManager.mu.RLock()
	defer tokenManager.mu.RUnlock()
	return tokenManager.currentToken != "" || tokenManager.envToken != ""
}

// SetURL 设置API URL
func SetURL(url string) {
	tokenManager.mu.Lock()
	defer tokenManager.mu.Unlock()
	tokenManager.url = url
}

// GetURL 获取API URL
func GetURL() string {
	tokenManager.mu.RLock()
	defer tokenManager.mu.RUnlock()
	return tokenManager.url
}

// SetModel 设置模型名称
func SetModel(model string) {
	tokenManager.mu.Lock()
	defer tokenManager.mu.Unlock()
	tokenManager.model = model
}

// GetModel 获取模型名称
func GetModel() string {
	tokenManager.mu.RLock()
	defer tokenManager.mu.RUnlock()
	return tokenManager.model
}
