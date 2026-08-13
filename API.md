# iCloud Hide My Email API 文档

## 概述

HTTP JSON API，所有接口返回统一格式：

```json
{
  "success": true,
  "data": {},
  "message": ""
}
```

**错误响应:**
- `400 Bad Request` — 参数错误
- `401 Unauthorized` — 缺 token / token 无效,或 iCloud 会话失效(后者带 `scope: "upstream"`)
- `403 Forbidden` — 非 admin 尝试调用 admin only 接口
- `404 Not Found` — 账号或别名不存在(user 尝试操作不属于自己的别名也返回 404)
- `410 Gone` — 待验证的登录会话已超时(见密码登录第 2 步)
- `429 Too Many Requests` — iCloud 限流
- `502 Bad Gateway` — iCloud 服务错误

## 鉴权

所有接口都需要在请求头带 token(择一):

```http
Authorization: Bearer <token>
X-API-Key: <token>
```

Admin token 来自服务启动时的 `ADMIN_TOKEN` 环境变量,能调所有接口;子 token 由 admin 通过 `POST /api/tokens` 创建,只能:
- 创建自己的 HME 别名并被统计
- 查看/停用/激活/删除自己创建的别名
- 读自己创建的别名的邮件(必须传 `alias`)

---

## 管理 API Token (admin only)

### 列出所有 token

```http
GET /api/tokens
```

响应:
```json
{"success":true,"data":[
  {"id":"tk_xxx","name":"laptop","role":"user","alias_count":42,
   "created_at":"...","last_used_at":"..."}
]}
```

`secret` 不会返回。

### 创建子 token

```http
POST /api/tokens
Content-Type: application/json

{"name":"laptop"}
```

响应(仅这一次可见 secret 明文):
```json
{"success":true,"data":{
  "id":"tk_xxx","name":"laptop","role":"user",
  "secret":"<32 字节 base64url>","created_at":"..."
}}
```

### 删除 token

```http
DELETE /api/tokens/:id
```

不允许删 admin。

---

## 核心接口

### 1. 创建 HME 别名

```http
POST /api/create
Content-Type: application/json

{
  "account_id": "acc_1",
  "label": "注册某网站"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "email": "xyz123@icloud.com",
    "label": "注册某网站",
    "created_at": "2024-01-15T10:30:00Z",
    "account_id": "acc_1"
  }
}
```

**参数说明:**
- `account_id` (必填) — 账号 ID
- `label` (可选) — 别名标签，默认为 "Created YYYY-MM-DD HH:mm"

**错误情况:**
- `401` — Cookie 过期，需更新
- `502` — iCloud 服务错误，会自动重试 5 次

---

### 2. 读取邮件

```http
GET /api/inbox?account_id=acc_1&alias=xyz123@icloud.com&limit=20&offset=0&days=7
```

**响应 (走 IMAP,App Password):**
```json
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "alias": "xyz123@icloud.com",
    "count": 2,
    "limit": 20,
    "offset": 0,
    "total": 137,
    "total_exact": true,
    "method": "imap",
    "messages": [
      {
        "id": "1042",
        "from": "GitHub <noreply@github.com>",
        "to": "xyz123@icloud.com",
        "subject": "[GitHub] Please verify your email address",
        "date": "2026-07-09T14:32:10+08:00",
        "preview": "Almost done! To finish setting up your account, we just need to verify..",
        "html": "<html><body>Almost done! ...</body></html>",
        "folder": "INBOX"
      }
    ]
  }
}
```

**响应 (回退到 Web API,Cookie):** `method` 变为 `web_api`
```json
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "alias": "xyz123@icloud.com",
    "count": 1,
    "limit": 20,
    "offset": 0,
    "total": 1,
    "total_exact": false,
    "method": "web_api",
    "messages": [
      {
        "id": "AQMkAD...",
        "from": "GitHub <noreply@github.com>",
        "to": "xyz123@icloud.com",
        "subject": "[GitHub] Please verify your email address",
        "date": "Wed, 09 Jul 2026 06:32:10 GMT",
        "preview": "Almost done! To finish setting up your account.."
      }
    ]
  }
}
```

**参数说明:**
- `account_id` (必填) — 账号 ID
- `alias` (可选) — 只返回发到该别名的邮件;不传返回收件箱最近邮件
- `limit` (可选) — 每页条数，默认 20
- `offset` (可选) — 跳过前几条，默认 0。配合 `limit` 翻页
- `days` (可选) — 查找最近几天的邮件，默认 7 (仅 IMAP 模式)

**分页 (`total` / `total_exact`):**

`count` 是本页条数,`total` 是整个结果集的条数 —— 客户端拿 `total` 算页数,
别拿 `count`。`total_exact` 为 `false` 时 `total` 只是"至少这么多",不能当准数:

- **IMAP** — `total_exact: true`。先搜出全部匹配的 UID 再排序切页,总数是准的。
  单个邮箱的候选上限 2000 封(不限 `days` 时邮箱可能有上万封),超过会截断并把
  `total_exact` 置为 `false`
- **Web API** — 恒为 `false`。上游 `thread/search` 只有 `maxResults`,给不出结果集
  总数,只能按 `offset+limit` 拉一个窗口回来本地切,后面可能还有更早的邮件

**邮件读取双路径 (自动选择):**
1. **优先: IMAP (App Password)** — 设置了 App Password 时使用,支持服务端按收件人搜索
2. **回退: Web API (Cookie 认证)** — 无 App Password 或 IMAP 失败时,通过 iCloud mccgateway 端点读取

响应中 `"method": "imap"` 或 `"method": "web_api"` 标识实际使用的读取方式。

**别名过滤逻辑:**
- **IMAP (`FindByRecipient`):** 先用原生 IMAP `TO` 头搜索 (配合 `days` 时间范围);无结果时退回拉信封本地按 `To` 兜底过滤
- **Web API (`FindByAlias`):** iCloud Web API 不支持按收件人搜索,拉取 `(offset+limit)*3` (至少 50) 条后本地对 `Subject`/`From`/`To` 做包含匹配

**IMAP 分两阶段取:** 先只拉信封 (`UID`/`ENVELOPE`/`INTERNALDATE`) 把各邮箱的邮件
排序、算总数、切出当前页,再单独去取这一页的完整正文。正文是最贵的一步 (营销邮件
动辄几百 KB),不这么分的话翻第 1 页也得把几百封信的正文全下载一遍。

**返回字段差异 (两条路径):**
- `id` — IMAP 是 UID 数字串,Web API 是 iCloud GUID。IMAP 的 UID 只在单个邮箱内唯一,
  收件箱和垃圾箱合起来看时要连 `folder` 一起做键
- `date` — IMAP 走 RFC3339,Web API 是原始邮件头 RFC1123 串
- `preview` — 纯文本正文。IMAP 取 `text/plain` 分段,没有就从 HTML 剥出来;
  Web API 只有上游给的摘要
- `html` — `text/html` 正文原文,仅 IMAP 有。纯文本邮件、正文超过 512 KB 时为空。
  内容不可信,渲染前必须隔离 (面板放在沙箱 iframe 里,并用 CSP 掐掉脚本)
- `folder` — 邮件所在邮箱,`INBOX` 或垃圾箱名

---

## 账号管理接口

### 3. 列出所有账号

```http
GET /api/accounts
```

**响应:**
```json
{
  "success": true,
  "data": [
    {
      "id": "acc_1",
      "name": "主号",
      "host": "imap.mail.me.com"
    }
  ]
}
```

**注意:** 响应中不包含敏感信息（cookies、app_passwords）

---

### 4. 添加账号

**简化版（cookies 可选）:**
```http
POST /api/accounts
Content-Type: application/json

{
  "name": "新账号",
  "host": "icloud.com",
  "proxy": "http://user:pass@host:port"
}
```

**完整版（包含 Cookie）:**
```http
POST /api/accounts
Content-Type: application/json

{
  "name": "新账号",
  "cookies": "{\"x-apple-session-token\":\"token_value\"}",
  "host": "icloud.com",
  "proxy": "http://user:pass@host:port"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "id": "acc_3",
    "name": "新账号",
    "host": "icloud.com",
    "status": "pending"
  }
}
```

**参数说明:**
- `name` (必填) — 账号名称
- `cookies` (可选) — Cookie 字符串,支持两种格式:
  - JSON: `"{\"name\":\"value\"}"`
  - Header: `"name1=value1; name2=value2"`
- `host` (可选) — iCloud 域名,默认 `icloud.com`
- `proxy` (可选) — HTTP/SOCKS5 代理

**注意:** 不传 cookies 时,账号状态为 `pending`,需通过 `/login` 接口登录获取 Cookie

---

### 5. 账号密码登录（获取 Cookie）

分两步:先提交密码,Apple 校验通过后才会把验证码发到受信任设备,再提交验证码。

**第 1 步 — 提交密码**

```http
POST /api/accounts/:id/login
Content-Type: application/json

{
  "password": "用户的常规iCloud密码"
}
```

**参数说明:**
- `:id` (路径参数) — 账号 ID
- `password` (必填) — iCloud 账号的常规密码(**不是** App Password)

**响应 A — 账号没开双重认证,登录已完成:**
```json
{
  "success": true,
  "data": {
    "id": "acc_1",
    "status": "ok",
    "cookies_count": 12,
    "validated": true,
    "warning": ""
  }
}
```

**响应 B — 需要验证码,此时 Apple 已经把码发出去了:**
```json
{
  "success": true,
  "data": {
    "id": "acc_1",
    "status": "needs_2fa",
    "login_id": "0f4c...",
    "apple_id": "you@icloud.com",
    "expires_in": 300
  }
}
```

**第 2 步 — 提交验证码**

```http
POST /api/accounts/:id/login/verify
Content-Type: application/json

{
  "login_id": "0f4c...",
  "code": "123456"
}
```

响应同上面的响应 A。`410 Gone` 表示那份待验证会话超时或已用过,要从第 1 步重来。

**注意事项:**
- 密码是登录 appleid.apple.com 的**常规账号密码**,不是 App 专用密码。App Password 只能用于 IMAP 收信,走不通 SRP 登录
- 登录前账号必须已设置 `icloud_email` 字段
- 第 2 步必须带第 1 步返回的 `login_id`:两步复用同一个 Apple 会话。**不能重新提交密码** —— 那会让 Apple 重发一个新码,用户手上那个当场作废
- `login_id` 存在服务端内存里,有效期 `expires_in` 秒,进程重启即失效
- 登录成功后 Cookie 自动保存到 accounts.json,并立刻校验一次:`validated` 为 false 时 `warning` 里是原因(Cookie 存下来了,但这份会话用不了)

---

### 6. 删除账号

```http
DELETE /api/accounts/:id
```


**响应:**
```json
{
  "success": true,
  "data": {
    "id": "acc_3"
  }
}
```

**错误情况:**
- `404` — 账号不存在

---

### 7. 设置 App Password

```http
POST /api/accounts/:id/password
Content-Type: application/json

{
  "icloud_email": "your_email@icloud.com",
  "app_password": "xxxx-xxxx-xxxx-xxxx"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "id": "acc_1",
    "icloud_email": "your_email@icloud.com"
  }
}
```

**参数说明:**
- `icloud_email` (必填) — iCloud 邮箱地址
- `app_password` (必填) — App 专用密码

**用途:** App Password 用于 IMAP 邮件读取，生成方式见 [appleid.apple.com](https://appleid.apple.com)

---

## 别名管理接口

### 8. 列出所有别名

```http
GET /api/aliases?account_id=acc_1
```

**响应:**
```json
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "count": 15,
    "aliases": [
      {
        "email": "xyz123@icloud.com",
        "anonymousId": "abc123",
        "label": "注册某网站",
        "active": true,
        "createdAt": "2024-01-15T10:30:00Z"
      }
    ]
  }
}
```

**参数说明:**
- `account_id` (必填) — 账号 ID

**别名字段:**
- `email` — HME 邮箱地址
- `anonymousId` — 别名唯一标识（用于停用/激活/删除）
- `label` — 用户定义的标签
- `active` — 是否激活
- `createdAt` — 创建时间

---

### 9. 停用别名

```http
POST /api/aliases/:id/deactivate
Content-Type: application/json

{
  "account_id": "acc_1"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "anonymous_id": "abc123",
    "success": true
  }
}
```

**参数说明:**
- `:id` (路径参数) — 别名的 `anonymousId`
- `account_id` (必填) — 账号 ID

**说明:** 停用后别名不再接收邮件，但可随时激活恢复

---

### 10. 激活别名

```http
POST /api/aliases/:id/reactivate
Content-Type: application/json

{
  "account_id": "acc_1"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "anonymous_id": "abc123",
    "success": true
  }
}
```

**参数说明:**
- `:id` (路径参数) — 别名的 `anonymousId`
- `account_id` (必填) — 账号 ID

**说明:** 激活已停用的别名，恢复邮件接收

---

### 11. 删除别名

```http
DELETE /api/aliases/:id
Content-Type: application/json

{
  "account_id": "acc_1"
}
```

**响应:**
```json
{
  "success": true,
  "data": {
    "anonymous_id": "abc123"
  }
}
```

**参数说明:**
- `:id` (路径参数) — 别名的 `anonymousId`
- `account_id` (必填) — 账号 ID

**注意:** 删除不可恢复！如果直接删除失败，会先停用再删除

---

## 使用示例

### curl 示例

```bash
# 创建别名
curl -X POST http://localhost:8081/api/create \
  -H "Content-Type: application/json" \
  -d '{"account_id": "acc_1", "label": "GitHub"}'

# 读取邮件
curl "http://localhost:8081/api/inbox?account_id=acc_1&alias=xyz123@icloud.com&limit=10"

# 列出别名
curl "http://localhost:8081/api/aliases?account_id=acc_1"

# 停用别名
curl -X POST http://localhost:8081/api/aliases/abc123/deactivate \
  -H "Content-Type: application/json" \
  -d '{"account_id": "acc_1"}'

# 删除别名
curl -X DELETE http://localhost:8081/api/aliases/abc123 \
  -H "Content-Type: application/json" \
  -d '{"account_id": "acc_1"}'
```

### Python 示例

```python
import requests

BASE_URL = "http://localhost:8081/api"

# 创建别名
resp = requests.post(f"{BASE_URL}/create", json={
    "account_id": "acc_1",
    "label": "Netflix"
})
print(resp.json())

# 读取邮件
resp = requests.get(f"{BASE_URL}/inbox", params={
    "account_id": "acc_1",
    "alias": "xyz123@icloud.com",
    "limit": 10
})
print(resp.json())

# 列出别名
resp = requests.get(f"{BASE_URL}/aliases", params={"account_id": "acc_1"})
for alias in resp.json()["data"]["aliases"]:
    print(f"{alias['email']} - {alias['label']} (active: {alias['active']})")
```

---

## 认证说明

### Cookie 认证 (推荐,功能最完整)

用于：创建别名、列出别名、停用/激活/删除别名、**读取邮件**

**获取方式:**
1. 浏览器登录 [icloud.com](https://www.icloud.com) 或 [icloud.com.cn](https://www.icloud.com.cn) (国区)
2. F12 → Application → Cookies
3. 导出全部 Cookie 为 `{"key":"value"}` 格式 JSON

**关键 Cookie:**
- `X-APPLE-WEBAUTH-TOKEN` — 认证 token
- `X-APPLE-WEBAUTH-USER` — 含 dsid (`v=1:s=1:d=22789132008`)
- `X-APPLE-WEBAUTH-HSA-TRUST` — 设备信任 token
- `X-APPLE-DS-WEB-SESSION-TOKEN` — 会话 token

**有效期:** 约 24 小时

### App Password 认证 (IMAP 回退)

仅用于 Web API 失败时的邮件读取回退

**获取方式:**
1. 登录 [appleid.apple.com](https://appleid.apple.com)
2. 登录和安全 → App 专用密码
3. 生成新密码

---

## 技术说明

### 邮件读取实现

**Web API 路径** (`internal/mail/web_client.go`):
1. 调用 `setup.icloud.com.cn/setup/ws/1/validate` 获取 `mccgateway` URL
2. 调用 `mccgateway/mailws2/v1/thread/search` 读取邮件

**⚠️ 已知坑:**
- `validate` 返回的 mccgateway URL 可能带 `:443` 端口 (如 `p217-mccgateway.icloud.com.cn:443`)
- tls-client 的 cookie jar 按不带端口的 host 存储 cookie
- 带端口请求时 cookie 无法附加,导致 403
- **解决:** 解析 URL 后剥离端口号

**clientBuildNumber:** 与浏览器一致,当前 `2624Build22`

**IMAP 路径** (`internal/mail/client.go`):
- 标准 IMAP 协议,连接 `imap.mail.me.com:993`
- 需要 App Password

---

## 错误处理

上游(iCloud)报错时,响应里多两个字段:

| 字段 | 含义 |
| --- | --- |
| `scope` | 固定为 `"upstream"`,表示失败的是 iCloud 会话而不是本服务的 token。401 靠它区分「该重新登录 iCloud」和「面板 token 无效」 |
| `upstream_status` | iCloud 返回的**原始**状态码。对外的状态码是映射过的,排查要看这个 |

### 会话失效 (401)

```json
{
  "success": false,
  "message": "拉取别名列表失败: iCloud 会话失效,需要重新登录换 Cookie — HTTP 421: ...",
  "scope": "upstream",
  "upstream_status": 421
}
```

**解决:** 重新登录该账号换一份 Cookie(密码登录或粘贴浏览器 Cookie)。

上游 401 / 403 / 421 都归到这一档。421 不原样透传:它在 HTTP/2 里有连接层语义,
客户端可能拿它去换条连接重试,原始码放在 `upstream_status` 里。这三个状态码在客户端
内部也不重试 —— 会话已经不被 Apple 认可,重试只是白等退避时间。

### iCloud 限流 (429)

```json
{
  "success": false,
  "message": "创建邮箱失败: iCloud 限流,请稍后重试 — HTTP 429: ...",
  "scope": "upstream",
  "upstream_status": 429
}
```

### iCloud 服务错误 (502)

```json
{
  "success": false,
  "message": "创建邮箱失败: HTTP 503: ...",
  "scope": "upstream",
  "upstream_status": 503
}
```

**说明:** 这一档会自动重试(单次请求最多 3 次,创建别名整体最多 5 轮)

### 参数错误 (400)

```json
{
  "success": false,
  "message": "参数错误: account_id 必填"
}
```

---

## 限制

- **创建频率**: iCloud 限制别名创建频率，过快会返回 429
- **Cookie 有效期**: 约 24 小时，需定期更新
- **邮件读取**: 依赖 IMAP 连接，超时默认 30 秒
