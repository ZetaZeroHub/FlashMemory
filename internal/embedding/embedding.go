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
	// 简单估算，中英文混合每2字符约等于1 token，实际部署可用更准的分词器
	return len([]rune(text)) / 2
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
		}
		batches = append(batches, b)
	}

	// 3. 准备 Worker Pool
	var maxWorkers = cfg.EmbeddingMaxWorker
	logs.Infof("正在生成向量，总量为 %d，批次大小为 %d，最大并发数为 %d", len(records), batchSize, maxWorkers)
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
				// 		logs.Warnf("函数文本超长(约%d tokens)，已截断: %d", estimateTokens(txt), b.ids[i])
				// 		txt = string(runes[:500])
				// 	}
				// 	idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
				// }

				chunkMin, chunkMax := 200, 300
				for i, txt := range b.texts {
					tokens := estimateTokens(txt)
					if tokens > 500 { // 设置为500，留出余量
						logs.Warnf("函数文本超长(约%d tokens)，进行分块处理: %d", tokens, b.ids[i])
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
						logs.Warnf("为函数 %v 批量生成向量失败，降级到单条插入: %v", idIdx[k:l], err)
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
					if e := idx.FaissIndex.AddVector(id, avg); e != nil {
						logs.Errorf("为函数 %d 添加向量失败: %v", id, e)
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
		}
		batches = append(batches, b)
	}

	// 3. 准备 Worker Pool
	var maxWorkers = cfg.EmbeddingMaxWorker
	logs.Infof("正在生成 code_desc 向量，总量为 %d，批次大小为 %d，最大并发数为 %d", len(records), batchSize, maxWorkers)
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
				// 		logs.Warnf("code_desc文本超长(约%d tokens)，已截断: %d", estimateTokens(txt), b.ids[i])
				// 		txt = string(runes[:500])
				// 	}
				// 	idTexts = append(idTexts, idText{id: b.ids[i], text: txt})
				// }

				chunkMin, chunkMax := 200, 300
				for i, txt := range b.texts {
					tokens := estimateTokens(txt)
					if tokens > 500 { // 设置为500，留出余量
						logs.Warnf("函数文本超长(约%d tokens)，进行分块处理: %d", tokens, b.ids[i])
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
						logs.Infof("已处理 %d 条 code_desc 记录", len(id2vecs))
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
					if e := idx.FaissIndex.AddVector(id, avg); e != nil {
						logs.Errorf("为 code_desc %d 添加向量失败: %v", id, e)
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
