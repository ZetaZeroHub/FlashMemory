"""
FlashMemory CLI 入口
首次运行时自动下载对应平台的 Go 二进制文件
"""
import os
import sys
import platform
import stat
import shutil
import tarfile
import zipfile
import tempfile
import logging

logger = logging.getLogger("flashmemory")

REPO = "ZetaZeroHub/FlashMemory"
INSTALL_DIR = os.path.join(os.path.expanduser("~"), ".flashmemory", "bin")

# 平台映射
PLATFORM_MAP = {
    "Darwin": "darwin",
    "Linux": "linux",
    "Windows": "windows",
}

ARCH_MAP = {
    "x86_64": "amd64",
    "AMD64": "amd64",
    "arm64": "arm64",
    "aarch64": "arm64",
}


def get_platform_info():
    """检测当前平台和架构"""
    system = platform.system()
    machine = platform.machine()

    os_name = PLATFORM_MAP.get(system)
    arch = ARCH_MAP.get(machine)

    if not os_name:
        print(f"❌ 不支持的操作系统: {system}", file=sys.stderr)
        sys.exit(1)
    if not arch:
        print(f"❌ 不支持的 CPU 架构: {machine}", file=sys.stderr)
        sys.exit(1)

    return os_name, arch


def get_latest_version():
    """从 GitHub 获取最新 Release 版本号"""
    import requests

    try:
        resp = requests.get(
            f"https://api.github.com/repos/{REPO}/releases/latest",
            headers={"User-Agent": "flashmemory-pip"},
            timeout=15,
        )
        resp.raise_for_status()
        tag = resp.json().get("tag_name", "")
        return tag.lstrip("v")
    except Exception as e:
        logger.error(f"获取最新版本失败: {e}")
        return None


def download_file(url, dest):
    """下载文件（支持重定向）"""
    import requests

    resp = requests.get(
        url,
        headers={"User-Agent": "flashmemory-pip"},
        stream=True,
        timeout=120,
        allow_redirects=True,
    )
    resp.raise_for_status()

    total = int(resp.headers.get("content-length", 0))
    downloaded = 0

    with open(dest, "wb") as f:
        for chunk in resp.iter_content(chunk_size=8192):
            f.write(chunk)
            downloaded += len(chunk)
            if total > 0:
                pct = int(downloaded / total * 100)
                bar = "=" * (pct // 2) + ">" + " " * (50 - pct // 2)
                print(f"\r   [{bar}] {pct}%", end="", flush=True)

    if total > 0:
        print()  # newline after progress bar


def ensure_binary(binary_name):
    """确保二进制文件存在，不存在则下载"""
    ext = ".exe" if platform.system() == "Windows" else ""
    bin_path = os.path.join(INSTALL_DIR, f"{binary_name}{ext}")

    if os.path.isfile(bin_path):
        return bin_path

    # 需要下载
    print(f"\n📦 FlashMemory 首次运行，正在下载二进制文件...")

    os_name, arch = get_platform_info()
    print(f"   平台: {os_name}/{arch}")

    version = get_latest_version()
    if not version:
        print("❌ 无法获取版本信息，请检查网络", file=sys.stderr)
        sys.exit(1)

    print(f"   版本: v{version}")

    archive_ext = "zip" if os_name == "windows" else "tar.gz"
    archive_name = f"flashmemory_{version}_{os_name}_{arch}"
    url = f"https://github.com/{REPO}/releases/download/v{version}/{archive_name}.{archive_ext}"

    print(f"   下载: {url}")

    # 创建安装目录
    os.makedirs(INSTALL_DIR, exist_ok=True)

    # 下载到临时目录
    with tempfile.TemporaryDirectory() as tmp_dir:
        archive_path = os.path.join(tmp_dir, f"flashmemory.{archive_ext}")
        download_file(url, archive_path)
        print("   ✅ 下载完成")

        # 解压
        if archive_ext == "zip":
            with zipfile.ZipFile(archive_path, "r") as zf:
                zf.extractall(tmp_dir)
        else:
            with tarfile.open(archive_path, "r:gz") as tf:
                tf.extractall(tmp_dir)
        print("   ✅ 解压完成")

        # 复制文件到安装目录
        extracted_dir = os.path.join(tmp_dir, archive_name)
        if not os.path.isdir(extracted_dir):
            # 查找解压后的目录
            for d in os.listdir(tmp_dir):
                full = os.path.join(tmp_dir, d)
                if os.path.isdir(full) and d.startswith("flashmemory_"):
                    extracted_dir = full
                    break

        for item in os.listdir(extracted_dir):
            src = os.path.join(extracted_dir, item)
            dst = os.path.join(INSTALL_DIR, item)
            if os.path.isdir(src):
                if os.path.exists(dst):
                    shutil.rmtree(dst)
                shutil.copytree(src, dst)
            else:
                shutil.copy2(src, dst)

        # 设置执行权限 (非 Windows)
        if os_name != "windows":
            for name in ["fm", "fm_core", "fm_http"]:
                p = os.path.join(INSTALL_DIR, name)
                if os.path.isfile(p):
                    st = os.stat(p)
                    os.chmod(p, st.st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)

    print(f"   ✅ 安装完成: {INSTALL_DIR}\n")
    return bin_path


def run_binary(binary_name):
    """运行指定的二进制文件"""
    bin_path = ensure_binary(binary_name)

    # 设置 FAISS_SERVICE_PATH 环境变量
    faiss_dir = os.path.join(INSTALL_DIR, "FAISSService")
    if os.path.isdir(faiss_dir):
        os.environ["FAISS_SERVICE_PATH"] = faiss_dir

    # 执行二进制，透传所有参数
    args = [bin_path] + sys.argv[1:]

    if platform.system() == "Windows":
        import subprocess
        result = subprocess.run(args)
        sys.exit(result.returncode)
    else:
        os.execv(bin_path, args)


def main_fm():
    """fm 命令入口"""
    run_binary("fm")


def main_fm_http():
    """fm_http 命令入口 (已弃用，请使用 fm serve)"""
    print("⚠️  fm_http 已弃用，请使用: fm serve")
    print("   Deprecated: please use 'fm serve' instead.\n")
    run_binary("fm_http")


if __name__ == "__main__":
    main_fm()
