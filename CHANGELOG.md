# Changelog

## v0.8.0 (2026-09-29)

重构版本：修复了大量已确认的功能性缺陷，重建了验证基线，将产品明确收敛为 Windows 目标，并让刮削并行执行。

*Refactoring release: fixes a set of confirmed functional defects, establishes a verification baseline, pins the product to a Windows target, and makes scraping run in parallel.*

### Added

- **并行刮削**：队列使用可配置的工作线程池（默认 4，可设 1–16），同一游戏的多个数据源也并发查询。单一数据源仍受各自的按主机限流约束，因此缩短的是等待时间，而非提高任何单一服务的请求频率
  *Parallel scraping: the queue runs a configurable worker pool (default 4, 1–16) and a game's providers are queried concurrently. Each host keeps its own rate limit, so this overlaps waiting rather than raising the request rate any single service sees*
- **任务列表界面重做**：真实进度条与 `已完成/总数`、同时运行的多个任务、等待队列（超出部分折叠）、失败列表（含失败原因与一键重试）、暂停/继续/清空/忽略结果
  *Rebuilt task list: real progress bar with completed/total, every concurrently running task, a folded waiting list, a failure list with reasons and per-game retry, and pause/resume/clear/dismiss controls*
- 设置页新增「并行刮削」数量调节
  *New "Parallel Scrapes" control in Settings*
- 启动失败诊断：日志在创建窗口之前初始化，并在写入失败时逐级回退目录；若上次启动未能创建窗口，下次启动会弹窗说明原因（含日志路径与常见排查方向）
  *Start-up diagnostics: logging is initialised before the window is created and falls back through several directories; a launch that never produced a window is explained by a dialog on the next attempt, with the log path and the usual causes*
- 发布包附带 `run-with-console.cmd`，用于直接查看启动错误
  *The release package ships `run-with-console.cmd` to surface start-up errors directly*
- **BOM 容错**：`config.json` 与 `gameinfo.json` 若带 UTF-8/UTF-16 BOM 仍能正确解析
  *BOM tolerance: `config.json` and `gameinfo.json` parse correctly even with a UTF-8/UTF-16 byte order mark*
- CI 工作流：`gofmt` 校验、`go vet`、`go test -race`、覆盖率报告，以及前端 lint / 格式化校验 / 类型检查 / 单元测试 / 构建
  *CI workflow: gofmt check, go vet, `go test -race`, coverage artefact, plus frontend lint, format check, typecheck, unit tests and build*
- golangci-lint 与 ESLint + Prettier 配置
  *golangci-lint and ESLint + Prettier configuration*
- 封面 HTTP 服务（`internal/mediaserve`）、并发安全游戏缓存（`internal/library`）、原子文件工具（`internal/fsutil`）、平台抽象层（`internal/platform`）
  *Cover HTTP server (`internal/mediaserve`), a concurrency-safe game cache (`internal/library`), atomic file utilities (`internal/fsutil`) and a platform abstraction (`internal/platform`)*
- 大量单元测试：刮削器使用 `httptest` 完全离线，覆盖 Bangumi/VNDB 端点契约与字段合并等回归场景；前端纯逻辑测试改用 Node 内置运行器（无额外依赖）
  *Substantial unit tests: scrapers are fully offline via `httptest` and cover the Bangumi/VNDB endpoint contracts and the result-merge regression; frontend logic tests use Node's built-in runner with no extra dependency*

### Fixed

- **Bangumi 刮削器完全失效**：请求了不存在的 `GET /v0/search/subject/{kw}` 端点，每次调用都返回 404。改用官方 v0 规范的 `POST /v0/search/subjects`
  *Bangumi scraper never worked: it called a non-existent endpoint and got HTTP 404 for every query. Now uses the documented `POST /v0/search/subjects`*
- **VNDB 刮削器完全失效**：请求字段包含不存在的 `lang_image`，VNDB 对未知字段返回 400。移除该字段并改用 `titles` 做语言感知标题选择
  *VNDB scraper never worked: it requested the non-existent field `lang_image` and VNDB rejects unknown fields with HTTP 400. The field is gone and language-aware titles now come from `titles`*
- **保存设置后不生效**：扫描器与刮削流水线持有启动时的配置指针，修改游戏目录、扫描深度、语言或 API Key 都必须重启才生效
  *Saved settings were ignored: the scanner and scrape pipeline held the startup config pointer, so changes needed a restart*
- **游戏缓存数据竞争**：`a.games` 映射被 IPC、队列、监听器与后台扫描并发读写且无锁保护
  *Data race on the game cache: the map was read and written by IPC, the queue, the watcher and background scans with no lock*
- **强制重扫丢失用户数据**：Force Scan 会重建记录并丢弃星标、标签、已刮削元数据与游玩时长
  *Forced rescan discarded user data: stars, tags, scraped metadata and playtime were rebuilt from scratch*
- **「Force Scan」按钮实际执行普通扫描**：`ForceScanGames` 从未被调用
  *The "Force Scan" button performed a normal scan; `ForceScanGames` was never called*
- **空字段结果擦除游戏数据**：SteamGridDB 只返回封面，作为首选源时会清空标题、描述与开发商。改为逐字段合并
  *Empty results erased game data: SteamGridDB returns covers only, and being the preferred source blanked the title, description and developer. Results are now merged field by field*
- **封面下载在 Windows 上留下损坏文件**：`os.Remove` 在文件句柄关闭前执行，失败被忽略；`http.Get` 无超时
  *Cover download corrupted files on Windows: cleanup ran before the handle was closed and its error was dropped; `http.Get` had no timeout*
- **封面格式判断只信 Content-Type**：WebP/AVIF 被存为 `.jpg`
  *Cover format came only from Content-Type, so WebP/AVIF payloads were stored as `.jpg`*
- **游戏 ID 冲突**：仅用文件夹名作 ID，不同库中的同名文件夹互相覆盖
  *Game ID collision: the folder name alone was the ID, so same-named folders in different libraries overwrote each other*
- **前端 IPC 放大 O(N²)**：刮削 worker 循环内每次都全量拉取游戏列表；封面通过 base64 data URI 逐个经桥传输
  *O(N²) frontend IPC: the scrape worker re-fetched the whole library inside its loop, and covers crossed the bridge as base64 data URIs*
- **详情面板显示过期数据并就地修改父状态**
  *The detail panel showed stale metadata and mutated the parent's state object in place*
- **事件订阅从不退订**：StrictMode 与热重载下重复订阅，每个事件触发多次全量刷新
  *Event subscriptions were never removed, so StrictMode and hot reload registered duplicates and each event triggered several full reloads*
- **扫描器遇到含可执行文件的目录即停止递归**：库根目录里的一个安装程序会掩盖全部游戏
  *The scanner stopped descending as soon as a directory contained a launcher, so one stray installer at the library root hid every game*
- **两种「游戏目录」定义不一致**：扫描器要求启动器，缓存加载同时接受 `.gamemanager`
  *Two different definitions of "game directory": the scanner required a launcher while cache loading also accepted `.gamemanager`*
- **`PrimaryPlatform()` 与 `PrimaryPlatformID()` 结论矛盾**：前者用首选源，后者取 `Platforms[0]`
  *`PrimaryPlatform()` and `PrimaryPlatformID()` disagreed: one used the preferred source, the other took `Platforms[0]`*
- **日志源码位置全部错误**：固定调用栈深度使所有包装函数都指向 `logger.go`
  *Log source locations were wrong: a fixed stack depth made every wrapper report `logger.go`*
- **日志级别形同虚设**：直接调用 handler 绕过了 `Enabled`，低级别记录照样写入
  *The log level was not enforced: calling the handler directly bypassed `Enabled`, so every record was written*
- **`GetAppInfo().buildTime` 实际返回 `time.Now()`**
  *`GetAppInfo().buildTime` actually returned `time.Now()`*
- **`Stop()` 不可重入**：队列与监听器二次停止会 panic
  *`Stop()` was not idempotent: a second stop panicked for the queue and the watcher*
- **队列重复提交同一游戏**、`Status()` 统计随运行时长漂移
  *The queue accepted duplicate submissions and its status drifted with uptime*
- **配置中的失效目录标签永不清理**，且标签键与目录项写法不一致时会变为不可达；配置里保留已不存在的数据源
  *Stale directory labels accumulated in `config.json` forever, and a label whose key was spelled differently from its directory entry became unreachable; unsupported metadata sources were kept*
- **前端 20 处空 `catch {}`**：星标、标签、启动失败与成功无法区分
  *20 empty `catch {}` blocks made failures indistinguishable from success*
- **侧边栏 Folders 与设置不一致**：保存设置后前端配置副本不刷新
  *The sidebar's Folders section stayed stale after saving settings*
- **带 BOM 的 `config.json` 会导致整个配置被静默丢弃**（解析失败后回退默认值，只在日志里留一行）
  *A `config.json` with a byte order mark silently discarded the whole configuration (parse failure, fall back to defaults, one log line)*
- **启动失败完全没有反馈**：WebView2 控制器创建失败发生在 `OnStartup` 之前，进程直接退出，双击 exe 表现为「什么都没发生」
  *A start-up failure had no feedback at all: the WebView2 controller is created before `OnStartup`, the process exits, and double-clicking the executable simply did nothing*
- **并行刮削引入的数据竞态**：worker 无锁改写任务字段而 `Status()` 持锁读取；`SetConcurrency` 可能在 `Stop` 等待期间误用 WaitGroup
  *Data races introduced by parallel scraping: a worker rewrote task fields while `Status()` read them under the lock, and `SetConcurrency` could misuse the WaitGroup while `Stop` was waiting*
- **CI 无法在全新检出上编译**：`//go:embed all:frontend/dist` 在缺失该目录时直接失败，Go 检查步骤须先创建占位目录
  *CI could not compile a fresh checkout: `//go:embed all:frontend/dist` fails outright when the directory is absent, so the Go steps now create a placeholder first*

### Changed

- **Windows 收敛**：发布仅构建 Windows 产物；扫描与启动识别 `.exe/.com/.bat/.cmd/.ps1/.lnk`，外置脚本通过 `cmd` 或 PowerShell 启动；路径处理同时接受 `/` 与 `\`
  *Windows target: releases are Windows-only; the scanner and launcher recognise `.exe/.com/.bat/.cmd/.ps1/.lnk`, wrapper scripts run through `cmd` or PowerShell, and paths accept both separators*
- 日志默认写入本机缓存目录而非 NAS，避免多端争用；按天归档，级别可配置
  *Logs now default to the per-machine cache directory instead of the NAS, rotate daily, and honour a configurable level*
- 封面改由 asset server 以 HTTP 提供（`/covers/{id}/{variant}`），并使用版本参数做缓存失效
  *Cover art is served over HTTP by the asset server (`/covers/{id}/{variant}`) with version-based cache invalidation*
- 刮削器共用单一 HTTP 客户端：统一超时、重试与退避、按主机限流、`context` 取消
  *Scrapers share one HTTP client with a common timeout, retries with backoff, per-host rate limiting and context cancellation*
- 刮削错误区分「无结果」与「接口失败」，并区分认证/限流/网络错误
  *Scrape errors now distinguish "no result" from API failure, and classify auth, rate limit and network problems*
- 全部 JSON 落盘改为原子写入（临时文件 + rename）
  *All JSON persistence is atomic (temp file + rename)*
- `gameDir` 不再写入元数据（机器相关的挂载点不再泄漏到共享文件），并加入 `schemaVersion`
  *`gameDir` is no longer persisted (machine-specific mount points no longer leak into shared files) and a `schemaVersion` was added*
- 批量刮削统一走队列，前端不再自行并发
  *Batch scraping goes through the queue; the frontend no longer runs its own worker pool*
- 配置中移除 `igdb`（无对应刮削器），并会清理文件中遗留的该条目
  *Removed `igdb` from the config (no scraper is registered for it), and a leftover entry is now pruned from existing files*
- 文档署名更新为 **DeepSeek Harness**
  *Documentation attribution updated to DeepSeek Harness*

### Removed

- 前端 Vitest/Testing Library/jsdom 依赖（改用 Node 内置测试运行器）
  *Frontend Vitest/Testing Library/jsdom dependencies (replaced by Node's built-in test runner)*
- 已废弃的 Linux/macOS 发布产物
  *Retired Linux/macOS release artefacts*
- 死代码：`Pipeline.Scrape`、`.gamemanager/meta` 读取路径、`normalizeSearchName`、`isIncomplete`、`scrapeBatch`、`clearScrapeState`、`GetGameCover` 系列绑定、`UpdateGameInfo`、`ScrapeAllGames`、旧的侧边栏队列弹窗
  *Dead code: `Pipeline.Scrape`, the `.gamemanager/meta` read path, `normalizeSearchName`, `isIncomplete`, `scrapeBatch`, `clearScrapeState`, the `GetGameCover` bindings, `UpdateGameInfo`, `ScrapeAllGames`, and the old sidebar queue popup*

---

## v0.7.5 (2026-06-01)

### Added

- 队列弹出框显示 pending 任务列表，可滚动查看待处理游戏名
  *Pending task list in queue popup with scrollable game names*
- `queue:status` 事件新增 `currentGameId`，前端跟踪队列刮削的游戏卡片 spinner
  *currentGameId in queue:status so frontend shows scraping spinner on auto-scraped cards*
- 自定义滚动条样式：8px 宽，hover 时通过 border 技巧变粗不挤压内容
  *Custom scrollbar: 8px width, expands on hover without layout shift*

### Fixed

- 队列任务提交时携带 `Title`，修复 "Scraping: ..." 显示问题
  *Task Title set at submission time so queue shows correct game name*
- `App.css` 意外覆盖导致样式丢失/侧边栏布局失效
  *CSS accidental overwrite causing sidebar layout break*
- 自动刮削任务不显示卡片进度动画（手动/队列刮削状态分离跟踪）
  *Auto-scrape tasks now show card-level scraping spinner*

---

## v0.7.4-alpha (2026-05-31)

### Fixed

- 队列 `Status()` 计算：任务处理期间保留在 slice 中，统计更准确
  *Keep tasks in slice during processing for accurate Status counts*

---

## v0.7.3-alpha (2026-05-31)

### Changed

- 队列指示器从顶栏移至侧边栏底部，移除了顶栏刮削按钮
  *Moved queue indicator from top bar to sidebar bottom; removed top bar scrape buttons*

---

## v0.7.2-alpha (2026-05-31)

### Added

- `SaveConfig` 自动重启文件监听器并扫描新增路径
  *SaveConfig auto-restarts watcher and scans newly added paths*

---

## v0.7.1-alpha (2026-05-31)

### Fixed

- 过滤非游戏 exe（crashreport, bugreport, patch, redist 等工具程序）
  *Filter out utility exes (crashreport, patch, redist, etc.) from game list*

---

## v0.7.0-alpha (2026-05-31)

### Added

- **任务队列**：后台串行刮削（`internal/taskqueue/`），`autoScrapeNew` 提交至队列
  *Task queue: background sequential scraping, autoScrapeNew submits to queue*
- **文件监听**：`fsnotify` 监听游戏目录，新增/删除游戏自动处理（`internal/watcher/`）
  *File watcher: fsnotify-based directory monitoring for new/removed games*
- **实时推送**：Wails `EventsEmit/EventsOn` 推送队列状态、新游戏、删除事件
  *Real-time events: queue status, new game detection via Wails events*
- **队列控制**：顶栏暂停/继续/清空按钮
  *Queue controls: pause/resume/clear in top bar*
- 设置页：`File Watcher` 开关 + 防抖滑块（50-2000ms，默认 100ms）
  *Settings: File Watcher toggle + debounce slider*

### Changed

- 手动 `ScrapeGame`/`ScrapeAllGames` 仍为同步直接执行（前端 `useScrape` hook 兼容）
  *Manual scrape still synchronous for useScrape hook compatibility*
- 队列仅用于 `autoScrapeNew` 后台处理
  *Queue used only for autoScrapeNew background tasks*

---

## v0.6.3-alpha (2026-05-31)

### Changed

- 侧边栏新增 ☑ Show Unmatched 复选框，可一键隐藏所有未匹配游戏
  *Sidebar toggle to show/hide unmatched games globally*

---

## v0.6.2-alpha (2026-05-31)

### Added

- `.gamemanager/` 隐藏文件夹存储游戏元数据：`gameinfo.json` + `meta/` + `covers/`
  *.gamemanager/ folder structure with per-platform metadata + covers*
- Steam ACF 文件解析：提取游戏名、AppID、更新时间、磁盘大小
  *Steam ACF parsing: extract name, AppID, LastUpdated, SizeOnDisk*
- Steam 封面缓存读取：从 `appcache/librarycache/<appid>/` 遍历顶层+hash 子目录
  *Steam cache cover copy from librarycache (top-level + hash subdirs)*
- Steam 用户检测：读注册表获取客户端路径，遍历 `userdata/` 显示 PersonaName
  *Steam user detection via registry, persona name from localconfig.vdf*
- 路径标签多标签芯片输入，Folders 侧边栏分类筛选
  *Multi-label path tags with inline input + Folders sidebar filter*
- 优先数据源子菜单（🔥 侧滑展开），自动刮削跳过已有封面游戏
  *Preferred source sub-menu, skip auto-scrape when cover exists*

### Changed

- `DownloadCover` 本地文件存在时跳过下载；封面缓存复制后设置 Metadata.CoverURL
- 卸载 `local` 平台标签，用 Unmatched 标记未刮削游戏
- 详情面板居中 540px 弹窗，全操作上下文同步

### Fixed

- Steam 缓存路径：`userdata/grid/` → `appcache/librarycache/<appid>/<hash>/`
- HTML 实体解码（`&quot;` 等）统一使用 `html.UnescapeString`
- 右键菜单刮削统一走 `useScrape` hook
- 刮削后封面实时刷新（`refreshKey` 递增触发 re-fetch）

---

## v0.5.10-alpha (2026-05-31)

### Changed

- 刮削中卡片 45% 变暗；标签删除按钮绝对定位

## v0.5.9-alpha (2026-05-31)

### Fixed

- 详情页星标按钮样式；全部刮削器 HTML 实体解码

## v0.5.8-alpha (2026-05-31)

### Fixed

- 优先平台按钮即时更新；恢复丢失的按钮 CSS

## v0.5.7-alpha (2026-05-31)

### Changed

- 去重卡片平台标签；Steam 蓝色 `#1a4b8a`

---

## v0.5.6-alpha (2026-05-31)

### Changed

- 右键菜单启动游戏置于菜单顶部，绿色强调色（`context-launch`）
  *Context menu: Launch Game at top with green accent color*
- 详情弹窗启动按钮左对齐方形全宽绿色，布局重新整理对齐、统一留白
  *Detail: left-aligned full-width green square launch button, cleaned up layout and spacing*
- 重新刮削按钮移到底部，与 Open Folder / Metadata 同排，标注完整文字 `↻ Re-scrape Metadata`
  *Re-scrape moved to bottom bar with full label*
- 平台标签改为可点击链接（`↗` 箭头），点击在浏览器打开对应网页
  *Platform tags now clickable links to open web pages*
- 统一 Tags 区域：刮削分类标签（紫色）→ 用户自定义标签（橙色）→ [+] 内联添加
  *Unified Tags section: genre (purple) → user (amber) → [+] inline add*

---

## v0.5.5-alpha (2026-05-31)

### Changed

- 详情页启动按钮改为左对齐方形，重新刮削按钮标注 `↻ Re-scrape Metadata` 完整文字
  *Detail: launch button left-aligned square; re-scrape button with full text label*

---

## v0.5.4-alpha (2026-05-31)

### Changed

- 右键菜单启动游戏置顶 + 绿色强调色；详情页 52px 绿色圆形启动按钮
  *Context menu: Launch at top with green accent; detail: 52px green circular launch*

---

## v0.5.3-alpha (2026-05-31)

### Changed

- 游戏详情弹窗从右侧滑出改为居中悬浮对话框（540px，缩放淡入动画）
  *Game detail panel: centered floating dialog (540px, scale-in) replacing right slide-out*
- 详情面板同步右键菜单全部操作：星标、启动、重刮、优先数据源、打开平台页、标签、浏览目录/元数据
  *Detail panel now mirrors all context menu actions (star, launch, re-scrape, preferred source, tags, browse)*

### Fixed

- 最终消除 `local` 平台残留：`GameCard` 兜底值、`migratePlatform` 默认值改为空字符串
  *Final removal of 'local' platform fallbacks in GameCard and migratePlatform*

---

---

## v0.5.2-alpha (2026-05-31)

### Added

- 扫描到新游戏时自动刮削元数据（后台异步执行）
  *Auto-scrape metadata for newly discovered games after scan*
- 右键菜单侧滑子菜单（`Open Web Page` / `Preferred Source`），hover 右侧展开，不需点击
  *Side-sliding sub-menus on hover, no click-to-expand needed*
- 卡面 + 详情页展示所有平台标签（主平台不透明，附加半透明）
  *All platform badges on card + detail panel (primary opaque, extras semi-transparent)*
- 多源并行刮削 `ScrapeAll`：遍历所有启用的源，一游戏关联多平台
  *Multi-source ScrapeAll: tries ALL enabled sources, multi-platform per game*
- `GameInfo.PreferredSource` 字段：`PrimaryPlatform()` 优先读取首选来源
  *PreferredSource field: PrimaryPlatform() reads preferred source first*
- `ForceScanGames`：强制重读 steam_appid.txt 和重新生成 .gameinfo.json
  *ForceScanGames: re-read steam_appid.txt and regenerate .gameinfo.json*
- Unmatched 标签：未刮削游戏从 All Games 隐藏，仅在侧边栏 Unmatched 分类查看
  *Unmatched tag: unscraped games hidden from All Games, viewed via sidebar filter*
- 刮削代码模块化：`src/hooks/useScrape.ts`（`scrapeSingle` / `scrapeBatch`）
  *Modularized scrape: useScrape hook (scrapeSingle / scrapeBatch)*
- 手动刮削（详情面板/右键菜单）显示进度条 + 卡片角标
  *Manual scrape shows progress bar + card badge*

### Changed

- `Platform` / `PlatformID` → `Platforms []PlatformInfo` + `Aliases []string` 多平台数组
- `SetPlatform` append 非 prepend；侧边栏计数改为按所有平台统计
- 移除 `local` 平台：新游戏 platform 为空，刮削后自动设置
- 右键菜单刮削改用 App 级 `useScrape` hook（统一进度和角标）
- 刮削后封面自动刷新（`refreshKey` 递增触发 re-fetch）

### Fixed

- Steam 刮削器跳过 RJ 码目录名，避免劫持 DLsite 游戏
- 首选源刮削：preferred source 全量 ApplyResult，其他源仅添加平台+别名
- SanobaWitch → NEKOPARA 测试用例修复（Steam 333600 已验证）
- 右键重新刮削未触发进度条/角标 → 统一经 `useScrape` hook
- 刮削后游戏卡片封面不刷新 → 添加 `refreshKey` 依赖

---

## v0.5.1-alpha (2026-05-30)

多源刮削 + Steam 跳 RJ 码 + 强制重扫修复。
*Multi-source scrape, Steam RJ skip, force re-scan fix.*

## v0.5.0-alpha (2026-05-30)

多平台数据模型（`Platforms + Aliases`）+ 侧滑子菜单 + 启动/重刮。
*Multi-platform model + side-sliding menu + launch/re-scrape.*

---

## v0.4.0-alpha (2026-05-30)

### Added

- 三级标签系统：平台标签（自动）、分类标签（刮削器）、用户标签（手动），卡片覆盖层分色显示
  *Three-tier tag system: platform (auto), genre (scraper), user (manual) with color-coded cards*
- 侧边栏分段筛选：Platforms / Genres / My Tags 三个独立区块
  *Sidebar sections: Platforms, Genres, My Tags*

### Changed

- 刮削指示器改为 CSS 旋转圆环 + 完成 ✓ / 失败 ✗ 弹入动画
  *Scrape indicator: CSS spinner → ✓ / ✗ pop-in animation*
- 侧边栏完全重构层次结构

### Fixed

- 测试游戏库 `烟火` / `黑神话悟空` steam_appid 添加

---

## v0.3.3-alpha (2026-05-30)

### Fixed

- CamelCase 拆分错误：缩写词不再被拆成单字母
  *CamelCase split no longer breaks acronyms*
- DLsite 区分 404 / 无元数据
- 批量刮削每完成一个即刷新卡片

---

## v0.3.2-alpha (2026-05-30)

### Added

- RAWG.io 刮削器（免费 API，全平台覆盖）
  *RAWG.io scraper: free global API*
- 搜索名称智能拆分：CamelCase / 下划线 / 连字符变体依次尝试
  *Smart name normalization with fallback variations*

### Fixed

- 旧 .gameinfo.json BOM 残留（LoadFromDir 自动清洗）
- VNDB User-Agent / 非 JSON 响应检测
- Bangumi 超时诊断日志

---

## v0.3.1-alpha (2026-05-30)

### Changed

- 日志按会话归档（`session_2026-05-30_14-30-01.log`）
  *Logger per-session files*

### Fixed

- Steam 搜索词错误拼接完整路径 → 仅用目录名
- `steam_appid.txt` UTF-8 BOM 去除
- `UnityCrashHandler64.exe` 过滤

---

## v0.3.0-alpha (2026-05-30)

### Added

- 游戏卡片右键菜单：星标、标签、浏览路径、浏览元数据
  *Game card context menu: star, tags, browse*
- 自定义标签系统

---

## v0.2.5-alpha (2026-05-30)

### Fixed

- 跨盘符绝对路径扫描修复（`E:\...` 拼合错误）

---

## v0.2.4-alpha (2026-05-30)

### Added

- 结构化日志系统（slog + 按天归档，覆盖全链路操作）

---

## v0.2.3-alpha (2026-05-29)

### Fixed

- 已有配置自动补齐缺失的数据源

---

## v0.2.2-alpha (2026-05-29)

### Added

- SteamGridDB + Bangumi 刮削器

---

## v0.2.1-alpha (2026-05-29)

### Added

- 强制重刮、游戏启动、多平台 CI、README/DESIGN 文档

### Fixed

- macOS 产物路径、Ubuntu CI 22.04 固定

---

## v0.2.0-alpha (2026-05-29)

### Added

- 多源元数据刮削（Steam/VNDB/DLsite）、流水线优先级、双封面、详情面板、并行进度、CI/CD

### Changed

- 配置模型重构（metadataSources）、机器名自动检测、internal/ 包化

### Fixed

- WebView2 封面 base64 传输、跨客户端机器名冲突

---

## v0.1.0-alpha (2026-05-29)

### Added

- Wails v2 + React 脚手架、配置模型、扫描器、.gameinfo.json、UI 骨架、单元测试
