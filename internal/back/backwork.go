package back

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/embedding"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/module_analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/kinglegendzzh/flashmemory/internal/visualize"

	_ "database/sql"
)

// GitBuildInfo 表示单次构建的git信息
type GitBuildInfo struct {
	BranchName   string           `json:"branch_name"`    // git分支名称
	CommitHash   string           `json:"commit_hash"`    // git最新hash
	CommitDate   string           `json:"commit_date"`    // git提交日期
	BuildDate    time.Time        `json:"build_date"`     // 构建日期
	Path         string           `json:"path,omitempty"` // 构建路径（相对路径），全路径构建时为空
	IndexedFiles int              `json:"indexed_files"`  // 索引的文件数量
	AllFuncs     int              `json:"all_funcs"`      // 索引的函数数量
	Type         GitBuildInfoType `json:"type"`           // 构建类型
	MetaData     string           `json:"meta_data"`      // 构建元数据
}

type GitBuildInfoType string

const (
	GitBuildInfoTypeFull GitBuildInfoType = "full"
	GitBuildInfoTypeAny  GitBuildInfoType = "any"
)

// GitInfoHistory 表示git信息历史记录
type GitInfoHistory struct {
	Latest  *GitBuildInfo  `json:"latest"`  // 当前最新构建信息
	History []GitBuildInfo `json:"history"` // 历史构建信息列表
}

// BuildIndex 构建全量或指定目录索引
func BuildIndex(projDir, subDir string, full bool, open bool) error {
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if e := os.MkdirAll(gitgoDir, 0755); e != nil {
		return fmt.Errorf("Failed to create index directory: %w", e)
	}
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	// 如果 temp 文件已存在，说明已有索引进程在运行
	if _, err := os.Stat(tempFilePath); err == nil {
		logs.Warnf("Indexing has been run, indexing skipped...")
	} else if !os.IsNotExist(err) {
		logs.Warnf("Index temporary file already exists, indexing skipped...")
	}

	// 创建临时文件，标记开始索引
	f, err := os.Create(tempFilePath)
	if err != nil {
		logs.Warnf("Failed to create index temporary file %q: %v", tempFilePath, err)
	}
	f.Close()
	fm, err := InitFaissManager(projDir, open)
	if err != nil {
		return fmt.Errorf("Failed to initialize FaissManager: %w", err)
	}
	// 如果需要全量，先 Reset（删除老索引）
	if full {
		if e := fm.Reset(); e != nil {
			return fmt.Errorf("Failed to reset Faiss index: %w", e)
		}
	}
	// 后续不再启动/停止服务，直接注入
	return indexCodeWithManager(fm, projDir, "master", "", full, subDir)
}

// IncrementalUpdate 增量更新索引
func IncrementalUpdate(projDir, branch, commit string, open bool) error {
	fm, err := InitFaissManager(projDir, open)
	if err != nil {
		return fmt.Errorf("Failed to initialize FaissManager: %w", err)
	}
	return indexCodeWithManager(fm, projDir, branch, commit, false, "")
}

// DeleteIndex 删除索引文件（.gitgo 目录下所有内容）
func DeleteIndex(projDir string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		logs.Warnf("Index directory does not exist: %s", gitgo)
		return nil // 无索引，直接返回
	}
	indexDBPath := filepath.Join(gitgo, "code_index.db")
	if _, err := os.Stat(indexDBPath); err == nil {
		logs.Infof("Delete index database %q", indexDBPath)
		db, err := index.EnsureIndexDB(projDir)
		if err == nil {
			defer db.Close()
			_, err = db.Exec("DELETE FROM functions")
			if err == nil {
				logs.Infof("Delete function index record successfully")
			}
			_, err = db.Exec("DELETE FROM calls")
			if err == nil {
				logs.Infof("Delete call index record successfully")
			}
			_, err = db.Exec("DELETE FROM externals")
			if err == nil {
				logs.Infof("Deletion of external index records successful")
			}
		} else {
			logs.Warnf("Failed to connect to index database: %v", err)
			return err
		}
	}
	err := ResetIndex(projDir, "")
	if err != nil {
		logs.Warnf("Failed to reset index: %v", err)
		return err
	}
	return nil
}

func RefreshFaiss(projDir string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}

	faissIndexPath := filepath.Join(gitgo, "code_index.faiss")
	faissIndexMetaPath := filepath.Join(gitgo, "code_index.faiss.meta")
	localIndexPath := filepath.Join(gitgo, "code_index.local")
	moduleFaissPath := filepath.Join(gitgo, "module.faiss")
	moduleFaissMetaPath := filepath.Join(gitgo, "module.faiss.meta")

	// 清除客户端缓存
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": projDir,
	})
	resp, err := httpClient.Post(index.DefaultFaissServerURL+"/delete_index", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		logs.Warnf("Dropping index failed, but ignored error: %v", err)
	}

	reqBody, _ = json.Marshal(map[string]interface{}{
		"index_id": fmt.Sprintf("%s_module", projDir),
	})
	resp, err = httpClient.Post(index.DefaultFaissServerURL+"/delete_index", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		logs.Warnf("Deleting module vector failed, but ignored error: %v", err)
	}
	defer httpClient.CloseIdleConnections()
	defer resp.Body.Close()

	os.Remove(faissIndexPath)
	os.Remove(faissIndexMetaPath)
	os.Remove(localIndexPath)
	os.Remove(moduleFaissPath)
	os.Remove(moduleFaissMetaPath)
	return nil
}

func ResetIndex(projDir, subPath string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	faissIndexPath := filepath.Join(gitgo, "code_index.faiss")
	faissIndexMetaPath := filepath.Join(gitgo, "code_index.faiss.meta")
	localIndexPath := filepath.Join(gitgo, "code_index.local")
	tempPath := filepath.Join(gitgo, "indexing.temp")
	fullIndexTemp := filepath.Join(gitgo, "full_index.temp")
	graphPath := filepath.Join(gitgo, "graph.json")
	moduleFaissPath := filepath.Join(gitgo, "module.faiss")
	moduleFaissMetaPath := filepath.Join(gitgo, "module.faiss.meta")
	// 清除客户端缓存
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": projDir,
	})
	resp, err := httpClient.Post(index.DefaultFaissServerURL+"/delete_index", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		logs.Warnf("Dropping index failed, but ignored error: %v", err)
	}
	defer httpClient.CloseIdleConnections()
	defer resp.Body.Close()
	os.Remove(faissIndexPath)
	os.Remove(faissIndexMetaPath)
	os.Remove(localIndexPath)
	os.Remove(tempPath)
	os.Remove(fullIndexTemp)
	os.Remove(graphPath)
	os.Remove(moduleFaissPath)
	os.Remove(moduleFaissMetaPath)
	return nil
}

func DeleteSomeIndex(projDir string, subPath string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	indexDBPath := filepath.Join(gitgo, "code_index.db")
	if subPath != "" {
		// 检查数据库文件是否存在
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			logs.Warnf("Index database does not exist: %s", indexDBPath)
			return nil // 数据库不存在，直接返回
		}

		// 打开数据库连接
		db, err := index.EnsureIndexDB(projDir)
		if err != nil {
			return fmt.Errorf("Failed to open index database: %w", err)
		}
		defer db.Close()
		// 标准化子路径（确保使用正确的路径分隔符，文件不加/，目录加/）
		normalizedSubPath := filepath.ToSlash(subPath)
		fileInfo, statErr := os.Stat(filepath.Join(projDir, normalizedSubPath))
		isDir := false
		if statErr == nil && fileInfo.IsDir() {
			isDir = true
		}
		if isDir && !strings.HasSuffix(normalizedSubPath, "/") {
			normalizedSubPath += "/"
		}

		// 执行删除操作
		var query string
		var pattern string
		if isDir {
			query = "DELETE FROM functions WHERE file LIKE ?"
			pattern = normalizedSubPath + "%"
		} else {
			query = "DELETE FROM functions WHERE file = ?"
			pattern = normalizedSubPath
		}

		result, err := db.Exec(query, pattern)
		if err != nil {
			return fmt.Errorf("Failed to delete subpath index record: %w", err)
		}

		// 获取受影响的行数
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			logs.Warnf("Failed to get the number of deleted rows: %v", err)
		} else {
			logs.Infof("Successfully deleted %d index records for subpath '%s'", pattern, rowsAffected)
		}

		// todo 删除externals和calls
	}
	return nil
}

func DeleteSomeModuleDesc(projDir string, subPath string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	indexDBPath := filepath.Join(gitgo, "code_index.db")
	if subPath != "" {
		// 检查数据库文件是否存在
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			logs.Warnf("Index database does not exist: %s", indexDBPath)
			return nil // 数据库不存在，直接返回
		}

		// 打开数据库连接
		db, err := index.EnsureIndexDB(projDir)
		if err != nil {
			return fmt.Errorf("Failed to open index database: %w", err)
		}
		defer db.Close()

		// 标准化子路径（确保使用正确的路径分隔符）
		normalizedSubPath := filepath.ToSlash(subPath)
		if !strings.HasSuffix(normalizedSubPath, "/") {
			normalizedSubPath += "/"
		}

		// 执行删除操作
		query := "DELETE FROM code_desc WHERE path LIKE ?"
		pattern := normalizedSubPath + "%"

		result, err := db.Exec(query, pattern)
		if err != nil {
			return fmt.Errorf("Failed to delete subpath index record: %w", err)
		}

		// 获取受影响的行数
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			logs.Warnf("Failed to get the number of deleted rows: %v", err)
		} else {
			logs.Infof("Successfully deleted %d index records for subpath '%s'", subPath, rowsAffected)
		}
	}
	return nil
}

func ResetModuleDesc(projDir string) error {
	moduleGraphPath := filepath.Join(projDir, ".gitgo", "module_graphs")
	moduleAnalyzerTempPath := filepath.Join(projDir, ".gitgo", "module_analyzer.temp")
	os.Remove(moduleGraphPath)
	os.Remove(moduleAnalyzerTempPath)
	return nil
}

func DeleteModuleDesc(projDir string) error {
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		logs.Warnf("Failed to open index database: %v", err)
		return err
	}
	defer db.Close()
	_, err = db.Exec("DELETE FROM code_desc")
	if err == nil {
		logs.Infof("Deletion of module analysis record successful")
	}
	ResetModuleDesc(projDir)
	return err
}

// SaveGitBuildInfo 保存git构建信息到.gitgo/info.json
// 参数:
//   - projDir: 项目目录路径
//   - subPath: 子路径（相对路径），全路径构建时为空字符串
//   - indexedFiles: 索引的文件数量
//
// 返回:
//   - error: 错误信息
func SaveGitBuildInfo(projDir string, subPath string, indexedFiles, allFuncs int, type_ GitBuildInfoType) error {
	// 初始化git信息变量
	var branchName, commitHash, commitDate string

	// 检查是否为git仓库并获取git信息
	if utils.IsGitRepository(projDir) {
		var err error
		branchName, err = utils.GetCurrentBranchName(projDir)
		if err != nil {
			log.Printf("Failed to get current branch name: %v, null value will be used", err)
		}

		commitHash, err = utils.GetCurrentBranchCommitHash(projDir)
		if err != nil {
			log.Printf("Failed to obtain current commit hash: %v, null value will be used", err)
		}

		if commitHash != "" {
			commitDate, err = utils.GetCommitDate(projDir, commitHash)
			if err != nil {
				log.Printf("Failed to get commit date: %v, null value will be used", err)
			}
		}
		log.Printf("git repository detected, branch: %s, commit: %s", branchName, commitHash)
	} else {
		log.Println("The current project is not a git warehouse, and git related fields will be empty.")
	}

	// 创建当前构建信息
	currentBuildInfo := GitBuildInfo{
		BranchName:   branchName,
		CommitHash:   commitHash,
		CommitDate:   commitDate,
		BuildDate:    time.Now(),
		Path:         subPath,
		IndexedFiles: indexedFiles,
		AllFuncs:     allFuncs,
		Type:         type_,
	}

	// 确保.gitgo目录存在
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if err := os.MkdirAll(gitgoDir, 0755); err != nil {
		return fmt.Errorf("Failed to create .gitgo directory: %w", err)
	}

	// info.json文件路径
	infoFilePath := filepath.Join(gitgoDir, "info.json")

	// 读取现有的git信息历史
	var gitHistory GitInfoHistory
	if data, err := os.ReadFile(infoFilePath); err == nil {
		if err := json.Unmarshal(data, &gitHistory); err != nil {
			log.Printf("Failed to parse existing git information, new record will be created: %v", err)
			gitHistory = GitInfoHistory{}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Failed to read git information file: %w", err)
	}

	// 如果存在最新记录，将其移动到历史记录中
	if gitHistory.Latest != nil {
		gitHistory.History = append(gitHistory.History, *gitHistory.Latest)
	}

	// 更新最新记录
	gitHistory.Latest = &currentBuildInfo

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(gitHistory, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize git information: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(infoFilePath, jsonData, 0644); err != nil {
		return fmt.Errorf("Failed to write git information file: %w", err)
	}

	if branchName != "" && commitHash != "" {
		log.Printf("Successfully saved build information to %s (branch: %s, commit: %s, index files: %d)", infoFilePath, branchName, commitHash[:8], indexedFiles)
	} else {
		log.Printf("Successfully saved build information to %s (non-git repository, index files: %d)", infoFilePath, indexedFiles)
	}
	return nil
}

// indexCode 内部通用索引逻辑，抽自 main.go 的流程
func indexCodeWithManager(fm *FaissManager, projDir, branchName, commitHash string, forceFull bool, filePath string) error {
	ext := ".local"
	if fm.faissState {
		ext = ".faiss"
	}
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if e := os.MkdirAll(gitgoDir, 0755); e != nil {
		return fmt.Errorf("Failed to create index directory: %w", e)
	}
	// 索引文件路径
	indexDBPath := filepath.Join(gitgoDir, "code_index.db")
	faissIndexPath := filepath.Join(gitgoDir, "code_index"+ext)

	log.Println("Indexing code files...")
	os.MkdirAll(gitgoDir, 0755)

	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	// 如果 temp 文件已存在，说明已有索引进程在运行
	if _, err := os.Stat(tempFilePath); err == nil {
		logs.Warnf("Indexing has been run, indexing skipped...")
	} else if !os.IsNotExist(err) {
		logs.Warnf("Index temporary file already exists, indexing skipped...")
	}

	// 创建临时文件，标记开始索引
	f, err := os.Create(tempFilePath)
	if err != nil {
		logs.Warnf("Failed to create index temporary file %q: %v", tempFilePath, err)
	}
	f.Close()

	// 确保退出时删除临时文件
	defer func() {
		if err := os.Remove(tempFilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to delete index temporary file %q: %v", tempFilePath, err)
		}
		logs.Infof("Indexing completed, index temporary file %q deleted", tempFilePath)
	}()

	// 强制清理全局索引临时文件
	fullIndexTemp := filepath.Join(gitgoDir, "full_index.temp")
	if _, err := os.Stat(fullIndexTemp); err == nil {
		if err := os.Remove(fullIndexTemp); err != nil && !os.IsNotExist(err) {
			logs.Warnf("Failed to delete global index temporary files %q: %v", fullIndexTemp, err)
		}
	}

	// 检查是否存在索引文件，如果存在则进行增量更新
	var filesToProcess []string
	var incrementalUpdate bool

	// 检查索引文件是否存在
	dbExists := false
	if _, err := os.Stat(indexDBPath); err == nil {
		if _, err := os.Stat(faissIndexPath); err == nil {
			dbExists = true
			log.Println("Existing index file detected, incremental update will be done")
		}
	}

	// 如果强制全量索引，则跳过增量更新逻辑
	if forceFull {
		log.Println("Forced full indexing has been specified, incremental updates will be ignored")
		dbExists = false
	}

	if dbExists {
		// 检查.git目录是否存在
		gitDir := filepath.Join(projDir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			log.Println(".git directory detected, parsing commit records for change files...")

			// 打开数据库，准备查询和更新
			db, err := index.EnsureIndexDB(projDir)
			if err != nil {
				return err
			}
			defer db.Close()

			// 确保branch_index表存在
			if err := index.EnsureBranchIndexTable(db); err != nil {
				log.Printf("Failed to ensure branch_index table exists: %v, full index will be performed", err)
			} else {
				// 获取最新的分支索引信息
				branchInfo, err := index.GetLatestBranchIndexInfo(db, branchName)
				if err != nil {
					log.Printf("Failed to obtain branch index information: %v, full index will be performed", err)
				} else {
					// 获取当前commit hash或使用指定的commit hash
					currentCommitHash := commitHash
					if currentCommitHash == "" {
						// 使用git工具函数获取当前分支的最新commit hash
						currentCommitHash, err = utils.GetCurrentBranchCommitHash(projDir)
						if err != nil {
							log.Printf("Failed to obtain commit hash of current branch: %v, full index will be performed", err)
							currentCommitHash = ""
						}
					}

					if currentCommitHash != "" {
						if branchInfo == nil {
							// 没有找到分支索引信息，获取当前commit的变更文件
							log.Printf("The branch index information was not found. Get the change file of the current branch commit %s...", currentCommitHash)
							changedFiles, err := utils.GetChangedFilesByCommitHash(projDir, currentCommitHash)
							if err != nil {
								log.Printf("Failed to obtain the changed files of commit %s: %v, full index will be performed", currentCommitHash, err)
							} else if len(changedFiles) > 0 {
								log.Printf("%d changed files detected", len(changedFiles))

								// 删除这些文件的索引记录
								for _, file := range changedFiles {
									_, err := db.Exec("DELETE FROM functions WHERE file = ?", file)
									if err != nil {
										log.Printf("Failed to delete index record for file %s: %v", file, err)
									} else {
										log.Printf("Index record deleted for file %s", file)
									}
								}

								// 只处理变更的文件
								filesToProcess = changedFiles
								incrementalUpdate = true
							} else {
								log.Println("No changed files were detected and will be fully indexed.")
							}
						} else {
							// 找到了分支索引信息，获取两个commit之间的变更文件
							log.Printf("Retrieving change files between commit %s and %s...", branchInfo.CommitHash, currentCommitHash)
							log.Printf("Find the index information of branch %s, the commit of the last index: %s", branchInfo.BranchName, branchInfo.CommitHash)

							if branchInfo.CommitHash != currentCommitHash {
								changedFiles, err := utils.GetChangedFilesBetweenCommits(projDir, branchInfo.CommitHash, currentCommitHash)
								if err != nil {
									log.Printf("Failed to obtain changed files between commit %s and %s: %v, full index will be performed", branchInfo.CommitHash, currentCommitHash, err)
								} else if len(changedFiles) > 0 {
									log.Printf("%d changed files detected", len(changedFiles))

									// 删除这些文件的索引记录
									for _, file := range changedFiles {
										_, err := db.Exec("DELETE FROM functions WHERE file = ?", file)
										if err != nil {
											log.Printf("Failed to delete index record for file %s: %v", file, err)
										} else {
											log.Printf("Index record deleted for file %s", file)
										}
									}

									// 获取已索引的文件列表
									indexedFiles := branchInfo.GetIndexedFiles()

									// 合并已索引的文件和新变更的文件
									allIndexedFiles := make(map[string]bool)
									for _, file := range indexedFiles {
										allIndexedFiles[file] = true
									}

									// 移除已变更的文件（因为它们需要重新索引）
									for _, file := range changedFiles {
										delete(allIndexedFiles, file)
									}

									// 检查是否有缺失的文件（例如新增的文件）
									var allFiles []string
									err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
										if err != nil {
											return err
										}
										// 跳过exclude.json中指定的路径
										fullWalkPath := filepath.Join(projDir, path)
										excludeFile := filepath.Join(gitgoDir, "exclude.json")
										jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
										if utils.IsExcludedPath(fullWalkPath, jsonFile) {
											log.Printf("Skip specified file: %s", fullWalkPath)
											return filepath.SkipDir
										}
										if info.IsDir() {
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
											allFiles = append(allFiles, path)
										}
										return nil
									})

									if err != nil {
										log.Printf("Failed to traverse project directory: %v, only change files will be processed", err)
									} else {
										// 找出未索引的文件
										var missingFiles []string
										for _, file := range allFiles {
											if !allIndexedFiles[file] && utils.Contains(changedFiles, file) {
												log.Printf("--- File %s is not indexed, will be processed", file)
												missingFiles = append(missingFiles, file)
											}
										}

										if len(missingFiles) > 0 {
											log.Printf("%d unindexed files detected and will be processed together", len(missingFiles))
											filesToProcess = append(filesToProcess, missingFiles...)
										}
									}

									incrementalUpdate = true
								} else {
									log.Println("No changed files detected, will check for unindexed files")

									// 检查是否有未索引的文件
									var allFiles []string
									err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
										if err != nil {
											return err
										}
										// 跳过exclude.json中指定的路径
										fullWalkPath := filepath.Join(projDir, path)
										excludeFile := filepath.Join(gitgoDir, "exclude.json")
										jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
										if utils.IsExcludedPath(fullWalkPath, jsonFile) {
											log.Printf("Skip specified directory: %s", fullWalkPath)
											return filepath.SkipDir
										}
										if info.IsDir() {
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
											allFiles = append(allFiles, path)
										}
										return nil
									})

									if err != nil {
										log.Printf("Failed to traverse the project directory: %v, full index will be performed", err)
									} else {
										// 获取已索引的文件列表
										indexedFiles := branchInfo.GetIndexedFiles()
										indexedMap := make(map[string]bool)
										for _, file := range indexedFiles {
											indexedMap[file] = true
										}

										// 找出未索引的文件
										var missingFiles []string
										for _, file := range allFiles {
											if !indexedMap[file] {
												missingFiles = append(missingFiles, file)
											}
										}

										if len(missingFiles) > 0 {
											log.Printf("%d unindexed files detected and will be processed", len(missingFiles))
											filesToProcess = missingFiles
											incrementalUpdate = true
										} else {
											log.Println("All files are indexed and no updates are needed")
										}
									}
								}
							} else {
								log.Printf("The current commit %s has been indexed, check whether there are unindexed files.", currentCommitHash)

								// 检查是否有未索引的文件
								var allFiles []string
								err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
									if err != nil {
										return err
									}
									// 跳过exclude.json中指定的路径
									fullWalkPath := filepath.Join(projDir, path)
									excludeFile := filepath.Join(gitgoDir, "exclude.json")
									jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
									if utils.IsExcludedPath(fullWalkPath, jsonFile) {
										log.Printf("Skip specified directory: %s", fullWalkPath)
										return filepath.SkipDir
									}
									if info.IsDir() {
										// 跳过以点开头的隐藏目录
										if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
											return filepath.SkipDir
										}
										return nil
									}
									// 仅考虑特定的文件扩展名
									ext := filepath.Ext(path)
									if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
										allFiles = append(allFiles, path)
									}
									return nil
								})

								if err != nil {
									log.Printf("Failed to traverse the project directory: %v, full index will be performed", err)
								} else {
									// 获取已索引的文件列表
									indexedFiles := branchInfo.GetIndexedFiles()
									indexedMap := make(map[string]bool)
									for _, file := range indexedFiles {
										indexedMap[file] = true
									}

									// 找出未索引的文件
									var missingFiles []string
									for _, file := range allFiles {
										if !indexedMap[file] {
											missingFiles = append(missingFiles, file)
										}
									}

									if len(missingFiles) > 0 {
										log.Printf("%d unindexed files detected and will be processed", len(missingFiles))
										filesToProcess = missingFiles
										incrementalUpdate = true
									} else {
										log.Println("All files are indexed and no updates are needed")
									}
								}
							}
						}
					}
				}
			}
		} else {
			log.Println("The .git directory does not exist and the change information cannot be obtained. Full indexing will be performed.")
		}
	} else {
		log.Println("The index file does not exist or forced full indexing has been specified. Full indexing will be performed.")
	}

	log.Printf("Tag: %v, file to be indexed: %s", incrementalUpdate, filesToProcess)

	// 如果是强制全量索引，则删除分支索引信息
	if forceFull {
		log.Printf("Force full indexing and delete the index information of branch %s", branchName)
		db, err := index.EnsureIndexDB(projDir)
		if err == nil {
			defer db.Close()
			err = index.DeleteBranchIndexInfo(db, branchName)
			if err != nil {
				log.Printf("Failed to delete index information for branch %s: %v", branchName, err)
			}
		}
	}

	// 3. 打开或创建索引数据库
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		return fmt.Errorf("Failed to initialize index DB: %w", err)
	}
	defer db.Close()

	// 1. 遍历项目目录以查找代码文件
	var files []string
	if filePath != "" {
		log.Println("Selective update mode: files/folders specified using the -file parameter")
		fullPath := filepath.Join(projDir, filePath)
		info, err := os.Stat(fullPath)
		if err != nil {
			return err
		}
		// 打开数据库，准备删除旧记录
		db, err := index.EnsureIndexDB(projDir)
		if err != nil {
			return err
		}
		defer db.Close()
		if info.IsDir() {
			err = filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// 跳过exclude.json中指定的路径
				fullWalkPath := filepath.Join(fullPath, path)
				excludeFile := filepath.Join(gitgoDir, "exclude.json")
				jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
				if utils.IsExcludedPath(fullWalkPath, jsonFile) {
					log.Printf("Skip specified directory: %s", fullWalkPath)
					return filepath.SkipDir
				}
				if info.IsDir() {
					if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
						return filepath.SkipDir
					}
					return nil
				}
				ext := filepath.Ext(path)
				if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
					// 删除该文件的旧索引记录
					relPath, err := filepath.Rel(projDir, path)
					if err != nil {
						fmt.Errorf("Failed to get relative path of %s relative to %s: %w", fullPath, projDir, err)
					}
					relPath = filepath.ToSlash(relPath)
					n, err := db.Exec("DELETE FROM functions WHERE file like ?", relPath+"%")
					if err != nil {
						log.Printf("Failed to delete index record for file %s: %v", path, err)
					} else {
						log.Printf("The index record for file %s(%s) has been deleted, %s", path, relPath, n)
					}
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else {
			// 删除该文件的旧索引记录
			n, err := db.Exec("DELETE FROM functions WHERE file like ?", filePath+"%")
			if err != nil {
				log.Printf("Failed to delete index record for file %s: %v", filePath, err)
			} else {
				log.Printf("Index record deleted for file %s, %s", filePath, n)
			}
			files = append(files, filePath)
		}
	} else if incrementalUpdate && len(filesToProcess) > 0 {
		// 增量更新模式：只处理变更文件
		log.Println("Incremental update mode: only process changed files")
		log.Printf("Change file list: %v", strings.Join(filesToProcess, ";"))
		for _, file := range filesToProcess {
			// 获取文件的绝对路径
			absPath := filepath.Join(projDir, file)
			// 检查文件是否存在
			if _, err := os.Stat(absPath); err == nil {
				// 检查文件扩展名
				ext := filepath.Ext(absPath)
				if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(absPath, "__init__.py") {
					files = append(files, absPath)
				}
			}
		}
	} else if forceFull {
		// 全量索引模式：遍历整个项目目录
		log.Println("Full index mode: traverse the entire project directory")
		err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// 跳过exclude.json中指定的路径
			fullWalkPath := filepath.Join(projDir, path)
			excludeFile := filepath.Join(gitgoDir, "exclude.json")
			jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
			if utils.IsExcludedPath(fullWalkPath, jsonFile) {
				log.Printf("Skip specified file: %s", fullWalkPath)
				return filepath.SkipDir
			}
			if info.IsDir() {
				// 跳过以点开头的隐藏目录
				if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
					logs.Warnf("Skip directory: %s or %s", info.Name(), fullWalkPath)
					return filepath.SkipDir
				}
				return nil
			}
			// 仅考虑特定的文件扩展名
			ext := filepath.Ext(path)
			if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
				files = append(files, path)
			}
			return nil
		})
	}
	if err != nil {
		return err
	}
	if len(files) == 0 {
		log.Println("No source code files found in the provided directory.")
		return nil
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		logs.Errorf("Failed to load configuration file: %v", err)
		return err
	}
	// 2. 并发解析所有文件
	var allFuncs []parser.FunctionInfo
	var mu sync.Mutex
	var wg sync.WaitGroup
	concurrency := cfg.DefaultMaxWorker
	logs.Infof("Indexing %d files with %d concurrency", len(files), concurrency)
	fileChan := make(chan string, len(files))

	// 将文件放入channel
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)
	// 用于捕获第一个错误
	var firstErr error
	var firstErrOnce sync.Once

	// 启动并发worker
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				lang := parser.DetectLang(file)
				if lang == "" {
					continue
				}
				p := parser.NewParserDb(lang, db, projDir)
				funcs, err := p.ParseFile(file)
				if err != nil {
					firstErrOnce.Do(func() {
						firstErr = err
					})
					logs.Errorf("Error parsing file %s: %v\n", file, err)
					return
				}
				mu.Lock()
				allFuncs = append(allFuncs, funcs...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Printf("Parsed %d files, extracted %d functions.\n", len(files), len(allFuncs))

	if firstErr != nil {
		logs.Errorf("Error parsing files: %v", firstErr)
		return firstErr
	}

	log.Println("Analyzing code...")
	// 3. 分析函数（考虑依赖关系顺序）
	llmAnalyzer := analyzer.NewLLMAnalyzerHttp(&sync.Map{}, true, cfg.DefaultMaxWorker, db, projDir)

	// 添加批处理间隔，帮助连接池清理
	log.Printf("Starting analysis of %d functions, which will be processed in batches to optimize network connectivity...", len(allFuncs))
	time.Sleep(2 * time.Second) // 给连接池一些清理时间

	results, err := llmAnalyzer.AnalyzeAll(allFuncs)
	if err != nil {
		logs.Errorf("Error analyzing functions: %v", err)
		return err
	}
	fmt.Printf("Analyzed %d functions with AI summaries.\n", len(results))

	// 3.1 进行模块级分析（文件/目录级别）
	// 创建一个新的数据库连接，专门用于模块分析
	dbPath := filepath.Join(projDir, ".gitgo", "code_index.db")
	moduleDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("Failed to open module to analyze database: %v", err)
	} else {
		// 确保数据库连接可用
		if err := moduleDB.Ping(); err != nil {
			log.Printf("Module analysis database connection test failed: %v", err)
		} else {
			// 使用新的数据库连接进行后台分析
			go func(results []analyzer.LLMAnalysisResult, moduleDB *sql.DB, projDir string, cfg *config.Config) {
				defer moduleDB.Close() // 分析完成后关闭这个专用连接
				log.Println("Module level analysis (file/directory level) in progress, running asynchronously in the background...")
				// 默认不跳过LLM描述生成
				skipLLM := false
				if err := module_analyzer.AnalyzeAllModules(results, moduleDB, projDir, cfg, skipLLM, ""); err != nil {
					log.Printf("Module level analysis failed: %v", err)
				} else {
					log.Println("Module level analysis completed")
				}
			}(results, moduleDB, projDir, cfg)
		}
	}

	log.Println("Indexing code...")
	// 5. 初始化索引存储（SQLite和Faiss）

	// 确保storage_path是绝对路径
	absGitgoDir, err := filepath.Abs(gitgoDir)
	if err != nil {
		return err
	}

	idx := fm.Indexer
	err = idx.SaveAnalysisToDBHttp(results, projDir)
	if err != nil {
		return err
	}
	// 构建嵌入向量并添加到Faiss索引
	if incrementalUpdate {
		// 增量更新模式：先加载现有索引
		err = idx.FaissIndex.LoadFromFile(fm.faissDir)
		if err != nil {
			log.Printf("Failed to load existing Faiss index: %v, new index will be created", err)
		} else {
			log.Println("Successfully loaded existing Faiss index")
		}
	}

	err = embedding.EnsureEmbeddingsBatch(idx)
	if err != nil {
		log.Printf("Failed to add vector to function: %v", err)
		return err
	}

	// 保存索引到文件
	err = idx.FaissIndex.SaveToFile(absGitgoDir + "/code_index" + ext)
	if err != nil {
		log.Printf("Failed to save Faiss index: %v", err)
	} else {
		log.Println("Successfully saved Faiss index")

		// 保存分支索引信息
		if commitHash != "" {
			// 获取当前commit hash
			currentCommitHash := commitHash
			if currentCommitHash == "" {
				var err error
				currentCommitHash, err = utils.GetCurrentBranchCommitHash(projDir)
				if err != nil {
					log.Printf("Failed to obtain current branch commit hash: %v, unable to save branch index information", err)
				}
			}

			if currentCommitHash != "" {
				// 获取所有已处理的文件列表
				var processedFiles []string
				if incrementalUpdate && len(filesToProcess) > 0 {
					processedFiles = filesToProcess
				} else {
					// 全量索引模式，获取所有文件
					processedFiles = files
				}

				// 创建分支索引信息
				branchInfo := index.BranchIndexInfo{
					BranchName: branchName,
					CommitHash: currentCommitHash,
					IndexedAt:  time.Now(),
				}
				branchInfo.SetIndexedFiles(processedFiles)

				// 保存到数据库
				err := index.SaveBranchIndexInfo(db, branchInfo)
				if err != nil {
					log.Printf("Failed to save branch index information: %v", err)
				} else {
					log.Printf("The index information of branch %s (commit: %s) was successfully saved, and a total of %d files were indexed.",
						branchInfo.BranchName, branchInfo.CommitHash, len(processedFiles))
				}
			}
		}
	}

	log.Println("Building knowledge graph...")
	// 构建知识图谱
	kg := graph.BuildGraph(results, projDir)
	// （可选）保存图结构用于调试
	err = kg.SaveGraphJSON(filepath.Join(projDir, ".gitgo", "graph.json"))
	if err != nil {
		logs.Errorf("Failed to save graph structure: %v", err)
		return err
	}
	// 12. （可选） 展示统计
	//visualize.PrintStats(visualize.ComputePackageStats(kg))

	// 13. 保存git信息到.gitgo/info.json
	if err := SaveGitBuildInfo(projDir, filePath, len(files), len(allFuncs), GitBuildInfoTypeFull); err != nil {
		log.Printf("Failed to save git build information: %v", err)
	}
	// 创建全量索引标记
	fullIndexFile, err := os.Create(fullIndexTemp)
	if err != nil {
		logs.Warnf("Failed to create full index mark %q: %v", fullIndexTemp, err)
	}
	fullIndexFile.Close()
	logs.Infof("Indexing completed, full index mark %q has been created", fullIndexTemp)
	//fm.Stop()
	return nil
}

func ListGraph(projDir, subPath string) (err error) {
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if e := os.MkdirAll(gitgoDir, 0755); e != nil {
		return fmt.Errorf("Failed to create index directory: %w", e)
	}
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	// 如果 temp 文件已存在，说明已有索引进程在运行
	if _, err := os.Stat(tempFilePath); err == nil {
		logs.Warnf("Indexing has been run, indexing skipped...")
	} else if !os.IsNotExist(err) {
		logs.Warnf("Index temporary file already exists, indexing skipped...")
	}

	// 创建临时文件，标记开始索引
	f, err := os.Create(tempFilePath)
	if err != nil {
		logs.Warnf("Failed to create index temporary file %q: %v", tempFilePath, err)
	}
	f.Close()
	// 确保退出时删除临时文件
	defer func() {
		if err := os.Remove(tempFilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to delete index temporary file %q: %v", tempFilePath, err)
		}
		logs.Infof("Indexing completed, index temporary file %q deleted", tempFilePath)
	}()
	var files []string
	var totalPath string
	// 如果提供了子路径，则更新 projDir 路径
	totalPath = filepath.Join(projDir, subPath)
	logs.Infof("Update projDir + subPath path to: %s", totalPath)

	// 全量索引模式：遍历整个项目目录，或者子路径中的文件
	log.Printf("Traversal path: %s", totalPath)
	err = filepath.Walk(totalPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 跳过exclude.json中指定的路径
		fullWalkPath := filepath.Join(totalPath, path)
		excludeFile := filepath.Join(projDir, ".gitgo", "exclude.json")
		jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
		if utils.IsExcludedPath(fullWalkPath, jsonFile) {
			log.Printf("Skip specified file: %s", fullWalkPath)
			return filepath.SkipDir
		}
		// 如果是目录，进行排除处理
		if info.IsDir() {
			// 跳过以点开头的隐藏目录
			if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
				logs.Warnf("Skip directory: %s or %s", info.Name(), fullWalkPath)
				return filepath.SkipDir
			}
			return nil
		}
		// 仅考虑特定的文件扩展名
		ext := filepath.Ext(path)
		if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("An error occurred while traversing the directory: %w", err)
	}

	// 初始化索引DB
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		return fmt.Errorf("Failed to initialize index DB: %w", err)
	}
	defer db.Close()
	cfg, err := config.LoadConfig()
	if err != nil {
		logs.Errorf("Failed to load configuration file: %v", err)
		return err
	}
	var allFuncs []parser.FunctionInfo
	var mu sync.Mutex
	var wg sync.WaitGroup
	concurrency := 1
	logs.Infof("Indexing %d files with %d concurrency", len(files), concurrency)
	fileChan := make(chan string, len(files))

	// 将文件放入channel
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)
	// 用于捕获第一个错误
	var firstErr error
	var firstErrOnce sync.Once

	// 启动并发worker进行文件解析
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				lang := parser.DetectLang(file)
				if lang == "" {
					continue
				}
				p := parser.NewParserDb(lang, db, projDir)
				funcs, err := p.ParseFile(file)
				if err != nil {
					firstErrOnce.Do(func() {
						firstErr = err
					})
					logs.Errorf("Error parsing file %s: %v\n", file, err)
					return
				}
				mu.Lock()
				allFuncs = append(allFuncs, funcs...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Printf("Parsed %d files, extracted %d functions.\n", len(files), len(allFuncs))

	if firstErr != nil {
		logs.Errorf("Error parsing files: %v", firstErr)
		return firstErr
	}

	log.Println("Analyzing code...")
	// 3. 分析函数（考虑依赖关系顺序）
	llmAnalyzer := analyzer.NewLLMAnalyzerHttp(&sync.Map{}, true, cfg.DefaultMaxWorker, db, projDir)

	// 添加批处理间隔，帮助连接池清理
	log.Printf("Starting analysis of %d functions, which will be processed in batches to optimize network connectivity...", len(allFuncs))
	time.Sleep(2 * time.Second) // 给连接池一些清理时间

	results, err := llmAnalyzer.AnalyzeAll(allFuncs)
	if err != nil {
		logs.Errorf("Error analyzing functions: %v", err)
		return err
	}
	fmt.Printf("Analyzed %d functions with AI summaries.\n", len(results))

	// 3.1 进行模块级分析（文件/目录级别）
	// 创建一个新的数据库连接，专门用于模块分析
	dbPath := filepath.Join(projDir, ".gitgo", "code_index.db")
	moduleDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("Failed to open module to analyze database: %v", err)
	} else {
		// 确保数据库连接可用
		if err := moduleDB.Ping(); err != nil {
			log.Printf("Module analysis database connection test failed: %v", err)
		} else {
			// 使用新的数据库连接进行后台分析
			go func(results []analyzer.LLMAnalysisResult, moduleDB *sql.DB, projDir string, cfg *config.Config, subPath string) {
				defer moduleDB.Close() // 分析完成后关闭这个专用连接
				log.Println("Module level analysis (file/directory level) in progress, running asynchronously in the background...")
				// 默认不跳过LLM描述生成
				skipLLM := false
				if err := module_analyzer.AnalyzeAllModules(results, moduleDB, projDir, cfg, skipLLM, subPath); err != nil {
					log.Printf("Module level analysis failed: %v", err)
				} else {
					log.Println("Module level analysis completed")
				}
			}(results, moduleDB, projDir, cfg, subPath)
		}
	}

	log.Println("Building knowledge graph...")

	// 4. 构建知识图谱
	kg := graph.BuildGraph(results, projDir)
	// （可选）保存图结构用于调试
	err = kg.SaveGraphJSONOnlyFunctions(filepath.Join(projDir, ".gitgo", "graph.json"))
	// 12. （可选） 展示统计
	visualize.PrintStats(visualize.ComputePackageStats(kg))

	// 13. 保存git信息到.gitgo/info.json
	if err := SaveGitBuildInfo(projDir, subPath, len(files), len(allFuncs), GitBuildInfoTypeAny); err != nil {
		log.Printf("Failed to save git build information: %v", err)
	}
	return err
}

func MakeExcludeFile(projDir string, exclude []string) (err error) {
	excludeFile := filepath.Join(projDir, ".gitgo", "exclude.json")
	// 自动创建文件夹
	err = os.MkdirAll(filepath.Dir(excludeFile), 0755)
	if err != nil {
		logs.Warnf("Failed to create folder %q: %v", filepath.Dir(excludeFile), err)
		return err
	}
	// 如果不存在exclude.json，则生成文件
	if _, err := os.Stat(excludeFile); os.IsNotExist(err) {
		file, err := os.Create(excludeFile)
		if err != nil {
			logs.Warnf("Failed to create exclude.json file %q: %v", excludeFile, err)
			return err
		}
		defer file.Close()
	}
	data, err := json.MarshalIndent(exclude, "", "  ")
	if err != nil {
		logs.Warnf("Failed to serialize exclude.json file %q: %v", excludeFile, err)
		return err
	}
	err = os.WriteFile(excludeFile, data, 0644)
	if err != nil {
		logs.Warnf("Failed to write exclude.json file %q: %v", excludeFile, err)
		return err
	}
	return err
}
