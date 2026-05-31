# A2 · 数据层基础 — 技术设计文档

**切片**：A2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| SQLite 驱动 | `modernc.org/sqlite` | 纯 Go，零 CGO，支持交叉编译 |
| 迁移框架 | 自定义（编号 SQL 文件）| 依赖少，逻辑透明，够用 |
| WAL 模式 | 开启 | 并发读写性能更好，适合后台任务和 UI 同时访问 |
| 连接模式 | 单连接 + mutex | 避免 SQLite 并发写冲突，够简单 |
| 数据目录 | macOS: `~/Library/Application Support/Forgify` | 平台惯例 |

---

## 2. 目录结构

```
backend/internal/storage/
├── db.go                  # 连接管理、初始化、迁移入口
├── datadir.go             # 跨平台数据目录
└── migrations/
    ├── 001_init.sql       # 基础表（本切片）
    └── ...                # 后续切片各自添加
```

---

## 3. 初始 Schema（001_init.sql）

```sql
CREATE TABLE IF NOT EXISTS app_config (
    key        TEXT PRIMARY KEY,
    value      TEXT,
    updated_at DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY, ...
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY, ...
);
```

后续切片（如 B1）添加 `002_api_keys.sql`、`003_tools.sql` 等。

---

## 4. 核心实现

### backend/internal/storage/db.go

```go
func Init(dataDir string) error {
    once.Do(func() {
        conn, _ := sql.Open("sqlite", dataDir+"/forgify.db?_journal_mode=WAL&_busy_timeout=5000")
        conn.SetMaxOpenConns(1) // SQLite 单写
        db = conn
        initErr = migrate(conn)
    })
    return initErr
}

func DB() *sql.DB { return db }

// Exec 加全局 mutex，保证写入串行化
func Exec(query string, args ...any) (sql.Result, error) {
    mu.Lock(); defer mu.Unlock()
    return db.Exec(query, args...)
}
```

---

## 5. 在 main.go 中初始化

```go
func main() {
    dataDir := storage.DefaultDataDir()
    if err := storage.Init(dataDir); err != nil {
        fmt.Fprintf(os.Stderr, "storage init failed: %v\n", err)
        os.Exit(1)
    }
    // 启动 HTTP server...
}
```

---

## 6. 验收测试

```
1. 首次运行：dataDir 目录自动创建，forgify.db 存在
2. 重复运行：schema_migrations 不重复插入
3. 新增迁移文件 002_test.sql，重启后自动执行
4. 删除 forgify.db，重新运行：数据库重新创建
5. migrate() 返回错误时 main.go 捕获并 os.Exit(1)
6. DB() 在 Init 前调用返回 nil
```
