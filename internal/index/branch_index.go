package index

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// BranchIndexInfo 存储索引与代码分支关联的信息
type BranchIndexInfo struct {
	ID           int       // 主键ID
	BranchName   string    // 分支名称
	CommitHash   string    // 提交哈希值
	IndexedFiles string    // 已索引的文件列表，以逗号分隔
	IndexedAt    time.Time // 索引时间
}

// EnsureBranchIndexTable 确保branch_index表存在
func EnsureBranchIndexTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS branch_index (
		id INTEGER PRIMARY KEY,
		branch_name TEXT NOT NULL,
		commit_hash TEXT NOT NULL,
		indexed_files TEXT,
		indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_branch_commit ON branch_index(branch_name, commit_hash);
	`
	_, err := db.Exec(schema)
	return err
}

// SaveBranchIndexInfo 保存分支索引信息
func SaveBranchIndexInfo(db *sql.DB, info BranchIndexInfo) error {
	// 确保表存在
	if err := EnsureBranchIndexTable(db); err != nil {
		return fmt.Errorf("Failed to ensure branch_index table exists: %v", err)
	}

	// 插入记录
	query := `INSERT INTO branch_index(branch_name, commit_hash, indexed_files, indexed_at) VALUES(?, ?, ?, ?)`
	_, err := db.Exec(query, info.BranchName, info.CommitHash, info.IndexedFiles, info.IndexedAt)
	if err != nil {
		return fmt.Errorf("Failed to save branch index information: %v", err)
	}

	return nil
}

// GetLatestBranchIndexInfo 获取指定分支的最新索引信息
func GetLatestBranchIndexInfo(db *sql.DB, branchName string) (*BranchIndexInfo, error) {
	// 确保表存在
	if err := EnsureBranchIndexTable(db); err != nil {
		return nil, fmt.Errorf("Failed to ensure branch_index table exists: %v", err)
	}

	// 查询最新记录
	query := `SELECT id, branch_name, commit_hash, indexed_files, indexed_at FROM branch_index WHERE branch_name = ? ORDER BY indexed_at DESC LIMIT 1`
	row := db.QueryRow(query, branchName)

	var info BranchIndexInfo
	err := row.Scan(&info.ID, &info.BranchName, &info.CommitHash, &info.IndexedFiles, &info.IndexedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // 没有找到记录，返回nil
		}
		return nil, fmt.Errorf("Failed to query branch index information: %v", err)
	}

	return &info, nil
}

// GetIndexedFiles 获取已索引的文件列表
func (info *BranchIndexInfo) GetIndexedFiles() []string {
	if info.IndexedFiles == "" {
		return []string{}
	}
	return strings.Split(info.IndexedFiles, ",")
}

// SetIndexedFiles 设置已索引的文件列表
func (info *BranchIndexInfo) SetIndexedFiles(files []string) {
	info.IndexedFiles = strings.Join(files, ",")
}

// IsFileIndexed 检查文件是否已被索引
func (info *BranchIndexInfo) IsFileIndexed(filePath string) bool {
	files := info.GetIndexedFiles()
	for _, f := range files {
		if f == filePath {
			return true
		}
	}
	return false
}

// GetMissingFiles 获取未索引的文件列表
func (info *BranchIndexInfo) GetMissingFiles(allFiles []string) []string {
	indexedFiles := info.GetIndexedFiles()
	indexedMap := make(map[string]bool)
	for _, f := range indexedFiles {
		indexedMap[f] = true
	}

	var missingFiles []string
	for _, f := range allFiles {
		if !indexedMap[f] {
			missingFiles = append(missingFiles, f)
		}
	}

	return missingFiles
}

// DeleteBranchIndexInfo 删除指定分支的所有索引信息
func DeleteBranchIndexInfo(db *sql.DB, branchName string) error {
	// 确保表存在
	if err := EnsureBranchIndexTable(db); err != nil {
		return fmt.Errorf("Failed to ensure branch_index table exists: %v", err)
	}

	// 删除记录
	query := `DELETE FROM branch_index WHERE branch_name = ?`
	_, err := db.Exec(query, branchName)
	if err != nil {
		return fmt.Errorf("Failed to delete branch index information: %v", err)
	}

	log.Printf("All index information for branch %s has been deleted", branchName)
	return nil
}
