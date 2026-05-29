# Changelog

## v0.2.1-alpha (2026-05-29)

### Added / 新增
- Force re-scrape button for single games and batch (`Force` / `Re-scrape`)
- Multi-platform GitHub Actions release (Windows, Linux, macOS)
- SHA256 checksum per release asset
- Pre-release badge on GitHub Releases
- Game launch button (`▶ Launch Game`) in detail panel

### Fixed / 修复
- macOS binary path inside `.app` bundle for release assets
- Ubuntu CI: pin to 22.04 for webkit2gtk-4.0 compatibility

---

## v0.2.0-alpha (2026-05-29)

### Added / 新增
- Metadata scraping system: Steam, VNDB, DLsite sources
- Pipeline with priority ordering via `metadataSources` config
- Dual cover download: portrait (`cover.jpg`) + landscape (`cover_landscape.jpg`)
- Base64 cover delivery bypassing WebView2 `file://` restrictions
- Game detail panel (right-side slide-in)
- Scraping progress bar with parallel limit (3 concurrent)
- Incomplete metadata detection for targeted re-scraping
- Language-aware scraping (Steam `l=schinese`, VNDB `zh-Hans`)
- Per-source settings with expandable inline panel (API keys, etc.)
- GitHub Actions CI with auto-release on version tags
- `CHANGELOG.md` (this file)
- `DESIGN.md` architecture and roadmap document

### Changed / 变更
- Settings page: card-based layout, toggle switches, reorderable sources
- Config: `metadataSources` replaces `vndbEnabled`/`dlsiteEnabled`, legacy migration
- Config: `machineName` removed, auto-detected from `os.Hostname()`
- Game directories: native folder picker with relative path conversion
- Collapsible sidebar layout with auto-derived categories
- Go source modularized into `internal/` sub-packages

### Fixed / 修复
- Cover images not displaying in WebView2 (base64 workaround)
- Machine name conflicts across shared config
- Steam portrait cover using `library_600x900_2x.jpg`

---

## v0.1.0-alpha (2026-05-29)

### Added / 新增
- Wails v2 + React + TypeScript project scaffold
- Config model with JSON persistence and legacy field migration
- Game scanner: recursive directory walk, `.exe` detection, `steam_appid.txt` parsing
- `.gameinfo.json` metadata file generation
- React frontend: sidebar navigation, game card grid, dark theme
- Settings page: machine config, game directories, scan depth
- 17 unit tests for scanner, config, and gameinfo packages
- Git version control with GitHub push
