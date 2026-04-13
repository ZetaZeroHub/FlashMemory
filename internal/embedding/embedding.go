package embedding

import (
	"fmt"
	"sync"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// 简单估算文本的 token 数量
func estimateTokens(text string) int {
	// 对于 BGE 等多语言/中文优化的词表，1个中文字符或标点基本就是 1 个 token。
	// 这里采用保守的 1:1 估算（rune 数），避免因为除以 2 导致超长的中文代码被漏判而引发 413 错误。
	return len([]rune(text))
}

// 按 token 数切分文本
func splitTextByToken(text string, chunkTokenMin, chunkTokenMax int) []string {
	runes := []rune(text)
	var res []string
	start := 0
	for start < len(runes) {
		end := start + chunkTokenMax*2
		if end > len(runes) {
			end = len(runes)
		}
		// 保证每块不少于 chunkTokenMin
		if end-start < chunkTokenMin*2 && end != len(runes) {
			end = start + chunkTokenMin*2
			if end > len(runes) {
				end = len(runes)
			}
		}
		res = append(res, string(runes[start:end]))
		start = end
	}
	return res
}

// EnsureEmbeddingsBatch 使用多批次 + 并发 Worker Pool 来生成 Embeddings 并入索引
func EnsureEmbeddingsBatch(idx *index.Indexer) error {
	// 1. 读出所有函数 id 和描述
	type rec struct {
		id   int
		desc string
		name string
		pkg  string
		file string
	}
	var records []rec

	rows, err := idx.DB.Query("SELECT id, description, name, package, file FROM functions")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.desc, &r.name, &r.pkg, &r.file); err != nil {
			continue
		}
		records = append(records, r)
	}

	dim := idx.FaissIndex.Dimension()
	cfg, err := config.LoadConfig()
	if err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return err
	}
	// 2. 切分批次
	var batchSize = cfg.EmbeddingMaxBatch
	type batch struct {
		ids   []int
		texts []string
		names []string
		descs []string
		paths []string
	}
	var batches []batch
	for i := 0; i < len(records); i += batchSize {
		j := i + batchSize
		if j > len(records) {
			j = len(records)
		}
		var b batch
		for _, r := range records[i:j] {
			b.ids = append(b.ids, r.id)
			text := fmt.Sprintf("name: %s\npacakge: %s\nfile path: %s\n%s", r.name, r.pkg, r.file, r.desc)
			b.texts = append(b.texts, text)
			b.names = append(b.names, r.name)
			b.descs = append(b.descs, r.desc)
			b.paths = append(b.paths, r.file)
		}
		batches = append(batches, b)
	}

	// 3. 准备 Worker Pool
	var maxWorkers = cfg.EmbeddingMaxWorker
	logs.Infof("Generating vectors, total amount is %d, batch size is %d, maximum concurrency is %d", len(records), batchSize, maxWorkers)
	jobs := make(chan batch)
	var wg sync.WaitGroup

	for w := 0; w < maxWorkers; w++ {
		go func() {
			for b := range jobs {
				// 3.1 检查每条文本长度，切分超长文本
				type idText struct {
					id   int
					text string
				}
				var idTexts []idText
				// for i, txt := range b.texts {
				// 	runes := []rune(txt)
				// 	if len(runes) > 500 {
				// 		logs.Warnf("The function text is too long (about %d tokens) and has been truncated: %d", estimateTokens(txt), b.ids[i])
				// 		txt = string(runes[:500])
				// 	}
				// 	idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
				// }

				chunkMin, chunkMax := 200, 300
				for i, txt := range b.texts {
					tokens := estimateTokens(txt)
					if tokens > 500 { // 设置为500，留出余量
						logs.Warnf("The function text is too long (about %d tokens), so it needs to be divided into chunks: %d", tokens, b.ids[i])
						chunks := splitTextByToken(txt, chunkMin, chunkMax)
						for _, ch := range chunks {
							idTexts = append(idTexts, idText{id: b.ids[i], text: ch})
						}
					} else {
						idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
					}
				}

				// 分批调用（注意id有重复，后续向量可合并或平均）
				// 由于分块后 id 可能重复，需聚合
				id2vecs := map[int][][]float32{}
				var texts []string
				var idIdx []int
				for _, it := range idTexts {
					texts = append(texts, it.text)
					idIdx = append(idIdx, it.id)
				}

				var batchEmbSize = 32
				for k := 0; k < len(texts); k += batchEmbSize {
					l := k + batchEmbSize
					if l > len(texts) {
						l = len(texts)
					}
					embs, err := search.SimpleEmbeddingBatch(texts[k:l], dim)
					if err != nil || len(embs) != l-k {
						logs.Warnf("Batch vector generation failed for function %v, downgraded to single insertion: %v", idIdx[k:l], err)
						for i := k; i < l; i++ {
							vec := search.SimpleEmbedding(texts[i], dim)
							id2vecs[idIdx[i]] = append(id2vecs[idIdx[i]], vec)
						}
					} else {
						for i, vec := range embs {
							id2vecs[idIdx[k+i]] = append(id2vecs[idIdx[k+i]], vec)
						}
					}
				}

				// 对于分块的结果，取平均向量
				for id, vecs := range id2vecs {
					avg := make([]float32, dim)
					for _, v := range vecs {
						for i := 0; i < dim; i++ {
							avg[i] += v[i]
						}
					}
					for i := range avg {
						avg[i] /= float32(len(vecs))
					}
					// 尝试向 Zvec 注入元数据以便建立 BM25 稀疏索引
					if zvecApp, ok := idx.FaissIndex.(*index.ZvecWrapper); ok {
						var fname, fdesc, fpath string
						for idxInBatch, batchId := range b.ids {
							if batchId == id {
								fname = b.names[idxInBatch]
								fdesc = b.descs[idxInBatch]
								fpath = b.paths[idxInBatch]
								break
							}
						}
						e := zvecApp.AddFunctionVector(id, avg, map[string]interface{}{
							"func_name":   fname,
							"description": fdesc,
							"file_path":   fpath,
						})
						if e != nil {
							logs.Errorf("Failed to add Zvec function vector %d: %v", id, e)
						}
					} else {
						if e := idx.FaissIndex.AddVector(id, avg); e != nil {
							logs.Errorf("Failed to add vector for function %d: %v", id, e)
						}
					}
				}
				wg.Done()
			}
		}()
	}

	// 4. 分发批次并等待完成
	for _, b := range batches {
		wg.Add(1)
		jobs <- b
	}
	close(jobs)
	wg.Wait()

	return nil
}

// EnsureCodeDescEmbeddingsBatch 使用多批次 + 并发 Worker Pool 来生成 code_desc 表的 Embeddings 并入索引
func EnsureCodeDescEmbeddingsBatch(idx *index.Indexer) error {
	// 1. 读出所有 code_desc 记录
	type rec struct {
		id          int
		name        string
		type_       string
		path        string
		parent_path string
		description string
	}
	var records []rec

	rows, err := idx.DB.Query("SELECT id, name, type, path, parent_path, description FROM code_desc")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.name, &r.type_, &r.path, &r.parent_path, &r.description); err != nil {
			continue
		}
		records = append(records, r)
	}

	dim := idx.FaissIndex.Dimension()
	cfg, err := config.LoadConfig()
	if err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
		return err
	}
	// 2. 切分批次
	var batchSize = cfg.EmbeddingMaxBatch
	type batch struct {
		ids   []int
		texts []string
		names []string
		descs []string
		paths []string
	}
	var batches []batch
	for i := 0; i < len(records); i += batchSize {
		j := i + batchSize
		if j > len(records) {
			j = len(records)
		}
		var b batch
		for _, r := range records[i:j] {
			b.ids = append(b.ids, r.id)
			text := fmt.Sprintf("name: %s\npath: %s\ndescription: %s", r.name, r.path, r.description)
			b.texts = append(b.texts, text)
			b.names = append(b.names, r.name)
			b.descs = append(b.descs, r.description)
			b.paths = append(b.paths, r.path)
		}
		batches = append(batches, b)
	}

	// 3. 准备 Worker Pool
	var maxWorkers = cfg.EmbeddingMaxWorker
	logs.Infof("Generating code_desc vectors, total amount %d, batch size %d, maximum concurrency %d", len(records), batchSize, maxWorkers)
	jobs := make(chan batch)
	var wg sync.WaitGroup

	for w := 0; w < maxWorkers; w++ {
		go func() {
			for b := range jobs {
				// 3.1 检查每条文本长度，切分超长文本
				type idText struct {
					id   int
					text string
				}
				var idTexts []idText
				// for i, txt := range b.texts {
				// 	runes := []rune(txt)
				// 	if len(runes) > 500 {
				// 		logs.Warnf("The code_desc text is too long (about %d tokens) and has been truncated: %d", estimateTokens(txt), b.ids[i])
				// 		txt = string(runes[:500])
				// 	}
				// 	idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
				// }

				chunkMin, chunkMax := 200, 300
				for i, txt := range b.texts {
					tokens := estimateTokens(txt)
					if tokens > 500 { // 设置为500，留出余量
						logs.Warnf("The function text is too long (about %d tokens), so it needs to be divided into chunks: %d", tokens, b.ids[i])
						chunks := splitTextByToken(txt, chunkMin, chunkMax)
						for _, ch := range chunks {
							idTexts = append(idTexts, idText{id: b.ids[i], text: ch})
						}
					} else {
						idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
					}
				}

				// 直接使用 SimpleEmbedding 单条生成向量，避免批量处理可能的 token 超限问题
				// 由于分块后 id 可能重复，需聚合
				id2vecs := map[int][][]float32{}

				// 逐条处理每个文本
				for _, it := range idTexts {
					// 使用 SimpleEmbedding 单条生成向量
					vec := search.SimpleEmbedding(it.text, dim)
					// 将向量添加到对应 ID 的向量列表中
					id2vecs[it.id] = append(id2vecs[it.id], vec)
					// 每处理10条记录输出一次日志
					if len(id2vecs)%10 == 0 {
						logs.Infof("%d code_desc records processed", len(id2vecs))
					}
				}

				// 对于分块的结果，取平均向量
				for id, vecs := range id2vecs {
					avg := make([]float32, dim)
					for _, v := range vecs {
						for i := 0; i < dim; i++ {
							avg[i] += v[i]
						}
					}
					for i := range avg {
						avg[i] /= float32(len(vecs))
					}
					if zvecApp, ok := idx.FaissIndex.(*index.ZvecWrapper); ok {
						e := zvecApp.AddModuleVector(id, avg)
						if e != nil {
							logs.Errorf("Failed to add Zvec module vector %d: %v", id, e)
						}
					} else {
						if e := idx.FaissIndex.AddVector(id, avg); e != nil {
							logs.Errorf("Failed to add vector for code_desc %d: %v", id, e)
						}
					}
				}
				wg.Done()
			}
		}()
	}

	// 4. 分发批次并等待完成
	for _, b := range batches {
		wg.Add(1)
		jobs <- b
	}
	close(jobs)
	wg.Wait()

	return nil
}
