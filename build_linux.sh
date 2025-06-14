#!/usr/bin/env bash
# build-linux.sh
# 将 Go 程序交叉编译成 Linux/amd64 可执行文件
# 用法：./build-linux.sh

set -euo pipefail

echo "➡️  开始编译，目标平台：Linux amd64..."

## 1⃣️ 依赖检查 --------------------------------------------------------------
if ! command -v zig >/dev/null 2>&1; then
  echo "❌  未找到 zig，请先手动执行：brew install zig"
  exit 1
fi

## 2⃣️ 交叉编译环境变量 ------------------------------------------------------
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=1
export CC="zig cc -target x86_64-linux-musl"

## 3⃣️ 待编译源文件列表 ------------------------------------------------------
sources=(
  "cmd/app/fm_http.go"
  "cmd/main/fm.go"
)

## 4⃣️ 循环编译 --------------------------------------------------------------
for src in "${sources[@]}"; do
  if [[ ! -r "$src" ]]; then
    echo "⚠️  源文件 $src 不可读，请检查权限"
    exit 1
  fi

  bin="$(basename "${src%.go}")_linux"
  echo "🔨  go build -o $bin $src"
  go build -trimpath -ldflags='-s -w' -o "$bin" "$src"
done

## 5⃣️ 结果展示 --------------------------------------------------------------
echo -e "\n✅ 编译完成，生成的文件："
ls -lh *_linux
