# B站票务三接口对比：getV2 / mall items_detail / stock/check

> 目的：为"实时票务状态监控"选对数据源。结论基于 2026-06-20 的实测（含真实采集出口、含 412 状态下的对照）。
> TL;DR：**mall items_detail/info 是最佳主轮询源（数据真实 + 风控低）**；getV2 数据真实但会周期性 412；**stock/check 数据真实性低，弃用**。

---

## 0. 一句话定位

| 接口 | 域 / 风控桶 | 数据真实性 | 风控（实测） | 角色 |
|---|---|---|---|---|
| **mall items_detail/info** | `mall.bilibili.com`（独立桶） | **高**（同 getV2 商详） | **低**（getV2 412 节点上仍可高频通过） | ✅ **主轮询源** |
| **getV2** | `show.bilibili.com` | 高 | **高**（持续轮询→周期性 412） | 兜底 / 交叉校验（低频） |
| **stock/check** | `show.bilibili.com` | **低**（3 不可信） | 极低（怎么打都不 412） | ❌ **弃用** |

---

## 1. 接口规格

### 1.1 getV2（项目商详，全量）
```
GET https://show.bilibili.com/api/ticket/project/getV2
    ?id=<project_id>&project_id=<project_id>&requestSource=neul-next
```
- ID 空间：`project_id`（如 1001701）。免登录。
- 返回：`data.screen_list[].ticket_list[]` 全量——`id`(skuId)/`desc`/`price`/`num`/`static_limit`/`sale_flag{number,display_name}`/`clickable`，外加项目级 `sale_flag`/`sale_start`/`current_time`/`id_bind`/价格区间/`wish_info` 等。

### 1.2 mall items_detail/info（项目商详，全量，另一个域）
```
POST https://mall.bilibili.com/mall-search-items/items_detail/info
Content-Type: application/json
{"itemsId": <project_id>}       # 注意：key 是 itemsId，不是 id；值直接用 project_id
```
- ID 空间：实测可直接把 `project_id` 作为 `itemsId` 提交（如 `1001701`）。免登录，不需要额外映射表。
- 返回：`data.screenList[].ticket_list[]`——**嵌套结构与 getV2 完全同构**（同 `id`/`desc`/`price`/`num`/`num_type`/`sale_flag`/`static_limit`/`clickable`），顶层 camelCase（`projectId`/`saleFlagNumber`/`salablePriceLow`/`idBind`/`buyerInfo`/`currentTime`/`hotProject`/`screenList`…）。注意 mall 可能省略项目级 `saleStart/saleEnd`，实现侧需从 screen 级 `sale_start/sale_end` 派生。

### 1.3 stock/check（单 SKU 库存态）
```
POST https://show.bilibili.com/api/ticket/stock/check
Content-Type: application/json
{"projectId": <project_id>, "skuId": <int>, "screenId": <int>}
```
- ID 空间：`project_id`+`screenId`+`skuId`（与 getV2 同空间）。免登录。
- 返回：`{"errno":0,"data":{"hasStock":bool,"stockStatus":1|2|3,"unpaidOrderId":null}}`
  - `stockStatus`：**1=暂时售罄, 2=已售罄, 3=有库存**；`hasStock` 同步。

---

## 2. 实测数据（同一节点真实出口，含 412 状态）

| 测试 | getV2 | mall items_detail | stock/check |
|---|---|---|---|
| 单节点低频通过率 | 周期性 412 / 5–9 成 | — | — |
| **412 节点上同时打** | `200 412 412 412 412`（被硬封） | **`15/15` 全 200** | **`10/10` 全 200** |
| 压测 | 易累计 412 | 15@5rps 全过 | 30@10rps 全过、裸请求都过 |
| 客户端指纹敏感性 | **极高**（同 IP：curl 9/10、原生 Go **0/3**、uTLS 7/10） | 低 | 低 |
| 登录 | 免 | 免 | 免 |

**关键现象**：同一个 IP，getV2 已 412，但 mall 与 stock/check 仍 100% 通——证明 412 是 **getV2 这条路被该 IP 累计请求打出来的硬风控，不是 IP 被封**（详见 §4）。

---

## 3. 数据能力对比（决定能不能用）

| 能力 | getV2 | mall | stock/check |
|---|---|---|---|
| 粒度 | 项目级（一次拿所有 SKU） | 项目级（一次拿所有 SKU） | 单 SKU（N 个 SKU = N 请求） |
| **精确余量 `num`** | ✅ | ✅ | ❌（只 3 态） |
| 价格 / 档位变动 | ✅ | ✅ | ❌ |
| 真实售卖状态 `sale_flag` | ✅ | ✅ | ❌ |
| 判断"未开售 vs 在售" | ✅（`sale_flag`/`sale_start`） | ✅ | ❌（未开售也返回 3） |
| 判断"已售罄" | ✅ | ✅ | ⚠️（2 勉强可信） |
| 判断"回流（有票了）" | ✅ | ✅ | ❌（见下，3 不可信） |
| SKU 合法性校验 | 严格 | 严格 | ❌（乱填 skuId 也返 3） |

### ⚠️ stock/check 为什么"数据真实性低、不应使用"
实测两条硬伤：
1. **不存在的 `skuId=999999` → 照样返回 `stockStatus=3`（有库存）**。说明 **3 是默认/兜底值，不代表真有票**——它甚至不校验 SKU 是否存在。
2. **未开售的 SKU 也返回 3**（库存物理存在但还没开售），无法区分"未开售"与"在售有票"。

→ 对一个靠"有票了 / 回流了"告警的监控系统，`stockStatus=3` 基本是噪声，会**大量误报**；很可能是 B站反脚本的误导值。**只有 2（已售罄）勉强有意义。结论：弃用。**

---

## 4. 风控机制（为什么 getV2 会 412，mall/stock 不会）

两级风控，**分清 429 与 412**：

| | 429 软限流 | 412 硬风控 |
|---|---|---|
| 触发 | **瞬时**频率超标 | **持续累计**到 IP 信誉阈值 |
| 性质 | 概率丢弃，立即可重试 | 硬封 **~9 分钟 TTL**，持续请求会一直续命 |
| 标识 | `server: openresty` | `x-sec-request-id`（安全层）+ 验证码页 |

- **getV2** 在 `show.bilibili.com`，是高风控接口；**持续轮询会累计触发 412**。典型现象：多节点持续高频轮询 getV2 → 全部 412 → 数据冻结，**周期性**（412→9min→恢复→又被打满→又 412）。
- **mall** 在 `mall.bilibili.com`，**独立风控桶**，实测打不出 412。
- **stock/check** 虽在 `show.bilibili.com`，但风控极松，怎么压都 200。
- **客户端指纹（JA3/JA4）是 getV2 的另一个独立闸门**：原生 Go 的 TLS 指纹被针对（同 IP 0/3），用 **uTLS 伪装 Chrome** 可拉到 ≈curl 水平（0→7/10）。但指纹修好后，**持续轮询 getV2 仍会累计 412**——指纹解决"单次能不能过"，不解决"持续打累计封"。

---

## 5. 推荐架构

```
主轮询（高频，真实数据，低风控）
  └─► mall items_detail/info(itemsId)  ──► num/sale_flag/price 全量真实  ──► 大盘 + 售罄/回流/开售告警

兜底 / 交叉校验（低频，省着用）
  └─► getV2(project_id)  ──► mall 临时失败时低频兜底；持续轮询会 412，必须独立限速 + uTLS 指纹

弃用
  └─► stock/check  ──► stockStatus=3 不可信，误报严重
```

**要点**
1. **mall 当主源**：同 getV2 的真实商详数据，但走独立桶不 412、项目级一次拿全。
2. **直接用 `project_id` 作为 `itemsId`**：不保留 `project_id → itemsId` 映射层；历史 `mall_items_id` 字段已删除。
3. **getV2 降为兜底/交叉校验**：仍是同源真值，但因会周期 412，只能低频用 + 必须 uTLS 伪装；mall 失败时的 getV2 fallback 也必须有独立预算，不能按 mall hot-path 频率执行。
4. **统一走 uTLS 浏览器 persona 客户端 + cookie jar**（即便 mall/stock 风控低，统一更稳）。
5. 所有接口均为**展示级缓存数据**（秒级延迟），非交易级强一致；mall/getV2 的 `num` 跌破 `static_limit.num` 时即真实剩余张数（见 `bilibili-getv2-monitoring.md` §5.1）。

---

## 6. 待验证
- **mall 满载压测**：当前已在 getV2 同 IP 412 状态下测到 mall burst 稳定通过；仍需持续观察多节点满载是否出现 mall 412。
- **mall 数据时效**：确认 mall 商详缓存延迟与 getV2 一致（理论同源）。
- **partial/empty response 抖动**：mall 高频后需监控 `screen_removed/sku_removed` 后短时间重新 added 的比例。
