# go-dianping 项目规则（skills）
# 机器人的"项目常识"：技术栈、分层约定、业务红线。审查质量取决于这份文件写得多具体。

## 项目与技术栈

- Go 1.27 + Gin + GORM(MySQL 8.4) + go-redis/v9 + RabbitMQ(amqp091-go)。
- 入口 `cmd/service/main.go`：连接 MySQL/Redis（可选 RabbitMQ），组装 Repository → Service → Handler，注册路由，监听 :8080。
- 目录：`internal/handler`（解析请求）、`internal/service`（业务规则）、`internal/repository`（数据库）、`internal/dto`、`internal/middleware`（登录鉴权）、`internal/cache`、`internal/mq`、`internal/redisx`（Lua 脚本）、`internal/model`。
- 验收命令：`make ci` = gofmt 检查 → go vet → go test → go build，改动必须能通过。

## 分层与调用约定

- Handler 只解析参数和组装响应；业务规则在 Service；SQL 和事务只在 Repository。反向调用（Repository 调 Service）是错误。
- 请求 Context 沿调用链传递；MySQL/Redis 客户端启动时创建复用，不要在请求内新建连接。
- 纯 Redis 功能（签到等）不建 Repository。

## 响应与错误契约

- 成功：`{"success":true,"data":...}`；失败：`{"success":false,"errorMsg":"..."}`。
- 状态码：参数错误 400、未登录 401、不存在 404、频率限制 429、系统错误 500、依赖故障 503。
- 鉴权 header 是 `authorization: TOKEN`（无 Bearer 前缀）；受保护接口无 Token 必须 401。
- 数据库故障、Redis 故障、资源不存在必须区分，不能都返回同一个错误。

## 数据库约定

- 用户表是 `tb_user`；一人一单靠 `(user_id, voucher_id)` 唯一索引。
- ID 一律 `int64`，解析后校验 `id > 0`。
- GORM `Scan` 无记录时检查 `RowsAffected`，不要假设返回 `gorm.ErrRecordNotFound`。
- 扣库存必须用"库存 > 0"的条件更新；扣库存与插订单在同一事务。

## 缓存与 Redis 规则

- 商铺详情缓存：空值缓存防穿透（数据库故障不能缓存为空值）、TTL 加随机偏移、singleflight 合并回源、分布式锁用唯一 token + Lua 比较后解锁。
- 更新商铺：先更新数据库再删缓存。
- 签到用 Bitmap（key 含月份，offset = 日期-1）；点赞排名用 ZSet；Feed 收件箱用 ZSet（score 是时间戳）。
- Lua 脚本运行错误不会自动回滚已执行的写入，注意 key 类型预检。

## 秒杀与异步（高风险）

- Lua 校验时间、库存、重复购买并预扣库存，接口成功只代表"已受理"。
- Worker 消费消息后事务落单，成功才 ACK；同一订单重复消费不得重复扣库存。
- 数据库提交结果不确定时保留消息重试，不能立即退库存；补偿必须幂等。
- 最终不得超卖、不得重复落单。

## 并发与生命周期

- 后台 goroutine（Feed Worker、秒杀 Worker）必须随 context 取消而退出；退出时先停接流量、再等 Worker、最后关连接。
- 共享状态加锁或用 channel；能被 `go test -race` 抓到的竞争是 P1。

## 安全

- 密码只存 bcrypt 哈希；日志、响应、错误信息不得包含密码、手机号、验证码、token。
- 不能让客户端指定作者/归属用户 ID；查询订单、笔记、文件先校验归属。
- 图片上传校验大小与真实文件类型；删除文件校验归属和路径范围，不能把用户输入当本地路径。

## 高风险区域（涉及即提高警惕）

`internal/redisx/`、`internal/mq/`、`internal/cache/`、`internal/middleware/`、`internal/handler/router.go`、`cmd/service/main.go`、`Makefile`、`.github/workflows/`。
