package back

import (
	_ "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/kinglegendzzh/flashmemory/internal/visualize"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
)

// BuildIndex 构建全量或指定目录索引
func BuildIndex(projDir, subDir string, full bool, open bool) error {
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if e := os.MkdirAll(gitgoDir, 0755); e != nil {
		return fmt.Errorf("创建索引目录失败: %w", e)
	}
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	// 如果 temp 文件已存在，说明已有索引进程在运行
	if _, err := os.Stat(tempFilePath); err == nil {
		logs.Warnf("索引已运行，跳过索引...")
	} else if !os.IsNotExist(err) {
		logs.Warnf("索引临时文件已存在，跳过索引...")
	}

	// 创建临时文件，标记开始索引
	f, err := os.Create(tempFilePath)
	if err != nil {
		logs.Warnf("创建索引临时文件失败 %q: %v", tempFilePath, err)
	}
	f.Close()
	fm, err := InitFaissManager(projDir, open)
	if err != nil {
		return fmt.Errorf("初始化 FaissManager 失败: %w", err)
	}
	// 如果需要全量，先 Reset（删除老索引）
	if full {
		if e := fm.Reset(); e != nil {
			return fmt.Errorf("重置 Faiss 索引失败: %w", e)
		}
	}
	// 后续不再启动/停止服务，直接注入
	return indexCodeWithManager(fm, projDir, "master", "", full, subDir)
}

// IncrementalUpdate 增量更新索引
func IncrementalUpdate(projDir, branch, commit string, open bool) error {
	fm, err := InitFaissManager(projDir, open)
	if err != nil {
		return fmt.Errorf("初始化 FaissManager 失败: %w", err)
	}
	return indexCodeWithManager(fm, projDir, branch, commit, false, "")
}

// DeleteIndex 删除索引文件（.gitgo 目录下所有内容）
func DeleteIndex(projDir string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	return os.RemoveAll(gitgo)
}

func ResetIndex(projDir string) error {
	gitgo := filepath.Join(projDir, ".gitgo")
	if _, err := os.Stat(gitgo); os.IsNotExist(err) {
		return nil // 无索引，直接返回
	}
	faissIndexPath := filepath.Join(gitgo, "code_index.faiss")
	faissIndexMetaPath := filepath.Join(gitgo, "code_index.faiss.meta")
	localIndexPath := filepath.Join(gitgo, "code_index.local")
	tempPath := filepath.Join(gitgo, "indexing.temp")
	os.Remove(faissIndexPath)
	os.Remove(faissIndexMetaPath)
	os.Remove(localIndexPath)
	os.Remove(tempPath)
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
		return fmt.Errorf("创建索引目录失败: %w", e)
	}
	// 索引文件路径
	indexDBPath := filepath.Join(gitgoDir, "code_index.db")
	faissIndexPath := filepath.Join(gitgoDir, "code_index"+ext)

	log.Println("正在索引代码文件...")
	os.MkdirAll(gitgoDir, 0755)

	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	// 如果 temp 文件已存在，说明已有索引进程在运行
	if _, err := os.Stat(tempFilePath); err == nil {
		logs.Warnf("索引已运行，跳过索引...")
	} else if !os.IsNotExist(err) {
		logs.Warnf("索引临时文件已存在，跳过索引...")
	}

	// 创建临时文件，标记开始索引
	f, err := os.Create(tempFilePath)
	if err != nil {
		logs.Warnf("创建索引临时文件失败 %q: %v", tempFilePath, err)
	}
	f.Close()

	// 检查是否存在索引文件，如果存在则进行增量更新
	var filesToProcess []string
	var incrementalUpdate bool

	// 检查索引文件是否存在
	dbExists := false
	if _, err := os.Stat(indexDBPath); err == nil {
		if _, err := os.Stat(faissIndexPath); err == nil {
			dbExists = true
			log.Println("检测到现有索引文件，将进行增量更新")
		}
	}

	// 如果强制全量索引，则跳过增量更新逻辑
	if forceFull {
		log.Println("已指定强制全量索引，将忽略增量更新")
		dbExists = false
	}

	if dbExists {
		// 检查.git目录是否存在
		gitDir := filepath.Join(projDir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			log.Println("检测到.git目录，正在解析提交记录以获取变更文件...")

			// 打开数据库，准备查询和更新
			db, err := index.EnsureIndexDB(projDir)
			if err != nil {
				return err
			}
			defer db.Close()

			// 确保branch_index表存在
			if err := index.EnsureBranchIndexTable(db); err != nil {
				log.Printf("确保branch_index表存在失败: %v，将进行全量索引", err)
			} else {
				// 获取最新的分支索引信息
				branchInfo, err := index.GetLatestBranchIndexInfo(db, branchName)
				if err != nil {
					log.Printf("获取分支索引信息失败: %v，将进行全量索引", err)
				} else {
					// 获取当前commit hash或使用指定的commit hash
					currentCommitHash := commitHash
					if currentCommitHash == "" {
						// 使用git工具函数获取当前分支的最新commit hash
						currentCommitHash, err = utils.GetCurrentBranchCommitHash(projDir)
						if err != nil {
							log.Printf("获取当前分支commit hash失败: %v，将进行全量索引", err)
							currentCommitHash = ""
						}
					}

					if currentCommitHash != "" {
						if branchInfo == nil {
							// 没有找到分支索引信息，获取当前commit的变更文件
							log.Printf("未找到分支索引信息，获取当前分支commit %s的变更文件...", currentCommitHash)
							changedFiles, err := utils.GetChangedFilesByCommitHash(projDir, currentCommitHash)
							if err != nil {
								log.Printf("获取commit %s的变更文件失败: %v，将进行全量索引", currentCommitHash, err)
							} else if len(changedFiles) > 0 {
								log.Printf("检测到%d个变更文件", len(changedFiles))

								// 删除这些文件的索引记录
								for _, file := range changedFiles {
									_, err := db.Exec("DELETE FROM functions WHERE file = ?", file)
									if err != nil {
										log.Printf("删除文件 %s 的索引记录失败: %v", file, err)
									} else {
										log.Printf("已删除文件 %s 的索引记录", file)
									}
								}

								// 只处理变更的文件
								filesToProcess = changedFiles
								incrementalUpdate = true
							} else {
								log.Println("未检测到变更文件，将进行全量索引")
							}
						} else {
							// 找到了分支索引信息，获取两个commit之间的变更文件
							log.Printf("正在获取commit %s和%s之间的变更文件...", branchInfo.CommitHash, currentCommitHash)
							log.Printf("找到分支 %s 的索引信息，上次索引的commit: %s", branchInfo.BranchName, branchInfo.CommitHash)

							if branchInfo.CommitHash != currentCommitHash {
								changedFiles, err := utils.GetChangedFilesBetweenCommits(projDir, branchInfo.CommitHash, currentCommitHash)
								if err != nil {
									log.Printf("获取commit %s和%s之间的变更文件失败: %v，将进行全量索引", branchInfo.CommitHash, currentCommitHash, err)
								} else if len(changedFiles) > 0 {
									log.Printf("检测到%d个变更文件", len(changedFiles))

									// 删除这些文件的索引记录
									for _, file := range changedFiles {
										_, err := db.Exec("DELETE FROM functions WHERE file = ?", file)
										if err != nil {
											log.Printf("删除文件 %s 的索引记录失败: %v", file, err)
										} else {
											log.Printf("已删除文件 %s 的索引记录", file)
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
										if info.IsDir() {
											// 跳过exclude.json中指定的路径
											excludeFile := filepath.Join(gitgoDir, "exclude.json")
											jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
											if err == nil {
												if utils.IsExcludedPath(path, jsonFile) {
													log.Printf("跳过指定目录: %s", path)
													return filepath.SkipDir
												}
											}
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(common.SupportedLanguages, ext) {
											allFiles = append(allFiles, path)
										}
										return nil
									})

									if err != nil {
										log.Printf("遍历项目目录失败: %v，将只处理变更文件", err)
									} else {
										// 找出未索引的文件
										var missingFiles []string
										for _, file := range allFiles {
											if !allIndexedFiles[file] && utils.Contains(changedFiles, file) {
												log.Printf("--- 文件 %s 未索引，将处理", file)
												missingFiles = append(missingFiles, file)
											}
										}

										if len(missingFiles) > 0 {
											log.Printf("检测到%d个未索引的文件，将一并处理", len(missingFiles))
											filesToProcess = append(filesToProcess, missingFiles...)
										}
									}

									incrementalUpdate = true
								} else {
									log.Println("未检测到变更文件，将检查是否有未索引的文件")

									// 检查是否有未索引的文件
									var allFiles []string
									err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
										if err != nil {
											return err
										}
										if info.IsDir() {
											// 跳过exclude.json中指定的路径
											excludeFile := filepath.Join(gitgoDir, "exclude.json")
											jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
											if err == nil {
												if utils.IsExcludedPath(path, jsonFile) {
													log.Printf("跳过指定目录: %s", path)
													return filepath.SkipDir
												}
											}
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(common.SupportedLanguages, ext) {
											allFiles = append(allFiles, path)
										}
										return nil
									})

									if err != nil {
										log.Printf("遍历项目目录失败: %v，将进行全量索引", err)
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
											log.Printf("检测到%d个未索引的文件，将进行处理", len(missingFiles))
											filesToProcess = missingFiles
											incrementalUpdate = true
										} else {
											log.Println("所有文件已索引，无需更新")
										}
									}
								}
							} else {
								log.Printf("当前commit %s已经索引过，检查是否有未索引的文件", currentCommitHash)

								// 检查是否有未索引的文件
								var allFiles []string
								err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
									if err != nil {
										return err
									}
									if info.IsDir() {
										// 跳过exclude.json中指定的路径
										excludeFile := filepath.Join(gitgoDir, "exclude.json")
										jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
										if err == nil {
											if utils.IsExcludedPath(path, jsonFile) {
												log.Printf("跳过指定目录: %s", path)
												return filepath.SkipDir
											}
										}
										// 跳过以点开头的隐藏目录
										if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
											return filepath.SkipDir
										}
										return nil
									}
									// 仅考虑特定的文件扩展名
									ext := filepath.Ext(path)
									if utils.Contains(common.SupportedLanguages, ext) {
										allFiles = append(allFiles, path)
									}
									return nil
								})

								if err != nil {
									log.Printf("遍历项目目录失败: %v，将进行全量索引", err)
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
										log.Printf("检测到%d个未索引的文件，将进行处理", len(missingFiles))
										filesToProcess = missingFiles
										incrementalUpdate = true
									} else {
										log.Println("所有文件已索引，无需更新")
									}
								}
							}
						}
					}
				}
			}
		} else {
			log.Println(".git目录不存在，无法获取变更信息，将进行全量索引")
		}
	} else {
		log.Println("索引文件不存在或已指定强制全量索引，将进行全量索引")
	}

	log.Printf("标签： %v, 待索引文件： %s ", incrementalUpdate, filesToProcess)

	// 如果是强制全量索引，则删除分支索引信息
	if forceFull {
		log.Printf("强制全量索引，删除分支 %s 的索引信息", branchName)
		db, err := index.EnsureIndexDB(projDir)
		if err == nil {
			defer db.Close()
			err = index.DeleteBranchIndexInfo(db, branchName)
			if err != nil {
				log.Printf("删除分支 %s 的索引信息失败: %v", branchName, err)
			}
		}
	}

	// 3. 打开或创建索引数据库
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		return fmt.Errorf("初始化索引DB失败: %w", err)
	}
	defer db.Close()

	// 1. 遍历项目目录以查找代码文件
	var files []string
	if filePath != "" {
		log.Println("选择性更新模式：使用 -file 参数指定的文件/文件夹")
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
				if info.IsDir() {
					// 跳过exclude.json中指定的路径
					excludeFile := filepath.Join(gitgoDir, "exclude.json")
					jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
					if err == nil {
						if utils.IsExcludedPath(path, jsonFile) {
							log.Printf("跳过指定目录: %s", path)
							return filepath.SkipDir
						}
					}
					if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				ext := filepath.Ext(path)
				if utils.Contains(common.SupportedLanguages, ext) {
					// 删除该文件的旧索引记录
					relPath, err := filepath.Rel(projDir, path)
					if err != nil {
						fmt.Errorf("获取 %s 相对于 %s 的相对路径失败: %w", fullPath, projDir, err)
					}
					relPath = filepath.ToSlash(relPath)
					n, err := db.Exec("DELETE FROM functions WHERE file like ?", relPath+"%")
					if err != nil {
						log.Printf("删除文件 %s 的索引记录失败: %v", path, err)
					} else {
						log.Printf("已删除文件 %s(%s) 的索引记录, %s", path, relPath, n)
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
				log.Printf("删除文件 %s 的索引记录失败: %v", filePath, err)
			} else {
				log.Printf("已删除文件 %s 的索引记录, %s", filePath, n)
			}
			files = append(files, filePath)
		}
	} else if incrementalUpdate && len(filesToProcess) > 0 {
		// 增量更新模式：只处理变更文件
		log.Println("增量更新模式：只处理变更文件")
		log.Printf("变更文件列表: %v", strings.Join(filesToProcess, ";"))
		for _, file := range filesToProcess {
			// 获取文件的绝对路径
			absPath := filepath.Join(projDir, file)
			// 检查文件是否存在
			if _, err := os.Stat(absPath); err == nil {
				// 检查文件扩展名
				ext := filepath.Ext(absPath)
				if utils.Contains(common.SupportedLanguages, ext) {
					files = append(files, absPath)
				}
			}
		}
	} else if forceFull {
		// 全量索引模式：遍历整个项目目录
		log.Println("全量索引模式：遍历整个项目目录")
		err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// 跳过exclude.json中指定的路径
				excludeFile := filepath.Join(gitgoDir, "exclude.json")
				jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
				if err == nil {
					if utils.IsExcludedPath(path, jsonFile) {
						log.Printf("跳过指定目录: %s", path)
						return filepath.SkipDir
					}
				}
				// 跳过以点开头的隐藏目录
				if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
					log.Printf("跳过目录: %s", info.Name())
					return filepath.SkipDir
				}
				return nil
			}
			// 仅考虑特定的文件扩展名
			ext := filepath.Ext(path)
			if utils.Contains(common.SupportedLanguages, ext) {
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
		return errors.New("No source code files found in the provided directory.")
	}

	// 2. 解析所有文件
	var allFuncs []parser.FunctionInfo
	for _, file := range files {
		lang := parser.DetectLang(file)
		if lang == "" {
			continue
		}
		p := parser.NewParserDb(lang, db, projDir)
		funcs, err := p.ParseFile(file)
		if err != nil {
			log.Printf("Error parsing file %s: %v\n", file, err)
			continue
		}
		allFuncs = append(allFuncs, funcs...)
	}
	fmt.Printf("Parsed %d files, extracted %d functions.\n", len(files), len(allFuncs))

	if err != nil {
		return err
	}

	log.Println("正在分析代码...")
	// 3. 分析函数（考虑依赖关系顺序）
	llmAnalyzer := analyzer.NewLLMAnalyzerHttp(&sync.Map{}, true, 3, db, projDir)
	results := llmAnalyzer.AnalyzeAll(allFuncs)
	fmt.Printf("Analyzed %d functions with AI summaries.\n", len(results))

	log.Println("正在构建知识图谱...")
	// 4. 构建知识图谱
	kg := graph.BuildGraph(results)
	// （可选）保存图结构用于调试
	err = kg.SaveGraphJSON(filepath.Join(projDir, ".gitgo", "graph.json"))
	if err != nil {
		logs.Errorf("保存图结构失败: %v", err)
		return err
	}

	log.Println("正在索引代码...")
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
			log.Printf("加载现有Faiss索引失败: %v，将创建新索引", err)
		} else {
			log.Println("成功加载现有Faiss索引")
		}
	}

	err = EnsureEmbeddingsBatch(idx)
	if err != nil {
		log.Printf("为函数添加向量失败: %v", err)
		return err
	}

	// 保存索引到文件
	err = idx.FaissIndex.SaveToFile(absGitgoDir + "/code_index" + ext)
	if err != nil {
		log.Printf("保存Faiss索引失败: %v", err)
	} else {
		log.Println("成功保存Faiss索引")

		// 保存分支索引信息
		if commitHash != "" {
			// 获取当前commit hash
			currentCommitHash := commitHash
			if currentCommitHash == "" {
				var err error
				currentCommitHash, err = utils.GetCurrentBranchCommitHash(projDir)
				if err != nil {
					log.Printf("获取当前分支commit hash失败: %v，无法保存分支索引信息", err)
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
					log.Printf("保存分支索引信息失败: %v", err)
				} else {
					log.Printf("成功保存分支 %s (commit: %s) 的索引信息，共索引 %d 个文件",
						branchInfo.BranchName, branchInfo.CommitHash, len(processedFiles))
				}
			}
		}
	}

	// 12. （可选） 展示统计
	//visualize.PrintStats(visualize.ComputePackageStats(kg))

	// 确保退出时删除临时文件
	defer func() {
		if err := os.Remove(tempFilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("删除索引临时文件失败 %q: %v", tempFilePath, err)
		}
		logs.Infof("索引完成，已删除索引临时文件 %q", tempFilePath)
	}()
	//fm.Stop()
	return nil
}

func ListGraph(projDir string) (err error) {
	var files []string
	// 全量索引模式：遍历整个项目目录
	log.Println("全量索引模式：遍历整个项目目录")
	err = filepath.Walk(projDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// 跳过exclude.json中指定的路径
			excludeFile := filepath.Join(projDir, ".gitgo", "exclude.json")
			jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
			if err == nil {
				if utils.IsExcludedPath(path, jsonFile) {
					log.Printf("跳过指定目录: %s", path)
					return filepath.SkipDir
				}
			}
			// 跳过以点开头的隐藏目录
			if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
				log.Printf("跳过目录: %s", info.Name())
				return filepath.SkipDir
			}
			return nil
		}
		// 仅考虑特定的文件扩展名
		ext := filepath.Ext(path)
		if utils.Contains(common.SupportedLanguages, ext) {
			files = append(files, path)
		}
		return nil
	})
	db, err := index.EnsureIndexDB(projDir)
	if err != nil {
		return fmt.Errorf("初始化索引DB失败: %w", err)
	}
	defer db.Close()
	var allFuncs []parser.FunctionInfo
	for _, file := range files {
		lang := parser.DetectLang(file)
		if lang == "" {
			continue
		}
		p := parser.NewParser(lang)
		funcs, err := p.ParseFile(file)
		if err != nil {
			log.Printf("Error parsing file %s: %v\n", file, err)
			continue
		}
		allFuncs = append(allFuncs, funcs...)
	}
	fmt.Printf("Parsed %d files, extracted %d functions.\n", len(files), len(allFuncs))
	log.Println("正在分析代码...")
	// 3. 分析函数（考虑依赖关系顺序）
	llmAnalyzer := analyzer.NewLLMAnalyzerHttp(&sync.Map{}, true, 3, db, projDir)
	results := llmAnalyzer.LoadAll(allFuncs)
	fmt.Printf("Analyzed %d functions with AI summaries.\n", len(results))
	log.Println("正在构建知识图谱...")
	// 4. 构建知识图谱
	kg := graph.BuildGraph(results)
	// （可选）保存图结构用于调试
	err = kg.SaveGraphJSON(filepath.Join(projDir, ".gitgo", "graph.json"))
	// 12. （可选） 展示统计
	visualize.PrintStats(visualize.ComputePackageStats(kg))
	return err
}

func MakeExcludeFile(projDir string, exclude []string) (err error) {
	excludeFile := filepath.Join(projDir, ".gitgo", "exclude.json")
	// 自动创建文件夹
	err = os.MkdirAll(filepath.Dir(excludeFile), 0755)
	if err != nil {
		logs.Warnf("创建文件夹失败 %q: %v", filepath.Dir(excludeFile), err)
		return err
	}
	// 如果不存在exclude.json，则生成文件
	if _, err := os.Stat(excludeFile); os.IsNotExist(err) {
		file, err := os.Create(excludeFile)
		if err != nil {
			logs.Warnf("创建exclude.json文件失败 %q: %v", excludeFile, err)
			return err
		}
		defer file.Close()
	}
	data, err := json.MarshalIndent(exclude, "", "  ")
	if err != nil {
		logs.Warnf("序列化exclude.json文件失败 %q: %v", excludeFile, err)
		return err
	}
	err = os.WriteFile(excludeFile, data, 0644)
	if err != nil {
		logs.Warnf("写入exclude.json文件失败 %q: %v", excludeFile, err)
		return err
	}
	return err
}
