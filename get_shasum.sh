#!/bin/bash
VERSION=0.1.4
file=tmp_download.tar.gz
for OS_ARCH in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  URL="https://github.com/ZetaZeroHub/FlashMemory/releases/download/v${VERSION}/flashmemory_${VERSION}_${OS_ARCH}.tar.gz"
  curl -sL "$URL" -o $file
  hash=$(shasum -a 256 $file | awk '{print $1}')
  echo "${OS_ARCH}: ${hash}"
done
rm -f $file
