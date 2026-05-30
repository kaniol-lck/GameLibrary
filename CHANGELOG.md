# Changelog

## v0.2.5-alpha (2026-05-30)

### Fixed

- 修复跨盘符绝对路径（如 `E:\SteamLibrary`）被错误拼接到 exeDir 后面导致扫描失效
  *Fixed cross-volume absolute paths (e.g. `E:\SteamLibrary`) incorrectly joined with exeDir, breaking game discovery*

---

## v0.2.4-alpha (2026-05-30)

### Added

- 规范化日志系统：基于 `log/slog` 的异步文件日志，按天轮转归档
  *Structured logging system: slog-based async file logging with daily rotation*
- 日志覆盖所有核心操作：扫描（目录进入/游戏发现/exe 识别/过滤）、刮削（源尝试/成功/失败/ID）、配置读写、游戏启动、封面下载
  *Logs cover all core operations: scan, scrape, config, launch, cover download*
- 日志格式包含毫秒精度时间戳、调用源位置、结构化键值对
  *Log format: millisecond timestamps, source locations, structured key-value pairs*

---

## v0.2.3-alpha (2026-05-29)

### Fixed

- 已有配置升级时自动补齐缺失的数据源（如新增的 Bangumi / SteamGridDB）
  *Auto-migrate missing sources into existing config on load (merge defaults)*

---

## v0.2.2-alpha (2026-05-29)

### Added

- 新增 SteamGridDB 刮削器：通过 Steam AppID 获取高清封面图
  *SteamGridDB scraper: high-quality cover art via Steam AppID (needs API key)*
- 新增 Bangumi (bgm.tv) 刮削器：中文元数据（标题、简介、封面）
  *Bangumi scraper: Chinese metadata from bgm.tv (title, description, cover)*
- 默认数据源从 4 个扩展为 6 个（新增 bangumi、steamgriddb）
  *Default sources expanded from 4 to 6 (bangumi, steamgriddb added)*

---

## v0.2.1-alpha (2026-05-29)

### Added

- 单游戏和批量强制重新刮削按钮（`Force` / `Re-scrape`）
  *Force re-scrape buttons for single games and batch (`Force` / `Re-scrape`)*
- 多平台 GitHub Actions 发布（Windows / Linux / macOS）
  *Multi-platform GitHub Actions release (Windows, Linux, macOS)*
- 每个发布资产附带 SHA256 校验文件
  *SHA256 checksum file per release asset*
- GitHub Release 标记为 pre-release
  *Pre-release flag on GitHub Releases*
- 详情面板新增启动游戏按钮（`▶ Launch Game`）
  *Game launch button (`▶ Launch Game`) in detail panel*
- 项目文档：README badges、CHANGELOG.md、DESIGN.md
  *Project docs: README badges, CHANGELOG.md, DESIGN.md*

### Fixed

- macOS 发布产物路径修复（`.app` 包内二进制）
  *macOS binary path inside `.app` bundle for release assets*
- Ubuntu CI 固定为 22.04 以兼容 webkit2gtk-4.0
  *Pin Ubuntu CI to 22.04 for webkit2gtk-4.0 compatibility*

---

## v0.2.0-alpha (2026-05-29)

### Added

- 元数据刮削系统：Steam、VNDB、DLsite 三个数据源
  *Metadata scraping system: Steam, VNDB, DLsite sources*
- 按 `metadataSources` 优先级排序的刮削流水线
  *Scraping pipeline with priority ordering via config*
- 双封面下载：竖版 `cover.jpg`（卡片）+ 横版 `cover_landscape.jpg`（详情）
  *Dual cover download: portrait for cards + landscape for detail panel*
- Base64 图片传输，绕过 WebView2 的 `file://` 限制
  *Base64 cover delivery bypassing WebView2 file:// restrictions*
- 右侧滑出的游戏详情面板
  *Right-side slide-in game detail panel*
- 并行刮削进度条（3 并发限制）
  *Parallel scraping progress bar (3 concurrent limit)*
- 不完整元数据检测，自动补刮缺失字段
  *Incomplete metadata detection for targeted re-scraping*
- 语言感知刮削（Steam `l=schinese`，VNDB `zh-Hans`）
  *Language-aware scraping (Steam l=schinese, VNDB zh-Hans)*
- 数据源专属设置面板（可展开，API Key 等）
  *Per-source settings with expandable inline panel*
- GitHub Actions CI，推送 tag 自动发布 Release
  *GitHub Actions CI with auto-release on version tags*

### Changed

- 设置页：卡片式布局、toggle 开关、可排序数据源列表
  *Settings page: card-based layout, toggle switches, reorderable sources*
- 配置模型：`metadataSources` 替代 `vndbEnabled` / `dlsiteEnabled`，旧配置自动迁移
  *Config: metadataSources replaces vndbEnabled/dlsiteEnabled, legacy migration*
- 机器名从 `config.json` 移除，改为 `os.Hostname()` 自动检测
  *Machine name removed from shared config, auto-detected via os.Hostname()*
- 游戏目录选择改用系统原生文件夹对话框，自动转换相对路径
  *Game directories: native folder picker with relative path conversion*
- 可折叠侧边栏布局，自动派生游戏分类
  *Collapsible sidebar layout with auto-derived categories*
- Go 源码模块化重组织为 `internal/` 子包
  *Go source modularized into internal/ sub-packages*

### Fixed

- 封面图片在 WebView2 中无法显示（改用 base64 传输）
  *Cover images not displaying in WebView2 (base64 workaround)*
- 多客户端共享配置导致机器名互相覆盖
  *Machine name conflicts across shared config*
- Steam 封面使用竖版 `library_600x900_2x.jpg` 替代横版 header
  *Steam portrait cover using library_600x900_2x.jpg*

---

## v0.1.0-alpha (2026-05-29)

### Added

- Wails v2 + React + TypeScript 项目脚手架
  *Wails v2 + React + TypeScript project scaffold*
- 配置模型（JSON 持久化、旧字段迁移）
  *Config model with JSON persistence and legacy migration*
- 游戏扫描器：递归目录遍历、`.exe` 识别、`steam_appid.txt` 解析
  *Game scanner: recursive walk, .exe detection, steam_appid.txt parsing*
- `.gameinfo.json` 元数据文件生成
  *Game info metadata file generation*
- React 前端：侧边栏导航、游戏卡片网格、暗色主题
  *React frontend: sidebar nav, game card grid, dark theme*
- 设置页：机器配置、游戏目录管理、扫描深度
  *Settings page: machine config, game directories, scan depth*
- 17 个单元测试（扫描器、配置、游戏信息）
  *17 unit tests for scanner, config, and gameinfo*
- Git 版本控制与 GitHub 推送
  *Git version control and GitHub push*
