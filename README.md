# fnmdp

B 站会员购票务状态实时监控：开售提醒、售罄回流、余量下探、票档变动、多项目状态墙。

> [!WARNING]
> **🧠💤 先打个预防针**：这是作者拿来测 AI 编程能力的娱乐项目——全程大脑离线，需求一甩、回车一敲，剩下全交给 AI。它确实能跑，也真在线上扛过了一整季，但请把它当成一场整活实验，而不是生产级参考答案。代码哪里看着不对劲？多半是 AI 的锅，作者只负责点「同意」。

**🎫 本季已收摊**：线上实例 `fnmdp.bilibili.bi` 已经下线，代码留在这里当个纪念。**明年 BW 再见 👋**

（本项目是个人作品，**与哔哩哔哩官方无关**。）

## 架构

```
采集节点 node (Go) ──WebSocket──▶ 中央主控 master (Go) ──WebSocket/REST──▶ 前端 web (React)
```

- **node**：轮询公开、免登录的商品详情接口，把结果归一化后经 WebSocket 推给 master。主轮询源是 `mall items_detail/info`，`getV2` 只做低频兜底/交叉校验（见 [`docs/bilibili-endpoints-comparison.md`](docs/bilibili-endpoints-comparison.md)）。
- **master**：给节点分配项目、三级去重（节点 L1 → 跨节点 L2 → 状态机 L3）、事件分类，向前端推送状态与事件。Postgres / Valkey 可选，连不上时自动降级为内存实现。
- **web**：React + HeroUI v3 的实时状态墙。
- **protocol**：[`protocol/schema.json`](protocol/schema.json) 是所有 WebSocket 消息的单一真源，Go 与 TS 两端各有一致性测试校验。

| 目录 | 内容 |
|---|---|
| `shared/` | Go 共享库：mall/getV2 客户端与归一化、去重键、协议类型 |
| `master/` | Go 中央主控 |
| `node/` | Go 采集节点 |
| `web/` | React 前端（Vite） |
| `protocol/` | 消息 schema |
| `deploy/` | Kubernetes / Docker Compose 部署**样例** |
| `docs/` | 上游接口说明与选型笔记 |

## 本地开发

```sh
# Go（go.work 工作区，根目录没有 go.mod，要写明包路径）
go build ./shared/... ./master/... ./node/...
go test  ./shared/... ./master/... ./node/...

go run ./master                                                   # :8080，无 PG/Valkey 时用内存存储
go run ./node --standalone --projects 1001701 --interval 30s      # 不连 master，单机打印轮询结果
go run ./shared/cmd/getv2 --id 1001701                            # 单次低频 getV2 探针

# Web
pnpm install
pnpm --dir web dev      # http://localhost:5173，/api 与 /live 代理到本地 master
pnpm --dir web build
pnpm --dir web test
```

> [!NOTE]
> 前端依赖付费组件库 [HeroUI Pro](https://heroui.pro)（`@heroui-pro/react`）。仓库只包含依赖声明，不包含其源码；没有 Pro 授权（`HEROUI_AUTH_TOKEN`）时 `pnpm install` 只会装出空壳，前端无法构建。Go 部分不受影响。

### 主要环境变量

| 组件 | 变量 | 说明 |
|---|---|---|
| master | `LISTEN_ADDR` | 默认 `:8080` |
| master | `NODE_TOKEN` | 节点接入口令。**务必设置**，为空时任何节点都能接入 |
| master | `ADMIN_TOKEN` | `/api/admin/projects` 管理接口口令。**务必设置**，为空时不鉴权 |
| master | `PGHOST` `PGPORT` `PGUSER` `PGPASSWORD` `PGDATABASE` `PGSSLMODE` | Postgres，可选 |
| master | `VALKEY_ADDR` `VALKEY_PASSWORD` `VALKEY_DB` `VALKEY_TLS` | Valkey，可选 |
| node | `MASTER_URL` | 例如 `wss://fnmdp.example.com/ingest` |
| node | `NODE_ID` `NODE_NAME` `REGION` `NODE_TOKEN` | 节点身份与口令 |

调度、限速等其余参数见 [`master/internal/config/config.go`](master/internal/config/config.go)
与 [`deploy/k8s/configmap.yaml`](deploy/k8s/configmap.yaml)。

## 部署

见 [`deploy/README.md`](deploy/README.md)。里面的命名空间、域名、网关名都是样例，按自己的环境替换。镜像请用各目录下的 `Dockerfile` 自行构建（构建上下文是仓库根目录，例如 `docker build -f node/Dockerfile -t fnmdp-node .`）。

CI 默认只做构建和测试；发布镜像、部署到集群需要在仓库变量里打开 `PUBLISH_IMAGES` / `DEPLOY_ENABLED`，并配置对应的 secrets（见 [`.github/workflows/ci.yml`](.github/workflows/ci.yml)）。

## 关于数据采集

- 只读取公开、免登录的商品详情接口；不登录、不下单、不提供任何抢票功能。
- 节点按固定的浏览器特征（UA、TLS 指纹、匿名 buvid）发请求，并自带限速、429 退避与 412 冷却。请遵守上游的访问频率限制，别拿它去给别人的服务器添堵。
- 本项目与哔哩哔哩无任何关联，仅供学习研究；使用者自行承担由此产生的一切后果。

## License

[AGPL-3.0](LICENSE)

附加许可（AGPL-3.0 第 7 条）：作为本项目的版权持有人，作者额外允许将本项目的前端代码与 HeroUI Pro（`@heroui-pro/*`）组合、构建与分发，HeroUI Pro 部分不受 AGPL 约束、无需公开其源码；本项目自身代码仍按 AGPL-3.0 提供。

字体 [Inter](https://github.com/rsms/inter) 以 SIL OFL 1.1 授权，见
[`web/public/static/Inter-LICENSE.txt`](web/public/static/Inter-LICENSE.txt)。
