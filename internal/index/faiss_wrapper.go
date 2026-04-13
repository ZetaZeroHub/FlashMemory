package index

// FaissWrapper 定义了向量索引的标准接口
type FaissWrapper interface {
	// AddVector 添加一个向量到索引
	AddVector(funcID int, vector []float32) error

	// AddVectorsBatch 批量添加向量到索引
	AddVectorsBatch(funcIDs []int, vectors []float32) error

	// SearchVectors 查找最接近查询向量的topK个向量
	SearchVectors(query []float32, topK int) []int

	// AddModuleVector 添加一个代码模块的向量到模块集合索引
	AddModuleVector(modID int, vector []float32) error

	// SearchModuleVectors 查找与查询向量最接近的topK个模块向量
	SearchModuleVectors(query []float32, topK int) []int

	// SaveToFile 将索引保存到文件
	SaveToFile(path string) error

	// LoadFromFile 从文件加载索引
	LoadFromFile(path string) error

	// SetSimilarityMetric 设置相似度计算方法
	SetSimilarityMetric(metric string)

	// EnableCache 启用向量缓存
	EnableCache()

	// DisableCache 禁用向量缓存
	DisableCache()

	// ClearCache 清除向量缓存
	ClearCache()

	// GetCacheStats 获取缓存统计信息
	GetCacheStats() map[string]interface{}

	// Free 释放资源
	Free()

	Dimension() int

	GetScore(funcID int) float32
}
