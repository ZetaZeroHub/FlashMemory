```shell
curl -u zzh:zzh \    
  -X POST http://localhost:5532/api/index \
  -H 'Content-Type: application/json' \
  -d '{
    "project_dir": "/Users/apple/Public/openProject/flashmemory",
    "relative_dir": "config"
  }'
```

```shell
curl -u zzh:zzh \    
  -X POST http://localhost:5532/api/index \
  -H 'Content-Type: application/json' \
  -d '{
    "project_dir": "/Users/apple/Public/openProject/flashmemory",
    "relative_dir": "internal/utils"
  }'
```

```shell
curl -u zzh:zzh \
  -X POST http://localhost:5532/api/search \
  -H 'Content-Type: application/json' \
  -d '{
    "project_dir": "/Users/apple/Public/openProject/flashmemory",
    "query":       "调用ollama",
    "search_mode": "hybrid",
    "limit":       3
  }'
```