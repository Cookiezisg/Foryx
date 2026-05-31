# A1 · App Shell — 技术设计文档

**切片**：A1  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 桌面框架 | Electron | Chromium 内核，原生缩放，systray，hiddenInset 标题栏 |
| 布局模型 | SidebarNav + Tab 页面各自管理内部布局 | 每个 Tab 的左右分栏逻辑不同，各自封装更清晰 |
| 前端路由 | React 状态管理（非 URL Router）| 桌面 app 没有真实 URL |
| CSS 方案 | Tailwind CSS v4 | 快速布局，设计稿一致性 |
| 图标库 | Lucide React | 轻量、风格统一 |
| Inbox 角标 | React Context | 多处读取未读数，避免 prop drilling |
| 全屏感知 | Electron IPC（fullscreen-change）| main process 监听窗口事件，通过 preload 通知渲染进程 |

---

## 2. 布局模型

```
┌──────────────────────────────────────────────────────────┐
│  [SidebarNav 280px]  │ [当前 Tab 的页面（自管理内部布局）] │
└──────────────────────────────────────────────────────────┘
```

SidebarNav 为水平标签栏，宽度默认 280px，可拖动调整（280–450px）。  
标题栏使用 `titleBarStyle: 'hiddenInset'`，交通灯悬浮在页面上方，paddingTop 40px 给出安全区域（全屏时归零）。

---

## 3. 目录结构

```
forgify/
├── electron/
│   ├── main.ts          # BrowserWindow + Tray + spawn Go
│   └── preload.ts       # contextBridge: onFullscreenChange
├── frontend/src/
│   ├── main.tsx         # 读 URL ?port= 初始化 SSE
│   ├── App.tsx          # 主布局：侧边栏 + 主区
│   ├── types/
│   │   └── electron.d.ts
│   ├── context/
│   │   └── InboxContext.tsx
│   ├── components/
│   │   ├── SidebarNav.tsx   # 水平标签 pill 导航
│   │   └── SplitView.tsx    # 可复用左右分栏容器
│   └── pages/
│       ├── HomePage.tsx
│       ├── ChatPage.tsx
│       ├── AssetsPage.tsx
│       └── InboxPage.tsx
```

---

## 4. Electron 主进程

### electron/main.ts 要点

```typescript
// 窗口配置
new BrowserWindow({
  titleBarStyle: 'hiddenInset',
  trafficLightPosition: { x: 12, y: 10 },
  minWidth: 960, minHeight: 640,
  webPreferences: { preload, contextIsolation: true },
})

// 关闭时隐藏而非退出（托盘常驻）
let isQuitting = false
app.on('before-quit', () => { isQuitting = true })
mainWindow.on('close', (e) => { if (!isQuitting) { e.preventDefault(); mainWindow.hide() } })

// 全屏状态通知渲染进程
mainWindow.on('enter-full-screen', () => mainWindow.webContents.send('fullscreen-change', true))
mainWindow.on('leave-full-screen', () => mainWindow.webContents.send('fullscreen-change', false))
```

### electron/preload.ts

```typescript
contextBridge.exposeInMainWorld('electronAPI', {
  platform: process.platform,
  onFullscreenChange: (callback) => {
    const handler = (_, isFullscreen) => callback(isFullscreen)
    ipcRenderer.on('fullscreen-change', handler)
    return () => ipcRenderer.removeListener('fullscreen-change', handler)
  },
})
```

---

## 5. 前端层

### App.tsx 要点

```tsx
const TITLE_BAR_HEIGHT = 40  // macOS hiddenInset 安全区

useEffect(() => {
  const unsubscribe = window.electronAPI?.onFullscreenChange((isFullscreen) => {
    setTitleBarHeight(isFullscreen ? 0 : TITLE_BAR_HEIGHT)
  })
  return () => unsubscribe?.()
}, [])

// 侧边栏 paddingTop 随全屏状态变化
<aside style={{ width: sidebarWidth, paddingTop: titleBarHeight }} ...>
```

### SidebarNav.tsx

水平 pill 式标签栏，激活项展开显示文字（CSS transition），内联样式避免 Tailwind 动态类名问题。

---

## 6. 验收测试

```
1. npm run dev 启动，Electron 窗口正常显示，交通灯位置正确
2. 四个 Tab 可切换，内容区切换正常
3. 进入全屏 → paddingTop 归零；退出全屏 → 恢复 40px
4. 点击关闭 → 窗口隐藏，进程仍在（托盘图标存在）
5. 托盘图标出现，右键菜单正常，"退出 Forgify"完全退出
6. 托盘左键：toggle 窗口显示/隐藏
7. 侧边栏拖动手柄：宽度可调整（280–450px）
8. 窗口最小 960×640 限制有效
9. Inbox 无未读时无角标
```
