# DeepWiki(Go版) API 文档（占位）

API 契约以《DeepWiki(Go版) 系统设计基线》§5、§6 与《01_API_正式版》为唯一权威：
- 端点总表：§5.1（18 个端点，/api/v1 前缀，/metrics 除外）
- 统一信封与错误码：§5.2、§5.3（v2 新增 50302/50303/50304/50203 四个基础设施错误码）
- 关键端点 Schema：§6.1~§6.7（ingest / ask / ask-stream SSE / events SSE / config / health / 其余端点）
- health 响应新契约：见《03_企业级技术栈变更总纲》§7（postgres/opensearch/rabbitmq/redis/etcd/git 六个依赖字段）

本目录在后续迭代中补充 OpenAPI / 示例集合。
