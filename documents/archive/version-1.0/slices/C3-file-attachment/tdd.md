# C3 · 文件附件 — 技术设计文档

**切片**：C3  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| Excel 解析 | `github.com/xuri/excelize/v2` | 纯 Go，无 CGO，成熟 |
| PDF 解析 | `github.com/ledongthuc/pdf` | 纯 Go，提取文本足够 |
| Word 解析 | `github.com/gomutex/godocx` | 纯 Go，提取文本 |
| 图片传输 | Base64 编码后作为 Eino multipart message | Eino 支持多模态 |
| 临时存储 | 内存（不写磁盘）| 附件只在本次对话上下文中使用，无需持久化 |

---

## 2. 目录结构

```
internal/
└── attachment/
    ├── reader.go        # 各类文件的内容提取
    ├── injector.go      # 将附件内容注入 Eino messages
    └── validator.go     # 文件类型和大小校验

frontend/src/
└── components/chat/
    ├── AttachmentBar.tsx    # 输入框上方的附件预览区
    └── DropZone.tsx         # 拖拽区域高亮
```

---

## 3. Go 层

### `internal/attachment/validator.go`

```go
package attachment

import (
    "fmt"
    "path/filepath"
    "strings"
)

const MaxFileSize = 20 * 1024 * 1024 // 20MB

var supportedExts = map[string]string{
    ".txt": "text", ".md": "text", ".csv": "text",
    ".json": "text", ".yaml": "text", ".yml": "text",
    ".xml": "text", ".log": "text",
    ".py": "text", ".js": "text", ".ts": "text",
    ".go": "text", ".sql": "text", ".sh": "text",
    ".xlsx": "excel", ".xls": "excel",
    ".pdf": "pdf",
    ".docx": "word",
    ".png": "image", ".jpg": "image", ".jpeg": "image",
    ".gif": "image", ".webp": "image",
}

type FileInfo struct {
    Name    string
    Size    int64
    Kind    string // "text"|"excel"|"pdf"|"word"|"image"
    Content []byte
}

func Validate(name string, size int64) error {
    if size > MaxFileSize {
        return fmt.Errorf("文件超过 20MB 限制")
    }
    ext := strings.ToLower(filepath.Ext(name))
    if _, ok := supportedExts[ext]; !ok {
        return fmt.Errorf("不支持 %s 文件，请使用文本或 Office 文件", ext)
    }
    return nil
}

func Kind(name string) string {
    ext := strings.ToLower(filepath.Ext(name))
    return supportedExts[ext]
}
```

### `internal/attachment/reader.go`

```go
package attachment

import (
    "bytes"
    "fmt"
    "strings"

    "github.com/xuri/excelize/v2"
    "github.com/ledongthuc/pdf"
    "github.com/gomutex/godocx"
)

const maxRows = 100

// Extract 返回文件的文字内容（图片返回空，由 injector 单独处理）
func Extract(info *FileInfo) (string, error) {
    switch info.Kind {
    case "text":
        return string(info.Content), nil

    case "excel":
        return extractExcel(info.Content)

    case "pdf":
        return extractPDF(info.Content)

    case "word":
        return extractWord(info.Content)

    case "image":
        return "", nil // 图片不提取文字，由 injector 作为多模态传入
    }
    return "", nil
}

func extractExcel(data []byte) (string, error) {
    f, err := excelize.OpenReader(bytes.NewReader(data))
    if err != nil { return "", err }
    var sb strings.Builder
    for _, sheet := range f.GetSheetList() {
        rows, _ := f.GetRows(sheet)
        sb.WriteString(fmt.Sprintf("\n## Sheet: %s\n\n", sheet))
        truncated := false
        for i, row := range rows {
            if i >= maxRows {
                truncated = true
                break
            }
            sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
            if i == 0 {
                sep := make([]string, len(row))
                for j := range sep { sep[j] = "---" }
                sb.WriteString("|" + strings.Join(sep, "|") + "|\n")
            }
        }
        if truncated {
            sb.WriteString(fmt.Sprintf("\n*(已截取前 %d 行)*\n", maxRows))
        }
    }
    return sb.String(), nil
}

func extractPDF(data []byte) (string, error) {
    r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
    if err != nil { return "", err }
    var sb strings.Builder
    numPages := r.NumPage()
    if numPages > maxRows { numPages = maxRows }
    for i := 1; i <= numPages; i++ {
        p := r.Page(i)
        text, _ := p.GetPlainText(nil)
        sb.WriteString(text)
    }
    return sb.String(), nil
}

func extractWord(data []byte) (string, error) {
    doc, err := godocx.OpenReader(bytes.NewReader(data))
    if err != nil { return "", err }
    return doc.Text(), nil
}
```

### `internal/attachment/injector.go`

```go
package attachment

import (
    "encoding/base64"
    "fmt"

    "github.com/cloudwego/eino/schema"
)

// InjectIntoMessages 将附件注入 Eino messages
func InjectIntoMessages(messages []*schema.Message, files []*FileInfo) ([]*schema.Message, error) {
    if len(files) == 0 {
        return messages, nil
    }

    var textParts []string
    var imageParts []*schema.ImagePart

    for _, f := range files {
        if f.Kind == "image" {
            b64 := base64.StdEncoding.EncodeToString(f.Content)
            imageParts = append(imageParts, &schema.ImagePart{
                MimeType: mimeType(f.Name),
                Data:     b64,
            })
            continue
        }
        content, err := Extract(f)
        if err != nil {
            return nil, err
        }
        textParts = append(textParts,
            fmt.Sprintf("[附件: %s]\n%s\n[附件结束]", f.Name, content))
    }

    // 找到最后一条 user message，在其前面插入文件内容
    last := messages[len(messages)-1]
    if last.Role == schema.User {
        prefix := strings.Join(textParts, "\n\n") + "\n\n"
        if len(imageParts) > 0 {
            // 多模态消息
            last.MultiContent = append(imageParts, &schema.TextPart{Text: prefix + last.Content})
            last.Content = ""
        } else {
            last.Content = prefix + last.Content
        }
    }
    return messages, nil
}

func mimeType(name string) string {
    ext := strings.ToLower(filepath.Ext(name))
    m := map[string]string{
        ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
        ".gif": "image/gif", ".webp": "image/webp",
    }
    if v, ok := m[ext]; ok { return v }
    return "image/png"
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
// 前端通过 multipart/form-data 上传文件
mux.HandleFunc("POST /api/attachments/prepare", s.prepareAttachment)
```

---

## 5. 前端组件

### `DropZone.tsx`

```tsx
export function DropZone({ onFiles }: { onFiles: (files: File[]) => void }) {
    const [dragging, setDragging] = useState(false)

    return (
        <div
            onDragOver={e => { e.preventDefault(); setDragging(true) }}
            onDragLeave={() => setDragging(false)}
            onDrop={e => {
                e.preventDefault()
                setDragging(false)
                onFiles(Array.from(e.dataTransfer.files))
            }}
            className={`relative ${dragging ? 'ring-2 ring-blue-500 ring-inset' : ''}`}
        >
            {dragging && (
                <div className="absolute inset-0 bg-blue-500/10 flex items-center
                                justify-center pointer-events-none z-10 rounded-lg">
                    <p className="text-blue-400 text-sm">松开以添加文件</p>
                </div>
            )}
            {/* children (消息列表) */}
        </div>
    )
}
```

---

## 6. 验收测试

```
1. 拖入 sample.txt → 发送"总结这个文件" → AI 引用文件内容回答
2. 拖入 data.xlsx → AI 收到 Markdown 表格格式
3. 拖入 photo.png 到 Claude 模型 → AI 描述图片内容
4. 拖入 .zip → 显示"不支持 .zip 文件"错误，不发送
5. 拖入 25MB 文件 → 显示"超过 20MB 限制"错误
6. 附件区预览：显示文件名，点 × 删除附件，不影响文字输入
7. Excel 超过 100 行 → AI 收到截断提示
```
