# Changelog

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
