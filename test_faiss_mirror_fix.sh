#!/bin/bash

echo "=== Faiss安装修复测试脚本 ==="
echo ""

# 检查Python版本
echo "1. 检查Python版本："
python3 --version 2>/dev/null || python --version
echo ""

# 检查当前配置的镜像源
echo "2. 检查当前配置的镜像源："
if [ -f "fm.yaml" ]; then
    pip_path=$(grep "pip_path:" fm.yaml | cut -d'"' -f2)
    echo "配置文件中的pip镜像源: $pip_path"
else
    echo "未找到fm.yaml配置文件"
fi
echo ""

# 测试不同镜像源的faiss-cpu可用性
echo "3. 测试不同镜像源的faiss-cpu版本可用性："

mirrors=(
    "https://pypi.org/simple"
    "https://mirrors.aliyun.com/pypi/simple"
    "https://pypi.tuna.tsinghua.edu.cn/simple"
    "https://mirrors.cloud.tencent.com/pypi/simple"
    "https://mirrors.163.com/pypi/simple"
)

for mirror in "${mirrors[@]}"; do
    echo "测试镜像源: $mirror"
    # 检查faiss-cpu==1.7.2版本（Python 3.6兼容）
    pip index versions faiss-cpu -i "$mirror" 2>/dev/null | grep "1.7.2" && echo "  ✓ faiss-cpu==1.7.2 可用" || echo "  ✗ faiss-cpu==1.7.2 不可用"
    # 检查faiss-cpu==1.7.4版本（Python 3.7兼容）
    pip index versions faiss-cpu -i "$mirror" 2>/dev/null | grep "1.7.4" && echo "  ✓ faiss-cpu==1.7.4 可用" || echo "  ✗ faiss-cpu==1.7.4 不可用"
    echo ""
done

echo "4. 修复总结："
echo "✓ 已修复faiss-cpu版本兼容性问题："
echo "  - Python 3.6: 使用 faiss-cpu==1.7.2"
echo "  - Python 3.7: 使用 faiss-cpu==1.7.4"
echo "  - Python 3.8+: 使用最新版本"
echo ""
echo "✓ 已添加多镜像源自动切换机制："
echo "  - 优先使用配置的镜像源"
echo "  - 失败时自动尝试其他镜像源"
echo "  - 最后尝试官方源"
echo ""
echo "现在可以重新启动服务测试修复效果！"
