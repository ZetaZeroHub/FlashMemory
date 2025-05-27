```shell
curl -u root:root -X POST http://localhost:5532/api/index \
-H 'Content-Type: application/json' \
-d '{
"project_dir": "/Users/apple/Public/openProject/flashmemory",
"relative_dir": "config"
}'
```

```shell
curl -u root:root -X POST http://localhost:5532/api/index \
-H 'Content-Type: application/json' \
-d '{
"project_dir": "/Users/apple/Public/openProject/flashmemory",
"relative_dir": "internal/utils"
}'
```

```shell
curl -u root:root -X POST http://localhost:5532/api/search \
-H 'Content-Type: application/json' \
-d '{
"project_dir": "/Users/apple/Public/openProject/flashmemory",
"query":       ""我要找一个判断文件是否隐藏的代码"",
"search_mode": "hybrid",
"limit":       3
}'
```

```shell
curl -u root:root \
-X POST http://localhost:5532/api/functions \
-H 'Content-Type: application/json' \
-d '{
"project_dir":  "/Users/apple/Public/openProject/flashmemory"
}'
```