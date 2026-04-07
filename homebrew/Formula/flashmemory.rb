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
  version "0.1.2"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_amd64.tar.gz"
      sha256 "78a8812a8202bf502f321400fb858e0f174b85f4bdb213cb81943f051c3c921a"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_darwin_arm64.tar.gz"
      sha256 "7e8dcb756e2e79a64144319c4c7bfef5d2d81e68b453518281c739563edacfa3"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_amd64.tar.gz"
      sha256 "a12a22a9765d9b22f380590a0cb6385b0705fa568b5fec1d63456f1d83df2cec"
    end
    on_arm do
      url "https://github.com/ZetaZeroHub/FlashMemory/releases/download/v#{version}/flashmemory_#{version}_linux_arm64.tar.gz"
      sha256 "6a76f677eb7f76c00bfbfc1e8b3e445b6d744b3b8d387c3509077765221afe2f"
    end
  end

  def install
    # Execute binaries should go to libexec to avoid PATH conflicts when we write wrappers
    libexec.install "fm_http"
    libexec.install "fm"

    # Install FAISSService to libexec so it's available but not in PATH
    if File.directory?("FAISSService")
      (libexec/"FAISSService").install Dir["FAISSService/*"]
    end

    # Install example config
    if File.exist?("fm.yaml.example")
      (etc/"flashmemory").install "fm.yaml.example" => "fm.yaml"
    end

    # Create wrapper scripts in `bin` that set FAISS_SERVICE_PATH before calling the real binaries
    (bin/"fm").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{libexec}/fm" "$@"
    EOS
    (bin/"fm").chmod 0755

    (bin/"fm_http").write <<~EOS
      #!/bin/bash
      export FAISS_SERVICE_PATH="#{libexec}/FAISSService"
      exec "#{libexec}/fm_http" "$@"
    EOS
    (bin/"fm_http").chmod 0755
  end

  test do
    assert_match "OK", shell_output("#{bin}/fm --help 2>&1", 0)
  end
end
