#!/usr/bin/env node
/**
 * FlashMemory HTTP server wrapper - fm_http command
 * Forwards to the Go binary
 */
const { execFileSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const vendorDir = path.join(__dirname, "..", "vendor");
const ext = process.platform === "win32" ? ".exe" : "";
const binPath = path.join(vendorDir, `fm_http${ext}`);

if (!fs.existsSync(binPath)) {
  console.error("❌ FlashMemory HTTP 服务二进制未找到，请重新安装:");
  console.error("   npm install -g flashmemory");
  process.exit(1);
}

// Set FAISS_SERVICE_PATH if FAISSService exists alongside binary
const faissDir = path.join(vendorDir, "FAISSService");
if (fs.existsSync(faissDir)) {
  process.env.FAISS_SERVICE_PATH = faissDir;
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
