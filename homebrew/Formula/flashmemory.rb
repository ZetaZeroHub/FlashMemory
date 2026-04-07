# FlashMemory Homebrew Formula
#
# 使用方法:
#   1. 在 GitHub 上创建仓库: ZetaZeroHub/homebrew-flashmemory
#   2. 将此文件放入该仓库根目录，命名为 Formula/flashmemory.rb
#   3. 用户安装: brew tap ZetaZeroHub/flashmemory && brew install flashmemory
#
# 每次发布新版本后，需要更新 version、url 和 sha256
# 可使用 scripts/update-homebrew.sh 自动更新

class Flashmemory < Formula
  desc "Cross-language code analysis and semantic search system"
  homepage "https://github.com/ZetaZeroHub/FlashMemory"
  version "0.0.1"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    end
  end

  def install
    bin.install "fm_http"
    bin.install "fm"

    # Install FAISSService to libexec so it's available but not in PATH
    if File.directory?("FAISSService")
      (libexec/"FAISSService").install Dir["FAISSService/*"]
    end

    # Install example config
    if File.exist?("fm.yaml.example")
      (etc/"flashmemory").install "fm.yaml.example" => "fm.yaml"
    end

    # Create wrapper scripts that set FAISS_SERVICE_PATH
    (bin/"fm").unlink if File.exist?(bin/"fm")
    (bin/"fm_http").unlink if File.exist?(bin/"fm_http")

    # Re-install with wrapper
    (bin/"fm_raw").write buildpath/"fm" if File.exist?(buildpath/"fm")
    (bin/"fm_http_raw").write buildpath/"fm_http" if File.exist?(buildpath/"fm_http")

    (bin/"fm").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{bin}/fm_raw" "$@"
    EOS

    (bin/"fm_http").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{bin}/fm_http_raw" "$@"
    EOS
  end

  test do
    assert_match "OK", shell_output("#{bin}/fm --help 2>&1", 0)
  end
end
