# D3 · Python 沙箱 — 技术设计文档

**切片**：D3  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| Python 管理 | 内置 uv 二进制（随 Forgify 打包）| 用户无需安装 Python |
| 进程隔离 | `exec.CommandContext`（OS 子进程）| 简单可靠，超时用 context cancel |
| 输入输出 | stdin/stdout JSON | 通用协议，无需 RPC |
| 依赖缓存 | SHA256(requirements sorted) 作为 venv 目录名 | 相同依赖复用同一 venv |
| 网络权限 | 默认不限制 | 工具通常需要调用外部 API |

---

## 2. 目录结构

```
internal/
└── sandbox/
    ├── executor.go    # 执行 Python 函数
    ├── installer.go   # uv 管理虚拟环境和依赖
    └── runner.py.tmpl # 注入工具代码的 runner 模板（embed）
```

---

## 3. Go 层

### `internal/sandbox/installer.go`

```go
package sandbox

import (
    "context"
    "crypto/sha256"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strings"

    "forgify/internal/storage"
)

// uvPath 返回 Forgify 内置 uv 二进制的路径
func uvPath() string {
    // uv 二进制与 Forgify 可执行文件放在同一目录
    exe, _ := os.Executable()
    return filepath.Join(filepath.Dir(exe), "uv")
}

// venvKey 根据依赖列表生成唯一的 venv 目录名
func venvKey(requirements []string) string {
    sorted := make([]string, len(requirements))
    copy(sorted, requirements)
    sort.Strings(sorted)
    h := sha256.Sum256([]byte(strings.Join(sorted, ",")))
    return fmt.Sprintf("%x", h[:8])
}

// EnsureVenv 确保给定依赖列表的虚拟环境存在并安装了依赖
// 已存在则直接返回路径（缓存命中）
func EnsureVenv(ctx context.Context, requirements []string) (venvDir string, err error) {
    key := venvKey(requirements)
    venvDir = filepath.Join(storage.DataDir(), "venvs", key)

    // 检查缓存是否存在（sentinel 文件）
    sentinel := filepath.Join(venvDir, ".installed")
    if _, err = os.Stat(sentinel); err == nil {
        return venvDir, nil // 缓存命中
    }

    // 创建虚拟环境（uv 自动下载 Python 3.12）
    cmd := exec.CommandContext(ctx, uvPath(), "venv", "--python", "3.12", venvDir)
    if out, err := cmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("创建 venv 失败: %w\n%s", err, out)
    }

    // 安装依赖
    if len(requirements) > 0 {
        args := append([]string{"pip", "install", "--quiet"}, requirements...)
        cmd = exec.CommandContext(ctx,
            uvPath(), args...)
        cmd.Env = append(os.Environ(), "VIRTUAL_ENV="+venvDir)
        if out, err := cmd.CombinedOutput(); err != nil {
            return "", fmt.Errorf("安装依赖失败: %w\n%s", err, out)
        }
    }

    // 写入 sentinel 文件标记安装完成
    os.WriteFile(sentinel, []byte("ok"), 0644)
    return venvDir, nil
}
```

### `internal/sandbox/executor.go`

```go
package sandbox

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "text/template"
    "time"

    _ "embed"

    "forgify/internal/service"
)

//go:embed runner.py.tmpl
var runnerTemplate string

const defaultTimeout = 30 * time.Second

type RunResult struct {
    Output   any    `json:"output"`
    Duration int64  `json:"durationMs"`
    Error    string `json:"error,omitempty"`
}

type Executor struct{}

func (e *Executor) Run(
    ctx context.Context,
    tool *service.Tool,
    params map[string]any,
) (*RunResult, error) {
    // 超时控制
    ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
    defer cancel()

    // 确保 venv 存在
    venvDir, err := EnsureVenv(ctx, tool.Requirements)
    if err != nil { return nil, err }

    // 生成 runner 脚本（注入工具代码）
    runnerCode, err := buildRunner(tool.Code, tool.Name)
    if err != nil { return nil, err }

    // 写临时文件
    tmpDir, _ := os.MkdirTemp("", "forgify-run-*")
    defer os.RemoveAll(tmpDir)
    runnerFile := filepath.Join(tmpDir, "runner.py")
    os.WriteFile(runnerFile, []byte(runnerCode), 0644)

    // Python 二进制路径（在 venv 中）
    pythonBin := filepath.Join(venvDir, "bin", "python")
    if _, err := os.Stat(pythonBin); err != nil {
        pythonBin = filepath.Join(venvDir, "Scripts", "python.exe") // Windows
    }

    // 准备输入
    inputJSON, _ := json.Marshal(params)

    // 执行
    cmd := exec.CommandContext(ctx, pythonBin, runnerFile)
    cmd.Stdin = bytes.NewReader(inputJSON)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    start := time.Now()
    runErr := cmd.Run()
    duration := time.Since(start).Milliseconds()

    if ctx.Err() == context.DeadlineExceeded {
        return nil, fmt.Errorf("工具运行超过 %d 秒，已停止", int(defaultTimeout.Seconds()))
    }

    result := &RunResult{Duration: duration}
    if runErr != nil {
        result.Error = stderr.String()
        return result, fmt.Errorf(result.Error)
    }

    var output any
    if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
        result.Error = "工具返回了非 JSON 格式的数据"
        return result, fmt.Errorf(result.Error)
    }
    result.Output = output
    return result, nil
}

func buildRunner(toolCode, funcName string) (string, error) {
    tmpl, err := template.New("runner").Parse(runnerTemplate)
    if err != nil { return "", err }
    var buf bytes.Buffer
    tmpl.Execute(&buf, map[string]string{
        "ToolCode": toolCode,
        "FuncName": funcName,
    })
    return buf.String(), nil
}
```

### `internal/sandbox/runner.py.tmpl`

```python
import sys, json, traceback

# ---- TOOL CODE START ----
{{.ToolCode}}
# ---- TOOL CODE END ----

if __name__ == "__main__":
    try:
        params = json.load(sys.stdin)
        result = {{.FuncName}}(**params)
        print(json.dumps(result, ensure_ascii=False, default=str))
    except Exception as e:
        print(json.dumps({
            "error": str(e),
            "type": type(e).__name__,
            "traceback": traceback.format_exc()
        }, ensure_ascii=False), file=sys.stderr)
        sys.exit(1)
```

---

## 4. 错误类型映射

```go
// sandbox/errors.go
func ClassifyError(stderr string) string {
    switch {
    case strings.Contains(stderr, "SyntaxError"):
        return "代码存在语法错误：" + extractSyntaxError(stderr)
    case strings.Contains(stderr, "ModuleNotFoundError"):
        return "依赖模块未找到：" + extractModuleName(stderr)
    case strings.Contains(stderr, "TimeoutError"):
        return "工具运行超时"
    default:
        return stderr
    }
}
```

---

## 5. uv 打包策略

构建 Forgify 时，将 uv 二进制复制到输出目录：

```makefile
# Makefile
build:
    cp $(shell which uv) dist-electron/uv
    npm run build
```

Windows/Linux 类似，确保 uv 与 Go 后端可执行文件同目录（`dist-electron/`）。

---

## 6. 验收测试

```
1. 执行无依赖工具（纯 Python 标准库），1 秒内返回结果
2. 执行需要 requests 依赖的工具，首次运行自动安装（3-10 秒），第二次 < 1 秒
3. 超时工具（time.sleep(60)）→ 30 秒后强制终止，返回超时错误
4. 语法错误工具 → 返回具体错误位置
5. 工具返回非 dict → 返回格式错误提示
6. 在无 Python 环境的机器上运行（验证 uv 自动下载 Python）
```
