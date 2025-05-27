#!/usr/bin/env bash
set -euo pipefail

# —— 固定配置 ——
API_USER="root"                      # 请替换为你的 API 用户名
# shellcheck disable=SC2034
API_PASS="root"                      # 请替换为你的 API 密码
FAISS_SERVICE_PATH="/Users/apple/Public/openProject/flashmemory/cmd/app/FAISSService"  # 请替换为 FAISSService 的绝对路径
PORT="5532"                                   # 服务监听端口，可按需修改
CONFIG="/Users/apple/Public/openProject/flashmemory/fm.yaml"

echo "配置："
echo "  API_USER= $API_USER"
echo "  FAISS_SERVICE_PATH= $FAISS_SERVICE_PATH"
echo "  PORT= $PORT"
echo

export API_USER
export API_PASS
export FAISS_SERVICE_PATH
export PORT

go run cmd/app/fm_http.go -c $CONFIG
