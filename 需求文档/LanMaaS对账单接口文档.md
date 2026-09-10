# LanMaaS 对账单接口文档

## 文档信息

| 项目 | 内容 |
| --- | --- |
| 文档版本 | V1.2 |
| 编制日期 | 2026-09-10 |
| 适用范围 | `v1.0.0-rc.10-lx-1.1` 分支后续开发 |
| 核心约束 | 数据库结构不变；不调整原有统计逻辑；只读分析现有 `logs` 数据；兼容 SQLite、MySQL、PostgreSQL |

## 1 文档定位

本文件定义对账单功能的后端接口契约。方案新增四个管理员接口，以现有消费日志为只读数据源，在请求发生时完成筛选选项查询与账单聚合。

- 不依赖 `quota_data` 的异步汇总结果。
- 不新增或修改数据库表、字段、索引。
- 不生成或写入新的统计数据。
- 不修改现有日志统计与数据看板接口、控制器和模型方法。
- 用户筛选从消费日志中提取实际产生过消费的用户快照。
- 模型筛选从消费日志中提取实际产生过消费的模型名称。
- 本期已确认只汇总 `LogTypeConsume = 2` 的消费日志，不纳入退款抵扣。

## 2 接口总览

| 方法 | 路径 | 用途 | 权限 |
| --- | --- | --- | --- |
| GET | `/api/reconciliation/bills` | 分页查询聚合后的对账明细 | 管理员 |
| GET | `/api/reconciliation/options/users` | 搜索消费日志中已有的用户选项 | 管理员 |
| GET | `/api/reconciliation/options/models` | 搜索消费日志中已有的模型选项 | 管理员 |
| GET | `/api/reconciliation/export` | 导出当前筛选条件下的全部聚合结果 | 管理员 |

## 3 通用约定

| 项目 | 约定 |
| --- | --- |
| 认证授权 | 新增接口沿用现有会话认证并使用 `middleware.AdminAuth`。 |
| 响应封装 | JSON 接口沿用 `success`、`message`、`data` 结构。 |
| 日期格式 | `start_date` 与 `end_date` 使用 `YYYY-MM-DD`，起止日期均包含在统计范围内。 |
| 时间边界 | 将结束日期转换为次日零点，查询条件使用 `created_at >= start` 且 `created_at < endExclusive`。 |
| 时区 | 日期边界与 `period` 按应用部署时区 `time.Local` 计算。 |
| 统计粒度 | `granularity` 只允许 `day` 或 `month`。日粒度返回 `YYYY-MM-DD`，月粒度返回 `YYYY-MM`。 |
| 费用口径 | `amount_usd = quota / QuotaPerUnit`。页面继续使用现有账单货币格式化逻辑。 |
| 分页 | `p` 默认 1；`page_size` 默认 20，最大 100；`total` 是聚合后的行数。 |
| 日期范围 | 单次查询最多包含 366 个自然日。 |
| 稳定排序 | `period` 倒序、`username` 升序、`model_name` 升序、`user_id` 升序。 |

## 4 对账明细查询

### 4.1 请求

```http
GET /api/reconciliation/bills
```

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `start_date` | string | 是 | 开始日期，格式为 `YYYY-MM-DD`。 |
| `end_date` | string | 是 | 结束日期，格式为 `YYYY-MM-DD`，不得早于开始日期。 |
| `user_id` | integer | 否 | 精确筛选用户编号；不传表示全部用户。 |
| `model_name` | string | 否 | 精确筛选模型名称；不传表示全部模型。 |
| `granularity` | string | 是 | `day` 或 `month`。 |
| `p` | integer | 否 | 页码，默认 1。 |
| `page_size` | integer | 否 | 每页条数，默认 20，最大 100。 |

固定的近 7 天、近 30 天和近 90 天由前端换算为 `start_date` 与 `end_date`。自定义区间只有在起止日期完整、格式正确且顺序合法时才发起请求。

请求示例：

```http
GET /api/reconciliation/bills?start_date=2026-08-10&end_date=2026-09-08&user_id=12&model_name=gpt-5&granularity=day&p=1&page_size=20
```

### 4.2 成功响应

```json
{
  "success": true,
  "message": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 128,
    "items": [
      {
        "user_id": 12,
        "username": "alice",
        "period": "2026-09-08",
        "model_name": "gpt-5",
        "quota": 617280000,
        "amount_usd": 1234.56
      }
    ]
  }
}
```

### 4.3 响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.page` | integer | 当前页码。 |
| `data.page_size` | integer | 当前每页条数。 |
| `data.total` | integer | 符合条件的聚合行总数。 |
| `data.items[].user_id` | integer | 用户编号，用于身份分组和行标识。 |
| `data.items[].username` | string | 消费日志中的账号快照。 |
| `data.items[].period` | string | `YYYY-MM-DD` 或 `YYYY-MM`。 |
| `data.items[].model_name` | string | 消费日志中的模型名称。 |
| `data.items[].quota` | integer | 该用户、模型、周期内的消费额度之和。 |
| `data.items[].amount_usd` | number | 系统美元基准金额。最终显示格式由前端处理。 |

### 4.4 聚合规则

1. 数据源固定为 `LOG_DB` 中现有的 `logs` 表。
2. 基础条件固定为 `logs.type = LogTypeConsume`。
3. 日期使用左闭右开的时间范围。
4. 分组键为 `user_id`、`model_name` 和 `period`。
5. `username` 使用组内非空账号快照输出，不参与身份判定。
6. `day` 按自然日聚合，`month` 按自然月聚合；切换粒度不改变原始日期范围。
7. `total` 使用相同条件的分组子查询计算，保证 `total` 与 `items` 口径一致。
8. 列表查询只执行读取和聚合，不更新 `logs`、`quota_data` 或 `users`。

## 5 日志筛选选项接口

### 5.1 用户筛选

```http
GET /api/reconciliation/options/users?keyword=ali&p=1&page_size=20
```

接口固定只读查询 `LogTypeConsume = 2` 的消费日志，按 `user_id` 去重，并返回日志中的非空账号快照。用户已从当前用户表删除或改名时，历史消费用户仍可作为筛选项。

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `keyword` | string | 否 | 账号关键词。 |
| `user_id` | integer | 否 | 精确查询用户编号，用于恢复已选项显示。 |
| `p` | integer | 否 | 页码，默认 1。 |
| `page_size` | integer | 否 | 每页条数，默认 20，最大 100。 |

成功响应：

```json
{
  "success": true,
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 1,
    "items": [
      {
        "user_id": 12,
        "username": "alice"
      }
    ]
  }
}
```

### 5.2 模型筛选

```http
GET /api/reconciliation/options/models?keyword=deepseek&p=1&page_size=20
```

接口固定只读查询 `LogTypeConsume = 2` 的消费日志，按实际 `model_name` 去重。`keyword` 对完整模型名执行包含搜索，返回值始终是日志中的实际模型名，不受模型元数据的精确、前缀、包含或后缀规则影响。

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `keyword` | string | 否 | 模型名称关键词。 |
| `p` | integer | 否 | 页码，默认 1。 |
| `page_size` | integer | 否 | 每页条数，默认 20，最大 100。 |

成功响应：

```json
{
  "success": true,
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 1,
    "items": [
      {
        "model_name": "deepseek-v4-flash"
      }
    ]
  }
}
```

## 6 对账导出

### 6.1 请求

```http
GET /api/reconciliation/export?start_date=2026-08-10&end_date=2026-09-08&user_id=12&model_name=gpt-5&granularity=month
```

筛选参数与明细查询相同，但不接收 `p` 和 `page_size`。服务端必须复用明细查询的过滤条件与聚合逻辑。

### 6.2 文件约定

| 项目 | 约定 |
| --- | --- |
| 成功响应 | 返回 XLSX 二进制流。`Content-Type` 为 `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`。 |
| 文件名 | 使用 UTF-8 文件名 `对账单_YYYY-MM-DD.xlsx`，日期为导出当天。 |
| 工作表 | 工作表名为“对账单”，列为“用户、时间、模型、费用”。 |
| 费用单元格 | 写入数值，应用千分位和两位小数格式，币种与当前账单货币配置一致。 |
| 排序 | 与明细接口相同；导出全部聚合行，不受页面分页影响。 |
| 大数据量 | 使用流式写入，最大导出 100000 行，请求超时 2 分钟；超过上限时明确报错，不得静默截断。 |

### 6.3 空数据响应

空数据时不生成附件，也不设置附件响应头。

```json
{
  "success": false,
  "message": "当前筛选条件下没有可导出的数据"
}
```

## 7 校验与错误处理

| 场景 | 响应约定 | 页面行为 |
| --- | --- | --- |
| 缺少日期或粒度 | `success = false`，`message` 指明缺失参数。 | 保留原列表并提示错误。 |
| 日期格式错误 | `message = 日期格式应为 YYYY-MM-DD`。 | 不提交新筛选。 |
| 开始日期晚于结束日期 | `message = 开始时间需早于结束时间`。 | 不刷新列表。 |
| 粒度非法 | `message = 时间粒度仅支持 day 或 month`。 | 恢复最近一次有效值。 |
| 用户或模型无匹配 | 日志选项接口成功返回空 `items`。 | 下拉显示无匹配文案。 |
| 查询无结果 | 列表成功返回 `total = 0` 和空 `items`。 | 显示“暂无数据”。 |
| 导出无结果 | 返回 JSON 失败响应且不设置附件头。 | 显示固定提示。 |
| 非管理员访问 | 由 `AdminAuth` 返回 401 或 403。 | 跳转 403 页面。 |
| 数据库或文件生成失败 | 服务端记录具体错误，客户端接收通用错误文案。 | 显示错误并允许重试。 |

## 8 安全与兼容要求

- 四个新增接口必须使用 `middleware.AdminAuth`。
- 所有查询使用参数绑定。禁止拼接用户输入到 SQL。
- JSON 编解码遵循项目约定，业务代码使用 `common/json.go` 的包装函数。
- 新增查询必须兼容 SQLite、MySQL 5.7.8 及以上、PostgreSQL 9.6 及以上。
- 数据库日期表达式集中封装在新增模型文件中，不修改原有统计实现。
- 接口不得返回日志 `content`、IP、令牌名、请求编号和渠道信息。
- 在不新增索引的约束下，使用日期范围、导出行数和超时限制控制查询成本。

## 9 验收清单

- [ ] 日期、用户、模型和粒度条件叠加后，列表与导出结果一致。
- [ ] 用户和模型下拉只包含已有消费日志中的去重值，模糊搜索返回实际可筛选值。
- [ ] 用户已删除、账号已改名、模型已下线或使用规则匹配时，历史日志选项仍可选择。
- [ ] 日月粒度的 `period` 格式、分组数量和费用总和正确。
- [ ] 结束日期全天的数据均被统计，不存在跨日重复或遗漏。
- [ ] `total` 按聚合行计算，分页默认 20，筛选变化后回到第 1 页。
- [ ] 排序满足时间倒序、账号升序，并在字段相同时保持稳定。
- [ ] 非管理员无法调用新增对账接口。
- [ ] 原有统计接口的请求、响应和结果保持不变。
- [ ] SQLite、MySQL、PostgreSQL 使用同一组测试数据得到相同结果。
- [ ] 导出文件名、列顺序、数值格式和空数据行为符合约定。
