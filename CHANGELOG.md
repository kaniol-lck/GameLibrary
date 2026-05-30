# Changelog

## v0.5.2-alpha (2026-05-31)

### Added

- 扫描到新游戏时自动刮削元数据（后台异步执行）
  *Auto-scrape metadata for newly discovered games after scan*
- 右键菜单优先数据源子菜单，可单选切换各平台，持久化到 `preferredSource`
  *Context menu preferred source picker with radio buttons, persisted to .gameinfo.json*
- 卡面各平台标签全部可见（主平台不透明，附加半透明）
  *All platform badges visible on card (primary opaque, extras semi-transparent)*
- 多源并行刮削 `ScrapeAll`：遍历所有启用的源，一游戏关联多平台（steam+dlsite）
  *Multi-source ScrapeAll: tries ALL enabled sources, multi-platform per game*
- `GameInfo.PreferredSource` + `Aliases` 字段，旧 `platform/platformId` 自动迁移
  *PreferredSource + Aliases fields, legacy platform/platformId auto-migrated*
- `ForceScanGames`：强制重读 steam_appid.txt 和重新生成 .gameinfo.json
  *ForceScanGames: re-read steam_appid.txt and regenerate .gameinfo.json*

### Changed

- `Platform` / `PlatformID` 字段 → `Platforms []PlatformInfo` 多平台数组
- `PrimaryPlatform()` 读取 `preferredSource`，不再依赖数组顺序
- `SetPlatform` append 而非 prepend
- 侧边栏平台计数改为统计所有平台（非仅主平台）

### Fixed

- Steam 刮削器跳过 RJ 码目录名，避免劫持 DLsite 游戏
  *Steam scraper skips RJ-code directory names*
- DJsite RJ195371 刮削失败：SanobaWitch → NEKOPARA（Steam 333600 已验证）
  *Replaced SanobaWitch test with NEKOPARA (verified Steam 333600)*

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
