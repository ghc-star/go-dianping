# go-dianping

Go 版大众点评练手项目：验证码登录与会话、商铺缓存、笔记与点赞、关注 Feed、图片上传与附近商铺（Redis GEO）、优惠券秒杀（Lua 预扣 + 消息异步落单）。

## 技术栈

Go 1.27 · Gin · GORM (MySQL 8.4) · go-redis/v9 · RabbitMQ

## 本地运行

1. 准备 MySQL 和 Redis（Windows 下可参照 `start-local.ps1`），执行表结构和数据初始化 SQL。
2. 设置环境变量 `MYSQL_DSN`、`REDIS_ADDR`（可选 `RABBITMQ_URL`）。
3. 启动：

```powershell
make run
# 或 go run ./cmd/service
```

## 开发与验收

```powershell
make help      # 查看所有目标
make ci        # 格式检查 → vet → 测试 → 编译（与 CI 同一套命令）
make test-race # 竞态检测（需要 CGO）
```

## CI/CD

- push 到 master 或提 PR 时自动执行 `make ci` 与竞态检测（GitHub Actions）。
- PR 会自动获得 AI 代码审查评论（`.github/workflows/ai-review.yml` + `.github/ai-review/` 下的任务书与项目规则）。

实现顺序与各接口验收标准见 `docs/复现顺序.md`。
