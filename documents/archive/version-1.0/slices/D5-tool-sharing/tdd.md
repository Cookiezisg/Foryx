# D5 · 工具分享 — 技术设计文档

**切片**：D5  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 文件格式 | JSON（扩展名 `.forgify-tool`）| 可读、可调试、无需额外解析库 |
| 文件对话框 | Electron dialog IPC | 通过 ipcMain 调用 Electron dialog API |
| 依赖识别 | 复用 `forge.ParseFunction` 的 requirements 提取 | 不重复实现 |
| 名称冲突 | 按 display_name 检查，不按 id | 导入同名工具是常见场景 |

---

## 2. 目录结构

```
internal/
└── service/
    └── tool_share.go   # 导出/导入逻辑

frontend/src/
└── components/tools/
    └── ImportButton.tsx  # 工具库的导入按钮
```

---

## 3. Go 层

### `internal/service/tool_share.go`

```go
package service

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
)

// ToolPackage 是 .forgify-tool 文件的格式
type ToolPackage struct {
    Version    string      `json:"version"`
    ExportedAt time.Time   `json:"exported_at"`
    Tool       ToolExport  `json:"tool"`
}

type ToolExport struct {
    Name         string   `json:"name"`
    DisplayName  string   `json:"display_name"`
    Description  string   `json:"description"`
    Category     string   `json:"category"`
    Code         string   `json:"code"`
    Requirements []string `json:"requirements"`
}

type ToolShareService struct {
    toolSvc *ToolService
}

func (s *ToolShareService) Export(toolID string) ([]byte, error) {
    tool, err := s.toolSvc.Get(toolID)
    if err != nil { return nil, err }

    pkg := ToolPackage{
        Version:    "1.0",
        ExportedAt: time.Now(),
        Tool: ToolExport{
            Name:         tool.Name,
            DisplayName:  tool.DisplayName,
            Description:  tool.Description,
            Category:     tool.Category,
            Code:         tool.Code,
            Requirements: tool.Requirements,
        },
    }
    return json.MarshalIndent(pkg, "", "  ")
}

// ImportResult 描述导入的结果，供前端决策
type ImportResult struct {
    Tool          *Tool
    ConflictName  string // 非空表示存在同名工具
    ConflictID    string
}

func (s *ToolShareService) Parse(data []byte) (*ImportResult, error) {
    var pkg ToolPackage
    if err := json.Unmarshal(data, &pkg); err != nil {
        return nil, fmt.Errorf("无效的工具文件格式")
    }
    if pkg.Version == "" || pkg.Tool.Code == "" {
        return nil, fmt.Errorf("工具文件缺少必要字段")
    }

    result := &ImportResult{
        Tool: &Tool{
            Name:         pkg.Tool.Name,
            DisplayName:  pkg.Tool.DisplayName,
            Description:  pkg.Tool.Description,
            Category:     pkg.Tool.Category,
            Code:         pkg.Tool.Code,
            Requirements: pkg.Tool.Requirements,
            Status:       "draft",
        },
    }

    // 检查同名冲突
    existing, _ := s.toolSvc.List("", pkg.Tool.DisplayName)
    for _, t := range existing {
        if t.DisplayName == pkg.Tool.DisplayName {
            result.ConflictName = t.DisplayName
            result.ConflictID = t.ID
            break
        }
    }

    return result, nil
}

// ImportNew 以新工具身份导入（生成新 ID）
func (s *ToolShareService) ImportNew(tool *Tool) error {
    tool.ID = uuid.NewString()
    return s.toolSvc.Save(tool)
}

// ImportReplace 替换已有工具代码（保留 ID 和测试历史）
func (s *ToolShareService) ImportReplace(existingID string, tool *Tool) error {
    tool.ID = existingID
    return s.toolSvc.Save(tool)
}

// ImportRename 重命名后导入（追加 _imported 后缀）
func (s *ToolShareService) ImportRename(tool *Tool) error {
    tool.ID = uuid.NewString()
    tool.Name = tool.Name + "_imported"
    tool.DisplayName = tool.DisplayName + "_imported"
    return s.toolSvc.Save(tool)
}
```

---

## 4. HTTP API 路由 + Electron IPC

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/tools/{id}/export", s.exportTool)   // 返回 JSON bytes
mux.HandleFunc("POST /api/tools/import/parse", s.importToolFileParse)
mux.HandleFunc("POST /api/tools/import/confirm", s.confirmToolImport)
```

```typescript
// electron/main.ts — 文件对话框通过 Electron IPC 处理
ipcMain.handle('show-save-dialog', async (_, options) => {
    return dialog.showSaveDialog(mainWindow, options)
})
ipcMain.handle('show-open-dialog', async (_, options) => {
    return dialog.showOpenDialog(mainWindow, options)
})
```

---

## 5. 前端组件

### `ImportButton.tsx`

```tsx
export function ImportButton({ onImported }: { onImported: () => void }) {
    const [importResult, setImportResult] = useState<ImportResult | null>(null)

    const handleClick = async () => {
        const path = await OpenImportDialog()
        if (!path) return

        try {
            const result = await ImportToolFile(path)

            if (result.conflictName) {
                setImportResult(result) // 显示冲突处理弹窗
            } else {
                await ConfirmToolImport(result, 'new')
                showSecurityNotice() // 首次导入安全提示
                onImported()
            }
        } catch (e: any) {
            alert(e.message)
        }
    }

    return (
        <>
            <button onClick={handleClick}
                className="px-3 py-1.5 text-sm bg-neutral-700 rounded-lg">
                导入工具
            </button>

            {importResult && (
                <ConflictModal
                    conflictName={importResult.conflictName}
                    onAction={async (action) => {
                        await ConfirmToolImport(importResult, action)
                        setImportResult(null)
                        onImported()
                    }}
                    onCancel={() => setImportResult(null)}
                />
            )}
        </>
    )
}

function ConflictModal({ conflictName, onAction, onCancel }:
    { conflictName: string; onAction: (a: string) => void; onCancel: () => void }) {
    return (
        <Modal title="工具名称冲突" onClose={onCancel}>
            <p className="text-sm mb-4">
                已有同名工具 <strong>{conflictName}</strong>，如何处理？
            </p>
            <div className="flex gap-2 justify-end">
                <button onClick={onCancel} className="text-sm px-3 py-1.5">取消</button>
                <button onClick={() => onAction('rename')}
                    className="text-sm px-3 py-1.5 bg-neutral-700 rounded">重命名导入</button>
                <button onClick={() => onAction('replace')}
                    className="text-sm px-3 py-1.5 bg-red-700 rounded">替换</button>
            </div>
        </Modal>
    )
}

// 首次导入安全提示（存 app_config 防重复）
async function showSecurityNotice() {
    const shown = await GetConfig('import_security_shown')
    if (shown) return
    alert('导入的工具包含 Python 代码，运行前请确认来源可信。')
    await SetConfig('import_security_shown', 'true')
}
```

---

## 6. 验收测试

```
1. 导出工具 → 生成 JSON 文件，字段完整（version, tool.code, requirements）
2. 导入该文件 → 工具出现在工具库，status='draft'
3. 导入同名工具 → 弹出冲突对话框，三个选项可用
4. 选择"重命名" → 工具名追加 _imported 后缀
5. 选择"替换" → 已有工具代码被替换，测试历史保留
6. 导入格式错误文件 → 显示"无效的工具文件格式"
7. 首次导入显示安全提示，第二次不再显示
```
