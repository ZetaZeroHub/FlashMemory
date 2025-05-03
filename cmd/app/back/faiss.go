package back

import (
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FaissManager struct {
	process  *os.Process
	Indexer  *index.Indexer
	opts     map[string]interface{}
	gitgoDir string
}

var (
	fm   *FaissManager
	once sync.Once
)

// InitFaissManager 在程序启动时调用，完成 Faiss 服务进程与 Indexer 的单例初始化
func InitFaissManager(projDir string) (*FaissManager, error) {
	var err error
	once.Do(func() {
		// 1. 启动 Faiss 服务（借用原 InitFaiss 的查目录+启动逻辑，不在此展开）
		proc, _, e := InitFaiss()
		if e != nil {
			err = fmt.Errorf("初始化 Faiss 服务失败: %w", e)
			return
		}

		// 2. 确保 .gitgo 目录存在
		gitgo := filepath.Join(projDir, ".gitgo")
		if e := os.MkdirAll(gitgo, 0755); e != nil {
			err = fmt.Errorf("创建索引目录失败: %w", e)
			return
		}

		// 3. 构造 FaissWrapper，并尝试加载已有索引文件
		opts := map[string]interface{}{
			"storage_path": gitgo,
			"server_url":   index.DefaultFaissServerURL,
			"index_id":     "code_index",
		}
		fw := index.NewFaissWrapper(128, opts)
		idxFile := filepath.Join(gitgo, "code_index.faiss")
		if _, statErr := os.Stat(idxFile); statErr == nil {
			if loadErr := fw.LoadFromFile(idxFile); loadErr == nil {
				fmt.Println("► 成功加载已有 Faiss 索引")
			}
		}

		// 4. 打开或创建索引数据库
		db, dbErr := index.EnsureIndexDB(projDir)
		if dbErr != nil {
			err = fmt.Errorf("初始化索引数据库失败: %w", dbErr)
			return
		}

		// 5. 构建单例
		fm = &FaissManager{
			process:  proc,
			Indexer:  &index.Indexer{DB: db, FaissIndex: fw},
			opts:     opts,
			gitgoDir: gitgo,
		}
	})
	return fm, err
}

// Stop 在程序退出时调用，统一停止 Faiss 服务
func (m *FaissManager) Stop() {
	err := utils.StopFaissService(m.process)
	if err != nil {
		fmt.Println("停止 Faiss 服务失败:", err)
		return
	}
}

// Reset 清空内存中 Indexer 的 FaissWrapper，并删除磁盘上的 .faiss 文件
func (m *FaissManager) Reset() error {
	// 1. 新建一个空的 FaissWrapper
	m.Indexer.FaissIndex = index.NewFaissWrapper(m.Indexer.FaissIndex.Dimension(), m.opts)
	// 2. 删除磁盘文件
	idxFile := filepath.Join(m.gitgoDir, "code_index.faiss")
	if err := os.Remove(idxFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除索引文件失败: %w", err)
	}
	return nil
}

func InitFaiss() (*os.Process, string, error) {
	// 获取FAISSService目录的路径
	var faissServiceDir string

	// 如果方法4未找到，继续尝试其他方法
	if faissServiceDir == "" {
		// 方法1：尝试从源文件路径获取（适用于go run）
		sourceDir, err := utils.GetSourceFileDir()
		log.Printf("正在从源文件路径获取FAISSService目录: %s", sourceDir)
		if err == nil {
			// 检查源文件目录下是否存在FAISSService
			tempDir := filepath.Join(sourceDir, "FAISSService")
			if _, err := os.Stat(tempDir); err == nil {
				faissServiceDir = tempDir
				log.Printf("从源文件目录找到FAISSService: %s", faissServiceDir)
			}
		}
	}

	// 方法2：如果方法1失败，尝试从可执行文件路径获取（适用于编译后的二进制文件）
	if faissServiceDir == "" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("无法获取可执行文件路径: %v", err)
		}
		execDir := filepath.Dir(execPath)
		tempDir := filepath.Join(execDir, "FAISSService")
		log.Printf("正在从可执行文件路径获取FAISSService目录: %s", execDir)
		if _, err := os.Stat(tempDir); err == nil {
			faissServiceDir = tempDir
			log.Printf("从可执行文件目录找到FAISSService: %s", faissServiceDir)
		}
	}

	// 方法3：如果前两种方法都失败，尝试从当前工作目录获取
	if faissServiceDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("无法获取当前工作目录: %v", err)
		}
		log.Printf("正在从当前工作目录获取FAISSService目录: %s", cwd)
		tempDir := filepath.Join(cwd, "cmd", "main", "FAISSService")
		if _, err := os.Stat(tempDir); err == nil {
			faissServiceDir = tempDir
			log.Printf("从当前工作目录找到FAISSService: %s", faissServiceDir)
		}
	}

	// 如果所有方法都失败，报错退出
	if faissServiceDir == "" {
		log.Fatalf("无法找到FAISSService目录，请确保FAISSService目录存在于源文件目录或可执行文件目录下")
	}

	// 1. 启动或确认 FAISS service 已就绪
	if err := utils.CheckPythonEnvironment("cpu"); err != nil {
		return nil, faissServiceDir, fmt.Errorf("Python环境检查失败: %w", err)
	}

	// 启动Faiss服务
	faissProcess, err := utils.StartFaissService(faissServiceDir)
	if err != nil {
		log.Fatalf("启动Faiss服务失败: %v", err)
	}

	log.Println("正在启动Faiss服务...")

	// 轮询检测Faiss服务状态
	maxRetries := 60
	retryInterval := time.Second
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(index.DefaultFaissServerURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			log.Println("Faiss服务已成功启动")
			break
		}
		if i == maxRetries-1 {
			log.Fatalf("Faiss服务启动超时，超过%d秒仍未响应", maxRetries)
		}
		log.Printf("等待Faiss服务启动... (尝试 %d/%d)", i+1, maxRetries)
		time.Sleep(retryInterval)
	}
	return faissProcess, faissServiceDir, nil
}
