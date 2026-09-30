# GameLibrary — 设计文档

> 本项目在 **DeepSeek Harness** 辅助下开发。Current version: **0.8.0** | Last updated: 2026-09-29

本文描述代码当前的实际结构；用户文档见 `README.md`，逐版本变更见 `CHANGELOG.md`。

## 版本路线图

| 版本 | 阶段 | 核心内容 | 状态 |
|------|------|---------|------|
| 0.1.0 – 0.7.5 | Phase 1 / Phase 2 | 骨架、扫描、多源刮削、标签体系、任务队列、文件监听（明细见 `CHANGELOG.md`） | ✅ |
| 0.8.0 | Phase 2.5 — 重构与优化 | 并发安全库缓存、运行时状态重建、共享 HTTP 客户端、**并行刮削（线程池 + 各源并发）**、封面 HTTP 服务、任务列表界面、类型化刮削错误、启动失败诊断、Windows 收敛、测试与 CI 基线 | ✅ |
| 0.9.0 | Phase 3 — 启动与锁 | 锁文件、心跳检测、运行状态 UI | 📋 |
| 0.10.0 | Phase 4 — 时长统计 | 进程存活监控、多端时长聚合、统计展示 | 📋 |
| 0.11.0 | Phase 5 — 云存档 | 存档路径配置、符号链接/复制同步、备份版本 | 📋 |
| 1.0.0 | 正式发布 | 稳定版 | 📋 |

---

## 1. 项目概述

GameLibrary 是一个跨机器、便携式的游戏库管理器。管理程序与游戏文件一同存放在 NAS 网络挂载路径中，所有客户端共享同一份程序与配置，通过相对路径寻址实现零配置跨机迁移。

- **便携优先** — 单个 exe 放入 NAS 根目录即可运行，不需要服务端，也没有安装步骤
- **文件即数据** — 元数据存为 JSON 文件，免数据库，拷贝即迁移，可人工阅读与编辑
- **NAS 即云端** — 游戏文件、封面、元数据统一存放于共享目录，多端共用同一份真相

### 目标平台

**Windows 是唯一支持的目标平台，也是唯一的发布产物。**

- 启动器识别、Shell 集成（Explorer / Notepad）、Steam 注册表探测都是 Windows 专有逻辑，位于 `internal/platform/platform_windows.go`
- `internal/platform/platform_other.go`（`//go:build !windows`）只提供 stub：Shell 回落 `open` / `xdg-open`，脚本启动器返回 `ErrUnsupported`，`SteamPath()` 返回 `""`。它只为让模块在别的平台仍可编译可测，CI 因此能在 `ubuntu-22.04` 上跑 Go 套件（含竞态检测）
- 路径处理仍接受 Unix 风格输入（见 3.2），因为配置文件是人工可编辑的文本

---

## 2. 架构设计

### 2.1 部署形态

```
NAS / SMB 共享目录
GameLibrary\
├── GameLibrary.exe     ← 单文件程序 (Wails + WebView2)
├── config.json         ← 多端共享配置（相对路径）
├── logs\               ← 仅当 logToLibrary = true
└── Games\
    └── GameA\
        ├── .gamemanager\
        │   ├── gameinfo.json       ← 游戏元数据
        │   ├── covers\             ← cover.<ext> / cover_landscape.<ext>
        │   └── meta\<source>.json  ← 各源返回快照（只写不读回）
        └── game.exe

客户端 A: Z:\ → NAS        客户端 B: Y:\ → NAS
机器名在运行期取 os.Hostname()；日志默认写本机 %LocalAppData%\GameLibrary\logs
封面由内置 HTTP 服务提供：GET /covers/{id}/{variant}
```

同一份 `config.json` 在任意盘符下都可用，因为目录项以 `.\Games` 这种相对形式存储；同一份 `gameinfo.json` 可被多端读写，因为其中不含机器相关信息（`gameDir` 不落盘）。

### 2.2 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 运行时 | Wails v2 | Go + WebView2，编译为单 exe |
| 后端 | Go 1.23+ | 扫描、刮削、并发缓存、HTTP 资产服务 |
| 前端 | React 19 + TypeScript 5 + Vite 7 | 构建产物 `frontend/dist` 内嵌进 exe |
| 样式 | Pure CSS（暗色主题） | 无第三方 UI 库 |
| 数据 | JSON 文件 | 免数据库，原子写入 |
| IPC | Wails Bindings | `App` 的导出方法自动生成类型安全绑定 |
| 媒体 | 内置 HTTP handler | 封面走 asset server，不经过 IPC |
| 文件监听 | fsnotify | 按扫描深度监听库目录 |

### 2.3 代码结构

```
GameLibrary/
├── main.go / app.go / scrape_api.go / library_api.go
├── internal/
│   ├── config/       # 配置模型、归一化、旧版迁移、机器名
│   ├── fsutil/       # 路径解析与原子写入
│   ├── game/         # GameInfo 模型与 .gamemanager 持久化
│   ├── library/      # 并发安全游戏缓存（RWMutex + 深拷贝读取）
│   ├── logger/       # slog + 按天轮转 handler
│   ├── mediaserve/   # /covers/... 的 http.Handler
│   ├── platform/     # 启动器识别、Shell、Steam 注册表（_windows / _other）
│   ├── scanner/      # 目录遍历、游戏识别、ACF 解析
│   ├── scraper/      # Source 接口、Pipeline、共享 HTTP 客户端、六个数据源
│   ├── taskqueue/    # 单 worker 任务队列
│   └── watcher/      # fsnotify 监听
├── frontend/
│   ├── wailsjs/      # Wails 生成的绑定（提交到 Git）
│   └── src/
│       ├── api/      # client.ts（唯一导入绑定）与 events.ts（事件订阅）
│       ├── lib/      # 纯逻辑：platform / cover / format / filters + lib.test.ts
│       ├── hooks/    # useScrape.ts
│       ├── components/  # Sidebar / GameCard / GameDetail / ContextMenu / Settings
│       └── App.tsx / App.css
├── testdata/         # 测试用模拟游戏目录
├── .github/workflows/  # ci.yml（普通推送/PR）、release.yml（v* tag）
├── .golangci.yml / wails.json
└── DESIGN.md / CHANGELOG.md / README.md
```

`package main` 按职责拆成三个文件：`app.go` 只放结构体、生命周期与装配，`scrape_api.go` 放扫描/刮削/队列与配置保存，`library_api.go` 放查询、用户字段与设置辅助。

### 2.4 运行时状态重建（0.8.0 的关键修复）

`App` 不直接持有扫描器与刮削流水线，而是把「由配置派生出来的组件」打成一个包，读取方通过 `App.snapshot()` 取当前值：

```go
type runtimeState struct {
    cfg      *config.Config
    scanner  *scanner.Scanner
    pipeline *scraper.Pipeline
    covers   *scraper.CoverFetcher
}
```

`App` 持有 `stateMu sync.RWMutex` + `state *runtimeState`（读者不得修改快照）以及 `scanMu sync.Mutex`（串行化后台扫描）。

**为什么需要这一层：** 旧实现里 `Scanner` 与 `Pipeline` 在启动时构造一次，各自持有指向「启动那一刻的 `*config.Config`」的指针。用户在设置页改了游戏目录、扫描深度、语言或 API Key 之后，`config.json` 确实写入了，但内存中的扫描器与流水线仍读旧对象 —— 表现为「保存了但没反应」，必须重启才生效。现在 `scanner.New(root, cfg)` 可以简单地保存指针，因为配置变化时换的是整个扫描器对象。

`App.SaveConfig` 的顺序：

| 步骤 | 动作 |
|------|------|
| 1 | `cfg.Normalize(exeDir)`：补默认值、归一化目录、同步数据源、清理失效目录标签 |
| 2 | `cfg.Save(exeDir)`：原子写入 `config.json` |
| 3 | `buildState(cfg)`：新建 `Pipeline`（重新注册六个源并 `Configure`）、`Scanner`、`CoverFetcher`（复用 `pipeline.HTTP()`），整体替换 `a.state` |
| 4 | `logger.Reconfigure(...)`：日志级别与位置也属于配置 |
| 5 | `restartWatcher()`：按新目录列表与扫描深度重建监听 |
| 6 | `loadLibrary()`：`scanner.LoadLibrary()` 后 `library.ReplaceAll` |
| 7 | `emit("library:changed", {configSaved: true})` |
| 8 | 后台 goroutine 取 `scanMu`，逐个新目录 `scanDirs(dir, false)`，不阻塞设置对话框 |

替换只发生在持 `stateMu` 写锁的一瞬间，网络请求与磁盘遍历期间不持锁。

### 2.5 并发模型

游戏缓存（`internal/library.Store`）由四个独立参与者读写，这是 0.8.0 之前最严重的问题（无锁 `map` 并发读写，可直接让 webview 调用崩溃）：

| 参与者 | 触发者 | 对缓存的访问 |
|--------|--------|-------------|
| Wails IPC 处理器 | 前端每次调用一个 goroutine | `Get` / `List` / `Mutate` / `Replace` / `RemoveWithin` |
| 队列 worker | 单条后台 goroutine | `Get` → 刮削 → `Replace`（网络请求在锁外） |
| 监听回调 | watcher goroutine | `RemoveWithin`（目录消失），随后派发后台扫描 |
| 后台扫描 | `SaveConfig`、监听回调 | `Put`（`scanDirs`）；`ReplaceAll`（`doScan`、`loadLibrary`） |

- **每次读取都返回深拷贝。** `Get` / `List` / `Mutate` / `Replace` 都走 `GameInfo.Clone()`，调用方可以随意修改拿到的对象，不会污染共享状态，也不需要在调用侧加锁
- **每次写入都在同一把锁内完成并落盘。** `Mutate(id, fn)` 与 `Replace(info)` 在临界区内克隆草稿 → 执行闭包 → `Save()` → 写回；闭包返回错误则整体回滚，缓存与文件都不会半更新
- **`scanMu` 串行化后台扫描。** 监听器在解压/写盘时会连续产生大量事件，`scanMu` 保证同一棵子树不被重叠遍历，也让 `SaveConfig` 的后台扫描与监听触发的扫描排队执行；IPC 入口 `ScanGames` / `ForceScanGames` 同样先取它
- **队列只有一个 worker。** 外部数据源普遍限流，串行比并发重试更有效，单 worker 也让「当前任务 / 待处理列表」可以如实呈现给 UI。串行是刻意设计，不是实现限制
- **`stateMu` 只保护 `runtimeState` 指针本身**，不覆盖网络请求或磁盘 IO

### 2.6 Wails IPC 绑定机制

```
Go (package main + internal/*)
  │  Wails 在编译期分析 App 的导出方法
  ▼
frontend/wailsjs/go/main/App.js    ← JS 代理
frontend/wailsjs/go/main/App.d.ts  ← TS 类型声明
frontend/wailsjs/go/models.ts      ← Go struct → TS class
```

`app.go` 用类型别名（`Config` / `GameInfo` / `ScanResult` / `Executable` / `SavePath` / `Metadata`）把各包的模型固定在同一命名空间，绑定名不会随内部包重构而漂移。

- **只有导出方法会被绑定。** 封面 handler 是 `func (a *App) assetHandler() *mediaserve.Handler` —— 未导出，避免基础设施被暴露成前端 API
- **`GetAppInfo` 返回类型化的 `AppInfo`**（`exeDir` / `machineId` / `machineName` / `version` / `buildTime` / `logDir` / `platform` / `coverBaseUrl`），不再返回 `map[string]string`：前者字段访问有类型检查，后者每个字段都是一次未校验的字符串查表
- **`buildTime` 由构建注入。** 未注入时为 `"development"`；此前该字段返回 `time.Now()`，即「构建时间」随每次调用变化。`isDevBuild()` 在 `buildTime == "development"` 或 `GAMELIBRARY_DEV=1` 时为真，决定日志是否镜像到 stderr

### 2.7 封面服务与缓存失效

旧实现通过 IPC 返回封面：每次调用完整读取图片、base64 编码、把字符串推过 JS 桥。每张图膨胀约三分之一，N 张卡片的网格就是 N 次全尺寸传输。现在 `main.go` 把 `mediaserve.Handler` 挂到 asset server 的 `Handler` 字段，嵌入资源未命中的请求落到它：

| 请求 | 结果 |
|------|------|
| `GET /covers/{id}`、`/cover`、`/portrait` | 竖版封面 |
| `GET /covers/{id}/landscape` | 横版封面 |
| 其它路径、空 id、含 `/`、`\`、`..` 的 id | 404 |
| 无记录、文件不存在、是目录、大小为 0 | 404（前端显示自己的占位图） |
| 非 `GET` / `HEAD` | 405 |

路径解析结果交给 `mediaserve.CoverResolver` 接口，生产实现是 `library.Store.CoverPath`：按 `.jpg → .jpeg → .png → .webp` 在 `.gamemanager/covers/` 中查找，找不到再回落到游戏根目录（兼容未重刮的旧库）。

**缓存失效由版本号驱动**：`GameInfo.CoverVersion` 是封面每次被重写时更新的 Unix 秒时间戳（由 `MarkCoverUpdated()` 设置），刮削路径上只有真正写入了新封面才 bump，URL 附带 `?v=<coverVersion>`；handler 对带 `v` 的请求返回 `Cache-Control: public, max-age=31536000, immutable`，无 `v` 的返回 `no-cache`。前端 `coverUrl(id, kind, coverVersion)` 拼 URL、`coverKey()` 判断版本是否变过，图片加载失败交给浏览器 `onError`，不在 JS 里探测文件。Content-Type 由扩展名决定，响应体经 `http.ServeContent` 输出，因此支持 Range 与条件请求。

---

## 3. 核心模块

### 3.1 Config (`internal/config/`)

`config.json` 位于库根目录、被所有客户端读写，因此有两条硬约束：**不放机器相关字段**，**目录项用可移植形式存储**。

| 字段 | JSON | 默认值 | 说明 |
|------|------|--------|------|
| `SchemaVersion` | `schemaVersion` | 2 | 写入时总是落当前值 |
| `MachineID` | `machineId` | `machine-<hostname>` | 缺失时按主机名生成 |
| `GameDirectories` | `gameDirectories` | `[".\\Games"]` | 可移植目录列表，顺序即侧边栏顺序 |
| `GameDirectoryLabels` | `gameDirectoryLabels` | `{}` | 目录 → 标签（多标签） |
| `MaxScanDepth` | `maxScanDepth` | 3（上限 10） | 子目录递归深度 |
| `Language` | `language` | `zh-CN` | 刮削语言偏好 |
| `SteamUserID` | `steamUserId` | `""` | 选中的 Steam 账号 |
| `WatcherEnabled` / `WatcherDebounceMs` | `watcherEnabled` / `watcherDebounceMs` | true / 150 | 监听开关与事件归并窗口 |
| `ScrapeConcurrency` | `scrapeConcurrency` | 4（1–16） | 同时刮削的游戏数；各数据源仍受按主机限流 |
| `LogLevel` / `LogToLibrary` | `logLevel` / `logToLibrary` | `info` / false | 日志级别；是否写共享目录而非本机缓存目录 |
| `Sources` | `metadataSources` | 6 个内置源 | 顺序即优先级；`MetadataSource{key,name,enabled,settings}` |

| 函数 | 职责 |
|------|------|
| `Default()` / `Load(root)` | 全新配置 / 读取；文件不存在时创建默认配置并落盘（只读共享目录写失败仅告警，不阻止启动），随后 `Normalize` |
| `Normalize(root)` | 补默认值、夹取深度、规范日志级别、归一化目录、`SyncSources`、`AutoLabelPaths` |
| `NormalizeDir(root, dir)` | 单一目录项规范化：任意分隔符、`./`、`.\`、绝对路径都收敛到一种存储形式 |
| `SyncSources()` | 去重、保留用户顺序/开关/设置，把 `DefaultSources()` 中新增的源追加到末尾 |
| `AutoLabelPaths(root)` | 疑似 Steam 库的目录自动加 `Steam` 标签；已不存在的目录对应标签被删除 |
| `Save(root)` / `SourceSettings` / `SourceEnabled` / `EnabledSourceKeys` / `ResolveDir` | 原子写入（0644，末尾换行）/ 数据源查询与路径解析 |
| `decode`（`Load` 内部） | 旧版迁移：无 `metadataSources` 时读 `vndbEnabled` / `dlsiteEnabled` / `bangumiEnabled` / `steamgriddbEnabled` 布尔并套到默认源上 |

`internal/config/hostname.go` 提供 `Hostname()`；机器名刻意在运行期取得，不写入共享配置（见 7.2）。

### 3.2 fsutil (`internal/fsutil/`)

库根目录通常在 SMB 共享上，因此两件事在任何地方都成立：人类写的路径可能是任意分隔符风格，而写坏的 JSON 比缺失的 JSON 更糟。

| 函数 | 说明 |
|------|------|
| `Resolve(base, path)` | 可相对、可 Unix 风格 → 绝对原生路径。接受 `.\Games`、`./Games`、`../shared/Games`、`E:\SteamLibrary` |
| `ToNative(path)` | 任意混合分隔符 → 本机分隔符（先统一为 `/`，再 `filepath.FromSlash`） |
| `ToPortable(root, abs)` | 绝对路径 → 存储形式：相对库根时返回 `.\<rel>`；`..` 层级超过 `maxRelativeClimb = 3` 时退回绝对路径；分隔符始终存为 `\` |
| `IsWithin(root, path)` | `path` 等于 `root` 或在其之下（标签匹配、移除子树、`steam_appid.txt` 上溯边界） |
| `WriteFileAtomic(path, data, perm)` / `ReadFile` / `Exists` / `DirExists` | 原子写入 / 读取（「不存在」统一为 `os.ErrNotExist`）/ 存在的普通文件 / 存在的目录 |

`ToPortable` 保留最多 3 层 `..`：NAS 上常见布局是程序在 `<share>\apps\GameLibrary`、游戏在 `<share>\Games`，这种相对路径跨盘符仍然成立；再远就退回绝对路径更诚实。

`WriteFileAtomic` 的顺序：`MkdirAll` 目标目录 → 在**同一目录**创建临时文件 `.name.tmp*` → 写入 → `Sync()` 落盘 → `Chmod` → `Close` → `Rename` 覆盖目标。同目录内 rename 在 NTFS 与 SMB 上都是原子的，崩溃或共享断开只会留下旧文件或新文件；Windows 上目标可能仍被其他客户端打开导致 rename 失败，此时删除目标后重试一次。

### 3.3 Game (`internal/game/`)

元数据位于 `GameDir/.gamemanager/`：`gameinfo.json`（核心记录）、`covers/`（封面，`cover.<ext>` 与 `cover_landscape.<ext>`）、`meta/<source>.json`（各源快照，只写）。`SchemaVersion = 2`。

| 字段 | JSON | 说明 |
|------|------|------|
| `SchemaVersion` | `schemaVersion` | 写入时总是当前值 |
| `ID` | `id` | `steam_<appid>` 或 `local_<sanitized>_<8位哈希>` |
| `Title` / `TitleNative` / `Type` | `title` / `titleNative` / `type` | 显示名 / 原文名 / 目前恒为 `game` |
| `Platforms` | `platforms` | `[]PlatformInfo{platform,id,name}`，一游戏可关联多个源 |
| `Aliases` / `PreferredSource` | `aliases` / `preferredSource` | 标题变体 / 权威来源，决定 `ApplyResult` 的胜出者 |
| `Executables` | `executables` | `[]Executable{path,name,primary}` |
| `SavePaths` / `Metadata` | `savePaths` / `metadata` | Phase 5 预留字段，当前不写入 / 刮削结果 |
| `ScannedAt` | `scannedAt` | 首次识别时间（RFC3339，UTC） |
| `TotalPlaytime` / `LastPlayedAt` | `totalPlaytime` / `lastPlayedAt` | 累计分钟数（Phase 4 填充）/ 启动游戏时写入，用于排序 |
| `Starred` / `Tags` / `CoverVersion` | `starred` / `tags` / `coverVersion` | 星标 / 用户标签 / 封面版本（驱动缓存失效） |
| `GameDir` | —（`json:"-"`） | 记录从哪个目录读入；机器相关，永不落盘 |

`Metadata` 含 `coverUrl`、`coverLandscape`（值为本地文件名）、`releaseDate`、`developer`、`publisher`、`tags`、`description`、`links`。

| 方法 | 说明 |
|------|------|
| `Clone()` / `Save()` | 深拷贝（切片与 `Links` 都复制）/ 落 `schemaVersion` 后原子写入，`GameDir` 为空即报错 |
| `SaveMeta(source)` / `LoadMeta(source)` | 写/读 `.gamemanager/meta/<source>.json`；**只有写入被使用**，没有读回流程 |
| `NewID(root, gameDir, steamAppID)` | Steam appID 全局唯一，直接用作 `steam_<id>`；其余按「相对库根的路径」做 FNV-32a 哈希，前缀是清洗过的目录名 |
| `PreferredPlatformInfo()` | 首选源命中则用它，否则第一个带平台键的条目 |
| `PrimaryPlatform()` / `PrimaryPlatformID()` | **都从上面这一个函数派生**，不可能给出互相矛盾的结论（旧实现一个用首选源、一个取 `Platforms[0]`） |
| `SetPlatform(platform, id, name)` | 插入或更新平台链接；此前没有首选源时顺带设为首选 |
| `AddAlias` / `AddTag` / `RemoveTag` / `HasTag` | 忽略空白、重复与大小写差异 |
| `PrimaryExecutable()` / `MarkCoverUpdated()` / `SortByRecentPlay` | 主启动器（无标记则第一个）/ 写入 `CoverVersion` / 最近游玩在前、同档按标题排序 |
| `New(...)` / `LoadFromDir(gameDir)` | 新建记录（有 Steam appID 时直接写入平台与首选源）/ 读取，必要时走旧格式迁移 |

读入期迁移：`platforms` 为空时回读旧版的扁平 `platform` / `platformId` 并合成列表、设为首选源；`schemaVersion` 为 0 视为 1，`id` 与平台 ID 去 BOM 与空白，`type` 为空补 `game`；旧位置 `.gameinfo.json` 存在时解析 → 补齐 ID → 把根目录封面搬进 `covers/`（**先创建目标目录**，Windows 上 rename 到不存在的目录会失败）→ 写新文件成功后才删除旧文件。

### 3.4 Library (`internal/library/`)

`Store` 是游戏 ID → 记录的并发安全缓存，是 2.5 节并发模型的核心。

| 方法 | 语义 |
|------|------|
| `New(root)` / `Len()` / `IDs()` | 空缓存（`root` 用于可移植标识与路径判断）/ 计数 / ID 列表 |
| `Get(id)` / `List()` | 返回**副本**（`List` 按最近游玩排序）；未命中返回 `(nil, false)` |
| `Put(info)` / `ReplaceAll(infos)` | 插入或替换（存入时克隆）/ 用一次加载的结果整体替换缓存 |
| `Replace(info)` | 替换一个**已存在**的游戏：校验 ID、落盘、更新缓存，同一临界区；不存在返回 `ErrNotFound` |
| `Mutate(id, fn)` | 读-改-写：克隆 → 闭包 → 落盘 → 写回；闭包返回错误则放弃整个修改并返回该错误 |
| `Remove(id)` / `RemoveWithin(dir)` | 删除一个 / 删除目录内全部（返回被删 ID，监听器报告目录消失时使用） |
| `CoverPath(id, landscape)` | 封面路径，不存在返回 `""`；`covers/` 优先，回落游戏根目录；按 `.jpg/.jpeg/.png/.webp` 顺序 |

`Mutate` 与 `Replace` 是「写」的两个正规入口：前端触发的单字段修改走 `Mutate`（只提交闭包），刮削产生的新记录走 `Replace`。`Save(id)`、`GameDirOf(id)`、`WithinRoot(dir)`、`NormalizeTitle(s)` 目前只被单元测试引用。

### 3.5 Scanner (`internal/scanner/`)

| 入口 | 说明 |
|------|------|
| `New(root, cfg)` / `Config()` / `Root()` | 构造与自省；配置变化时整体替换实例（见 2.4） |
| `ScanAll()` / `ForceScanAll()` | 遍历所有配置目录；后者全部重新识别但保留用户数据 |
| `ScanDir(dir)` / `ForceScanDir(dir)` | 单目录版本，供文件监听调用 |
| `LoadLibrary()` | 只读地把已记录的游戏加载出来（不写盘），用于启动时填缓存 |

**「游戏目录」只有一个定义**：`IsGameDirectory(entries)` 为真，当目录里存在启动器文件（见 3.11）**或**存在 `.gamemanager` 目录；`IsGameDirPath(dir)` 是磁盘路径版本。此前扫描器要求启动器、而缓存加载同时接受 `.gamemanager`，两者对同一目录结论不同。

遍历规则：**配置的根目录永远会被遍历下去**，即使根目录自己也像一个游戏（旧实现在发现可执行文件后立即停止递归，于是库根目录里一个安装程序就能把整个游戏库藏起来）；非根目录一旦被判定为游戏就停止下降，隐藏目录（`.` 开头）一律跳过；深度达到 `MaxScanDepth` 时停止，单目录识别失败（无启动器且无既有记录）产出 `ScanResult{Error: "no launchable file found"}`，不影响其它目录。

识别顺序（`identify`）：

| 步骤 | 行为 |
|------|------|
| 1 | 读 `.gamemanager/gameinfo.json`（`LoadFromDir`，含旧格式迁移）；非强制模式且已有记录则直接返回（`IsNew: false`） |
| 2 | `readSteamAppID`：在自身及最多两级父目录找 `steam_appid.txt`，**不越过库根目录**，去 BOM/换行，含空白视为无效 |
| 3 | `readACF`：目录形如 `...\steamapps\common\<Game>` 时在同级 `steamapps` 里按 `installdir` 匹配 `appmanifest_*.acf`，解析 `appid` / `name` / `installdir` / `LastUpdated` / `SizeOnDisk` |
| 4 | `findLaunchers`：列出可启动文件，过滤 `unins*`、`UnityCrashHandler*` 与工具程序（`crashreport`、`patcher`/`update`、`dxsetup`、`vcredist`、`dotnet`、`steamsetup`、`uplayinstaller` 等） |
| 5 | 无启动器：有既有记录则保留该记录（可执行文件被移除不等于游戏损坏），否则报错 |
| 6 | `game.New(...)` → 有既有记录则 `preserveUserData` → 有 ACF 名称则以 ACF 标题覆盖 → `info.Save()` 原子写入，产出 `ScanResult{IsNew: existing == nil}` |

`pickPrimaryLauncher` 的顺序：优先在 `.exe` 中挑，名称包含 `game` → `launcher` → `start` → `main` → `app`；都不匹配则取路径最短者。脚本（`.bat`/`.cmd`/`.ps1`）只在没有 `.exe` 时参与竞争。

`preserveUserData(old, fresh)` 在强制重扫时保留 `Title`、`TitleNative`、`Type`、`Platforms`、`Aliases`、`PreferredSource`、`Metadata`、`SavePaths`、`Starred`、`Tags`、`TotalPlaytime`、`LastPlayedAt`、`CoverVersion`、`ScannedAt`，并尽量沿用用户选定的主启动器（同名路径仍存在时）。强制重扫此前会重建记录并丢掉这些内容。

### 3.6 Scraper (`internal/scraper/`)

```go
type Source interface {
    Key() string
    Configure(cfg SourceConfig) error
    Search(ctx context.Context, q Query) (*Result, error)
}
```

**同一游戏的多个数据源并发查询。** 这些源是彼此独立的服务，每个主机在共享 HTTP 客户端里已有各自的限流器，因此并发只把"各源延迟之和"变成"最慢的那个源"。逐源串行曾是整库刮削的主要瓶颈。结果按**配置优先级顺序**而非完成顺序装配，因此下游的首选源选择保持确定性；认证类错误取优先级最高的那个上报。

| 概念 | 说明 |
|------|------|
| `Query` | `{GameDir, Name, PlatformIDs}`；`NewQuery(info)` 从记录构造，`PlatformID(source)` / `SteamAppID()` 读取。**不再传裸 appID**：过去四个源忽略 ID、一个源忽略目录，DLsite 的 RJ 码甚至会被当成 Steam appID 递过去 |
| `SourceConfig` | `{Language, APIKey, HTTP, Timeout}`（超时 20s）。它属于接口的一部分，因此流水线能重配所有源 —— 旧实现用具体类型断言逐个设置，新增源会被静默漏掉 |
| `Pipeline` | `NewPipeline(cfg)` 创建一个共享 HTTP 客户端；`Register` / `Registered`（排序后的键）/ `Configure`（重新下发语言、API Key、客户端）/ `ScrapeAll(ctx, info)`；`HTTP()` 暴露客户端给封面下载 |
| `ApplyResult(info, result, sourceKey)` | 逐字段合并：空字段绝不覆盖已有值。SteamGridDB 只提供封面，作为首选源时曾把标题、描述、开发商整条抹掉 |
| `mergeMetadata` | `Description` / `Developer` / `Publisher` / `ReleaseDate` 非空才写；标签去重追加；`Links` 逐键合并并跳过 `platformId` |

`ScrapeAll` 按配置顺序遍历数据源，未注册或未启用的记 debug 跳过；`ErrNoResult`（或 `nil, nil`）按「无匹配」处理；其它错误记 warn 并继续，其中认证类错误会被记住。全部源都没结果时，有认证错误则返回它，否则返回 `ErrNoResult`。**只收集、不裁决**：首选源由调用方（`scrapeOne`）决定。

| 错误类型 | 用途 |
|----------|------|
| `ErrNoResult` / `NoResult` / `IsNoResult` | 「源正常答复但没有匹配」，是正常结果而非故障。此前「没有匹配」与「接口挂了」都是普通 error，无法区分，也无法对瞬时故障重试 |
| `APIError{Source,Status,Kind,URL,Message,Err}` | 用户可能可以处理的失败；`Error()` 形如 `source: HTTP 500 server: ...`，`Unwrap()` 暴露底层错误，`IsRetryable()` 对限流/服务端/网络为真 |
| `ErrorKind` | `KindNoResult` / `KindNetwork` / `KindAuth` / `KindRateLimited` / `KindServer` / `KindClient` / `KindParse`（`no-result` / `network` / `auth` / `rate-limited` / `server` / `client` / `parse`）；`KindOf(err)` 提取分类，默认 `KindNetwork` |

`HTTPClient` 是全部刮削器与封面下载共用的唯一客户端（旧实现有七个连接池：每源一个加 `http.DefaultClient`）：

| 项 | 值 |
|----|----|
| 超时与连接池 | 20s（同时作为 `http.Client.Timeout` 与 `ResponseHeaderTimeout`）；`MaxIdleConns 16`、`MaxIdleConnsPerHost 4`、`IdleConnTimeout 60s`、`TLSHandshakeTimeout 10s`、`ExpectContinueTimeout 1s`、`Proxy: http.ProxyFromEnvironment` |
| User-Agent 与体积上限 | `GameLibrary/<ClientVersion>`；`DefaultMaxBytes = 2 MiB`，各源自行收窄（DLsite 3 MiB，其余 1–2 MiB） |
| 重试 | `DefaultRetryPolicy`：3 次尝试、基础退避 400ms、上限 5s、可接受的最长 `Retry-After` 20s，退避带 0–50% 抖动 |
| 限流与 `Do` | `LimitHost(host, minInterval)` + 互斥保护的 `hostLimiter`；`Do(ctx, source, req)` 先限流再请求，非 2xx 一律转成 `*APIError`（并 honoring `Retry-After`），可重试错误按策略退避。**成功的响应必定是 2xx**，杜绝「错误页被解析成空结果」 |
| 测试钩子 | `Underlying()` / `SetTransport()` / `SetRetryPolicy()` |

各源在 `Configure` 中自行声明限流间隔：VNDB 1600ms、DLsite 1200ms、RAWG 1200ms、Bangumi 1000ms、SteamGridDB 800ms，无需全局协调即可待在文档预算内。

辅助文件：`httpx.go`（错误模型与 HTTP 客户端）、`text.go`（`Unescape` / `StripHTML` / `CleanText` / `Truncate` / `PrepareDescription`，`MaxDescriptionRunes = 500`，按 rune 截断以免切断多字节字符）、`names.go`（RJ 码识别、`SearchName`、`SearchTerms` 变体生成与版本后缀清洗）、`lang.go`（`Lang` 与各源的线上语言码）、`cover.go`（封面下载）。

`CoverFetcher` 与流水线共用 HTTP 客户端（旧实现用无超时的 `http.Get`，卡住的 CDN 能永久占住队列 worker）：`Existing(gameDir, kind)` 忽略大小为 0 的文件与目录；`Fetch(ctx, gameDir, gameID, url, kind, force)` 把 URL 为空视为「源没有图」而非错误，`force=false` 时已有封面直接返回，否则下载并原子写入 `.gamemanager/covers/<kind><ext>`，再删掉其它扩展名的旧副本（先写后删，不留无封面窗口）；扩展名由**字节内容**决定（嗅探 magic bytes，回落 `Content-Type`，只信后者曾把 WebP 存成 `.jpg`）；单文件上限 12 MiB。

| key | 接口 | 认证 | 备注 |
|-----|------|------|------|
| `steam` | `store.steampowered.com/api/appdetails`（精确查询）、`/api/storesearch/`（名称搜索） | 不需要 | 已知 appID 时取竖版与横版封面；目录名含 RJ 码时直接放弃；`total>0` 但 `items` 为空按无结果处理（旧实现会越界 panic） |
| `vndb` | `POST api.vndb.org/kana/vn` | 不需要 | 请求字段必须存在于文档中；曾因请求不存在的 `lang_image` 导致每次 400、从未返回结果；语言感知标题改由 `titles{lang,title,latin,official,main}` 决定 |
| `bangumi` | `POST api.bgm.tv/v0/search/subjects`（`type=4` 游戏） | 不需要 | 旧实现请求了不存在的 `GET /v0/search/subject/{kw}`，每次 404；中文标题优先作为显示名，原文保留为 `titleNative` |
| `dlsite` | 作品页 HTML 解析（maniax 与 home 两个分区） | 不需要 | 从文件夹名取 RJ 码（锚定词边界、大小写不敏感），已有确认过的 ID 优先；页面须实际包含该商品码才被接受；带 `Cookie: adultchecked=1` 与浏览器风格 UA |
| `rawg` | `api.rawg.io/api/games` | **需要 API Key** | 无 Key 时直接返回 `KindAuth`，而不是被解析成「零结果」；横版封面来自 `background_image` |
| `steamgriddb` | `api.steamgriddb.com/api/v2/grids/steam/{appID}` | **需要 API Key** | 只提供封面（竖版 + 横版），按宽高比区分取向；无 appID 或 key 分别给出无结果与认证错误 |

### 3.7 TaskQueue (`internal/taskqueue/`)

工作线程池队列，当前承载元数据刮削。池大小默认为 **1**（未考虑限流的调用方因此保持串行语义），应用按 `config.scrapeConcurrency` 提升到 4（可设 1–16）。`Task{Type, GameID, Title, Status, Error, StartedAt, FinishedAt}`（`TaskScrape` 是唯一的 `TaskType`），`TaskStatus` 为 `pending` / `running` / `done` / `error`。

| 方法 | 说明 |
|------|------|
| `New(worker)` | 启动 1 个 worker 与一个 2 秒心跳 goroutine（心跳让任务列表的"运行中"状态保持新鲜）；worker 收到随 `Stop()` 取消的 context |
| `SetConcurrency(n)` | 扩大线程池（上限 16）。**不支持缩小**：多出的 worker 阻塞在唤醒通道上不消耗资源，而中途取消它会丢弃一个任务 |
| `Submit(task)` | 入队；**拒绝重复**：同一 `(GameID, Type)` 处于 pending 或 running 时返回 `false`（旧实现会排两次同样的刮削，UI 出现两行、API 流量翻倍）；`Title` 为空时以 GameID 填充。入队时按池大小唤醒全部 worker，使一批任务立刻铺开而不是从单个 worker 慢慢流出 |
| `ResetProgress()` | 清空已完成/失败计数与失败列表。批次边界**必须由调用方显式声明**：有线程池时队列会在两次入队之间瞬间排空，早先按"队列为空"推断新批次会导致计数在批次中途被清零（该缺陷由 `TestFailuresAreCapped` 暴露） |
| `Pause()` / `Resume()` / `Clear()` | 暂停不打断正在执行的任务（半个刮削会留下不完整记录）；`Clear` 只丢 pending |
| `Status()` | 返回 `Status{Pending, Running, Paused, Concurrency, CurrentTitle, CurrentGameID, RunningTitles, PendingTitles, Completed, Failed, Total, Failures}`；已完成任务在处理结束时从切片中移除，因此统计不随运行时长漂移。`RunningTitles` 列出全部在跑的任务，`Failures` 是最近 20 条失败及原因 |
| `SetObserver(onChange, onTaskDone)` / `Stop()` | 注册回调（任一可为 nil）/ 通过 `sync.Once` 幂等：取消 context、关闭 done 通道、等待**所有** worker 退出 |

`Concurrency`、`Completed`、`Failed`、`Total`、`RunningTitles`、`Failures` 都是为任务列表界面存在的：界面不再需要从零散事件里重建这些数字。

### 3.8 Watcher (`internal/watcher/`)

```go
type Events struct {
    NewGameDirs []string // 现在含启动器、且此前无记录的目录
    ChangedDirs []string // 已知目录内容变化
    RemovedDirs []string // 已不存在的目录
}
```

- 监听配置目录及其子目录，深度跟随 `MaxScanDepth`（`New(maxDepth, debounce, handler)` 两者都由配置驱动）
- 三类事件都上报：**新建的、含启动器的目录**；**已存在的目录中出现了启动器**（把游戏解压进已有目录很常见，只监听「创建目录」的旧实现对此无感，必须手动扫描）；**目录消失**
- 事件按 `debounce`（默认 150ms）窗口归并，一次 flush 只回调一批；flush 前复核目录当前状态：新目录必须真的含启动器，被删目录必须真的不存在
- 隐藏目录与隐藏路径被忽略；监听目录上限 `maxWatchedDirs = 2000`，超出部分留给下一次完整扫描，避免深层网络库耗尽文件句柄。`WatchedCount()` 报告已注册数，`Stop()` 幂等

### 3.9 Logger (`internal/logger/`)

基于标准库 `log/slog` 与自定义 `rotatingHandler` 的结构化日志。行格式刻意保持纯文本可读，因为主要消费者是把日志贴进 issue 的用户：

```
2026-05-30 14:30:01.234 [INFO] [scanner.go:42] scan started gameDirectories=[.\Games] maxDepth=3
```

| 项 | 说明 |
|----|------|
| `Options` | `{LibraryRoot, LocalDir, ToLibrary, Level, Console}` |
| 目录与文件 | `ToLibrary` 为真写 `<LibraryRoot>/logs`，否则写 `<LocalDir>/logs`（`DefaultLocalDir()` 取 `os.UserCacheDir()`，失败回落 `os.UserConfigDir()`）；文件为 `gamemanager_YYYY-MM-DD.log`，按天轮转 |
| 级别与控制台 | `debug` / `info` / `warn` / `error` → `[DEBG]` / `[INFO]` / `[WARN]` / `[ERRO]`；`Console` 同时镜像到 stderr（由 `isDevBuild()` 决定） |
| 源码位置 | `callerPC()` 沿栈一直走到第一个不在 `internal/logger/` 的帧；固定跳帧数会让每个类型化包装函数都指向 `logger.go` 自己 |
| 初始化前与失败 | 初始化前所有函数是空操作（测试无需设置）；日志文件建不起来时退化为仅控制台，写失败绝不向上传播成业务错误 |

`Init` / `Reconfigure`（别名）/ `Close` / `Dir()`（结果出现在 `AppInfo.logDir`）。类型化包装覆盖：应用启停、配置加载/保存/迁移、扫描开始结束与逐目录细节、启动器发现与过滤、主启动器选择、刮削逐源尝试/跳过/失败/成功、封面下载、游戏启动与信息保存、队列任务开始结束（含耗时）、监听器重启与目录增删。

### 3.10 MediaServe (`internal/mediaserve/`)

`http.Handler`，路由与状态码见 2.7。实现要点：`CoverResolver` 接口只有 `CoverPath(id string, landscape bool) string`，生产实现是 `library.Store`，测试用内存假实现；`RoutePrefix = "/covers/"`；路径解析容忍结尾 `/`，但拒绝空段与含 `/`、`\`、`..` 的 ID；`LogStartup()` 在挂载时记一条 debug 日志。

### 3.11 Platform (`internal/platform/`)

把全部平台相关操作集中到一处。`platform.go` 是与平台无关的部分：

| 名称 | 说明 |
|------|------|
| `OpenPath(path)` / `EditFile(path)` | 在 Shell 中打开目录/文件（Windows：`explorer`）/ 用文本编辑器打开（Windows：`notepad`） |
| `LaunchGame(path, workDir)` | 启动启动器，返回 `*exec.Cmd` 以便回收进程；工作目录为游戏目录 |
| `SteamPath()` / `SteamUsers()` | Steam 安装目录或 `""` / 已检测到的账号 |
| `IsLauncher(name)` / `LaunchKindFor(name)` / `LauncherExtensions()` / `ErrUnsupported` | 扩展名是否可启动 / 启动方式 / 全部可启动扩展名 / 当前平台不支持 |

| 扩展名 | `LaunchKind` | 实际命令 |
|--------|--------------|---------|
| `.exe`、`.com`、`.lnk`、`.url`、`.appref-ms` | `LaunchDirect` | 直接 `exec.Command(path)` 或交给系统关联 |
| `.bat`、`.cmd` | `LaunchCommandScript` | `cmd /c "<path>"`（加引号，NAS 路径常含空格） |
| `.ps1` | `LaunchPowerShellScript` | `powershell -NoProfile -ExecutionPolicy Bypass -File <path>` |

按扩展名而非「只认 exe」处理，是因为 NAS 库里的游戏经常在可执行文件旁边附带（甚至只有）一个包装脚本；`scriptInterpreter` 只构造 argv、不执行任何东西，因此可被单元测试覆盖。

Windows 实现：`SteamPath()` 先读注册表 `HKCU\Software\Valve\Steam` 的 `SteamPath`，再读 `HKLM\SOFTWARE\WOW6432Node\Valve\Steam` 的 `InstallPath`（`advapi32.dll` 的 `RegOpenKeyExW` / `RegQueryValueExW` / `RegCloseKey`），最后只在固定盘符 `C:`–`G:` 上探测 `\Program Files (x86)\Steam`、`\Steam`、`\Games\Steam`（不遍历映射的网络盘，否则挂载点多的客户端启动会被拖住）；`SteamUsers()` 遍历 `userdata/<数字ID>/` 并从 `config/localconfig.vdf` 提取 `PersonaName`，名字解析失败也照常返回账号；启动时设置 `SysProcAttr`，工作目录始终是游戏目录。非 Windows 实现只保证可编译：Shell 回落 `open`/`xdg-open`（`EditFile` 优先 `$EDITOR`），脚本启动器返回 `ErrUnsupported`，Steam 相关返回空值。

---

## 4. 后端 API 表面

导出方法即前端可调用面。

**扫描 / 刮削 / 队列**

| 方法 | 返回 | 说明 |
|------|------|------|
| `ScanGames()` / `ForceScanGames()` | `[]ScanResult` | 识别新游戏 / 全部重新识别并保留用户数据（此前「Force Scan」按钮实际调的是普通扫描） |
| `ScrapeGame(id)` | `*ScrapeReport` | 同步刮削单个游戏，`forceCover=true`（显式刮削应能替换坏封面） |
| `QueueScrapeAll(force)` | `int` | 全部入队；`force=false` 时跳过元数据已完整的游戏；返回接受的任务数 |
| `GetQueueInfo()` / `QueuePause()` / `QueueResume()` / `QueueClear()` | `taskqueue.Status` 等 | 队列快照与控制 |

**库查询与用户字段**

| 方法 | 返回 |
|------|------|
| `GetGameList()` / `GetGame(id)` | 全部记录（按最近游玩排序）/ 单条或 `nil` |
| `GetGamePathLabels()` | `map[string][]string`：游戏 ID → 所属目录标签（最长的目录前缀匹配胜出）。此前前端自己用 JS 重做路径归一化与前缀匹配 |
| `CommonPaths()` | `map[string]string`：`exeDir` 与每个配置目录的绝对路径，供 UI 显示与打开 |
| `ToggleGameStar` / `AddGameTag` / `RemoveGameTag` | 星标与用户标签 |
| `SetPreferredSource` / `SetPrimaryExecutable` | 源必须是该游戏已关联的平台 / 路径必须在该游戏的可执行文件列表中 |

**启动与 Shell 集成**

| 方法 | 说明 |
|------|------|
| `LaunchGame(id)` | 启动主启动器；校验文件存在；启动后写 `LastPlayedAt` 并在后台回收进程（Phase 4 将在此接入时长统计） |
| `OpenGameDirectory(id)` / `OpenGameMetadata(id)` | 在资源管理器中打开目录 / 用编辑器打开 `gameinfo.json` |
| `OpenDirectory(dir)` / `OpenBrowser(url)` | 打开配置目录（相对路径按库根解析）/ 只接受 `http://` 与 `https://` |

**设置与应用信息**

| 方法 | 返回 |
|------|------|
| `GetConfig()` / `SaveConfig(cfg)` | 配置读取 / 归一化 → 落盘 → 重建运行时状态（见 2.4） |
| `PickGameDirectory()` | 文件夹选择器结果，已转成可移植形式（此前无条件加 `.\` 前缀，库根之外的目录会得到非法路径） |
| `GetSteamUsers()` / `GetSteamPath()` / `SetSteamUser(id)` | `[]SteamUserInfo{id,name}` / Steam 目录或 `""` / 保存选择 |
| `GetMachineName()` / `GetAppInfo()` | 本机主机名 / `AppInfo{exeDir,machineId,machineName,version,buildTime,logDir,platform,coverBaseUrl}` |

**前端事件**（名称在 `app.go` 中定义为常量，避免后端与 UI 因拼写漂移）：

| 事件 | 载荷 |
|------|------|
| `queue:status` / `queue:done` | `taskqueue.Status` / `{gameId, title, error}` |
| `library:changed` | `{removed}` / `{updated, new}` / `{configSaved}` / `{launched}` 之一 |
| `scan:complete` | `{total}` |

`ScrapeReport` 为 `{gameId, title, source, sources[], error}`；失败语义由 `describeScrapeError` 转成用户能处理的说明 —— 「没有任何源匹配」、「缺少 API Key（去设置里配置）」、「源正在限流，稍后重试」、「源无法访问（检查网络或代理）」、「已取消」。

---

## 5. 前端设计

### 5.1 分层

| 位置 | 职责 |
|------|------|
| `src/api/client.ts` | **唯一**导入 `wailsjs/go/main/App` 的模块。导出 `api` 对象（按库 / 扫描刮削 / 队列 / 游戏操作 / 设置分组）与模型类型别名（`AppInfo`、`Game`、`ScanResult`、`QueueStatus`、`AppConfig`、`ScrapeReport`、`SteamUser`） |
| `src/api/events.ts` | `useWailsEvents(handlers)`：订阅后端事件并**在卸载时逐一退订**（`EventsOn` 的取消函数此前被丢弃，StrictMode 与热重载下重复注册，每个事件触发多次全量刷新）；处理器经 ref 读取，内联箭头函数不会导致重订阅，只有事件名集合变化才重订阅 |
| `src/lib/platform.ts` | 平台展示元数据（label / 颜色 / 图标 / URL 构造）的唯一来源，以及 `gamePlatforms`、`isUnmatched`、`platformId`、`primaryGameUrl` |
| `src/lib/cover.ts` / `src/lib/format.ts` | `coverUrl(id, kind, coverVersion)` 与 `coverKey` / `formatPlaytime`、`formatDate`、`errorMessage`（剥掉多余的 `Error:` 前缀，绝不产出 `undefined`） |
| `src/lib/filters.ts` | `NavKey` 联合类型、`navKeyId` / `parseNavKey`、`filterGames`、`navTitle`、`deriveCounts`、`needsScrape` |
| `src/lib/queue.ts` | `queueView(status)` 推导进度/状态/摘要，`hasQueueActivity`，`capList` 截断长列表并报告省略条数；纯函数，因此可在无 DOM 环境下单测 |
| `src/hooks/useScrape.ts` | 单游戏刮削状态与标记；批量刮削交给后端队列 |
| `src/App.tsx` | 全局状态、派生数据、事件订阅、组件装配 |

`NavKey` 是有判别字段的联合类型，取代了此前用 `slice(9)` / `slice(5)` / `slice(4)` 在八处反解魔术字符串的写法：

```ts
type NavKey =
  | { kind: "all" } | { kind: "starred" } | { kind: "unmatched" } | { kind: "settings" }
  | { kind: "platform"; id: string } | { kind: "genre"; id: string }
  | { kind: "usertag"; id: string } | { kind: "folder"; id: string };
```

`deriveCounts(games, ctx)` 一趟遍历产出侧边栏需要的全部计数（all / starred / unmatched / platforms / genres / userTags / folders），侧边栏因此不再每次渲染都重走整个库；`filterGames` 中「All Games」刻意排除未匹配游戏，它们有自己的视图。

### 5.2 状态与数据流

- `App.tsx` 持有 `selectedGameId`（**ID，不是对象**）。持有对象会让详情面板一直渲染拿到的那份快照，刮削后仍显示旧元数据，且对它的修改会就地改动父状态
- 派生数据全部 `useMemo`（`filterContext`、`visibleGames`、`counts`、`selectedGame`、`contextGame`）；`loadGames` 用递增序号保证「最新请求胜出」，避免监听器、扫描、队列连续触发刷新时旧响应覆盖新列表
- 事件订阅集中在 `useWailsEvents({...})`：`queue:status` 更新进度与当前游戏，`queue:done` 清空当前游戏并刷新库，`library:changed` 与 `scan:complete` 刷新库；`busy = isScanning || queueOutstanding > 0`，顶部按钮在忙时统一禁用

| 组件 | 关键点 |
|------|--------|
| `Sidebar` | 完全由 `deriveCounts` 的结果渲染（接 `NavCounts`，不再自己遍历游戏），顺序为 All Games / Starred / Unmatched / Platforms / Genres / Folders / My Tags，底部是本机名、版本、队列面板与设置入口；`selected` 由 `navKeyId(nav)` 生成 |
| `GameCard` | 一个 `status: "idle" \| "scraping" \| "ok" \| "error"` 取代旧的三个布尔（四种状态无法再互相矛盾）；封面失败按 `coverKey` 记录，重刮后会用新 URL 重试而不是永远停在占位图；启动失败显示在卡片上；组件 `memo` 化 |
| `GameDetail` | 居中弹窗；`runAction` 统一处理「调用 `api.*` → `await onUpdated()` → 显示内联反馈」，不再就地修改父状态；Esc 关闭、挂载即获得焦点 |
| `ContextMenu` | `useLayoutEffect` 量测后摆放，永不半出屏，窗口 resize 时重算；子菜单悬停 200ms 延迟关闭；动作失败在菜单内显示（旧实现点击即关闭，错误丢失） |
| `Settings` | 卡片式布局，7 张卡片：Game Directories、Scanning（扫描深度 + 并行刮削数）、Language、Metadata Sources、File Watcher、Logging、About |
| `QueuePanel` | 任务列表：确定进度条与 `已完成/总数`、全部在跑任务（并发时不止一个）、等待队列（超出 8 条折叠为「还有 N 个」）、失败列表（最多 5 条，含失败原因与单条重试）、暂停/继续/清空等待/忽略结果。批次结束后结果保留可见，直到用户忽略或开始下一批 |

### 5.3 测试与工具链

- 测试位于 `src/lib/lib.test.ts`，只覆盖纯逻辑（封面 URL、格式化、平台元数据、NavKey 编解码、筛选、计数、`needsScrape`）
- 运行器是 Node 内置的 `node --test "src/**/*.test.ts"`（`npm run test`），直接执行 `.ts`。**没有 Vitest / Testing Library / jsdom 依赖**：它们此前的唯一用途是渲染组件，而组件测试在无 DOM 的 CI 里性价比很低
- 类型检查 `tsc -b --noEmit`；Prettier 负责格式（`format:check` 在 CI 中执行）；ESLint flat config 启用 React hooks 与 refresh 规则，忽略 `wailsjs`、`dist`、`node_modules`、`coverage`
- `no-explicit-any`、`no-unused-vars` 与 `no-empty`（含空 catch）均为 **error**：生成的 Wails 模型已经完整声明了 UI 读取的字段，类型断言必然是错误；空 `catch` 会掩盖失败。重构前这些规则无法开启，因为代码里存在大量 `(game as any).platforms` 与 20 处空 `catch {}`

---

## 6. 数据格式

### 6.1 `config.json`

```json
{
  "schemaVersion": 2,
  "machineId": "machine-kaniol-pc",
  "gameDirectories": [".\\Games", ".\\..\\shared\\Games"],
  "gameDirectoryLabels": { ".\\Games": ["Steam"] },
  "maxScanDepth": 3,
  "language": "zh-CN",
  "steamUserId": "1037536352",
  "watcherEnabled": true,
  "watcherDebounceMs": 150,
  "scrapeConcurrency": 4,
  "logLevel": "info",
  "logToLibrary": false,
  "metadataSources": [
    { "key": "steam", "name": "Steam", "enabled": true },
    { "key": "vndb", "name": "VNDB (Visual Novel Database)", "enabled": true },
    { "key": "bangumi", "name": "Bangumi (bgm.tv)", "enabled": true },
    { "key": "dlsite", "name": "DLsite", "enabled": true },
    { "key": "rawg", "name": "RAWG.io", "enabled": true },
    { "key": "steamgriddb", "name": "SteamGridDB (Cover Art)", "enabled": false, "settings": { "apiKey": "..." } }
  ]
}
```

字段语义与默认值见 3.1。目录项写成 `.\Games` 或 `.\..\shared\Games`，同一份文件在任何盘符下都成立；`settings` 只在配置过 API Key 时出现（`omitempty`）。

### 6.2 `.gamemanager/gameinfo.json`

```json
{
  "schemaVersion": 2,
  "id": "steam_570",
  "title": "Dota 2",
  "titleNative": "Dota 2",
  "type": "game",
  "platforms": [{ "platform": "steam", "id": "570", "name": "Dota 2" }],
  "aliases": ["Dota2"],
  "preferredSource": "steam",
  "executables": [{ "path": "game.exe", "name": "game", "primary": true }],
  "metadata": {
    "coverUrl": "cover",
    "coverLandscape": "cover_landscape",
    "releaseDate": "2013-07-09",
    "developer": "Valve",
    "publisher": "Valve",
    "tags": ["Action", "Free to Play"],
    "description": "...",
    "links": { "steam": "https://store.steampowered.com/app/570/" }
  },
  "scannedAt": "2026-09-29T12:00:00Z",
  "totalPlaytime": 0,
  "lastPlayedAt": "2026-09-29T13:20:00Z",
  "starred": true,
  "tags": ["backlog"],
  "coverVersion": 1759150800
}
```

- `metadata.coverUrl` / `coverLandscape` 存的是**文件名**（`cover`、`cover_landscape`），真实扩展名由磁盘决定，对外 URL 由 2.7 的 HTTP 服务提供
- `metadata.links` 不含 `platformId`（合并时被剥离），平台标识统一存在 `platforms[].id`
- `gameDir` 不出现在文件中（`json:"-"`）：它是机器相关的挂载点，写进共享文件会随客户端互相覆盖
- `savePaths` 是 Phase 5 的预留字段，当前不写入；除 `id`、`title`、`type`、`executables`、`scannedAt`、`totalPlaytime` 外其余字段都带 `omitempty`

### 6.3 `meta/<source>.json` 与兼容规则

每条匹配结果都会写一份该源返回的元数据快照，用于事后核对「这个源当时给了什么」。**只有写入路径**：没有任何功能把它读回来覆盖记录，`LoadMeta` 目前仅被单元测试使用。

| 规则 | 内容 |
|------|------|
| 只增不改 | JSON 字段只允许新增；无法用「加字段」表达的格式变化才允许提升 `schemaVersion` |
| 写入即当前版本 | `GameInfo.Save` 与 `Config.Save` 总是落当前 `schemaVersion`，被迁移过的记录不会声称自己是旧版本 |
| 读入即迁移 | 配置缺少 `metadataSources` 时读旧的 `vndbEnabled` / `dlsiteEnabled` / `bangumiEnabled` / `steamgriddbEnabled`；元数据缺少 `platforms` 时读扁平的 `platform` / `platformId`；元数据仍在 `.gameinfo.json` 时迁移到 `.gamemanager/` 并搬走根目录封面 |
| 数据源列表自愈 | `SyncSources` 保留用户已有的顺序、开关与设置，把新增源追加到末尾；已下线的源（如 `igdb`）不会重新加入，但配置里已存在的条目会保留（见 9） |
| 目录标签自净 | `AutoLabelPaths` 删除对应目录已不再配置的标签，侧边栏不会累积死区块 |
| 机器相关字段绝不落盘 | `gameDir` 与机器名都在运行期确定 |

---

## 7. 设计决策记录

### 7.1 为什么选择便携架构而非 Server+Agent？

| Server + Agent | Portable NAS |
|----------------|--------------|
| 需要部署服务端并运维 | 单个 exe 放进共享目录即可运行 |
| 每端都要装 Agent | 各端运行同一个 exe |
| 数据库集中管理 | JSON 文件，无依赖，任何人可读可改 |
| 适合多用户并发 | 适合个人 / 家庭多端 |

个人使用场景下，便携架构在部署复杂度、数据可移植性与离线可用性上都占优；代价是没有服务端的一致性保证，因此才需要 7.8 的原子写入与 6.3 的迁移约束。

### 7.2 为什么机器名在运行期获取而不是存进配置？

`config.json` 位于共享目录、被所有客户端读写。若把机器名写进去，**最后保存的那个客户端会替其他所有客户端改名**。改为启动时 `os.Hostname()`（`config.Hostname()`），每台机器各自独立识别，配置里只留下一个不含机器信息的 `machineId`。

### 7.3 为什么用 `internal/` 而不是 `pkg/`？

`internal/` 是 Go 的约定，语义是「仅本模块可导入」。本项目不打算对外提供库接口，用 `internal/` 表达得更准确；`pkg/` 会暗示存在外部使用者。

### 7.4 为什么把 Wails 生成的绑定提交到 Git？

`frontend/wailsjs/` 由 Wails CLI 生成。提交后其他开发者与 CI 构建前端时不必先安装 Wails CLI，diff 中也能直接看出后端 API 的变化（一次绑定改动就是一次 API 变更）；ESLint 与 golangci-lint 都把它排除在检查之外，因为它不是手写代码。

### 7.5 为什么目录以可移植形式存储？

NAS 在每台客户端可能挂到不同盘符（`Z:`、`Y:` …）。存 `.\Games` 而不是 `Z:\Games`，同一份 `config.json` 到处可用。`ToPortable` 愿意保留最多 3 层 `..`，覆盖「程序在 `apps\GameLibrary`、游戏在 `Games`」这类常见 NAS 布局；再远就退回绝对路径 —— 假装它仍可移植反而是错的。

### 7.6 为什么日志默认写本机目录？

放在共享目录意味着每个客户端的日志互相交错，而且每条日志都要一次网络往返（调试期尤其昂贵）。默认写 `%LocalAppData%`（`os.UserCacheDir()`）下的 `GameLibrary\logs`，需要集中收集时再用 `logToLibrary` 打开。

### 7.7 为什么所有刮削器共用一个 HTTP 客户端？

此前有七个连接池（每源一个加 `http.DefaultClient`），超时、UA、重试与代理设置各不相同。合并后只有一处配置超时、一处实现退避重试、一处做按主机限流、一处能注入测试 transport，也让封面下载自动继承同一套策略（封面此前用无超时的 `http.Get`）。

### 7.8 为什么所有 JSON 都原子写入？

元数据文件位于 SMB 共享且被多端读写，「半个 JSON 文件」比「没有文件」危险得多：它可能静默丢掉一个游戏的元数据，而解析失败会被当成「没有记录」。先写同目录临时文件、`fsync`、再 rename，能保证任何时刻读者看到的都是完整的旧版本或完整的新版本。

### 7.9 为什么区分「无结果」与「接口失败」？

两者曾经都是普通 `error`：源正常答复但没有匹配、与源返回 500，在调用方看来完全一样。结果是「源没有结果」这条分支永远不可达，瞬时故障也永远不会被重试，用户看到的错误信息无法行动。现在 `ErrNoResult` 是哨兵值，其它失败是带 `ErrorKind` 的 `*APIError`，UI 能分别说出「没有源匹配」与「RAWG 需要 API Key」。

### 7.10 为什么每个源拿到的是 `Query` 而不是一个 appID？

旧签名是 `Search(gameDir, appID)`，其中 `appID` 实际是「游戏的第一个平台 ID」。六个源里有四个忽略它、一个忽略目录，而 DLsite 的 RJ 码会被当作 Steam appID 送出去。改成 `Query{GameDir, Name, PlatformIDs}` 后，每个源只读取真正适用于自己的字段，名称与语言变体也在同一个结构里传递。

### 7.11 为什么明确收敛到 Windows？

启动器语义（`cmd /c`、PowerShell 执行策略、`.lnk`）、Shell 集成（Explorer、Notepad）、Steam 探测（注册表、`userdata`）都是 Windows 特有的。假装跨平台只会得到一份「到处都是 if 分支、处处都没测过」的实现。现在的做法是：只在 Windows 上成立的行为写进 `platform_windows.go`，非 Windows 只保证编译与测试，发布只出 Windows 产物。

---

## 8. 项目约定

### 8.1 版本与发布

- 版本号是 semver `x.y.z`：`x` 用于不兼容的架构变更，`y` 用于成阶段的功能，`z` 用于功能补充、缺陷修复与工具链调整
- **除纯文档提交外，每次提交都要步进最小版本号并打 tag**；阶段性的 alpha 版本以 GitHub pre-release 形式发布（`release.yml` 固定 `prerelease: true`）
- 同一个版本号出现在四处，必须一致：`app.go` 的 `var version`、`scraper.ClientVersion`（HTTP User-Agent）、`frontend/package.json` 的 `version`、README 徽章
- 版本与构建时间通过 `-ldflags` 注入；未注入时 `buildTime = "development"`，这会打开控制台日志镜像（也可用 `GAMELIBRARY_DEV=1` 强制）：

```bash
go build -ldflags "-X main.version=0.8.0 -X main.buildTime=2026-09-29T10:00:00Z"
```

### 8.2 代码规范

| 约定 | 说明 |
|------|------|
| `gofmt` | CI 中强制：`gofmt -l $(git ls-files '*.go')` 非空即失败；golangci-lint 另启用 goimports（`local-prefixes: GameLibrary`） |
| 行尾符 | 仓库内一律 LF，并由 `.gitattributes` 强制以 LF 签出（`*.cmd` / `*.bat` 用 CRLF）。Windows 上 git 的系统配置默认 `core.autocrlf=true`，签出会把文本文件全变成 CRLF，而 `gofmt -l` 会把整棵源码树判为未格式化；Linux 作业签出的是 LF 所以永远绿，这个差异只在 Windows 作业与发布时才暴露 |
| Go 命令的包模式 | **必须显式写成 `./internal/... .`**：`frontend/node_modules` 恰好带有一个无关的 Go 包，`./...` 会把它一并扫进来；CI、README 与 golangci 排除列表都遵循这条 |
| 包划分与测试 | 按职责放入 `internal/` 子包，`package main` 按 API 面拆分；测试与源文件同目录（`*_test.go` / `*.test.ts`），新增测试一律用 `t.TempDir()` 而不向仓库写文件，可复用的模拟游戏目录放在 `testdata/` |
| 生成代码与产物 | `frontend/wailsjs/` 提交但不参与 lint；`build/`、`frontend/dist/`、`config.json`、`Games/`、`logs/`、覆盖率文件、`.gocache/`、`.npm-cache/`、`.tmp-*/` 均被忽略 |
| 文档更新 | 新功能与配置模型变更更新 `CHANGELOG.md`（必要时含 `DESIGN.md` 对应章节）；Bug 修复更新 `CHANGELOG.md`；发布更新 `DESIGN.md` 路线图 |
| 开发方式 | 本项目在 **DeepSeek Harness** 辅助下开发。改动以「能编译、能测试、能运行」为准：新增或修改的行为要有对应测试，交付前跑一遍 `gofmt` / `go vet` / `go test` 与前端 `tsc` / `lint` / `test`，涉及启动路径的改动要真正启动一次产物验证，而不只是构建通过 |

`CHANGELOG.md` 采用中英双行：中文为主描述，英文斜体为辅，按 `Added` / `Changed` / `Fixed` / `Removed` 分类：

```markdown
- 新增某某功能（`按钮名`）
  *Added some feature (`Button Name`)*
```

### 8.3 静态检查与 CI

**Go（`.golangci.yml`，v2 配置）**：启用 `errcheck`、`govet`、`ineffassign`、`staticcheck`、`unused`、`bodyclose`、`copyloopvar`、`errorlint`、`noctx`、`unconvert`、`wastedassign`；`errcheck` 只豁免 `(io.Closer).Close` 与 `(*os.File).Close`；排除 `frontend/`、`build/`、`.gocache/`；测试文件豁免 `errcheck` / `bodyclose` / `noctx`。

**前端**：ESLint flat config（`@eslint/js` + `typescript-eslint` recommended + `react-hooks` + `react-refresh`）、Prettier、`tsconfig` project references（`tsconfig.app.json` 覆盖 `src`，`tsconfig.node.json` 覆盖 `vite.config.ts`；`strict`、`noUnusedLocals/Parameters`、`@/*` 路径别名、`allowImportingTsExtensions`）。

**CI（`.github/workflows/ci.yml`）** 在非 `v*` tag 的推送、PR 与手动触发时运行：

| Job | 内容 |
|-----|------|
| Backend (Go) | `ubuntu-22.04` + `libgtk-3-dev`/`libwebkit2gtk-4.0-dev`；gofmt 校验 → `go vet ./internal/... .` → `go build ./internal/... .` → `go test -race -count=1 ./internal/... .` → 覆盖率并上传 artifact |
| Frontend (React) | Node 版本取自 `frontend/.nvmrc`；`npm ci` → `npm run lint` → `npm run format:check` → `npm run typecheck` → `npm run test` → `npm run build` |

**发布（`.github/workflows/release.yml`）** 仅在推送 `v*` tag 时运行：`windows-latest` → 跑一遍 Go 测试 → `wails build -platform windows/amd64 -ldflags="-X main.version=$VERSION -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -clean` → 产出 `GameLibrary-windows-amd64.exe`、同名 `.sha256`，并附带 `scripts/run-with-console.cmd`、`CHANGELOG.md` 与 `LICENSE` → 用 `.github/RELEASE_TEMPLATE.md` 作为正文创建 release。**只有含 `-alpha` 的 tag 标记为 pre-release**，其余为正式 release（`v0.8.0` 即正式）。

> 发布说明里承诺的 `run-with-console.cmd` 必须由工作流一并上传：它原先只存在于本机的发布目录，而该目录被 gitignore，导致说明提到的文件在 release 里并不存在。因此脚本存放在受版本控制的 `scripts/` 下。

---

## 9. 已移除或从未存在的能力

旧文档描述过的以下内容与当前代码不符，不应再被视为设计的一部分：

| 旧描述 | 现状 |
|--------|------|
| `Pipeline.Scrape`：按优先级选第一个匹配的源并返回 | 已删除，只有 `ScrapeAll`（收集所有匹配），首选源由 `scrapeOne` 裁决 |
| `.gamemanager/meta/` 快照可被读回并用于恢复元数据 | 只有写入，没有读回功能，`LoadMeta` 仅被测试使用 |
| `igdb` 是一个可配置的数据源 | 已从 `DefaultSources()` 移除，也没有注册 provider，永远不会运行；`SyncSources` 会**删除**配置中遗留的 `igdb` 条目（只保留有实现的数据源，否则设置页会出现一行既无说明又无法配置的死条目） |
| 封面经 `GetGameCover` / `GetGameCoverLandscape` 以 base64 data URI 通过 IPC 传输 | 已删除，改由 asset server 提供 `GET /covers/{id}/{variant}` |
| `UpdateGameInfo`、`ScrapeAllGames`、`GetGameCover*` 系列 API | 已删除 |
| 旧文档中的 `.gamemanager/thumbnails/`、`meta/steam.json` 布局 | 实际是 `.gamemanager/covers/` 与 `.gamemanager/meta/` |
| 批量刮削在浏览器内以 3 并发 worker 执行 | 批量刮削统一走后端队列，前端只负责发起与监听 |
| 详情面板从右侧滑出 | 现在是居中弹窗 |
| 存在 `local` 占位平台 | 无平台关联的游戏归入 Unmatched 视图 |
| 非 Windows 发布产物（Linux / macOS） | 发布只构建 Windows |
| 前端使用 Vitest / Testing Library / jsdom | 已移除，改用 Node 内置测试运行器 |
| `steam_windows.go` / `steam_other.go` 两个根目录文件 | 已并入 `internal/platform/` |
| `metadata.coverUrl` 存外链 | 存本地文件名，外链只存在于下载过程中的临时值里 |

---

## 10. 阶段进度

### Phase 1 — 骨架 ✅ / Phase 2 — 刮削、标签、队列、监听 ✅

当前代码中属于这两个阶段的核心资产：扫描器与 ACF / `steam_appid.txt` 识别、六个数据源的刮削流水线、封面本地化与 HTTP 服务、四级筛选（文件夹 / 平台 / 分类 / 用户标签）、任务队列与文件监听、结构化日志。逐版本明细见 `CHANGELOG.md`。

### Phase 2.5 — 重构与优化 ✅ (0.8.0)

- [x] `internal/library` 并发安全缓存（读取返回深拷贝）与 `internal/fsutil` 原子写入、`ToPortable` 路径处理、BOM 容错
- [x] `runtimeState` 重建：设置保存后立即生效，不必重启
- [x] 共享 HTTP 客户端（超时、重试退避、按主机限流、context 取消）与类型化刮削错误
- [x] **并行刮削**：队列工作线程池（默认 4）+ 同一游戏各数据源并发，结果仍按优先级顺序装配
- [x] **任务列表界面**：真实进度、全部在跑任务、等待列表折叠、失败原因与单条重试
- [x] 封面改由 HTTP 服务提供并由版本参数驱动缓存失效
- [x] **启动失败诊断**：日志先于窗口创建初始化并逐级回退目录；上次未创建窗口会在下次启动弹窗说明
- [x] 修正 Bangumi 与 VNDB 的接口契约（此前两者从未返回过结果）
- [x] 强制重扫保留用户数据；`ForceScanGames` 真正可用；游戏 ID 基于相对库根路径，跨库不再冲突
- [x] 扫描器不再因目录含 exe 而停止递归；「游戏目录」定义统一
- [x] 前端统一走 `api/client.ts`，事件订阅在卸载时退订
- [x] Windows 收敛；CI（gofmt / vet / race / 覆盖率）与前端工具链基线；Go 覆盖率 27.6% → 75.1%

### Phase 3 — 启动与锁 📋 (0.9.0)

- [ ] 锁文件机制：防止多端同时启动同一游戏
- [ ] 心跳检测与死锁清理（客户端崩溃后锁能自动失效）
- [ ] 运行状态采集与「正在运行」UI，并随 `library:changed` 事件推送到前端

### Phase 4 — 时长统计 📋 (0.10.0)

- [ ] 进程存活监控（`LaunchGame` 已返回 `*exec.Cmd` 并在后台 `Wait`，接入点已就绪）
- [ ] 会话记录文件与 `TotalPlaytime` 累加
- [ ] 多端时长聚合（NAS 上按机器分别记录，读取时求和）
- [ ] 时长展示（`formatPlaytime` 已在前端就位）

### Phase 5 — 云存档 📋 (0.11.0)

- [ ] 存档路径配置（手动 + PCGamingWiki 数据），落入预留的 `savePaths` 字段
- [ ] 符号链接方案（`mklink /J`）与文件复制同步方案的取舍
- [ ] PreSync / PostSync 流程；备份与版本管理

### 1.0.0 — 正式发布 📋

- [ ] Phase 3–5 功能冻结；全功能测试与文档收敛
- [ ] 去掉 alpha 标记，发布非 pre-release 产物
