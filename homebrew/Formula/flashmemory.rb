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
  version "0.1.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_amd64.tar.gz"
      sha256 "82679fa8dc98ab41911e13955ef79ee229d5e2d84668b5ce3cd4f2e8c85d56e2"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_arm64.tar.gz"
      sha256 "3e7c5c96119cb1f878192fe6246f3b8e8360d5e99124e89c7f4446a162090d6c"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_amd64.tar.gz"
      sha256 "99b76c71c566876cd10982e8cba439a7c8cf0ec1d2dd66408077514271e04ec2"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_arm64.tar.gz"
      sha256 "32a427daa0972fb9a52daf2021f53f40d2ed582e38f0460e19f61e38548ad9d9"
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
