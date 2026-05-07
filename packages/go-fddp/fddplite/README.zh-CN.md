# FDDP Lite 中文入口

这是 FDDP Lite 的中文入口页，不是完整 API 翻译。完整细节以英文文档为准：

- [FDDP Lite API](README.md)
- [FDDP Lite Getting Started](../../../docs/lite/getting-started.md)
- [Install FDDP](../../../docs/install.md)

## Lite 解决什么

FDDP Lite 是给小项目和 MVP 的低样板入口。

它让 Go/GORM 项目从普通模型开始，快速注册一个受控数据契约，然后让前端生成类型安全的 `api.load(...)` 和 `api.command...` 调用。

Lite 的目标不是绕过后端，也不是把数据库直接暴露给前端。后端仍然控制：

- 哪些字段能读。
- 哪些字段能过滤和排序。
- 哪些关系能展开。
- 当前用户和租户能看哪些行。
- 一次查询最多能有多深、多宽、多贵。

## 最短路径

先按 [Install FDDP](../../../docs/install.md) 安装当前版本，然后创建 starter：

```bash
npx fddp new my-fddp-app
```

启动后端：

```bash
cd my-fddp-app/backend
go mod tidy
go run .
```

生成前端类型：

```bash
cd ../frontend
npm install
npm run codegen
npm run typecheck
```

## 三个核心注册

`FieldGroup` 适合多个字段来自同一行数据：

```go
_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()
```

当前端请求 `me.profile.id`、`me.profile.name`、`me.profile.description` 时，后端可以一次查出 profile，而不是每个字段查一次。

`Collection` 适合列表：

```go
_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register()
```

这会发布一个租户范围内的项目列表，并且只允许已注册字段、过滤、排序、分页和 `owner` 关系展开。

`Command` 适合简单写操作：

```go
_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Idempotent().
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()
```

写操作仍然是显式命令，不是让前端自由改字段。

## 安全默认值

Lite 入口短，但底层仍然经过 FDDP Core 和 GORM adapter：

- 客户端字段不会直接变成 SQL 字段名。
- 未注册字段不能选择、过滤、排序。
- 未注册关系不能展开。
- `Self("UserID")` 和 `Tenant("TenantID")` 会加入后端行边界。
- Query Guard 会在 GORM 执行前拒绝过大、过深、过贵的请求。
- Command Guard 会限制写入请求体大小、深度和执行时间。

## 从小项目演进

本地开发和 MVP：

```go
app := fddplite.NewDevApp(db)
```

部署时：

```go
app := fddplite.NewProductionApp(db,
  fddp.WithIdentityResolver(identityResolver),
  fddp.WithContractVersion("contract_v1"),
)
```

项目变大后：

```go
app := fddplite.NewApp(db, options...)
```

简单领域继续用 Lite。复杂领域可以逐步下沉到 `gormadapter` 或 `app.Engine().RegisterResource(...)`，不需要一次性重写。

## 下一步

- 想快速跑通：看 [FDDP Lite Getting Started](../../../docs/lite/getting-started.md)。
- 想看完整 Lite API：看 [英文 Lite README](README.md)。
- 想看 GORM 映射细节：看 [gormadapter](../gormadapter/README.md)。
- 想看端到端 demo：看 [examples/demo](../../../examples/demo/README.md)。
