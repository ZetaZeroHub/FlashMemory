#!/bin/bash
# build-windows.sh
# 该脚本用于将 Go 程序编译为适用于 Windows amd64 架构的二进制文件
# 它会遍历 cmd/app/app.go 和 cmd/bot/bot.go 两个文件进行打包
# 使用方法: ./build-windows.sh

set -e  # 出错时退出

echo "开始编译，目标平台：Windows amd64..."
export CC=x86_64-w64-mingw32-gcc
export CXX=x86_64-w64-mingw32-g++
export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=1

# 定义需要编译的 Go 文件列表
files=("cmd/app/fm_http.go" "cmd/main/fm.go")

# 遍历编译每个文件
for filepath in "${files[@]}"; do
    # 获取不带路径和扩展名的基本名称（例如 "app" 或 "bot"）
    base=$(basename "$filepath" .go)
    output="${base}.exe"  # 构造输出文件名
    echo "正在编译 ${filepath} -> ${output} ..."
    go build -o "$output" "$filepath"
done

echo "编译成功，生成的二进制文件："
ls -1 *.exe
