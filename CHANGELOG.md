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
- 测试用例新增 `NEKOPARA_RJ157652`（Steam 333600 + DLsite 双平台）
  *Added NEKOPARA_RJ157652 test case (cross-platform Steam + DLsite)*

### Changed

- `Platform` / `PlatformID` 字段 → `Platforms []PlatformInfo` 多平台数组
- `PrimaryPlatform()` 读取 `preferredSource`，不再依赖数组顺序
- `SetPlatform` append 而非 prepend
- 侧边栏平台计数改为统计所有平台（非仅主平台）

### Fixed

- Steam 刮削器跳过 RJ 码目录名，避免劫持 DLsite 游戏
- SanobaWitch Steam AppID 688390 → 替换为 NEKOPARA 333600（已验证）

---

## v0.4.0-alpha (2026-05-30)