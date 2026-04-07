# FlashMemory 多平台发布指南

本指南详细记录了当您修改了代码并准备发布新版本时，具体需要执行的操作。

为了保证各个生态（GitHub、NPM、PyPI、Homebrew、curl安装脚本）同步更新到最新版本，请务必按以下顺序依次执行。

## 发布流程概览

假设我们要发布的新版本号为 `v0.1.2`（注：NPM和PyPI不需要 `v` 前缀，即 `0.1.2`）。

1. [第一步：触发 GitHub Releases (核心构建)](#第一步触发-github-releases-核心构建)
2. [第二步：更新 Homebrew 仓库](#第二步更新-homebrew-仓库)
3. [第三步：发布新版 NPM 包](#第三步发布新版-npm-包)
4. [第四步：发布新版 PyPI 包](#第四步发布新版-pypi-包)

> 注：基于 `curl` 的一键安装脚本（部署在 `fm.zzh.app`）会自动通过 GitHub API 动态获取最新版本进行下载，**不需要**在每次发布时做任何额外操作！

---

## 第一步：触发 GitHub Releases (核心构建)

这是最重要的一步，它将触发所有的跨平台二进制文件和 FAISSService 的编译和打包。我们在 GitHub Actions 中配置了当推送带 `v` 前缀的标签时自动执行流水线。

```bash
cd /Users/apple/Public/openProject/flashmemory

# 提交你所有的代码更改
git add .
git commit -m "feat: release v0.1.2"
# 推送代码到 master 分支
git push origin master

# 打上新的标签（tag）
git tag v0.1.2
# 将标签推送到 GitHub
git push origin v0.1.2
```

**✅ 检查状态：**
前往 GitHub 仓库的 [Actions](https://github.com/ZetaZeroHub/FlashMemory/actions) 页面，等待构建任务（`Release`）变绿并成功完成。一旦成功，去项目的 Releases 页面将会看到生成了 `v0.1.2` 和对应的 5 个压缩文件。

*(注意：在 GitHub Actions 彻底跑完之前，请不要去执行后面的步骤，因为后续的各个包都会试图前往 GitHub 下载这几个二进制产物)*

---

## 第二步：更新 Homebrew 仓库

Homebrew 从 GitHub Releases 中下载 `.tar.gz` 源码/二进制包。出于安全原因，Homebrew 强制要求提供下载链接和 `SHA256` 校验和。

**1. 获取新版本的 SHA256 校验和**
在本地终端中运行以下脚本，获取 4 个平台的 SHA256（将 `VERSION` 改为你刚刚发布的新版本号）：
```bash
VERSION=0.1.2
for OS_ARCH in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  echo "--- ${OS_ARCH} ---"
  curl -sL "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v${VERSION}/flashmemory_${VERSION}_${OS_ARCH}.tar.gz" | shasum -a 256
done
```

**2. 更新 `homebrew-flashmemory` 仓库**
打开你项目里的 `homebrew/Formula/flashmemory.rb` 文件：
- 把顶部的 `version "0.1.2"` 改成 `"0.1.2"`
- 把你在终端里刚刚获取到的 4 个新的 `sha256` 替换到文件里对应的位置。

最后把更改推送到你的私有 Tap 仓库：
```bash
# （假设你克隆的 homebrew tap 仓库在一个固定的地方比如 /tmp/homebrew-flashmemory）
cd /tmp/homebrew-flashmemory
cp /Users/apple/Public/openProject/flashmemory/homebrew/Formula/flashmemory.rb Formula/
git add .
git commit -m "update formula to v0.1.2"
git push origin main
```
*(一旦推送完成，Mac 或 Linux 上的用户执行 `brew upgrade flashmemory` 就会升级了)*

---

## 第三步：发布新版 NPM 包

NPM 发布的包会在用户安装时，根据其操作系统自动前往 GitHub Releases 获取刚才第一步编译好的二进制文件。所以我们只需要升级包的版本号并在本地推上去。

```bash
cd /Users/apple/Public/openProject/flashmemory/npm-package

# 修改 package.json 中的版本号
# 将 "version": "0.1.2" 改为 "version": "0.1.2"
# （您可以直接修改文件，或者用命令行 npm version patch）
vim package.json 

# 确保官方源登录状态并发布
npm publish --access=public --registry=https://registry.npmjs.org/
```

---

## 第四步：发布新版 PyPI 包

与 NPM 类似，Python 的 CLI 也会在首次运行时前往 GitHub Releases 获取最终二进制包。

```bash
cd /Users/apple/Public/openProject/flashmemory/pip-package

# 记得安装 twine
# pip3 install --upgrade build twine

# 修改包声明的版本号（共两个文件）
# 1. 将 pyproject.toml 里的 version = "0.1.2" 改为 "0.1.2"
# 2. 将 flashmemory/__init__.py 里的 __version__ = "0.1.2" 改为 "0.1.2"

# 清理历史版本的本地构建缓存
rm -rf dist/ build/ *.egg-info/

# 重新构建最新的安装包
python3 -m build

# 上传到 PyPI (需要复制粘贴你当初生成的 pypi- 开头的 API token)
python3 -m twine upload dist/*
```

---

## 🎉 发布完成

完成了上述 4 步后，你的所有分发渠道（Curl、NPM、PyPI、Brew、GitHub Release）都已全线同步到最新版本了！用户不仅可以通过你准备好的不同工具无缝拉取最新版，我们编写的生命周期钩子也会在他们安装的第一时间甚至首次运行的第一时间，自动配置好底层的并发服务依属环境。
