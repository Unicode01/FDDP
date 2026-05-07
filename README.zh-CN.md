# FDDP 中文说明

这是 FDDP 的中文入口页。完整、最新的 API 细节仍以英文文档为准：

- [English README](README.md)
- [Install FDDP](docs/install.md)
- [FDDP Lite Getting Started](docs/lite/getting-started.md)

## FDDP 是什么

FDDP 是一个后端主导的应用数据访问治理层。

它让后端发布受控的数据契约，前端通过生成的 TypeScript SDK 读取页面数据和执行简单命令。真正的权限、租户边界、字段到数据库的映射、查询成本、缓存安全和契约演进，都由后端掌握。

它不是为了替代所有 REST。更合理的分工是：

- REST 继续处理登录、支付、上传下载、Webhook、长任务、第三方回调和稳定公开接口。
- FDDP 处理页面数据、资料片段、仪表盘、筛选列表、关联展开和简单模型命令。

## 什么时候适合用

FDDP Lite 适合从小项目开始试用，尤其是：

- Go 后端。
- 使用 GORM 模型。
- 页面经常需要 profile、dashboard、list、relation expand。
- 不想为每个页面字段不断新增 REST/BFF 小接口。
- 希望从一开始保留权限、租户、查询限制和契约演进空间。

如果项目只是一次性脚本、极小后台页面、没有复杂权限，也可以继续用普通 REST。

## 最快开始

先按 [Install FDDP](docs/install.md) 安装当前版本，然后创建 starter：

```bash
npx fddp new my-fddp-app
```

启动后端：

```bash
cd my-fddp-app/backend
go mod tidy
go run .
```

生成并检查前端类型：

```bash
cd ../frontend
npm install
npm run codegen
npm run typecheck
```

## Lite 心智模型

FDDP Lite 的后端写法主要是三类注册：

- `FieldGroup`：多个字段来自同一行数据，例如 `me.profile.id/name/description`，后端可以一次查询。
- `Collection`：列表资源，包含字段、过滤、排序、分页和安全关联展开。
- `Command`：简单写操作，包含输入限制、幂等和缓存失效信息。

示例：

```go
app := fddplite.NewDevApp(db)

_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()

_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  Relation("owner", "Owner", "ID", "Name").
  Register()
```

前端生成类型后可以写：

```ts
const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: {
    first: 20,
    fields: ["id", "name"],
    expand: { owner: ["id", "name"] }
  }
});
```

## 从小到大

小项目可以从 `fddplite.NewDevApp(db)` 开始。

部署时切到 `fddplite.NewProductionApp(db, options...)`，补上真实的身份校验、查询限制、缓存和幂等存储。

当某个领域变复杂时，不需要重写整个项目。简单领域继续用 Lite，复杂领域可以下沉到 `gormadapter` 或 FDDP Core，只要契约兼容，前端 `api.load(...)` 调用可以保持稳定。

## 中文文档入口

- [FDDP Lite 中文入口](packages/go-fddp/fddplite/README.zh-CN.md)
- [安装与发布版本规则](docs/install.md)
- [英文 Lite 入门](docs/lite/getting-started.md)
- [Go runtime 文档](packages/go-fddp/README.md)
- [TypeScript SDK 文档](packages/nextjs-sdk/README.md)
- [中文架构原型](docs/prototypes/fddp-architecture-prototype-v9.md)

## 当前边界

当前版本仍处于 alpha 阶段。核心链路可用，但 API 仍可能演进。

生产使用前至少要处理：

- 用真实 JWT/session 校验替换 demo header 身份。
- 根据业务调好查询和命令限制。
- 多实例部署时补持久化 cache/idempotency store。
- 不要把文件上传、下载、长任务、支付这类业务流程接口强行放进 FDDP。

中文文档只做导读，避免和英文 API 文档长期漂移。
