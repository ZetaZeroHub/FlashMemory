package utils

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// DbWriteRequest 表示一个数据库写入请求
type DbWriteRequest struct {
	// 写入操作的类型标识，如 "function_insert", "call_insert" 等
	Type string
	// 写入操作的参数
	Args []interface{}
	// 写入操作的SQL语句
	SQL string
	// 结果通道，用于返回写入结果
	ResultChan chan error
}

// DbWriter 数据库写入管理器，用于串行化写入操作并实现自动重试
type DbWriter struct {
	db            *sql.DB
	requestChan   chan DbWriteRequest
	wg            sync.WaitGroup
	maxRetries    int
	retryInterval time.Duration
	done          chan struct{}
}

// NewDbWriter 创建一个新的数据库写入管理器
func NewDbWriter(db *sql.DB) *DbWriter {
	queueSize := 100
	maxRetries := 5
	retryInterval := 50 * time.Millisecond

	cfg, _ := getConfig()
	if cfg != nil {
		if cfg.DbWriterQueueSize > 0 {
			queueSize = cfg.DbWriterQueueSize
		}
		if cfg.DbWriterMaxRetries > 0 {
			maxRetries = cfg.DbWriterMaxRetries
		}
		if cfg.DbWriterRetryInterval > 0 {
			retryInterval = time.Duration(cfg.DbWriterRetryInterval) * time.Millisecond
		}
	}

	writer := &DbWriter{
		db:            db,
		requestChan:   make(chan DbWriteRequest, queueSize),
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
		done:          make(chan struct{}),
	}

	// 启动写入处理协程
	writer.wg.Add(1)
	go writer.processWrites()

	return writer
}

// 处理写入请求的协程
func (w *DbWriter) processWrites() {
	defer w.wg.Done()

	for {
		select {
		case req := <-w.requestChan:
			// 执行写入操作，带重试
			err := w.execWithRetry(req.SQL, req.Args...)
			if err != nil {
				logs.Errorf("数据库写入失败 [%s]: %v", req.Type, err)
			}
			// 将结果发送回请求方
			if req.ResultChan != nil {
				req.ResultChan <- err
				close(req.ResultChan)
			}
		case <-w.done:
			return
		}
	}
}

// ExecWithRetry 执行SQL写入操作，失败时自动重试
func (w *DbWriter) execWithRetry(query string, args ...interface{}) error {
	var err error
	for i := 0; i < w.maxRetries; i++ {
		_, err = w.db.Exec(query, args...)
		if err == nil {
			return nil
		}

		// 检查是否是数据库锁定错误
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			// 指数退避重试，每次重试等待时间增加
			waitTime := w.retryInterval * time.Duration(i+1)
			logs.Warnf("数据库锁定，等待 %v 后重试 (尝试 %d/%d)", waitTime, i+1, w.maxRetries)
			time.Sleep(waitTime)
			continue
		}

		// 其他错误直接返回
		return err
	}

	return fmt.Errorf("数据库写入重试%d次后仍失败: %w", w.maxRetries, err)
}

// Write 提交一个写入请求到队列
func (w *DbWriter) Write(reqType string, sql string, args ...interface{}) error {
	resultChan := make(chan error, 1)
	req := DbWriteRequest{
		Type:       reqType,
		SQL:        sql,
		Args:       args,
		ResultChan: resultChan,
	}

	// 发送请求到通道
	w.requestChan <- req

	// 等待结果
	return <-resultChan
}

// WriteAsync 异步提交一个写入请求到队列，不等待结果
func (w *DbWriter) WriteAsync(reqType string, sql string, args ...interface{}) {
	req := DbWriteRequest{
		Type:       reqType,
		SQL:        sql,
		Args:       args,
		ResultChan: nil, // 不需要返回结果
	}

	// 发送请求到通道
	w.requestChan <- req
}

// Close 关闭写入管理器，等待所有写入完成
func (w *DbWriter) Close() {
	close(w.done)
	w.wg.Wait()
	close(w.requestChan)
}

// PrepareDB 初始化数据库连接，设置WAL模式和连接池参数
func PrepareDB(db *sql.DB) error {
	// 设置WAL模式，提高并发性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("设置WAL模式失败: %w", err)
	}

	// 设置连接池参数，限制最大连接数
	db.SetMaxOpenConns(1) // 限制为1个连接，强制串行化
	db.SetMaxIdleConns(1)

	return nil
}
