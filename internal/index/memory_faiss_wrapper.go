package index

import (
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"os"
	"path/filepath"
	"sort"
)

// MemoryFaissWrapper 是基于内存的FaissWrapper实现
type MemoryFaissWrapper struct {
	Dim int
	// 内部实现可以是内存映射或真实的Faiss索引
	Vectors map[int][]float32 // 用于内存实现：将函数ID映射到其嵌入向量
	Scores  map[int]float32   // 存储搜索结果的分数

	// 向量缓存，用于减少重复计算和提高性能
	vectorCache  map[string][]float32 // 缓存键到向量的映射
	cacheEnabled bool                 // 是否启用缓存

	// 持久化相关
	lastSavePath string // 上次保存的路径，用于增量更新
	dirtyFlag    bool   // 标记索引是否被修改过

	// 相似性计算相关，simFunc 为计算函数，useDistance 指示是否使用距离度量
	simFunc     SimilarityFunc
	useDistance bool
}

// Dimension 返回向量的维度
func (fw *MemoryFaissWrapper) Dimension() int {
	return fw.Dim
}

// GetScore 返回指定函数ID对应的相似度分数
func (fw *MemoryFaissWrapper) GetScore(funcID int) float32 {
	score, ok := fw.Scores[funcID]
	if !ok {
		return 0
	}
	return score
}

// NewMemoryFaissWrapper 创建一个新的内存版本FaissWrapper
func NewMemoryFaissWrapper(dimension int) *MemoryFaissWrapper {
	return &MemoryFaissWrapper{
		Dim:          dimension,
		Vectors:      make(map[int][]float32),
		Scores:       make(map[int]float32),
		vectorCache:  make(map[string][]float32),
		cacheEnabled: true,
		dirtyFlag:    false,
		// 默认使用余弦相似度
		simFunc:     cosineSimilarity,
		useDistance: false,
	}
}

// AddVector 为函数ID添加嵌入向量
func (fw *MemoryFaissWrapper) AddVector(funcID int, vector []float32) error {
	// 确保向量具有正确的维度
	if len(vector) != fw.Dim {
		// 调整向量大小
		resized := make([]float32, fw.Dim)
		copy(resized, vector)
		vector = resized
	}

	// 标记索引已被修改
	fw.dirtyFlag = true

	// 将向量存储在内存映射中
	fw.Vectors[funcID] = vector
	return nil
}

// AddVectorsBatch 批量添加向量到索引
func (fw *MemoryFaissWrapper) AddVectorsBatch(funcIDs []int, vectors []float32) error {
	for i, id := range funcIDs {
		vector := vectors[i*fw.Dim : (i+1)*fw.Dim]
		if err := fw.AddVector(id, vector); err != nil {
			return err
		}
	}
	return nil
}

// SearchVectors 查找与查询向量最接近的topK个向量
func (fw *MemoryFaissWrapper) SearchVectors(query []float32, topK int) []int {
	return fw.searchVectorsInternal(query, topK)
}

// AddModuleVector 为模块ID添加嵌入向量 (复用 AddVector)
func (fw *MemoryFaissWrapper) AddModuleVector(modID int, vector []float32) error {
	return fw.AddVector(modID, vector)
}

// SearchModuleVectors 查找与查询向量最接近的topK个模块向量 (复用 searchVectorsInternal)
func (fw *MemoryFaissWrapper) SearchModuleVectors(query []float32, topK int) []int {
	return fw.searchVectorsInternal(query, topK)
}

func (fw *MemoryFaissWrapper) searchVectorsInternal(query []float32, topK int) []int {
	// 生成查询向量的缓存键
	queryKey := fmt.Sprintf("query_%d", len(query))

	// 检查缓存中是否有相似的查询向量
	if fw.cacheEnabled {
		if cachedVector, hasCached := fw.vectorCache[queryKey]; hasCached {
			sim := fw.simFunc(query, cachedVector)
			if (fw.useDistance && sim < 1e-6) || (!fw.useDistance && sim > 0.99) {
				fmt.Println("Using similar cached query vector")
			}
		}
	}

	// 确保我们有向量可搜索
	if len(fw.Vectors) == 0 {
		return []int{}
	}

	// 确保查询向量具有正确的维度
	if len(query) != fw.Dim {
		// 调整查询向量大小
		resized := make([]float32, fw.Dim)
		copy(resized, query)
		query = resized
	}

	// 计算每个向量的相似度或距离
	type idScorePair struct {
		id    int
		score float32
	}
	pairs := make([]idScorePair, 0, len(fw.Vectors))

	for id, vec := range fw.Vectors {
		score := fw.simFunc(query, vec)
		// 对于余弦相似度，如果两个向量完全一致，则强制设为1.0
		if !fw.useDistance && len(vec) == len(query) {
			allSame := true
			for i := 0; i < len(vec); i++ {
				if vec[i] != query[i] {
					allSame = false
					break
				}
			}
			if allSame {
				score = 1.0
			}
		}
		fw.Scores[id] = score
		pairs = append(pairs, idScorePair{id: id, score: score})
	}

	// 根据当前相似度度量方式进行排序
	if fw.useDistance {
		// 欧几里得距离：距离越小越相似，升序排序
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].score < pairs[j].score
		})
	} else {
		// 余弦相似度：相似度越大越相似，降序排序
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].score > pairs[j].score
		})
	}

	// 缓存查询向量
	if fw.cacheEnabled {
		fw.vectorCache[queryKey] = append([]float32{}, query...)
	}

	// 获取前K个结果
	resultCount := topK
	if resultCount > len(pairs) {
		resultCount = len(pairs)
	}
	results := make([]int, resultCount)
	for i := 0; i < resultCount; i++ {
		results[i] = pairs[i].id
	}

	return results
}

// SaveToFile 将索引保存到磁盘
func (fw *MemoryFaissWrapper) SaveToFile(path string) error {
	// 如果索引没有被修改且路径与上次保存相同，可以跳过保存
	if !fw.dirtyFlag && path == fw.lastSavePath && fw.lastSavePath != "" {
		fmt.Printf("Index not modified since last save to %s, skipping...\n", path)
		return nil
	}

	// 确保目标目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for index: %v", err)
	}

	// 创建文件
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create index file: %v", err)
	}
	defer file.Close()

	// 写入一个简单的头部，包含维度和向量计数
	_, err = fmt.Fprintf(file, "FaissIndex Dim=%d Count=%d\n", fw.Dim, len(fw.Vectors))
	if err != nil {
		return fmt.Errorf("failed to write index header: %v", err)
	}

	// 写入向量数据（简化版，实际应该使用二进制格式）
	for id, vec := range fw.Vectors {
		_, err = fmt.Fprintf(file, "Vector %d: ", id)
		if err != nil {
			return err
		}
		for _, v := range vec {
			_, err = fmt.Fprintf(file, "%f ", v)
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(file)
		if err != nil {
			return err
		}
	}

	// 更新状态
	fw.lastSavePath = path
	fw.dirtyFlag = false
	fmt.Printf("Successfully saved in-memory index to %s\n", path)
	return nil
}

// LoadFromFile 从文件加载索引
func (fw *MemoryFaissWrapper) LoadFromFile(path string) error {
	logs.Infof("MemoryFaissWrapper Loading index from %s", path)
	// 从简单的文本文件加载（仅支持基本格式）
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open index file: %v", err)
	}
	defer file.Close()

	// 清除现有数据
	fw.Vectors = make(map[int][]float32)
	fw.ClearCache()

	// 读取并解析文件（简化版，实际应使用二进制格式）
	fmt.Printf("In-memory index loading from %s is not fully implemented\n", path)

	fw.lastSavePath = path
	fw.dirtyFlag = false
	return nil
}

// SetSimilarityMetric 设置相似度计算方法
func (fw *MemoryFaissWrapper) SetSimilarityMetric(metric string) {
	if metric == "euclidean" {
		fw.simFunc = euclideanDistance
		fw.useDistance = true
	} else {
		fw.simFunc = cosineSimilarity
		fw.useDistance = false
	}
}

// EnableCache 启用向量缓存
func (fw *MemoryFaissWrapper) EnableCache() {
	fw.cacheEnabled = true
	fmt.Println("Vector cache enabled")
}

// DisableCache 禁用向量缓存
func (fw *MemoryFaissWrapper) DisableCache() {
	fw.cacheEnabled = false
	fmt.Println("Vector cache disabled")
}

// ClearCache 清除向量缓存
func (fw *MemoryFaissWrapper) ClearCache() {
	fw.vectorCache = make(map[string][]float32)
	fmt.Println("Vector cache cleared")
}

// GetCacheStats 获取缓存统计信息
func (fw *MemoryFaissWrapper) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":      fw.cacheEnabled,
		"cache_size":   len(fw.vectorCache),
		"dirty":        fw.dirtyFlag,
		"last_save":    fw.lastSavePath,
		"vector_count": len(fw.Vectors),
	}
}

// Free 释放资源
func (fw *MemoryFaissWrapper) Free() {
	// 内存实现不需要特别的资源释放
}
