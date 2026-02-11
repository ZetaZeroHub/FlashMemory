package common

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// ErrorType 定义错误类型
type ErrorType string

// 定义各种错误类型常量
const (
	// 通用错误类型
	ErrorTypeUnknown      ErrorType = "未知错误"
	ErrorTypeInternal     ErrorType = "内部错误"
	ErrorTypeInvalidInput ErrorType = "无效输入"
	ErrorTypeIO           ErrorType = "IO错误"
	ErrorTypeDatabase     ErrorType = "数据库错误"
	ErrorTypeNetwork      ErrorType = "网络错误"
	ErrorTypeTimeout      ErrorType = "超时错误"
	ErrorTypePermission   ErrorType = "权限错误"
	ErrorTypeNotFound     ErrorType = "未找到"

	// 大模型相关错误类型
	ErrorTypeLLMResponse      ErrorType = "大模型响应错误"
	ErrorTypeLLMTimeout       ErrorType = "大模型超时"
	ErrorTypeLLMInvalidOutput ErrorType = "大模型输出无效"
	ErrorTypeLLMRateLimit     ErrorType = "大模型调用频率限制"
	ErrorTypeLLMContextLength ErrorType = "大模型上下文长度超限"
	ErrorTypeLLMAPIError      ErrorType = "大模型API错误"
)

// AppError 是应用程序的自定义错误类型
type AppError struct {
	Type       ErrorType // 错误类型
	Err        error     // 原始错误
	Message    string    // 用户友好的错误消息
	StatusCode int       // HTTP状态码（如果适用）
	Details    any       // 附加错误详情
	Stack      string    // 错误堆栈
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Type)
}

// Unwrap 返回原始错误，兼容 errors.Unwrap
func (e *AppError) Unwrap() error {
	return e.Err
}

// WithMessage 添加或替换错误消息
func (e *AppError) WithMessage(message string) *AppError {
	e.Message = message
	return e
}

// WithDetails 添加错误详情
func (e *AppError) WithDetails(details any) *AppError {
	e.Details = details
	return e
}

// WithStatusCode 设置HTTP状态码
func (e *AppError) WithStatusCode(code int) *AppError {
	e.StatusCode = code
	return e
}

// IsType 检查错误是否为指定类型
func IsType(err error, errorType ErrorType) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == errorType
	}
	return false
}

// New 创建一个新的应用错误
func New(errorType ErrorType, message string) *AppError {
	return &AppError{
		Type:    errorType,
		Message: message,
		Stack:   getStackTrace(),
	}
}

// Wrap 包装一个已有错误
func Wrap(err error, errorType ErrorType, message string) *AppError {
	if err == nil {
		return nil
	}

	// 如果已经是AppError，则更新类型和消息
	var appErr *AppError
	if errors.As(err, &appErr) {
		if errorType != "" {
			appErr.Type = errorType
		}
		if message != "" {
			appErr.Message = message
		}
		return appErr
	}

	return &AppError{
		Type:    errorType,
		Err:     err,
		Message: message,
		Stack:   getStackTrace(),
	}
}

// getStackTrace 获取当前堆栈信息
func getStackTrace() string {
	// 使用errors包获取堆栈信息
	err := errors.New("")
	stack := fmt.Sprintf("%+v", err)
	// 移除第一行的空错误信息
	lines := strings.Split(stack, "\n")
	if len(lines) > 0 {
		return strings.Join(lines[1:], "\n")
	}
	return stack
}

// 以下是大模型相关错误的便捷创建函数

// NewLLMResponseError 创建大模型响应错误
func NewLLMResponseError(message string) *AppError {
	return New(ErrorTypeLLMResponse, message)
}

// NewLLMTimeoutError 创建大模型超时错误
func NewLLMTimeoutError(message string) *AppError {
	return New(ErrorTypeLLMTimeout, message)
}

// NewLLMInvalidOutputError 创建大模型输出无效错误
func NewLLMInvalidOutputError(message string) *AppError {
	return New(ErrorTypeLLMInvalidOutput, message)
}

// NewLLMRateLimitError 创建大模型调用频率限制错误
func NewLLMRateLimitError(message string) *AppError {
	return New(ErrorTypeLLMRateLimit, message)
}

// NewLLMContextLengthError 创建大模型上下文长度超限错误
func NewLLMContextLengthError(message string) *AppError {
	return New(ErrorTypeLLMContextLength, message)
}

// WrapLLMError 包装大模型相关错误
func WrapLLMError(err error, message string) *AppError {
	return Wrap(err, ErrorTypeLLMAPIError, message)
}

// IsLLMError 检查是否为大模型相关错误
func IsLLMError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		switch appErr.Type {
		case ErrorTypeLLMResponse, ErrorTypeLLMTimeout, ErrorTypeLLMInvalidOutput,
			ErrorTypeLLMRateLimit, ErrorTypeLLMContextLength, ErrorTypeLLMAPIError:
			return true
		}
	}
	return false
}

func IsLLMRateLimit(errMsg string) bool {
	return strings.Contains(errMsg, "429") ||
		strings.Contains(errMsg, "Too Many Requests") ||
		strings.Contains(errMsg, "TPM limit")
}
