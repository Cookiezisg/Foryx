# 桌面端打包方向讨论纪要（2026-04-30）

> 这份文档记录了关于"未来怎么把 Forgify 打包给用户"的讨论结论，主要服务于：
> - 前端开发期间随时对照，避免无意中踩坑
> - 未来某天真要发版时，作为打包流水线的输入

---

## 一、产品定位（已确定）

- **本地优先的单人工具**，数据放用户自己电脑上
- **不做网页部署**——服务器成本、用户系统、隐私问题都不划算
- **目标分发形态：原生桌面 app**（Wails 方案）

---

## 二、最终架构方向

```
分发：    dmg (mac) / setup.exe (win) / AppImage (linux)
外壳：    Wails 窗口（系统自带 webview，不是浏览器 tab）
内容：    现有 Go 后端 + 即将写的前端
```

后端代码、`internal/transport/httpapi`、SQLite、Python sandbox 这些**都不需要改**。只需新增 `cmd/desktop/main.go` 作为启动器，启 HTTP server + 开 Wails 窗口加载 localhost。

### Wails 集成方式：选"窗口外壳"模式

两种集成方式对比：

| | 做法 1：Wails 当窗口外壳 + 复用 httpapi | 做法 2：Wails 原生 binding 新增 transport |
|---|---|---|
| transport 层 | 不动 | 新增 `transport/wailsbind` |
| 占端口 | 是（127.0.0.1 + 随机端口） | 否 |
| 类型安全 | 一般 | 自动生成 TS 类型 |
| 网页版退路 | 保留 | 完全不能复用 |
| 维护成本 | 一份 transport | 两份 transport 同步 |

**选做法 1**。理由：
1. 不绑定 Wails 具体技术，未来想换 Tauri 或做网页版都不动 transport
2. 本地单人场景下"不占端口"价值不大
3. httpapi 的中间件（认证、日志、错误处理）不用重写
4. binding 的类型生成在团队/CI 里反而是负担

---

## 三、前端开发要避的坑

写前端时心里要装着"这是要跑在系统 webview 里的"，避开以下 API：

1. **不要依赖 Service Worker / PWA** 那套（webview 支持不一致）
2. **不要用 Chrome-only 实验性 API**：File System Access、WebUSB、WebSerial、WebBluetooth
3. **不要假设有大容量浏览器缓存**（webview 配额可能小）
4. **路由用 hash 模式或显式处理**，避免依赖浏览器 URL 行为
5. **跟后端通信用相对路径**（`fetch('/api/v1/...')`），不要写死 `http://localhost:8742`
6. **避免最新实验性 CSS**（容器查询新语法等极少数情况），主流 CSS 全没问题

正常 React/Vue/Svelte/fetch/WebSocket/IndexedDB/动画/路由——**全都没问题**。参考 Notion / Linear / Discord 的形态。

### 前端框架选择
- React + Vite ✅
- Vue + Vite ✅
- SvelteKit ⚠️（要用 `adapter-static`）
- Next.js ⚠️（要用 `output: export`，很多 Next 特性不能用）
- 避开纯 SSR 方案

### Wails 版本
**用 Wails v2**，v3 还在 alpha 不要追。

---

## 四、SQLite / CGO 相关

### 现状
- 用了 `mattn/go-sqlite3`（走 CGO）
- Makefile 里 `CGO_CFLAGS="-DSQLITE_ENABLE_FTS5"` 是历史遗留
- FTS5 已经在 2026-04-27 chat 重构时移除，目前没在用

### 决策：换成 `modernc.org/sqlite`
- 纯 Go 实现的 SQLite，无 CGO
- `GOOS=windows go build` 一行命令交叉编译
- 性能慢约 1.5-2x（个人本地工具完全感知不到）
- FTS5 内置，未来重新加全文搜索时不用改编译宏

### 迁移工作量
- 改 `go.mod` 几个依赖
- 改 driver 名："sqlite3" → "sqlite"
- 删 Makefile 的 `CGO_CFLAGS`
- 跑现有测试验证
- 估计 30 分钟到 2 小时

**这是改动桌面端友好度最高的一笔投入，建议尽早做。**

---

## 五、Python 沙箱怎么打包

SQLite 能编进二进制，Python 解释器**不能**——`exec.Command("python3", ...)` 编译期对它一无所知。

| 方案 | 思路 | 何时用 |
|---|---|---|
| A. 系统 Python | README 写明"需要 Python 3" | 开发者预览阶段 |
| **C. 捆绑 standalone Python + uv** | 打包 [python-build-standalone](https://github.com/astral-sh/python-build-standalone) + [uv](https://github.com/astral-sh/uv) → 每个 Forge 一份 venv | **沙箱迭代 1 已选定（2026-05-03）** |
| D. WASM 沙箱（Pyodide） | 安全性最高 | 本地单人场景属过度工程，不考虑 |

**已实施方案 C**。详见 [`sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md`](./sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md)。

### 方案 C 的取舍

收益：
- 用户零依赖（不装 Python 也能用 Forge）
- uv 把"装第三方包"压到秒级（缓存 + wheel resolver）
- 每个 Forge 一份 venv（按 EnvID = sha256(deps + pythonVersion) 共享，N=3 buffer LRU 清理），互不污染
- 升级 Python 版本只需替换 `resources/python/`，重新 Bootstrap

缺点：
- 包大 +60-100MB（uv ~15MB + cpython-3.12 standalone ~50MB，三平台分别打）
- macOS 公证范围扩大（python 二进制 + .dylib + 后续 forge 装的 wheel .so 都要 entitlements 放行，见下文）
- 三平台路径结构不一样（mac/linux 有 `bin/python3`，windows 是 `python.exe` 直接在根目录）→ 用 `paths.go::bundledPythonPath` 抹平
- Python 版本被冻结到捆绑那版（升级靠重新发版替换 resources）

### 资源目录约定

```
resources/
├── uv/{darwin-arm64,darwin-amd64,linux-amd64,windows-amd64}/uv[.exe]
└── python/{darwin-arm64,darwin-amd64,linux-amd64,windows-amd64}/{bin/python3 或 python.exe}
```

- **dev**: `$FORGIFY_DEV_RESOURCES` 指向项目根 `resources/`（`scripts/download-sandbox-resources.sh` 拉一次即可）
- **prod**: `cmd/desktop` 用 `embed.FS` 嵌入对应平台子集，启动期解压到 `cfg.DataDir/sandbox/{uv,python}/`
- Bootstrap 在 `infra/sandbox/preflight.go` 实现：解压 + macOS 重签 + 写 `.bootstrap-hash` 幂等

---

## 六、用户分发形态：四个层次

| 等级 | 形态 | 工程量 | 钱 | 用户体验 |
|---|---|---|---|---|
| L1 | tar.gz/zip 解压跑 | 极小 | 0 | 命令行风格 |
| L2 | + 自动开浏览器、托盘图标 | 小 | 0 | 像后台服务 |
| **L3** | **dmg/msi/AppImage 安装包** | **中** | **0** | **像普通软件，但首次启动有警告** |
| L4 | L3 + 代码签名 | 中 | $99/年（mac）+ $几百/年（win） | 双击直接开 |
| L5 | L4 + 自动更新 | 大 | 同上 + CDN | 跟商业软件一样 |

### 关键点：分发 ≠ 消除警告

做了 dmg/setup.exe 不等于消除 Gatekeeper / SmartScreen 警告。这是两件独立的事：
- **做安装包**：~1 天工程量，免费
- **macOS 公证**：$99/年 + 半天配 CI（性价比最高）
- **Windows 签名**：$几百/年（性价比最差，可一直拖）

### 推荐路径
- **v0.1 直接做 L3**（dmg + NSIS + AppImage），跳过 tar.gz 阶段
- README 第一段教用户绕过首次警告（mac 右键→打开 / win 点"仍要运行"）
- **v1.0 加 macOS 公证**（最高 ROI 的一笔投入）
- Windows 签名一直拖，自动更新等用户基数起来再做

### macOS 公证的内嵌二进制覆盖（沙箱迭代发现）

捆绑 uv + python-build-standalone 的场景下，公证不是简单"签 .app 完事"——有几个特殊点（2026-05 沙箱迭代讨论确定）：

#### 不公证时（v0.x 早期）

- 用户双击 .app → "无法验证开发者" 警告，右键→打开能绕过（一次性）
- **但 Python 子进程仍被内核 SIGKILL**——issue uv#16726：python-build-standalone 二进制带 `com.apple.provenance` xattr + 仅 ad-hoc 签，Gatekeeper 在内核层杀，无日志
- 应急：Bootstrap 阶段自己跑 `xattr -d com.apple.provenance + codesign --force --sign -` ad-hoc 重签所有 python 二进制（uv 内部就是这套），用户右键→打开 .app 后能正常用 forge

#### 公证时（v1.0+）

- 用 Developer ID 证书签 **.app 内所有可执行**（Forgify 主二进制 + uv + python3 + libpython.dylib + 所有 stdlib .so）
- 启用 Hardened Runtime
- **关键 entitlement**：`com.apple.security.cs.disable-library-validation = true`
  - 让 Python 解释器能 dlopen 任意第三方 `.so` 文件
  - 没这条，forge 后续 sync 装新依赖（pandas / numpy 等 wheel 里的 .so）会被 Hardened Runtime 拦——明明公证过的 .app 突然导入失败
  - 这是放行"运行时下载的扩展模块"的唯一途径
- `notarytool submit` 上传 Apple 审核 + `stapler staple` 钉 ticket 到 .app

公证 ticket 覆盖 .app 内**发版时存在**的所有内嵌二进制——uv + python-build-standalone 捆绑的全部一并被覆盖。**用户后续 forge 装新包**靠 disable-library-validation entitlement 而非公证本身放行。

#### 公证不解决的（5% 长尾）

公证只搞定 Gatekeeper / Hardened Runtime / library validation 这条线。还有几条独立的 macOS 用户态权限不归公证管——但这些都是普通 mac app 都有的事，不是 Forgify 特有：

- **TCC 隐私权限**：Python 想读 `~/Documents` / `~/Desktop` / 通讯录 / 日历等隐私目录，弹"forgify 想访问 xxx" 对话框（首次授权后记住）
- **macOS 14+ App Management**：动 `/Applications` 下的别的 app 时额外授权
- **网络出站**：默认放行，无需配置

所以"公证完了所有 sandbox 层面的事都没了"是准确的；剩下的是用户态隐私授权，那是产品 UX 问题不是打包问题。

#### Bootstrap 跟两阶段对应

| 阶段 | 是否公证 | Bootstrap 阶段 mac codesign 步 |
|---|---|---|
| v0.x | 否 | 自己跑 `xattr -d com.apple.provenance` + `codesign --force --sign -` ad-hoc 重签整个 python dir |
| v1.0+ | 是 | 跳过——公证 ticket 已覆盖；但 entitlements 必须含 `disable-library-validation` |

详见 [`sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md`](./sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md) §4.3。

---

## 七、Go 编译"知道"什么、不知道什么

| 依赖类型 | 自动发现？ | 怎么打包 |
|---|---|---|
| Go 库（go.mod） | ✅ 完全自动 | 静态编译进二进制 |
| 静态资源（图片/HTML/CSS） | ⚠️ 你声明 embed，它就读 | 编译时嵌入 |
| C 库（CGO） | ⚠️ C 代码能编入，libc 等系统库要看运气 | 部分静态、部分动态 |
| 外部程序（python/git/ffmpeg） | ❌ 完全不知道 | 你得自己处理 |
| npm 前端依赖 | ❌ 跟 Go 是两个世界 | 先 npm build，再 embed dist |

**核心心法：让所有依赖都"显式可见、可声明、可重现"**。打包麻烦的本质是"我以为有 X" —— 开发期就要不断模拟"这是别人的电脑"。

---

## 八、开发期"未来打包零摩擦"检查清单

### 立刻做（半天内）

#### 1. 路径绝对不要写死
```go
// ❌
db := sql.Open("sqlite", "/tmp/forgify-dev/forgify.db")

// ✅
db := sql.Open("sqlite", filepath.Join(cfg.DataDir, "forgify.db"))
```
所有路径过 config，默认值按平台决定：
- mac: `~/Library/Application Support/Forgify`
- win: `%APPDATA%\Forgify`
- linux: `~/.local/share/forgify`

可用 `github.com/adrg/xdg` 或自写 30 行 `paths.go`。

#### 2. 端口不硬编码
```go
listener, _ := net.Listen("tcp", "127.0.0.1:0")  // OS 分配
actualPort := listener.Addr().(*net.TCPAddr).Port
```
桌面 app 模式用随机端口，配 token 校验防局域网访问。

#### 3. 前端用相对路径调后端
```js
fetch('/api/v1/chat')  // ✅ 不是 http://localhost:8742/...
```
开发模式（浏览器）/ 生产模式（Wails）/ 未来网页版三种部署一行不改。

#### 4. 换掉 mattn/go-sqlite3 → modernc.org/sqlite
见第四节。

#### 5. Makefile 加 build-prod target
```makefile
build-prod:
	cd frontend && npm ci && npm run build
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
	  -o dist/forgify ./cmd/desktop

run-prod: build-prod
	./dist/forgify
```
平时偶尔跑一次，把"生产环境特有 bug"提前暴露。**不要等发版那天才第一次跑生产 build。**

### 前端开始写之前做

#### 6. 选定 Wails v2 + 静态导出能力的前端框架
（见第三节）

#### 7. 前端写"假装跑在 webview 里"的开发模式
```js
const isWebview = import.meta.env.VITE_TARGET === 'webview'
if (isWebview && 'serviceWorker' in navigator) {
  console.warn('Service worker disabled in webview mode')
}
```

#### 8. 状态持久化分层
- **临时 UI 状态**（展开/折叠）→ localStorage
- **用户数据**（chat 记录、collections）→ 走后端 API 存 SQLite，**别**存 IndexedDB

理由：用户数据放后端才能跨设备同步、备份、在网页版/桌面版间共享。早期偷懒存 IndexedDB 将来迁移会很痛。

#### 9. 锁定前端构建产物路径
约定 `frontend/dist/`，Go 这边写：
```go
//go:embed all:frontend/dist
var frontendFS embed.FS
```
**`all:` 前缀很重要**，否则下划线开头的文件（SvelteKit 的 `_app/`）不会嵌入。

#### 10. 锁定 Node 版本
`.nvmrc` + `package-lock.json` / `pnpm-lock.yaml`。避免"我能 build，CI 不能 build"。

### 第一个像样的功能完成后做

#### 11. 启动时做"环境自检"
```go
func preflightCheck() error {
    if _, err := exec.LookPath("python3"); err != nil {
        return fmt.Errorf("Python 3 not found, install from https://...")
    }
    if err := checkDataDirWritable(cfg.DataDir); err != nil {
        return err
    }
    return nil
}
```
桌面 app 没终端，缺什么要在 UI 上友好提示。

#### 12. 所有外部进程调用集中到 `internal/infra/extprocess/`
方便：打包时一眼看清外部依赖、统一健康检查、README 不漏。

#### 13. 维护 RUNTIME_DEPENDENCIES.md
专门列**Go 编译器看不见的依赖**（python/git/ffmpeg 等）。这是打包脚本的输入清单。

#### 14. 早期就跑 GoReleaser dry-run
```bash
goreleaser release --snapshot --clean
```
不用真发布，但提前确认"未来某天打包不会卡住"。

#### 15. CI 加跨平台编译检查
```yaml
- run: GOOS=windows GOARCH=amd64 go build ./...
- run: GOOS=darwin GOARCH=arm64 go build ./...
- run: GOOS=linux GOARCH=amd64 go build ./...
```
PR 阶段拦下"Windows 编不过"的代码。

#### 16. CGO 依赖检查
```bash
go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' ./...
```
CI 加这个检查，引入 CGO 依赖让 build fail。

#### 17. 早期就用版本号
```go
var version = "dev"  // 编译时通过 -ldflags 注入
```
`/api/v1/version` 端点返回。养成"每次发布打 tag"的习惯，后面接 GoReleaser 几乎零成本。

### 真要发版前一两周做

- GoReleaser 完整配置 + 真实发布
- dmg/setup.exe/AppImage 脚本（Wails 自带支持）
- README 用户安装指引（含绕过首次警告的截图）
- macOS 公证流水线（如果决定花 $99）

---

## 九、心法总结

打包之所以会"很麻烦"，本质上是因为**很多隐性假设到了用户机器上不成立**：
- "我以为有 python3"
- "我以为路径可写"
- "我以为端口空闲"
- "我以为前端 dist 在这个相对路径"

**避免麻烦的唯一办法是早暴露**。`make run-prod` 不是发版前才跑的命令，而是一周跑几次的常规检查。

让"开发模式"和"生产模式"的差距越小越好，差距越大，发版那天的坑越多。

---

## 十、常驻后台模式（讨论于 2026-05-01）

产品形态：用户关闭窗口时 app 不退出，scheduler 继续在后台跑。这是有定时任务的工具的合理形态。

### 关键认知：关闭窗口 ≠ 退出程序

| 用户动作 | 行为 |
|---|---|
| 点窗口 X / Cmd+W | **隐藏窗口**，进程继续 |
| 托盘菜单 → Quit / Cmd+Q | **真正退出**，scheduler 也停 |
| 系统注销/关机 | 收到 OS 信号，graceful shutdown |
| 双击图标第二次 | **不要起新进程**，把已有窗口拉到前台 |

Wails 配置：
```go
options.App{
    HideWindowOnClose: true,
    SingleInstanceLock: &options.SingleInstanceLock{...},
    OnShutdown: func(ctx) { /* graceful 关闭 scheduler/db */ },
}
```

### 必做的四件事
1. **系统托盘图标**——常驻 app 没托盘是"幽灵程序"。菜单至少：Show / Pause All Schedules / Quit
2. **单实例锁**——双开会导致 scheduler 重复触发、SQLite 写冲突。Wails 内置支持
3. **开机自启选项**——产品状态存配置（默认关），实现走平台 API（mac LaunchAgent / win 注册表 / linux .desktop）。**默认关，弹引导问用户**
4. **Graceful shutdown**——退出前停 scheduler、提交事务、杀掉 Python 子进程、记录"上次退出时间"

### Scheduler 自身的设计要点
- **时间源**：间隔用 monotonic clock（`time.Since`），具体时间点用 wall clock
- **休眠/唤醒**：不要用长 timer（`time.AfterFunc(24h)`），用 cron 表达式 + 当前 wall clock 重算下次触发；启动和系统唤醒时都重新扫一遍
- **错过任务策略**（必须明确决策）：补跑 / 跳过 / 只跑最新一次
- **状态全部持久化到 SQLite**——内存状态进程一关就没
- **任务执行隔离**：每个任务带 context 超时、panic recovery、并发上限、同任务不重叠
- **资源克制**：注意 macOS App Nap、SQLite 连接池闲时收缩、避免 busy-poll

### Notifier 接口（现在就要定，未来才实现）

scheduler 任务失败要通知用户——这是 application 层调用桌面端的唯一跨层场景。

**Phase 4 写 scheduler 时**就在 domain/app 层定义接口：
```go
// internal/domain/notification/notifier.go
type Notifier interface {
    Notify(ctx context.Context, n Notification) error
}
```
- `cmd/server` 注入 `LogNotifier`（打日志即可）
- `cmd/desktop` 未来注入真实桌面通知实现
- scheduler 代码依赖接口，永远不感知"桌面端"存在

### 桌面端代码归属：哪些现在写，哪些以后写

| 功能 | 何时写 | 写在哪 |
|---|---|---|
| Notifier 接口 | **Phase 4 现在** | `internal/domain/notification/` |
| LogNotifier 实现 | **Phase 4 现在** | `internal/infra/notification/` |
| 用户偏好（含 startOnLogin 配置项） | **Phase 4 现在** | `internal/app/preferences/` |
| Tray menu 实现 | 未来做客户端时 | `internal/infra/desktop/tray/` |
| 桌面端真实通知 | 未来做客户端时 | `internal/infra/desktop/notification/` |
| Auto-start 平台实现 | 未来做客户端时 | `internal/infra/desktop/autostart/` |
| Single-instance 锁 | 未来做客户端时 | Wails 内置 / `cmd/desktop` |

判断标准：**`cmd/server` 编译出来的二进制不应该含 Wails、不含托盘代码**。它要能在 docker / CI / headless Linux 上跑。

### HTTP / 事件机制对常驻后台模型的要求
- HTTP server 始终在跑（窗口不在也跑），scheduler 才能调 application 层
- application 层和 scheduler **不该假设"前端在线"**——任务该跑得跑，前端只是观察者
- **SSE/WebSocket 要支持断线重连**：用户关窗 → 重新打开 → 前端要能查询当前状态 + 订阅新事件流。in-memory Bridge 之外要有"事件持久化"或"状态快照"

### UX 注意点
- **首次关窗弹一次提示**："Forgify 仍在后台运行，schedule 不会停。可从托盘重新打开"，给"以后不再提示"
- **窗口状态恢复**（位置/大小/当前 tab）——存 localStorage 或 SQLite
- **可见性**：托盘图标 badge / 主窗口顶部状态条 / 历史日志页，让用户感知 scheduler 在工作
- **通知克制**：只通知失败/需要介入的事件，不是每次任务跑完都弹

### 配置项预留
- `desktop.minimizeToTrayOnClose` (默认 true)
- `desktop.startOnLogin` (默认 false)
- `desktop.notifyOnTaskFailure` (默认 true)
- `scheduler.missedTaskPolicy` ("skip" | "runOnce" | "runAll")

---

## 十一、为什么不走 Wails 原生 binding（再次确认）

未来某天会再被诱惑："既然是桌面 app，干脆扔掉 HTTP，直接用 Wails binding 不是更简单？"

**答案：不要这么做。** 理由记下来，免得几个月后忘了：

### HTTP 不比 binding 难
两边写起来工作量几乎一样：
```js
fetch('/api/v1/messages', {...})        // HTTP
ChatBinding.SendMessage({...})          // binding
```
"省掉 HTTP 一层"听起来优雅，但只是把工作挪到别处，不是消失。

### 走 binding 等于扔掉的东西
v1.2 重写的 `transport/httpapi` 里很多东西跟 HTTP 绑定：
- middleware（recover/logger/cors/locale/userid/notfound）→ Wails 没 middleware 概念，**全部要在每个 binding 方法里重新实现**
- response 包络 + errmap 错误码体系 → 要重新设计
- ~170 测试里通过 HTTP 测的部分 → 要么删要么改
- handlers → 函数签名要改写成 binding 方法

**等于把 v1.2 的 transport 层重写一遍。** v1.2 重写动机就是 transport 太烂，现在好不容易整理干净了，没理由再折腾。

### HTTP 真正给你的、binding 没有的能力
1. **调试**：浏览器 Network tab、curl/Postman、`make testend` 单跑后端验证
2. **SSE 流式输出**：chat 现在的 SSE 是天然契合 HTTP；Wails event 心智模型不一样、没有 Last-Event-ID 重连
3. **进程隔离**：前端崩了不影响后端，scheduler 还在跑
4. **可演进**：未来想做 CLI / cloud sync / 暴露 API → HTTP 现成；想换 Tauri → HTTP 通用，binding 锁死
5. **测试**：curl smoke 一行命令验全链路；binding 测试要起 Wails runtime

### binding 唯一真有用的优势：类型同步
自动生成 TS 类型确实有价值。但不必走 binding——`httpapi` 加 OpenAPI spec，前端用 `openapi-typescript` 生成 TS，效果一样且不锁技术栈。

### 触发"不走 binding"的判断标准
只有**全部满足**这些条件才该考虑 binding：
- ❌ 项目刚起步、没多少代码（你不是）
- ❌ API 调用极少、不需要中间件（你不是）
- ❌ 100% 确定永远只做桌面 app（你不一定）
- ❌ 需要极致性能（你不是 chat/scheduler）

一条不符合就别走 binding。

### 心法
**"看起来更简单的方案，往往是把复杂度藏到了别处。"** 去掉 HTTP 看起来省事，藏起来的成本是：重写 middleware、失去网络调试、失去 SSE 天然支持、锁死 Wails。HTTP 那一层不是负担，是**抽象边界**——让客户端能变（浏览器/桌面/CLI/移动）、让服务器能变（本地/云/嵌入），互不影响。这个抽象在 v1.2 重写时已经付了成本，扔掉等于浪费投入。

---

## 十二、Windows 平台支持（D10-D15 · 2026-05-06）

### 代码层就绪情况

V1.2 D2-D9 期间已经做的 Windows 工作（不少是顺手做的）：

| 模块 | Windows 支持 |
|---|---|
| `infra/sandbox/proc_windows.go` | Job Object + `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 强清理 + `taskkill /T /F` per-process。这是三平台里**最强**的进程树管理（macOS / Linux 只能近似）。|
| `infra/sandbox/embed_mise_windows_amd64.go` | `go:embed` `mise.exe`（92 MB），`miseExeName()` 返 `mise.exe`，extract 路径正确写到 `<dataDir>/sandbox/bin/mise.exe`。|
| `cmd/resources/main.go` | 知道 windows-amd64 用 `.zip` 归档（不是 `.tar.gz`），`extractZip` 提取 `mise.exe`。|
| 各 sandbox runtime installer（python/java/ruby/go/rust） | 每个都有 `if runtime.GOOS == "windows" + .exe 扩展名` 处理。|
| `infra/crypto/fingerprint.go` | `runtime.GOOS` 分支 — Windows 用 `MachineGuid` 注册表项作硬件指纹源。|
| `app/tool/shell/bash.go` | Windows 走 `cmd.exe /c`（故意不用 PowerShell——execution policy 问题）。LLM-facing description 已说明跨平台行为。|
| `pkg/pathguard/pathguard.go` | DefaultDenyList 含 `C:/Windows/`、`AppData/Microsoft/Credentials/`、Edge/Chrome saved logins 等 Windows 路径。非绝对路径在当前 OS 静默丢弃。|
| `cmd/server/main.go` 的 `defaultMCPConfigPath / defaultSkillsDir / defaultCatalogCachePath` | 用 `os.UserHomeDir()` → Windows 自动返 `%USERPROFILE%`，`filepath.Join` 自动用 `\`。|
| MCP marketplace（6 entries） | D11 审计：全部 Windows 兼容（playwright/markitdown/context7/duckduckgo/sqlite/everything 都用跨平台 runtime + stdio）。|

### 用户家目录

Windows 上 `~/.forgify/` 实际是 `%USERPROFILE%\.forgify\`（典型 `C:\Users\<name>\.forgify\`）。Go 的 `os.UserHomeDir()` 自动处理。

```
%USERPROFILE%\.forgify\
├── mcp.json              ← MCP server 配置（与 Claude Desktop schema 兼容）
├── skills\               ← Skill 库（每个 skill 一个 subdir）
└── .catalog.json         ← Capability Catalog 派生 cache
```

### Bash tool 在 Windows 上的行为

LLM 调 Bash 时，命令在 **`cmd.exe /c "<command>"`** 下跑。注意：

- `dir` 工作，`ls` 不工作（除非装了 git-bash 在 PATH）
- 路径分隔符 `\\` 工作，某些场景 `/` 也工作（cmd 容忍）
- 管道 `|`、重定向 `>`、`&&` 都工作
- 想要 PowerShell：LLM 自己写 `powershell -Command "..."` 前缀

故意不用 PowerShell 作默认 shell：**execution policy** 在锁定企业机会拦脚本式调用，cmd.exe 永远在且引号行为可预测。

### Wails 打 Windows 包

```bash
# Mac 上交叉编译 Windows 安装器（NSIS）
wails build -platform windows/amd64 -nsis

# 输出：build/bin/Forgify-amd64-installer.exe
```

要 NSIS（在 mac `brew install nsis`）。代码签名（$几百/年）按 [§六](#六用户分发形态四个层次) 的"暂时拖着"策略 — 用户首次运行会有 Windows SmartScreen 警告，点"更多信息→仍要运行"绕过。

### 当前缺什么

代码层完成度 ~95%。剩下的 5% 都需要真 Windows 机器才能搞：

- [ ] 真跑一遍 backend `forgify-server.exe` —— 验启动 + 所有 service 真 boot OK
- [ ] mise 真在 Windows 装 node / python runtime —— 验 `mise install` 不挂
- [ ] Bash tool 真跑 `cmd /c "echo hello"` —— 验输出捕获 / 退出码正确
- [ ] fsnotify 真在 Windows 行为 —— 不同 file lock 语义可能影响 skill watcher
- [ ] MCP server 真在 Windows stdio 启动 + handshake —— 测一个 `npx -y @modelcontextprotocol/server-everything`

这些**留作 D10-D15 的"待真 Windows 机验"清单**。等用户搞到 Windows VM/笔记本时按这个清单走一遍即可。

### 打 Windows 包前的流程

```bash
# 1. 拉 Windows mise binary（其他平台一并）
make resources ALL=1

# 2. 验代码层
GOOS=windows go vet ./...
GOOS=windows go build ./...

# 3. 打包
wails build -platform windows/amd64 -nsis

# 4. 真 Windows 机上跑过一遍上面"缺什么"清单
```
