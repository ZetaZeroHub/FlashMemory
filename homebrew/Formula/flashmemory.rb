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
  version "0.4.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_amd64.tar.gz"
      sha256 "dfcb6aa4baf5e4c9e139c6752671c9f829c1ab760f2aa6a12bb6fe1dbd339ea0"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_arm64.tar.gz"
      sha256 "a7cfb9b8a331f4514db05094a4815f834bf54e984725d1a7cb8811263adebcaf"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_amd64.tar.gz"
      sha256 "d1f48339f855f7e787c5d4b6ff48eeae38988707c909cae265dc1fa53911534d"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_arm64.tar.gz"
      sha256 "ef5e1449fa73a337cdc2f30b90198700f79ffb977740b33a5c79e4186192ee54"
    end
  end

  def install
    # Place actual binaries in libexec to avoid PATH conflicts
    libexec.install "fm"
    libexec.install "fm_core"
    libexec.install "fm_http"

    # Install FAISSService to libexec
    if File.directory?("FAISSService")
      (libexec/"FAISSService").install Dir["FAISSService/*"]
    end

    # Install example config
    if File.exist?("fm.yaml.example")
      (etc/"flashmemory").install "fm.yaml.example" => "fm.yaml"
    end

    # Create wrapper scripts in `bin` that set FAISS_SERVICE_PATH
    (bin/"fm").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{libexec}/fm" "$@"
    EOS
    (bin/"fm").chmod 0755

    (bin/"fm_core").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{libexec}/fm_core" "$@"
    EOS
    (bin/"fm_core").chmod 0755

    (bin/"fm_http").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{libexec}/fm_http" "$@"
    EOS
    (bin/"fm_http").chmod 0755
  end

  test do
    assert_match "FlashMemory", shell_output("#{bin}/fm version 2>&1", 0)
  end
end
