package back

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

type FaissManager struct {
	process    *os.Process
	Indexer    *index.Indexer
	opts       map[string]interface{}
	gitgoDir   string
	faissDir   string
	faissState bool

	// 所有曾经被这个 FaissManager 持有过的 wrapper（包含当前 Indexer.FaissIndex）。
	// Reset() 会替换 Indexer.FaissIndex 但旧 wrapper（含 zvec_bridge 子进程）仍在，
	// 必须等到 Free() 时一并释放，否则旧桥进程驻留持有 fcntl LOCK，
	// 后续 search/check 永远拿不到锁。
	allWrappers []index.FaissWrapper
}

var (
	fm   *FaissManager
	once sync.Once
)

// InitFaissManager 在程序启动时调用，完成 Faiss 服务进程与 Indexer 的单例初始化
func InitFaissManager(projDir string, open bool, engine string) (fm *FaissManager, err error) {
	// 1. 启动 Faiss 服务（借用原 InitFaiss 的查目录+启动逻辑，不在此展开）
	var proc *os.Process
	ext := ".local"
	var faissState bool
	if open && engine != "zvec" {
		logs.Infof("Starting Faiss service...")
		//proc, _, err = InitFaiss()
		ext = ".faiss"
		faissState = true
		if err != nil {
			err = fmt.Errorf("Failed to initialize Faiss service: %w", err)
			return nil, fmt.Errorf("Failed to initialize FaissManager: %w", err)
		}
	}

	// 2. 确保 .gitgo 目录存在
	gitgo := filepath.Join(projDir, ".gitgo")
	if e := os.MkdirAll(gitgo, 0755); e != nil {
		err = fmt.Errorf("Failed to create index directory: %w", e)
		return nil, fmt.Errorf("Failed to initialize FaissManager: %w", err)
	}

	// 3. 构造 FaissWrapper，并尝试加载已有索引文件
	opts := map[string]interface{}{
		"storage_path": gitgo,
		"server_url":   index.DefaultFaissServerURL,
		"index_id":     projDir,
	}
	
	cfg, _ := config.LoadConfig()
	if engine == "zvec" {
		opts["collection_path"] = filepath.Join(gitgo, "zvec_collections")
		if cfg != nil {
			opts["python_path"] = cfg.ZvecConfig.PythonPath
		}
	}

	dim := config.ResolveVectorDim(engine, cfg)
	fw, fwErr := index.NewFaissWrapperByEngine(engine, dim, opts)
	if fwErr != nil {
		return nil, fmt.Errorf("failed to initialize FAISS/Zvec wrapper: %w", fwErr)
	}
	idxFile := filepath.Join(gitgo, "code_index"+ext)
	if _, statErr := os.Stat(idxFile); statErr == nil {
		if loadErr := fw.LoadFromFile(idxFile); loadErr == nil {
			fmt.Println("► Successfully loaded existing Faiss index")
		}
	}

	// 4. 打开或创建索引数据库
	db, dbErr := index.EnsureIndexDB(projDir)
	if dbErr != nil {
		err = fmt.Errorf("Failed to initialize index database: %w", dbErr)
		return nil, fmt.Errorf("Failed to initialize FaissManager: %w", err)
	}

	// 5. 构建单例
	fm = &FaissManager{
		process:     proc,
		Indexer:     &index.Indexer{DB: db, FaissIndex: fw},
		opts:        opts,
		gitgoDir:    gitgo,
		faissDir:    idxFile,
		faissState:  faissState,
		allWrappers: []index.FaissWrapper{fw},
	}
	return fm, err
}

// Stop 在程序退出时调用，统一停止 Faiss 服务
func (m *FaissManager) Stop() {
	if m.faissState {
		return
	}
	err := utils.StopFaissService(m.process)
	if err != nil {
		fmt.Println("Failed to stop Faiss service:", err)
		return
	}
	fmt.Println("► FaissManager has stopped")
}

// Free 释放索引资源，关闭 zvec_bridge 子进程并释放 collection LOCK。
// 不调用此方法会导致桥进程长期驻留持有 fcntl LOCK，
// 后续 search/check 路径 spawn 的新桥永远拿不到锁。
//
// 释放 allWrappers 中**所有**曾被持有的 wrapper（不只是当前 Indexer.FaissIndex），
// 这样像 Reset() 那种偷偷换 wrapper 的路径残留下来的旧桥也会被一起干掉。
// Free 必须幂等：多次调用不应崩溃。
func (m *FaissManager) Free() {
	if m == nil {
		return
	}
	seen := make(map[index.FaissWrapper]bool, len(m.allWrappers))
	for _, w := range m.allWrappers {
		if w == nil || seen[w] {
			continue
		}
		seen[w] = true
		// 单个 wrapper 的 Free panic 不能拖垮其他释放
		func(wr index.FaissWrapper) {
			defer func() {
				if r := recover(); r != nil {
					logs.Warnf("FaissManager.Free: wrapper.Free panicked: %v", r)
				}
			}()
			wr.Free()
		}(w)
	}
	m.allWrappers = nil
	if m.Indexer != nil {
		m.Indexer.FaissIndex = nil
	}
	if m.faissState && m.process != nil {
		if err := utils.StopFaissService(m.process); err != nil {
			logs.Warnf("Failed to stop Faiss service: %v", err)
		}
	}
}

// Reset 清空内存中 Indexer 的 FaissWrapper，并删除磁盘上的 .faiss 文件
func (m *FaissManager) Reset() error {
	ext := ".local"
	if m.faissState {
		ext = ".faiss"
	}
	// 1. 新建一个空的 FaissWrapper —— 必须按当前 engine 选择（faiss/zvec/local），
	//    旧实现写死 NewFaissWrapper 让 zvec 模式 reset 时强行连 5533 报错。
	engine := config.GetEngine()
	newFw, fwErr := index.NewFaissWrapperByEngine(engine, m.Indexer.FaissIndex.Dimension(), m.opts)
	if fwErr != nil {
		return fmt.Errorf("failed to create new FAISS wrapper during reset: %w", fwErr)
	}
	// 注意：这里不立即 Free 旧 wrapper，而是登记到 allWrappers，
	// 等到 FaissManager.Free() 时统一释放。这样调用方能在 Reset 后继续
	// 用 m.Indexer 访问任何引用过的 wrapper（理论上没必要，但保持现有语义），
	// 同时杜绝旧桥进程在 Free 之前驻留持有 LOCK。
	m.Indexer.FaissIndex = newFw
	m.allWrappers = append(m.allWrappers, newFw)
	// 2. 删除磁盘文件
	idxFile := filepath.Join(m.gitgoDir, "code_index"+ext)
	if err := os.Remove(idxFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Failed to delete index file: %w", err)
	}
	return nil
}

func InitFaiss() (*os.Process, string, error) {
	// 获取FAISSService目录的路径
	var faissServiceDir string

	faissServiceDir = os.Getenv("FAISS_SERVICE_PATH")
	if faissServiceDir != "" {
		if _, err := os.Stat(faissServiceDir); os.IsNotExist(err) {
			log.Printf("FAISS_SERVICE_PATH specifies %s but directory does not exist, falling back to auto-discovery", faissServiceDir)
			faissServiceDir = ""
		} else {
			log.Printf("Using FAISSService directory from FAISS_SERVICE_PATH: %s", faissServiceDir)
		}
	}

	// 如果方法4未找到，继续尝试其他方法
	if faissServiceDir == "" {
		// 方法1：尝试从源文件路径获取（适用于go run）
		sourceDir, err := utils.GetSourceFileDir()
		log.Printf("Obtaining FAISSService directory from source file path: %s", sourceDir)
		if err == nil {
			// 检查源文件目录下是否存在FAISSService
			tempDir := filepath.Join(sourceDir, "FAISSService")
			if _, err := os.Stat(tempDir); err == nil {
				faissServiceDir = tempDir
				log.Printf("Find FAISSService: %s from the source file directory", faissServiceDir)
			}
		}
	}

	// 方法2：如果方法1失败，尝试从可执行文件路径获取（适用于编译后的二进制文件）
	if faissServiceDir == "" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("Unable to get executable path: %v", err)
		}
		execDir := filepath.Dir(execPath)
		tempDir := filepath.Join(execDir, "FAISSService")
		log.Printf("Retrieving FAISSService directory from executable path: %s", execDir)
		if _, err := os.Stat(tempDir); err == nil {
			faissServiceDir = tempDir
			log.Printf("Find FAISSService: %s from the executable directory", faissServiceDir)
		}
	}

	// 方法3：如果前两种方法都失败，尝试从当前工作目录获取
	if faissServiceDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Unable to get current working directory: %v", err)
		}
		log.Printf("Retrieving FAISSService directory from current working directory: %s", cwd)
		tempDir := filepath.Join(cwd, "cmd", "main", "FAISSService")
		if _, err := os.Stat(tempDir); err == nil {
			faissServiceDir = tempDir
			log.Printf("FAISSService found from current working directory: %s", faissServiceDir)
		}
	}

	// 如果所有方法都失败，报错退出
	if faissServiceDir == "" {
		log.Fatalf("The FAISSService directory cannot be found. Please ensure that the FAISSService directory exists in the source file directory or executable file directory.")
	}

	// 1. 启动或确认 FAISS service 已就绪
	if err := utils.CheckPythonEnvironment("cpu", faissServiceDir); err != nil {
		return nil, faissServiceDir, fmt.Errorf("Python environment check failed: %w", err)
	}

	// 启动Faiss服务
	faissProcess, err := utils.StartFaissService(faissServiceDir)
	if err != nil {
		log.Fatalf("Failed to start Faiss service: %v", err)
	}

	log.Println("Starting Faiss service...")

	// 轮询检测Faiss服务状态
	maxRetries := 60
	retryInterval := time.Second
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(index.DefaultFaissServerURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			log.Println("Faiss service has been started successfully")
			break
		}
		if i == maxRetries-1 {
			log.Fatalf("Faiss service startup timed out and has not responded for more than %d seconds.", maxRetries)
		}
		log.Printf("Waiting for the Faiss service to start... (try %d/%d)", i+1, maxRetries)
		time.Sleep(retryInterval)
	}
	return faissProcess, faissServiceDir, nil
}

// // EnsureEmbeddings 遍历 functions 表，为每条记录同时计算 description+code snippet 的向量并加入 FAISS。
// func EnsureEmbeddings(idx *index.Indexer, gitgoDir, projDir string) error {
// 	// 1. 从 DB 读出所有 id, description, file, start_line, end_line
// 	rows, err := idx.DB.Query(
// 		"SELECT id, description, file, start_line, end_line FROM functions",
// 	)
// 	if err != nil {
// 		return err
// 	}
// 	defer rows.Close()

// 	dim := idx.FaissIndex.Dimension()
// 	for rows.Next() {
// 		var (
// 			id                 int
// 			desc, relPath      string
// 			startLine, endLine int
// 		)
// 		if err := rows.Scan(&id, &desc, &relPath, &startLine, &endLine); err != nil {
// 			continue
// 		}

// 		//// 2. 读取这段代码片段
// 		//snippet, err := readSnippet(projDir, relPath, startLine, endLine)
// 		//if err != nil {
// 		//	// 读不到也不要中断，直接只用 desc
// 		//	fmt.Fprintf(os.Stderr, "warn: read snippet %s [%d:%d] failed: %v\n",
// 		//		relPath, startLine, endLine, err)
// 		//	snippet = ""
// 		//}
// 		//
// 		//// 3. 拼接 description + snippet
// 		//text := desc
// 		//if snippet != "" {
// 		//	text = desc + "\n```\n" + snippet + "\n```"
// 		//}

// 		// 4. 生成向量并入索引
// 		vec := search.SimpleEmbedding(desc, dim)
// 		if err := idx.FaissIndex.AddVector(id, vec); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// // EnsureEmbeddingsBatch 使用多批次 + 并发 Worker Pool 来生成 Embeddings 并入索引
// func EnsureEmbeddingsBatch(idx *index.Indexer) error {
// 	// 1. 读出所有函数 id 和描述
// 	type rec struct {
// 		id   int
// 		desc string
// 		name string
// 		pkg  string
// 		file string
// 	}
// 	var records []rec

// 	rows, err := idx.DB.Query("SELECT id, description, name, package, file FROM functions")
// 	if err != nil {
// 		return err
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		var r rec
// 		if err := rows.Scan(&r.id, &r.desc, &r.name, &r.pkg, &r.file); err != nil {
// 			continue
// 		}
// 		records = append(records, r)
// 	}

// 	dim := idx.FaissIndex.Dimension()
// 	cfg, err := config.LoadConfig()
// 	if err != nil {
// 		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
// 		return err
// 	}
// 	// 2. 切分批次
// 	var batchSize = cfg.EmbeddingMaxBatch
// 	type batch struct {
// 		ids   []int
// 		texts []string
// 	}
// 	var batches []batch
// 	for i := 0; i < len(records); i += batchSize {
// 		j := i + batchSize
// 		if j > len(records) {
// 			j = len(records)
// 		}
// 		var b batch
// 		for _, r := range records[i:j] {
// 			b.ids = append(b.ids, r.id)
// 			text := fmt.Sprintf("description: %s\nname is: %s\npacakge is: %s\nfile path is: %s", r.desc, r.name, r.pkg, r.file)
// 			b.texts = append(b.texts, text)
// 		}
// 		batches = append(batches, b)
// 	}

// 	// 3. 准备 Worker Pool
// 	var maxWorkers = cfg.EmbeddingMaxWorker
// 	logs.Infof("Generating vectors, total amount is %d, batch size is %d, maximum concurrency is %d", len(records), batchSize, maxWorkers)
// 	jobs := make(chan batch)
// 	var wg sync.WaitGroup

// 	for w := 0; w < maxWorkers; w++ {
// 		go func() {
// 			for b := range jobs {
// 				// 3.1 批量调用
// 				embs, err := search.SimpleEmbeddingBatch(b.texts, dim)
// 				if err != nil || len(embs) != len(b.texts) {
// 					logs.Warnf("Batch vector generation failed for function %d, downgraded to single insertion: %v", b.ids, err)
// 					for i, desc := range b.texts {
// 						vec := search.SimpleEmbedding(desc, dim)
// 						if e := idx.FaissIndex.AddVector(b.ids[i], vec); e != nil {
// 							logs.Errorf("Failed to add vector for function %d: %v", b.ids[i], e)
// 						}
// 					}
// 				} else {
// 					logs.Warnf("Successfully generated vectors in batches for function %d and entered them into the database one by one.", b.ids)
// 					for i, vec := range embs {
// 						if e := idx.FaissIndex.AddVector(b.ids[i], vec); e != nil {
// 							logs.Errorf("Failed to add vector for function %d: %v", b.ids[i], e)
// 						}
// 					}
// 				}
// 				wg.Done()
// 			}
// 		}()
// 	}

// 	// 4. 分发批次并等待完成
// 	for _, b := range batches {
// 		wg.Add(1)
// 		jobs <- b
// 	}
// 	close(jobs)
// 	wg.Wait()

// 	return nil
// }

// // readSnippet 从 projDir/relPath 的文件里按行号截取代码片段
// func readSnippet(projDir, relPath string, start, end int) (string, error) {
// 	absPath := filepath.Join(projDir, relPath)
// 	f, err := os.Open(absPath)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer f.Close()

// 	scanner := bufio.NewScanner(f)
// 	var sb strings.Builder
// 	lineNo := 1
// 	for scanner.Scan() {
// 		if lineNo >= start && lineNo <= end {
// 			sb.WriteString(scanner.Text())
// 			sb.WriteByte('\n')
// 		}
// 		if lineNo > end {
// 			break
// 		}
// 		lineNo++
// 	}
// 	if err := scanner.Err(); err != nil {
// 		return "", err
// 	}
// 	return sb.String(), nil
// }
