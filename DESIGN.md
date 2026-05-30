# GameLibrary — 设计文档

> AI-assisted project. Current version: **0.5.4-alpha** | Last updated: 2026-05-31

## 版本路线图

| 版本 | 阶段 | 核心功能 | 状态 |
|------|------|---------|------|
| 0.1.0-alpha | Phase 1 — 骨架 | 项目搭建、扫描器、基础 UI | ✅ |
| 0.2.0-alpha | Phase 2 — 刮削 | 多源元数据刮削、详情页、双封面 | ✅ |
| 0.2.1-alpha | Phase 2 — 增强 | 强制重刮、游戏启动、多平台 CI | ✅ |
| 0.2.2-alpha | Phase 2 — 数据源 | SteamGridDB + Bangumi 刮削器 | ✅ |
| 0.2.3-alpha | Phase 2 — 修复 | 配置迁移、缺失源自动补齐 | ✅ |
| 0.2.4-alpha | Phase 2 — 日志 | 结构化日志系统、按天归档 | ✅ |
| 0.2.5-alpha | Phase 2 — 修复 | 跨盘符绝对路径扫描修复 | ✅ |
| 0.3.0-alpha | Phase 2 — 交互 | 右键菜单、星标、标签系统 | ✅ |
| 0.3.1-alpha | Phase 2 — 修复 | 会话日志、BOM/路径/刮削修复 | ✅ |
| 0.3.2-alpha | Phase 2 — 数据源 | RAWG.io 刮削器、搜索名智能拆分 | ✅ |
| 0.3.3-alpha | Phase 2 — 修复 | CamelCase 拆分修正、刮削后实时刷新 | ✅ |
| 0.4.0-alpha | Phase 2 — 标签 | 三级标签系统、侧边栏分段筛选 | ✅ |
| 0.5.0-alpha | Phase 2 — 多平台 | 多数据源关联、别名、启动/刮削按钮 | ✅ |
| 0.5.1-alpha | Phase 2 — 修复 | 多源刮削、Steam 跳 RJ 码、强制重扫 | ✅ |
| 0.5.2-alpha | Phase 2 — 优先源 | 侧滑子菜单、Unmatched 标签、刮削模块化 | ✅ |
| 0.5.3-alpha | Phase 2 — 详情面板 | 居中弹窗、全操作同步、移除 local | ✅ |
| 0.5.4-alpha | Phase 2 — 启动按钮 | 右键+详情页启动按钮绿色强调置顶 | ✅ |
| 0.6.0-alpha | Phase 3 — 启动 | 锁机制、心跳检测、运行状态 | 📋 |
| 0.7.0-alpha | Phase 4 — 时长 | 进程监控、多端时长聚合、统计 | 📋 |
| 0.8.0-alpha | Phase 5 — 存档 | 云存档同步、符号链接、备份 | 📋 |
| 0.9.0-beta  | 测试完善 | 全功能测试、Bug 修复、文档 | 📋 |
| 1.0.0       | 正式发布 | 稳定版 | 📋 |

---

## 1. 项目概述

GameLibrary 是一个跨机器、便携式的游戏库管理器。管理程序与游戏文件一同存放在 NAS 网络挂载路径中，所有客户端共享同一份程序与配置，通过相对路径寻址实现零配置跨机迁移。

### 核心理念

- **便携优先** — 单 exe 放入 NAS 根目录即可运行，无需服务端部署
- **文件即数据** — 元数据存为 JSON 文件，免数据库，拷贝即迁移
- **NAS 即云端** — 游戏文件、存档、元数据统一存放于 NAS，多端共享

---

## 2. 架构设计

### 2.1 整体架构

```
┌──────────────────────────────────────────────────────┐
│  NAS                                                  │
│  GameLibrary\                                         │
│  ├── GameManager.exe        ← 单文件管理程序 (Wails)   │
│  ├── config.json            ← 全局配置 (多端共享)      │
│  ├── .gamemanager\          ← 系统数据 (规划中)        │
│  └── Games\                 ← 游戏库                   │
│      ├── GameA\                                       │
│      │   ├── .gameinfo.json ← 元数据                   │
│      │   └── game.exe                                 │
│      └── GameB\                                       │
│          └── .gameinfo.json                           │
└──────────────────────────────────────────────────────┘
         │                        │
    ┌────┴────┐              ┌────┴────┐
    │ 客户端 A │              │ 客户端 B │
    │ Z:\ → NAS│              │ Y:\ → NAS│
    │          │              │          │
    │ 机器名   │              │ 机器名   │
    │ 从系统   │              │ 从系统   │
    │ 自动获取 │              │ 自动获取 │
    └─────────┘              └─────────┘
```

### 2.2 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 运行时 | Wails v2 | Go + WebView2，编译为单 exe |
| 后端 | Go 1.23+ | 文件扫描、进程管理、API 服务 |
| 前端 | React 18 + TypeScript + Vite | SPA 嵌入 WebView2 |
| 样式 | Pure CSS (dark theme) | 无第三方 UI 库 |
| 数据 | JSON 文件 | 免数据库，Git 友好 |
| IPC | Wails Bindings | Go ↔ JS 自动生成类型安全的绑定 |

### 2.3 目录结构

```
GameLibrary/
├── main.go                     # Wails 入口 (package main)
├── app.go                      # App 主体 + 类型别名
├── internal/                   # 内部包 (不可外部导入)
│   ├── config/
│   │   ├── config.go           # 配置模型 + 读写 + 旧版迁移
│   │   └── config_test.go
│   ├── game/
│   │   ├── gameinfo.go         # 游戏元数据模型 + JSON 持久化
│   │   └── gameinfo_test.go
│   ├── scanner/
│   │   ├── scanner.go          # 目录递归扫描 + exe 识别
│   │   └── scanner_test.go
│   ├── scraper/
│   │   ├── scraper.go          # 刮削流水线 + Source 接口
│   │   ├── steam.go            # Steam 刮削器
│   │   ├── vndb.go             # VNDB 刮削器
│   │   ├── dlsite.go           # DLsite 爬虫
│   │   ├── bangumi.go          # Bangumi 刮削器
│   │   ├── steamgriddb.go      # SteamGridDB 封面刮削器
│   │   ├── cover.go            # 封面下载工具
│   │   └── scraper_test.go
│   └── logger/
│       └── logger.go           # 结构化日志系统 (slog + 按天归档)
├── frontend/                   # React SPA
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── wailsjs/                # Wails 自动生成的 TS 绑定 (提交)
│   │   ├── go/main/App.d.ts    # Go 方法声明
│   │   ├── go/main/App.js      # Go 方法调用
│   │   ├── go/models.ts        # Go 结构体的 TS 类
│   │   └── runtime/            # Wails 运行时
│   └── src/
│       ├── App.tsx             # 主布局 (侧边栏 + 内容区)
│       ├── App.css             # 全局样式
│       ├── main.tsx            # ReactDOM 入口
│   └── components/
│           ├── Sidebar.tsx      # 可折叠侧边栏导航
│           ├── GameCard.tsx     # 游戏封面卡片（含星标/标签覆盖层）
│           ├── GameDetail.tsx   # 游戏详情面板
│           ├── ContextMenu.tsx  # 右键上下文菜单（星标/标签/浏览路径/元数据）
│           └── Settings.tsx     # 设置页面
├── testdata/                   # 测试用模拟游戏目录
│   ├── simple_steam_game/
│   ├── deep_exe_game/
│   ├── multi_exe_game/
│   ├── local_game/
│   ├── visual_novel/
│   ├── collection/
│   ├── already_scanned/
│   └── not_a_game/
├── DESIGN.md                   # 本文件
├── README.md                   # 用户文档
├── wails.json                  # Wails 项目配置
├── go.mod / go.sum
└── .gitignore
```

### 2.4 数据流

```
用户点击 Scan Games
  → App.ScanGames() [Go]
    → scanner.ScanAll()
      → 遍历 config.GameDirectories
        → scanDir() 递归扫描
          → 发现 .exe → identifyGame()
            → 读 steam_appid.txt (向上3层)
            → 创建 GameInfo → 写入 .gameinfo.json
    → refreshGameCache()
      → 遍历目录加载所有 .gameinfo.json
  ← 返回 ScanResult[]
    → React setGames() → 渲染 GameCard 网格

用户修改设置
  → Settings 组件编辑 config
  → SaveConfig(cfg) [Go]
    → config.Save(exeDir) → 写 config.json
```

### 2.5 Wails IPC 绑定机制

```
Go (package main + internal/*)
  │
  │  Wails 编译时分析 App struct 的导出方法
  │  自动生成 TypeScript 绑定文件
  ▼
frontend/wailsjs/go/main/App.js   ← JS 代理函数
frontend/wailsjs/go/main/App.d.ts ← TS 类型声明
frontend/wailsjs/go/models.ts     ← Go struct → TS class

前端调用:
  import { GetGameList } from '../wailsjs/go/main/App'
  const games = await GetGameList()  // 类型: game.GameInfo[]
```

`app.go` 中使用类型别名保持 Wails 生成的命名空间一致：
```go
type Config = config.Config          // 实际在 internal/config/
type GameInfo = game.GameInfo        // 实际在 internal/game/
type ScanResult = scanner.ScanResult // 实际在 internal/scanner/
```

---

## 3. 核心模块

### 3.1 Config (`internal/config/`)

| 字段 | 类型 | 说明 |
|------|------|------|
| `machineId` | string | 自动生成，当前未使用（规划中用于锁文件） |
| `gameDirectories` | []string | 相对路径的游戏目录列表 |
| `maxScanDepth` | int | 扫描深度 (1-10) |
| `language` | string | 刮削语言偏好 (zh-CN/en-US/ja-JP) |
| `steamApiKey` | string | Steam API Key (规划中) |
| `metadataSources` | []MetadataSource | 元数据源列表，顺序即优先级 |

**MetadataSource:**
| 字段 | 类型 | 说明 |
|------|------|------|
| `key` | string | 标识 (steam/vndb/dlsite/igdb) |
| `name` | string | 显示名称 |
| `enabled` | bool | 是否启用 |

**兼容性：** `Load()` 自动将旧版 `vndbEnabled`/`dlsiteEnabled` 字段迁移为 `metadataSources`。

### 3.2 GameInfo (`internal/game/`)

每个游戏目录下的 `.gameinfo.json` 文件，记录：

| 字段 | 说明 |
|------|------|
| `id` | 唯一标识 (steam_123456 或目录名) |
| `title` | 游戏标题 (初始为目录名) |
| `platform` | 来源平台 (steam/local/vndb/dlsite) |
| `executables` | 可执行文件列表，含 Primary 标记 |
| `savePaths` | 存档路径配置 (规划中) |
| `metadata` | 刮削元数据 (封面/开发商/标签等) |
| `starred` | 用户星标标记 |
| `tags` | 用户自定义标签列表 |
| `totalPlaytime` | 累计游玩时长 (规划中，需多端聚合) |

### 3.3 Scanner (`internal/scanner/`)

递归扫描逻辑：

```
ScanAll()
  for each gameDir in config.GameDirectories:
    resolve relative path → scanDir(absDir, depth=0)

scanDir(dir, depth):
  if dir 包含 .exe → identifyGame(dir) → 生成 .gameinfo.json
  if depth >= maxDepth → 停止
  else → 递归进入子目录 (跳过 . 开头的隐藏目录)

identifyGame(dir):
  1. 检查 dir/.gameinfo.json → 已存在则返回 (IsNew=false)
  2. 向上 3 层查找 steam_appid.txt → 解析 AppID
  3. 列出 dir 中所有 .exe (过滤 unins*.exe)
  4. pickPrimaryExec: game > launcher > start > main > app > 最短名
  5. game.New() → game.Save() → 写入 .gameinfo.json
```

**游戏识别优先级：**
1. `steam_appid.txt` 精确匹配 → Steam ID
2. 未来：文件名哈希 / VNDB 搜索 / DLsite RJ 号匹配
3. 兜底：目录名作为 ID，platform=local

### 3.4 Logger (`internal/logger/`)

结构化日志系统，基于 Go `log/slog` + 自定义 `dailyHandler`：

```
App.startup()
  → logger.Init(exeDir)
    → 创建 exeDir/logs/ 目录
    → 初始化 slog Logger + dailyHandler

写入日志:
  logger.Info("scan started", "gameDirectories", dirs, "maxDepth", 3)
    → 格式化: 2026-05-30 14:30:01.234 [INFO] [scanner.go:42] scan started gameDirectories=[.\Games] maxDepth=3
    → 写入 logs/gamemanager_2026-05-30.log

按天轮转:
  dailyHandler.Handle()
    → 检查当前日期
    → 跨天则关闭旧文件，创建新日期文件
    → 追加写入
```

**日志级别：** `[DEBG]`—调试 / `[INFO]`—常规操作 / `[WARN]`—可恢复异常 / `[ERRO]`—严重错误

**日志覆盖范围：**

| 模块 | 记录内容 |
|------|---------|
| 应用生命周期 | 启动（版本/目录/主机名）、关闭、错误 |
| 配置 | 加载、保存、默认创建、字段迁移 |
| 扫描器 | 扫描开始/结束统计、目录进入/跳过、游戏发现（ID/标题/平台/可执行文件数）、exe 发现/过滤（unins 前缀）、主 exe 选择 |
| 刮削器 | 刮削开始、每个源尝试/跳过/失败/空结果、刮削成功（源/结果标题/平台 ID） |
| 封面下载 | 下载成功/失败，含 URL 和封面类型 |
| 游戏操作 | 启动成功/失败、游戏信息更新/保存 |

**设计要点：** 未初始化时（如单元测试）所有日志函数为空操作，无需额外配置。

---

## 4. 前端设计

### 4.1 组件树

```
App
├── Sidebar
│   ├── Brand (logo + title + collapse button)
│   ├── Nav Items
│   │   ├── "All Games" (count badge)
│   │   ├── Divider + "Categories"
│   │   └── Dynamic categories from game.type / game.tags
│   └── Bottom (machine name + Settings link, fixed)
└── Main Area
    ├── Top Bar (machine badge + Scan button)
    ├── Alert / Scan Summary
    └── Content
        ├── Content Header (title + count)
        ├── Game Grid (filtered by selected category)
        │   └── GameCard × N
        └── Settings (when selectedNav === 'settings')
```

### 4.2 页面

| 页面 | 路由 | 说明 |
|------|------|------|
| 游戏库 | `selectedNav === 'all'` | 封面墙，按最近游玩/标题排序 |
| 分类视图 | `selectedNav === 'type:xxx'` / `'tag:xxx'` | 按 type/tag 过滤 |
| 设置 | `selectedNav === 'settings'` | 卡片式布局，6 个设置卡片 |

### 4.3 设置页面卡片

1. **Game Directories** — 列表增删 + Browse 按钮 (调系统文件夹选择器)
2. **Scanning** — 深度滑块 (1-10)
3. **Language** — 下拉选择
4. **Metadata Sources** — 可排序列表，toggle 开关，▲▼ 排序
5. **About** — 展示自动获取的机器名

### 4.4 标签系统 (Phase 2 — 标签)

三级标签架构，各标签类型对应侧边栏不同区块筛选：

| 层级 | 来源 | 存储字段 | 卡片样式 | 侧边栏 |
|------|------|---------|---------|--------|
| 平台标签 | `platform` 字段（自动） | `GameInfo.Platform` | 左上角角标（Steam=蓝/DLsite=粉/Local=灰） | Platforms 区块 |
| 分类标签 | 刮削器返回的 genres/tags | `Metadata.Tags` | 左下角紫色 overlay | Genres 区块 |
| 用户标签 | 右键菜单手动添加 | `GameInfo.Tags` | 下中黄色 overlay，`#` 前缀 | My Tags 区块 |

```
侧边栏层次:
├── All Games
├── Starred
├── ── Platforms ──
│   ├── Steam (10)
│   ├── DLsite (3)
│   └── Local (2)
├── ── Genres ──
│   ├── Action (5)
│   ├── RPG (3)
│   └── ...
├── ── My Tags ──
│   └── #favorites (2)
└── Settings
```

---

## 4. 元数据刮削系统 (Phase 2)

### 4.1 架构

```
app.go: ScrapeGame / ScrapeAllGames
  │
  ▼
scraper.Pipeline (按 metadataSources 优先级依次调用)
  │
  ├── SteamScraper
  │     ├── 有 steam_appid → appdetails API  (精确查询)
  │     └── 无 steam_appid → storesearch API (名称搜索)
  │
  ├── VNDBScraper
  │     └── POST api.vndb.org/kana/vn  (名称搜索)
  │
  ├── DLsiteScraper
  │     ├── 目录名含 RJ\d{6,8} → 精确爬取作品页
  │     └── 无 RJ 号 → 跳过
  │
  └── IGDBScraper (暂缓，需 OAuth)
        └── Twitch IGDB API v4
```

### 4.2 Scraper 接口

```go
type ScrapeResult struct {
    Title       string
    TitleNative string
    Description string
    Developer   string
    Publisher   string
    ReleaseDate string
    Tags        []string
    CoverURL    string
    Links       map[string]string
}

type Scraper interface {
    Key() string
    Search(gameDir string, appID string) (*ScrapeResult, error)
}
```

### 4.3 流水线逻辑

```
Pipeline.Scrape(gameInfo):
  for src in config.Sources (按列表顺序 = 优先级):
    if !src.Enabled → 跳过
    scraper := registry[src.Key]
    result, err := scraper.Search(gameDir, gameInfo.PlatformID)
    if err != nil || result == nil → 继续下一个源
    return result, src.Key  ← 第一个匹配即返回
  return nil  ← 所有源均无结果
```

### 4.4 各源特点

| 源 | 识别方式 | 需要 Key | 封面来源 |
|----|---------|---------|---------|
| Steam | steam_appid / 名称搜索 | 否 | Steam CDN / SteamGridDB |
| VNDB | 名称搜索 | 否 | VNDB 图片 |
| DLsite | RJ 号精确匹配 | 否 | DLsite 图片 |
| IGDB | 名称搜索 | 是 (需注册) | IGDB 图片 |

### 4.5 封面下载

独立于刮削器的封面获取流程：

```
获取封面:
  1. 刮削器返回 CoverURL
  2. 下载到 .gamemanager/thumbnails/{gameId}.jpg
  3. 写入 GameInfo.Metadata.CoverURL = 本地相对路径
  4. 前端加载本地文件，无需网络请求
```

### 4.6 包结构

```
internal/scraper/
├── scraper.go      # Scraper 接口 + Pipeline + Registry
├── steam.go        # Steam 刮削器
├── vndb.go         # VNDB 刮削器
├── dlsite.go       # DLsite 刮削器
├── cover.go        # 封面下载工具
└── scraper_test.go
```

### 4.7 前端交互

```
游戏库封面墙
  ├── 点击卡片 → 右侧滑出详情面板
  │     ├── 封面大图
  │     ├── 标题 / 原名
  │     ├── 平台标签
  │     ├── 开发商 / 发行商
  │     ├── 描述
  │     ├── 标签列表
  │     ├── 可执行文件列表
  │     └── [Scrape Metadata] 按钮
  ├── 顶栏 [Scrape All] 批量刮削按钮
  └── 刮削进度指示器
```

### 4.8 详情面板数据流

```
用户点击游戏卡片
  → App.tsx: setSelectedGame(game)
  → 右侧面板打开
  → 显示 .gameinfo.json 中已有 metadata
  → 用户点击 [Scrape Metadata]
    → App.ScrapeGame(game.id)
      → Pipeline.Scrape(gameInfo)
        → 依次尝试各源
      → gameInfo.Metadata = result → gameInfo.Save()
      → 下载封面到本地
    ← React 刷新面板
```

---

## 5. 设计决策记录

### 5.1 为什么选择便携架构而非 Server+Agent？

| Server+Agent | Portable NAS |
|--------------|-------------|
| 需要部署服务端 | 单 exe 放入 NAS 即可 |
| 需各端装 Agent | 各端直接运行同一 exe |
| 数据库集中管理 | JSON 文件化，无依赖 |
| 适合多用户 | 适合个人/家庭 |

对于个人使用场景，便携架构在部署复杂度、数据可移植性、离线能力上均占优。

### 5.2 为什么 MachineName 不存 config.json？

`config.json` 存放于 NAS，所有客户端共享。如果存储 MachineName，最后保存的客户端会覆盖其他客户端的设置。改为 `os.Hostname()` 自动获取，每台机器独立识别。

### 5.3 为什么使用 internal/ 而非 pkg/？

`internal/` 是 Go 的约定，表示包仅限模块内部使用，外部项目无法导入。本项目不打算对外提供库接口，使用 `internal/` 更准确。

### 5.4 为什么 Wails 绑定文件提交到 Git？

`frontend/wailsjs/` 由 Wails 自动生成，提交后其他开发者无需安装 Wails CLI 即可构建前端。这是 Wails 官方推荐的做法。

### 5.5 为什么游戏目录选择后转为相对路径？

NAS 在每台客户端可能挂载为不同盘符 (Z:/ Y: 等)。存储相对路径 (`.\Games`) 而非绝对路径 (`Z:\Games`) 确保跨机兼容。

---

## 6. 项目约定

本节记录开发过程中需要遵守的约束和规范。

### 6.1 版本号规则 (Semver)

- 格式：`x.y.z-alpha`
- `x`（主版本）— 重大架构重构、不兼容的 API 变更
- `y`（次版本）— 重大功能更新（如整个 Phase 完成）
- `z`（修订版本）— 小功能添加、Bug 修复、CI/文档更新
- 每次提交（除纯文档外）必须步进最小版本号，生成新 tag
- 所有 alpha 版 tag 对应 GitHub pre-release
- 版本号硬编码在 `app.go` 的 `var version`，CI 通过 `-ldflags` 覆盖

### 6.2 发布流程

1. 本地确认编译通过：`wails build`
2. 本地运行全部测试：`go test ./...`
3. 提交代码：`git commit`
4. 打 tag 并推送：`git tag vX.Y.Z-alpha && git push && git push origin vX.Y.Z-alpha`
5. CI 自动构建 Windows / Linux / macOS 三平台产物并发布 GitHub Release

### 6.3 文档更新要求

| 变更类型 | 需更新的文件 |
|---------|------------|
| 新功能 | `CHANGELOG.md`（中英双语）、`DESIGN.md`（如涉及架构） |
| Bug 修复 | `CHANGELOG.md` |
| 版本发布 | `README.md` badges（如有）、`DESIGN.md` 路线图 |
| 配置模型变更 | `DESIGN.md` 对应章节 + 兼容迁移逻辑 |
| 新的数据源 | `DESIGN.md` 刮削系统章节 |

### 6.4 CHANGELOG 格式

- 中文为主要描述，英文斜体为辅助
- 按 `Added` / `Changed` / `Fixed` 分类
- 每个条目：中文一行 + 英文斜体一行

```markdown
- 新增某某功能（`按钮名`）
  *Added some feature (`Button Name`)*
```

### 6.5 代码规范

- Go 源码按功能模块放入 `internal/` 子包（`config` / `game` / `scanner` / `scraper` / `logger`）
- Wails 绑定文件（`frontend/wailsjs/`）提交到 Git
- React 组件放在 `frontend/src/components/`
- 测试文件与源文件同目录，命名 `*_test.go`
- 编译产物 `build/`、`*.exe`、`config.json` 均被 `.gitignore` 排除
- 测试用模拟数据放在 `testdata/` 目录

### 6.6 CI 约束

- GitHub Actions 仅在推送 `v*` tag 时触发
- 使用 `ubuntu-22.04`（非 `latest`，因 Wails v2 依赖 webkit2gtk-4.0）
- macOS 产物取 `.app/Contents/MacOS/` 内二进制
- 每个平台生成独立 SHA256 校验文件
- Release 标记为 `prerelease: true`

### 6.7 数据兼容性

- `config.json` 新增字段时必须提供默认值和旧版迁移逻辑
- `metadataSources` 新增默认源时用合并逻辑（保留用户已有源的顺序和状态）
- 不可删除已有 JSON 字段（仅追加）

---

## 7. 开发进度

### Phase 1 — 骨架 ✅ (已完成)

- [x] Wails v2 + React + TypeScript 项目初始化
- [x] Config 配置模型、JSON 读写、旧版迁移
- [x] GameInfo 元数据模型、JSON 持久化
- [x] 游戏扫描器：递归遍历、exe 识别、steam_appid 解析
- [x] React 前端：侧边栏布局、封面墙、空状态
- [x] 设置页面：游戏目录管理、扫描深度、语言、元数据源
- [x] 系统文件夹选择器 (Browse) + 相对路径转换
- [x] 机器名从 os.Hostname() 自动获取
- [x] 17 个单元测试全部通过
- [x] Git 版本控制 + GitHub 推送

### Phase 2 — 刮削 ✅ (已完成) — `0.2.4-alpha`

- [x] Steam API 集成 (appdetails + storesearch)
- [x] VNDB API 集成
- [x] DLsite 爬虫 (RJ 号识别)
- [x] Bangumi 刮削器 (中文元数据)
- [x] SteamGridDB 刮削器 (高清封面)
- [x] 刮削流水线：Pipeline 按 metadataSources 优先级依次查询
- [x] 封面下载到游戏目录 (竖版 cover + 横版 cover_landscape)
- [x] 游戏详情页（右侧滑出面板）
- [x] 刮削器单元测试
- [x] 批量刮削进度条 + 并行限制 (3 并发)
- [x] 不完整数据检测 (缺封面/描述/日期自动补刮)
- [x] 语言感知刮削 (Steam/VNDB 按 zh-CN 返回本地化结果)
- [x] 数据源专属设置 (展开式配置面板，API Key 等)
- [x] 结构化日志系统 (slog + 按天归档，覆盖扫描/刮削/配置/启动)

### Phase 3 — 启动与锁 (规划中)

- [ ] 进程启动 (os/exec)
- [ ] 锁文件机制 (防多端同时启动)
- [ ] 心跳检测与死锁清理
- [ ] 启动按钮 + 运行状态 UI

### Phase 4 — 时长统计 (规划中)

- [ ] 进程存活监控
- [ ] 会话文件记录
- [ ] 多端时间聚合
- [ ] 时长展示

### Phase 5 — 云存档 (规划中)

- [ ] 符号链接方案 (mklink /J)
- [ ] 文件复制同步方案
- [ ] PreSync / PostSync 流程
- [ ] 存档路径配置 (手动 + PCGamingWiki)
- [ ] 备份与版本管理

---

## 8. 构建与开发

```bash
# 环境要求: Go 1.23+, Node.js 18+
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 开发模式 (热重载)
wails dev

# 生产构建
wails build

# 运行测试
go test ./...
```
