import urllib.request
import hashlib
import sys

version = "0.1.5"
platforms = ["darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"]

for plat in platforms:
    url = f"https://github.com/ZetaZeroHub/FlashMemory/releases/download/v{version}/flashmemory_{version}_{plat}.tar.gz"
    req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    try:
        with urllib.request.urlopen(req) as response:
            data = response.read()
            h = hashlib.sha256(data).hexdigest()
            print(f"{plat}: {h}")
    except Exception as e:
        print(f"Failed {plat}: {e}")
