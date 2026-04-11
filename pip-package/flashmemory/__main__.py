"""
Entry point for `python -m flashmemory.zvec_bridge`.

This enables Go binary to launch the bridge via module mode
without needing to locate the .py file on disk.
"""
from flashmemory.zvec_bridge import main

if __name__ == "__main__":
    main()
