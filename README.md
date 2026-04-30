# 勿忘我 · rip.lgbt

一个为逝去的跨性别者、性别多元者与友跨人士保留名字的纪念站。
本仓库由原 Cloudflare Workers 单文件迁移到自建 stack：

- 后端：Go 1.22 单进程二进制（HTTP API + 静态 SPA + Telegram 投稿机器人 + chromedp 截图）
- 前端：Vue 3 + Vite SPA，构建产物嵌入 Go 二进制
- 数据：SQLite（pure-Go，免 CGO）+ 本地文件目录
- 部署：单容器 Docker，挂载 `./data` 持久化

## 主要功能

- 公开纪念页（首页索引、详情页、献花、留言）
- Telegram 投稿机器人：用户在私聊里通过分步对话填写投稿，每条用户消息读取后立即删除，机器人编辑同一条主消息更新到下一步；按钮可跳到任意步骤、跳过、提交。
- 管理后台：账号 + TOTP/Passkey 二步验证；查看投稿队列；审稿页面包含真实页面的 chromedp 截图预览 + 原始 markdown，可直接接受、拒绝、要求修改某一节。要求修改后机器人会找到投稿者，编辑回那一步等待新内容。
- TG 一次性登录链接：管理员在 bot 内输入 `/login`，bot 直接给出 10 分钟有效的登录链接。
- 自动清理：每 6 小时清扫过期 session、过期登录链接、30 天前软删除的草稿。

## 目录结构

```
backend/                    # Go 后端
├── cmd/server/main.go
├── internal/
│   ├── admin/              # 管理员 / 设置 HTTP API
│   ├── auth/               # 密码 + TOTP + WebAuthn + 一次性登录
│   ├── bot/                # Telegram 机器人
│   ├── config/             # 环境变量 + secrets.json 自动生成
│   ├── db/                 # SQLite + 嵌入式 migration
│   │   └── migrations/0001_init.sql
│   ├── http/               # 路由、中间件、SPA fallback、定时任务
│   ├── markdown/           # 自定义 markdown 引擎（移植自旧 frontend.js）
│   ├── memorial/           # 公开列表/详情/评论/献花
│   ├── preview/            # chromedp 截图
│   ├── settings/           # key/value 设置存储
│   └── submission/         # 草稿状态机 + 审稿动作
├── go.mod / go.sum

frontend/                   # Vue 3 + Vite
├── index.html
├── vite.config.ts
├── src/
│   ├── api/client.ts
│   ├── components/
│   ├── pages/
│   ├── router/index.ts
│   ├── stores/
│   └── styles/
└── package.json

Dockerfile
docker-compose.yml
template.md                 # 投稿模板（驱动 bot 流程）
data/                       # 运行时持久化（compose 挂载）
.secrets/                   # docker secret 存放区（密码文件）
```

## 部署

### 1. 准备超管密码文件

```bash
mkdir -p .secrets data
printf 'a-strong-password' > .secrets/superadmin_password
chmod 600 .secrets/superadmin_password
```

### 2. （可选）准备 .env

```bash
cp .env.example .env
# 编辑 SITE_URL / HOST_PORT 等
```

### 3. 启动

```bash
docker compose up -d --build
```

第一次启动后：

- 浏览器访问 `http://<host>:8080/`，看到首页（暂无条目）。
- 访问 `/admin/login`，用 `admin` + `.secrets/superadmin_password` 登录。
- 系统会强制要求设置 TOTP 与 Passkey；按提示绑定。
- 进入 **设置**：填入 Telegram BotFather 给的 token、Bot Username、模式（polling 或 webhook）。点击保存后，可通过 `POST /api/admin/settings/reload-bot` 让 bot 立即生效（也可重启容器）。
- 进入 **管理员**：把自己的 Telegram numeric id 加入白名单。然后在 TG 里给 bot 发 `/login` 拿一次性登录链接验证。
- 投稿：另一个 Telegram 帐号给 bot 发 `/submit`，按提示走完即可。

## 本地开发

后端：

```bash
cd backend
go env -w GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn
go mod tidy
SUPERADMIN_USERNAME=admin SUPERADMIN_PASSWORD=test1234 \
  SITE_URL=http://localhost:8080 \
  LISTEN_ADDR=:8080 \
  DATA_DIR=./.devdata \
  go run ./cmd/server
```

前端：

```bash
cd frontend
npm install
npm run dev      # http://localhost:5173 反代到后端 :8080
```

测试：

```bash
cd backend
go test ./...
```

## 关键环境变量

| 变量 | 说明 |
| ---- | ---- |
| `SUPERADMIN_USERNAME` | 默认超管账号（默认 `admin`） |
| `SUPERADMIN_PASSWORD` | 超管密码（明文，二选一） |
| `SUPERADMIN_PASSWORD_FILE` | 密码文件路径（推荐，docker secret） |
| `SITE_URL` | 站点公开 URL（用于派生 WebAuthn RP ID 与 TG 登录链接） |
| `LISTEN_ADDR` | 监听地址，默认 `:8080` |
| `DATA_DIR` | 数据目录，默认 `./data`（容器内 `/data`） |
| `TZ` | 时区，默认 `Asia/Shanghai` |
| `CHROMIUM_PATH` | chromium 路径（Dockerfile 默认设置） |

其他敏感密钥（JWT 密钥、CSRF 密钥、IP hash pepper、预览签名密钥、WebAuthn RP ID、session cookie name）首次启动会写入 `<DATA_DIR>/secrets.json`，并在后续启动时复用。删除该文件会全部重置（所有现有 session、登录链接、ip 冷却记录会全部失效）。

## 数据持久化

`./data/` 卷挂载，包含：

- `app.db` / `app.db-wal` / `app.db-shm`：SQLite
- `secrets.json`：自动生成的密钥
- `uploads/drafts/<id>/`：投稿过程中收到的图片
- `uploads/memorials/<entry_id>/`：投稿被接受后迁移到的资源（如有）

定时任务每 6 小时清理：过期 session / 一次性登录链接 / 30 天前的软删除草稿（含其图片目录）。

## TG 投稿流程要点

机器人在私聊中维护一份草稿（DB 持久化），用户每发一条文本，机器人会立即删除该消息，把自己的「主消息」编辑到下一步提示。下面这些操作均通过 inline keyboard：

- ◀ 上一步 / 下一步 ▶
- 跳过（仅可选项）
- 📋 跳到任意步骤
- ✅ 提交审核

提交审核时，bot 会调 `internal/preview` 用 headless chromium 抓取 `/admin/preview/<draft_id>` 真实预览，把 PNG + 链接以 photo+caption 形式发给所有有 `telegram_id` 的管理员。

## 安全说明

- 密码用 argon2id 哈希存储；登录会话使用随机 token 写入 `admin_sessions`，HTTP-only cookie。
- TOTP 用标准 RFC6238 (`pquerna/otp`)。
- Passkey 用 `go-webauthn/webauthn`，作为登录后的第二因子；passwordless 留待后续。
- 投稿预览路由 `/admin/preview/<id>` 默认仅登录管理员可访问；headless 抓图通过 HMAC token 验证。
- IP 哈希用 secrets.json 中的随机 pepper + SHA-256，避免直接保存原始 IP。

## 已迁移自原 frontend.js 的内容

- 数据形状（Person / Profile / facts / websites）
- 校验规则（必填字段、entryId 格式、正文至少一项）
- 评论 + 献花 SQL & 限流（30s 评论 / 24h 献花 / 120s 投稿）
- 自定义 markdown 语法：`<PhotoScroll>`、`<CapDownQuote>`、`<BlurBlock>`、`<DottedNumber>`、`<TextRing>`、`<Sakura>`、`<ChannelBackupButton>`、`<Hexagon>`、`<details>/<summary>`、`<div style="...">`、HTML 标题/段落/blockquote、脚注、ruby、span 内联样式、行内图片/链接、bold/em/code、水平线、列表
- HTML shell + 站点头/脚

## 测试覆盖

- `internal/config`：secrets.json 创建 / 缺字段补齐 / RPID 推导
- `internal/db`：迁移幂等、settings 读写
- `internal/auth`：argon2id 验证
- `internal/markdown`：核心语法 + 自定义节点
- `internal/memorial`：列表/详情/留言冷却/献花冷却
- 端到端：`/api/health`、`/api/memorials`、`/api/auth/login`、未授权 admin 端点 401

## 已知后续工作

- WebAuthn passwordless 登录（当前 passkey 仅作 second factor）
- bot webhook 模式的 HTTP handler（当前实现只 SetWebhook，HTTP server 端点尚未挂在 chi 路由中）
- 评论/献花的 HTTP-level rate limit（目前依赖 ip-hash 冷却）
- 旧 markdown 引擎里的部分细节（如更复杂的 nested HTML 块）需要更多 fixture 验证
- frontend `baseStyles()` 全部 1800+ 行 CSS 中的少量动效/sakura/纸纹细节未完全移植（核心样式已搬过来，剩余视觉细节欢迎按需补）
