# D1 · 工具库 — 技术设计文档

**切片**：D1  
**状态**：待 Review

---

## 1. 目录结构

```
internal/
├── storage/migrations/
│   └── 005_tools.sql
└── service/
    └── tool.go

frontend/src/
└── components/tools/
    ├── ToolLibrary.tsx       # 工具库主视图
    ├── ToolCard.tsx          # 单个工具卡片
    └── ToolEmptyState.tsx    # 空状态
```

---

## 2. 数据库迁移

### `005_tools.sql`

```sql
CREATE TABLE IF NOT EXISTS tools (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,           -- snake_case 函数名（唯一标识符）
    display_name TEXT NOT NULL,          -- 显示名称（中文）
    description TEXT NOT NULL DEFAULT '',
    code        TEXT NOT NULL,           -- Python 源码
    requirements JSON NOT NULL DEFAULT '[]', -- 第三方依赖列表
    parameters  JSON NOT NULL DEFAULT '[]',  -- 解析出的参数 schema
    category    TEXT NOT NULL DEFAULT 'other',
    status      TEXT NOT NULL DEFAULT 'draft'
                CHECK(status IN ('draft','tested','failed')),
    last_test_at  DATETIME,
    last_test_passed BOOLEAN,
    created_at  DATETIME DEFAULT (datetime('now')),
    updated_at  DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_status   ON tools(status);
```

---

## 3. Go 服务层

### `internal/service/tool.go`

```go
package service

import (
    "encoding/json"
    "forgify/internal/storage"
    "time"

    "github.com/google/uuid"
)

type ToolParameter struct {
    Name     string `json:"name"`
    Type     string `json:"type"`   // "string"|"int"|"float"|"bool"|"list"|"dict"
    Required bool   `json:"required"`
    Doc      string `json:"doc"`
}

type Tool struct {
    ID           string          `json:"id"`
    Name         string          `json:"name"`
    DisplayName  string          `json:"displayName"`
    Description  string          `json:"description"`
    Code         string          `json:"code"`
    Requirements []string        `json:"requirements"`
    Parameters   []ToolParameter `json:"parameters"`
    Category     string          `json:"category"`
    Status       string          `json:"status"`
    LastTestAt   *time.Time      `json:"lastTestAt,omitempty"`
    LastTestPassed *bool         `json:"lastTestPassed,omitempty"`
    CreatedAt    time.Time       `json:"createdAt"`
    UpdatedAt    time.Time       `json:"updatedAt"`
}

type ToolService struct{}

func (s *ToolService) Save(t *Tool) error {
    if t.ID == "" {
        t.ID = uuid.NewString()
    }
    t.UpdatedAt = time.Now()
    req, _ := json.Marshal(t.Requirements)
    params, _ := json.Marshal(t.Parameters)
    _, err := storage.DB().Exec(`
        INSERT INTO tools (id, name, display_name, description, code, requirements,
                           parameters, category, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            display_name=excluded.display_name, description=excluded.description,
            code=excluded.code, requirements=excluded.requirements,
            parameters=excluded.parameters, category=excluded.category,
            status=excluded.status, updated_at=excluded.updated_at
    `, t.ID, t.Name, t.DisplayName, t.Description, t.Code,
        string(req), string(params), t.Category, t.Status,
        t.CreatedAt, t.UpdatedAt)
    return err
}

func (s *ToolService) List(category, query string) ([]*Tool, error) {
    sql := `SELECT id, name, display_name, description, code, requirements,
                   parameters, category, status, last_test_at, last_test_passed,
                   created_at, updated_at
            FROM tools WHERE 1=1`
    args := []any{}
    if category != "" && category != "all" {
        sql += " AND category = ?"
        args = append(args, category)
    }
    if query != "" {
        sql += " AND (name LIKE ? OR display_name LIKE ? OR description LIKE ?)"
        q := "%" + query + "%"
        args = append(args, q, q, q)
    }
    sql += " ORDER BY updated_at DESC"
    return s.scan(sql, args...)
}

func (s *ToolService) Get(id string) (*Tool, error) {
    tools, err := s.scan(`SELECT id, name, display_name, description, code,
        requirements, parameters, category, status, last_test_at, last_test_passed,
        created_at, updated_at FROM tools WHERE id = ?`, id)
    if err != nil || len(tools) == 0 {
        return nil, err
    }
    return tools[0], nil
}

func (s *ToolService) Delete(id string) error {
    _, err := storage.DB().Exec("DELETE FROM tools WHERE id = ?", id)
    return err
}

func (s *ToolService) UpdateTestResult(id string, passed bool) error {
    now := time.Now()
    status := "tested"
    if !passed { status = "failed" }
    _, err := storage.DB().Exec(`
        UPDATE tools SET status=?, last_test_at=?, last_test_passed=?, updated_at=?
        WHERE id=?`, status, now, passed, now, id)
    return err
}

func (s *ToolService) scan(query string, args ...any) ([]*Tool, error) {
    rows, err := storage.DB().Query(query, args...)
    if err != nil { return nil, err }
    defer rows.Close()
    var tools []*Tool
    for rows.Next() {
        t := &Tool{}
        var req, params string
        var lastTestAt *time.Time
        var lastTestPassed *bool
        rows.Scan(&t.ID, &t.Name, &t.DisplayName, &t.Description, &t.Code,
            &req, &params, &t.Category, &t.Status,
            &lastTestAt, &lastTestPassed, &t.CreatedAt, &t.UpdatedAt)
        json.Unmarshal([]byte(req), &t.Requirements)
        json.Unmarshal([]byte(params), &t.Parameters)
        t.LastTestAt = lastTestAt
        t.LastTestPassed = lastTestPassed
        tools = append(tools, t)
    }
    return tools, nil
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/tools", s.listTools)
mux.HandleFunc("DELETE /api/tools/{id}", s.deleteTool)
```

---

## 5. 前端组件

### `ToolLibrary.tsx`

```tsx
const CATEGORIES = ['all', 'email', 'data', 'web', 'file', 'other']

export function ToolLibrary() {
    const [tools, setTools] = useState<Tool[]>([])
    const [category, setCategory] = useState('all')
    const [query, setQuery] = useState('')

    useEffect(() => {
        fetch(`http://127.0.0.1:${port}/api/tools?category=${category}&q=${query}`).then(r => r.json()).then(setTools)
    }, [category, query])

    return (
        <div className="p-6">
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold">工具库</h2>
                <button className="px-3 py-1.5 bg-blue-600 rounded-lg text-sm">
                    + 锻造新工具
                </button>
            </div>

            <input
                placeholder="搜索工具..."
                value={query}
                onChange={e => setQuery(e.target.value)}
                className="w-full mb-3 px-3 py-2 bg-neutral-800 rounded-lg text-sm"
            />

            <div className="flex gap-2 mb-4">
                {CATEGORIES.map(c => (
                    <button key={c}
                        onClick={() => setCategory(c)}
                        className={`px-3 py-1 rounded-full text-xs ${
                            category === c
                                ? 'bg-blue-600 text-white'
                                : 'bg-neutral-800 text-neutral-400'
                        }`}>
                        {c === 'all' ? '全部' : c}
                    </button>
                ))}
            </div>

            {tools.length === 0
                ? <ToolEmptyState />
                : <div className="grid grid-cols-3 gap-3">
                    {tools.map(t => (
                        <ToolCard key={t.id} tool={t}
                            onDelete={() => ListTools(category, query).then(setTools)} />
                    ))}
                  </div>
            }
        </div>
    )
}
```

### `ToolCard.tsx`

```tsx
export function ToolCard({ tool, onDelete }: { tool: Tool; onDelete: () => void }) {
    const statusColor = {
        draft: 'text-neutral-400',
        tested: 'text-green-400',
        failed: 'text-red-400',
    }[tool.status]

    const statusLabel = { draft: '草稿', tested: '已测试', failed: '测试失败' }[tool.status]

    const handleDelete = async () => {
        if (!confirm(`删除后，使用该工具的工作流将无法运行。确认删除 ${tool.displayName}？`)) return
        await DeleteTool(tool.id)
        onDelete()
    }

    return (
        <div className="border border-neutral-700 rounded-xl p-4 flex flex-col gap-2">
            <div className="flex items-start justify-between">
                <span className="font-medium text-sm">📦 {tool.displayName}</span>
                <span className={`text-xs ${statusColor}`}>{statusLabel}</span>
            </div>
            <p className="text-xs text-neutral-500 line-clamp-2">{tool.description}</p>
            <p className="text-xs text-neutral-600">{tool.category}</p>
            {tool.lastTestAt && (
                <p className="text-xs text-neutral-600">
                    {tool.lastTestPassed ? '✅' : '❌'} {relativeTime(tool.lastTestAt)}
                </p>
            )}
            <div className="flex gap-2 mt-auto">
                <button className="text-xs text-blue-400 hover:underline">详情</button>
                <button onClick={handleDelete} className="text-xs text-red-400 hover:underline">删除</button>
            </div>
        </div>
    )
}
```

---

## 6. 验收测试

```
1. 无工具时显示空状态引导
2. 创建 3 个不同分类工具，分类筛选只显示对应分类
3. 搜索"邮件"，只显示名称或描述含"邮件"的工具
4. 删除工具 → 弹出确认 → 确认后工具消失
5. 工具状态标签颜色：草稿灰色、已测试绿色、测试失败红色
```
