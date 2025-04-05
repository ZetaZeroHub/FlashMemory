package utils

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetChangedFilesByCommitHash 根据特定的commit hash获取变更的文件列表
// 参数:
//   - repoPath: 仓库路径
//   - commitHash: 提交的哈希值
//
// 返回:
//   - []string: 变更的文件路径列表
//   - error: 错误信息
func GetChangedFilesByCommitHash(repoPath string, commitHash string) ([]string, error) {
	// 确保commitHash不为空
	if commitHash == "" {
		return nil, fmt.Errorf("commit hash不能为空")
	}

	// 构建git命令：获取指定commit的变更文件列表
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", commitHash)
	cmd.Dir = repoPath

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取commit %s的变更文件失败: %v\n%s", commitHash, err, string(output))
	}

	// 解析输出，获取文件列表
	files := strings.Split(strings.TrimSpace(string(output)), "\n")

	// 过滤空行
	var result []string
	for _, file := range files {
		if file != "" {
			// 转换为绝对路径
			absPath := filepath.Join(repoPath, file)
			result = append(result, absPath)
		}
	}

	return result, nil
}

// GetChangedFilesBetweenCommits 获取两个commit之间的变更文件列表
// 参数:
//   - repoPath: 仓库路径
//   - oldCommit: 旧的commit hash
//   - newCommit: 新的commit hash
//
// 返回:
//   - []string: 变更的文件路径列表
//   - error: 错误信息
func GetChangedFilesBetweenCommits(repoPath string, oldCommit, newCommit string) ([]string, error) {
	// 确保commit hash不为空
	if oldCommit == "" || newCommit == "" {
		return nil, fmt.Errorf("commit hash不能为空")
	}

	// 构建git命令：获取两个commit之间的变更文件列表
	cmd := exec.Command("git", "diff", "--name-only", fmt.Sprintf("%s..%s", oldCommit, newCommit))
	cmd.Dir = repoPath

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取commit %s和%s之间的变更文件失败: %v\n%s", oldCommit, newCommit, err, string(output))
	}

	// 解析输出，获取文件列表
	files := strings.Split(strings.TrimSpace(string(output)), "\n")

	// 过滤空行并转换为绝对路径
	var result []string
	for _, file := range files {
		if file != "" {
			// 转换为绝对路径
			absPath := filepath.Join(repoPath, file)
			result = append(result, absPath)
		}
	}

	return result, nil
}

// GetCurrentBranchCommitHash 获取当前分支的最新commit hash
// 参数:
//   - repoPath: 仓库路径
//
// 返回:
//   - string: commit hash
//   - error: 错误信息
func GetCurrentBranchCommitHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("获取当前分支commit hash失败: %v\n%s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// GetBranchCommitHash 获取指定分支的最新commit hash
// 参数:
//   - repoPath: 仓库路径
//   - branch: 分支名称
//
// 返回:
//   - string: commit hash
//   - error: 错误信息
func GetBranchCommitHash(repoPath string, branch string) (string, error) {
	if branch == "" {
		return "", fmt.Errorf("分支名称不能为空")
	}

	cmd := exec.Command("git", "rev-parse", branch)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("获取分支%s的commit hash失败: %v\n%s", branch, err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}
