/**
 * FlashMemory NPM 安装脚本
 * 在 npm install 时自动下载对应平台的二进制文件
 */
const https = require("https");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const os = require("os");

const REPO = "ZetaZeroHub/FlashMemory";
const VENDOR_DIR = path.join(__dirname, "vendor");

// 平台映射: Node.js -> Go
const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getPackageVersion() {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(__dirname, "package.json"), "utf8")
  );
  return pkg.version;
}

function getPlatformInfo() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!platform) {
    console.error(`不支持的操作系统: ${process.platform}`);
    process.exit(1);
  }
  if (!arch) {
    console.error(`不支持的 CPU 架构: ${process.arch}`);
    process.exit(1);
  }

  return { platform, arch };
}

function getLatestVersion() {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: "api.github.com",
      path: `/repos/${REPO}/releases/latest`,
      headers: { "User-Agent": "flashmemory-npm-installer" },
    };

    https
      .get(options, (res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          try {
            const json = JSON.parse(data);
            const version = json.tag_name.replace(/^v/, "");
            resolve(version);
          } catch (e) {
            reject(new Error("无法获取最新版本: " + e.message));
          }
        });
      })
      .on("error", reject);
  });
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const follow = (url) => {
      const lib = url.startsWith("https") ? https : require("http");
      lib
        .get(url, { headers: { "User-Agent": "flashmemory-npm" } }, (res) => {
          // Handle redirects (GitHub uses them)
          if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            return follow(res.headers.location);
          }
          if (res.statusCode !== 200) {
            return reject(new Error(`下载失败 HTTP ${res.statusCode}: ${url}`));
          }
          const file = fs.createWriteStream(dest);
          res.pipe(file);
          file.on("finish", () => {
            file.close();
            resolve();
          });
        })
        .on("error", reject);
    };
    follow(url);
  });
}

function extractArchive(archivePath, destDir) {
  const ext = archivePath.endsWith(".zip") ? "zip" : "tar.gz";

  if (ext === "zip") {
    // Windows: use PowerShell unzip
    if (process.platform === "win32") {
      execSync(
        `powershell -Command "Expand-Archive -Force '${archivePath}' '${destDir}'"`,
        { stdio: "inherit" }
      );
    } else {
      execSync(`unzip -qo "${archivePath}" -d "${destDir}"`, {
        stdio: "inherit",
      });
    }
  } else {
    execSync(`tar xzf "${archivePath}" -C "${destDir}"`, { stdio: "inherit" });
  }
}

async function main() {
  const { platform, arch } = getPlatformInfo();

  console.log(`\n📦 FlashMemory 安装器`);
  console.log(`   平台: ${platform}/${arch}\n`);

  // 获取版本
  let version;
  try {
    version = await getLatestVersion();
    console.log(`   版本: v${version}`);
  } catch (e) {
    console.error("⚠️  无法获取最新版本:", e.message);
    console.error("   跳过二进制下载，稍后可手动安装");
    process.exit(0); // Don't fail npm install
  }

  // 构造下载 URL
  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archiveName = `flashmemory_${version}_${platform}_${arch}`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${archiveName}.${ext}`;

  // 创建目录
  fs.mkdirSync(VENDOR_DIR, { recursive: true });
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "flashmemory-"));
  const archivePath = path.join(tmpDir, `flashmemory.${ext}`);

  try {
    // 下载
    console.log(`   下载: ${url}`);
    await downloadFile(url, archivePath);
    console.log(`   ✅ 下载完成`);

    // 解压
    extractArchive(archivePath, tmpDir);
    console.log(`   ✅ 解压完成`);

    // 复制到 vendor 目录
    const extractedDir = path.join(tmpDir, archiveName);
    const files = fs.readdirSync(extractedDir);
    for (const file of files) {
      const src = path.join(extractedDir, file);
      const dest = path.join(VENDOR_DIR, file);
      if (fs.statSync(src).isDirectory()) {
        // 递归复制目录
        execSync(
          process.platform === "win32"
            ? `xcopy /E /I /Y "${src}" "${dest}"`
            : `cp -r "${src}" "${dest}"`
        );
      } else {
        fs.copyFileSync(src, dest);
      }
    }

    // 设置执行权限 (非 Windows)
    if (platform !== "windows") {
      const bins = ["fm", "fm_core", "fm_http"];
      for (const bin of bins) {
        const binPath = path.join(VENDOR_DIR, bin);
        if (fs.existsSync(binPath)) {
          fs.chmodSync(binPath, 0o755);
        }
      }
    }

    console.log(`   ✅ 安装完成: ${VENDOR_DIR}\n`);
  } catch (e) {
    console.error(`\n⚠️  安装失败: ${e.message}`);
    console.error("   请检查网络连接或手动下载二进制文件\n");
    process.exit(0); // Don't fail npm install
  } finally {
    // 清理临时文件
    try {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    } catch {}
  }
}

main();
