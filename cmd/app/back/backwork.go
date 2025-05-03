package back

import (
	_ "database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/visualize"
)

var (
	SupportedLanguages = []string{
		".go",
		".py",
		".js", ".jsx",
		".ts", ".tsx",
		".java",
		".cpp", ".cc", ".cc", ".cxx", ".c++", ".hpp", ".h",
		".c",
		".rb",
		".php",
	}
)

// BuildIndex 构建全量或指定目录索引
func BuildIndex(projDir, subDir string, full bool) error {
	// 如果全量，先删除旧索引
	if full {
		if err := DeleteIndex(projDir); err != nil {
			return fmt.Errorf("删除旧索引失败: %w", err)
		}
	}
	process, err := indexCode(projDir, "master", "", full, subDir)
	utils.StopFaissService(process)
	if err != nil {
		return fmt.Errorf("构建索引失败: %w", err)
	}
	return nil
}

// IncrementalUpdate 基于分支和 commit 增量更新索引
func IncrementalUpdate(projDir, branch, commit string) error {
	// 默认分支 master
	if branch == "" {
		branch = "master"
	}
	process, err := indexCode(projDir, branch, commit, false, "")
	utils.StopFaissService(process)
	if err != nil {
		return fmt.Errorf("增量更新索引失败: %w", err)
	}
	return nil
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
func indexCode(projDir, branchName, commitHash string, forceFull bool, filePath string) (*os.Process, error) {

	faissProcess, faissServiceDir, err := InitFaiss()
	if err != nil {
		return faissProcess, fmt.Errorf("启动Faiss服务失败: %w", err)
	}

	// 2. 创建 .gitgo 目录
	gitgoDir := filepath.Join(projDir, ".gitgo")
	if err := os.MkdirAll(gitgoDir, 0755); err != nil {
		return faissProcess, fmt.Errorf("创建索引目录失败: %w", err)
	}

	// 索引文件路径
	indexDBPath := filepath.Join(gitgoDir, "code_index.db")
	faissIndexPath := filepath.Join(gitgoDir, "code_index.faiss")

	log.Println("正在索引代码文件...")
	os.MkdirAll(gitgoDir, 0755)

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
				log.Fatalf("打开索引数据库失败: %v", err)
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
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(SupportedLanguages, ext) {
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
											// 跳过以点开头的隐藏目录
											if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
												return filepath.SkipDir
											}
											return nil
										}
										// 仅考虑特定的文件扩展名
										ext := filepath.Ext(path)
										if utils.Contains(SupportedLanguages, ext) {
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
										// 跳过以点开头的隐藏目录
										if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
											return filepath.SkipDir
										}
										return nil
									}
									// 仅考虑特定的文件扩展名
									ext := filepath.Ext(path)
									if utils.Contains(SupportedLanguages, ext) {
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
		return faissProcess, fmt.Errorf("初始化索引DB失败: %w", err)
	}
	defer db.Close()

	// 1. 遍历项目目录以查找代码文件
	var files []string
	if filePath != "" {
		log.Println("选择性更新模式：使用 -file 参数指定的文件/文件夹")
		info, err := os.Stat(filePath)
		if err != nil {
			log.Fatalf("无法访问指定的 -file 路径: %v", err)
		}
		// 打开数据库，准备删除旧记录
		db, err := index.EnsureIndexDB(projDir)
		if err != nil {
			log.Fatalf("打开索引数据库失败: %v", err)
		}
		defer db.Close()
		if info.IsDir() {
			err = filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
						return filepath.SkipDir
					}
					return nil
				}
				ext := filepath.Ext(path)
				if utils.Contains(SupportedLanguages, ext) {
					// 删除该文件的旧索引记录
					n, err := db.Exec("DELETE FROM functions WHERE file like ?", path+"%")
					if err != nil {
						log.Printf("删除文件 %s 的索引记录失败: %v", path, err)
					} else {
						log.Printf("已删除文件 %s 的索引记录, %s", path, n)
					}
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				log.Fatalf("遍历指定文件夹失败: %v", err)
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
				if utils.Contains(SupportedLanguages, ext) {
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
				// 跳过以点开头的隐藏目录
				if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
					log.Printf("跳过目录: %s", info.Name())
					return filepath.SkipDir
				}
				return nil
			}
			// 仅考虑特定的文件扩展名
			ext := filepath.Ext(path)
			if utils.Contains(SupportedLanguages, ext) {
				files = append(files, path)
			}
			return nil
		})
	}
	if err != nil {
		log.Fatalf("遍历项目目录失败: %v", err)
	}
	if len(files) == 0 {
		log.Println("No source code files found in the provided directory.")
		return faissProcess, errors.New("No source code files found in the provided directory.")
	}

	// 2. 解析所有文件
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

	if err != nil {
		return faissProcess, err
	}

	log.Println("正在分析代码...")
	// 3. 分析函数（考虑依赖关系顺序）
	llmAnalyzer := analyzer.NewLLMAnalyzer(map[string]string{}, true, 3)
	results := llmAnalyzer.AnalyzeAll(allFuncs)
	fmt.Printf("Analyzed %d functions with AI summaries.\n", len(results))

	log.Println("正在构建知识图谱...")
	// 4. 构建知识图谱
	kg := graph.BuildGraph(results)
	// （可选）保存图结构用于调试
	// _ = kg.SaveGraphJSON(filepath.Join(*projDir, ".gitgo", "graph.json"))

	log.Println("正在索引代码...")
	// 5. 初始化索引存储（SQLite和Faiss）

	// 确保storage_path是绝对路径
	absGitgoDir, err := filepath.Abs(gitgoDir)
	if err != nil {
		log.Fatalf("获取gitgo目录绝对路径失败: %v", err)
	}

	// 创建FaissWrapper，传入存储路径选项
	faissOptions := map[string]interface{}{
		"storage_path": absGitgoDir,
		"server_url":   index.DefaultFaissServerURL, // 使用默认的Faiss HTTP服务URL
		"index_id":     "code_index",                // 设置一个有意义的索引ID
	}
	idx := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, faissOptions)} // 假设128维向量
	err = idx.SaveAnalysisToDB(results)
	if err != nil {
		log.Fatalf("保存分析结果到数据库失败: %v", err)
	}
	// 构建嵌入向量并添加到Faiss索引
	if incrementalUpdate {
		// 增量更新模式：先加载现有索引
		err = idx.FaissIndex.LoadFromFile(faissServiceDir)
		if err != nil {
			log.Printf("加载现有Faiss索引失败: %v，将创建新索引", err)
		} else {
			log.Println("成功加载现有Faiss索引")
		}
	}

	// 为每个分析结果添加向量
	for id, res := range results {
		// 获取res.Description的嵌入向量
		vec := search.SimpleEmbedding(res.Description, idx.FaissIndex.Dimension())
		idx.FaissIndex.AddVector(id+1, vec) // SQLite行ID从1开始
	}

	// 保存索引到文件
	err = idx.FaissIndex.SaveToFile(absGitgoDir + "/code_index.faiss")
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

	log.Println("索引完成。")

	// 12. （可选） 展示统计
	visualize.PrintStats(visualize.ComputePackageStats(kg))

	defer utils.StopFaissService(faissProcess)
	return faissProcess, nil
}
