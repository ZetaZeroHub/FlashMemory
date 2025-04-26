package main

import (
	_ "database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/visualize"
)

// BuildIndex 构建全量或指定目录索引
func BuildIndex(projDir string, full bool) error {
	// 如果全量，先删除旧索引
	if full {
		if err := DeleteIndex(projDir); err != nil {
			return fmt.Errorf("删除旧索引失败: %w", err)
		}
	}
	// 调用通用索引流程（不处理命令行只处理调用参数）
	return indexCode(projDir, "master", "", full, "")
}

// IncrementalUpdate 基于分支和 commit 增量更新索引
func IncrementalUpdate(projDir, branch, commit string) error {
	// 默认分支 master
	if branch == "" {
		branch = "master"
	}
	return indexCode(projDir, branch, commit, false, "")
}

// DeleteIndex 删除索引文件（.gitgo 目录下所有内容）
func DeleteIndex(projDir string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	return os.RemoveAll(gitgo)
}

// indexCode 内部通用索引逻辑，抽自 main.go 的流程
func indexCode(projDir, branchName, commitHash string, forceFull bool, filePath string) error {
	// 1. 启动或确认 FAISS service 已就绪
	if err := utils.CheckPythonEnvironment("cpu"); err != nil {
		return fmt.Errorf("Python环境检查失败: %w", err)
	}
	// 假设外部已启动 FaissService

	// 2. 创建 .gitgo 目录
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if err := os.MkdirAll(gitgoDir, 0755); err != nil {
		return fmt.Errorf("创建索引目录失败: %w", err)
	}

	// 3. 打开或创建索引数据库
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		return fmt.Errorf("初始化索引DB失败: %w", err)
	}
	defer db.Close()

	// 4. 解析代码函数
	var allFuncs []parser.FunctionInfo
	err = parser.WalkAndParse(projDir, func(fi parser.FunctionInfo) {
		allFuncs = append(allFuncs, fi)
	})
	if err != nil {
		return err
	}

	// 5. AI 分析函数
	llm := analyzer.NewLLMAnalyzer(nil, true, 3)
	results := llm.AnalyzeAll(allFuncs)

	// 6. 构建知识图谱
	kg := graph.BuildGraph(results)

	// 7. 保存分析结果到 SQLite
	idx := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, map[string]interface{}{
		"storage_path": gitgoDir,
		"index_id":     "code_index",
	})}
	if err := idx.SaveAnalysisToDB(results); err != nil {
		return fmt.Errorf("保存分析到DB失败: %w", err)
	}

	// 8. 增量或全量加载旧索引
	faissPath := filepath.Join(gitgoDir, "code_index.faiss")
	if !forceFull {
		_ = idx.FaissIndex.LoadFromFile(faissPath)
	}

	// 9. 插入或更新向量
	for id, res := range results {
		vec := search.SimpleEmbedding(res.Description, idx.FaissIndex.Dimension())
		n := id + 1
		idx.FaissIndex.AddVector(n, vec)
	}

	// 10. 保存到文件
	if err := idx.FaissIndex.SaveToFile(faissPath); err != nil {
		return fmt.Errorf("保存Faiss索引失败: %w", err)
	}

	// 11. 更新分支索引信息
	if err := index.EnsureBranchIndexTable(db); err == nil {
		if commitHash == "" {
			commitHash, _ = utils.GetCurrentBranchCommitHash(projDir)
		}
		bi := index.BranchIndexInfo{BranchName: branchName, CommitHash: commitHash, IndexedAt: time.Now()}
		bi.SetIndexedFiles([]string{}) // 可选记录实际文件列表
		_ = index.SaveBranchIndexInfo(db, bi)
	}

	// 12. （可选） 展示统计
	visualize.PrintStats(visualize.ComputePackageStats(kg))

	return nil
}
