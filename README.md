# GameLibrary

[![Version](https://img.shields.io/badge/version-0.8.0-blue)](https://github.com/kaniol-lck/GameLibrary/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/kaniol-lck/GameLibrary/release.yml)](https://github.com/kaniol-lck/GameLibrary/actions)
[![CI](https://img.shields.io/github/actions/workflow/status/kaniol-lck/GameLibrary/ci.yml?label=ci)](https://github.com/kaniol-lck/GameLibrary/actions)
[![Downloads](https://img.shields.io/github/downloads/kaniol-lck/GameLibrary/total?color=brightgreen)](https://github.com/kaniol-lck/GameLibrary/releases)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2-DF0000?logo=wails)](https://wails.io/)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D6)](https://github.com/kaniol-lck/GameLibrary/releases)
![AI](https://img.shields.io/badge/AI--assisted-DeepSeek%20Harness-8A2BE2)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

> 本项目在 **DeepSeek Harness** 辅助下开发。所有改动均经过本地构建、测试与运行验证。
>
> *Developed with assistance from DeepSeek Harness. Every change is verified by building, testing and running it locally.*

跨机器、便携式的游戏库管理器。将管理程序与游戏文件一同放在 NAS 网络挂载路径中，通过相对路径管理游戏库，支持自动识别与多源元数据刮削。

*Cross-machine, portable game library manager. Put the executable and the games on a NAS share; the library is addressed by relative path, with automatic discovery and multi-source metadata scraping.*

## 技术栈

| 层 | 技术 |
|----|------|
| 运行时 | [Wails v2](https://wails.io/) (Go + WebView2) |
| 后端 | Go 1.23+ |
| 前端 | React 19 + TypeScript 5 + Vite 7 |
| 数据存储 | 纯文件 (JSON)，免数据库 |
| 刮削源 | Steam / VNDB / Bangumi / DLsite / RAWG / SteamGridDB |

**目标平台为 Windows。** 扫描与启动识别 `.exe`、`.com`、`.bat`、`.cmd`、`.ps1`、`.lnk`；其余平台可以编译与测试，但不受支持，也不发布产物。

*Windows is the target platform. Other platforms still compile and are covered by CI, but are not supported or shipped.*

## 功能

- **游戏自动识别** — 递归扫描目录，识别启动器与 `steam_appid.txt`，并解析 Steam ACF 清单获取标题
- **便携部署** — 单个 `.exe` 放入 NAS，相对路径寻址，任意挂载盘符通用
- **多端管理** — 各客户端运行同一份程序，所有数据以文件形式存储于 NAS
- **元数据刮削** — 6 个数据源；同一游戏的各源并发查询，跨游戏也可并行（默认 4，可调），各源仍受按主机限流；带任务列表、真实进度、暂停/继续、失败原因与单条重试
- **游戏分类** — 平台 / 分类标签 / 用户标签 / 目录标签四级筛选
- **文件监听** — 新增游戏目录或向已有目录解压游戏时自动识别并入库
- **封面本地化** — 封面下载到游戏目录，由内置 HTTP 服务提供，不经过 IPC

## 使用方法

### 部署

1. 将 `GameLibrary.exe` 放到 NAS 的游戏库根目录：

```
Z:\GameLibrary\
├── GameLibrary.exe          ← 管理程序
├── config.json              ← 首次运行自动生成（多端共享）
└── Games\                   ← 游戏目录（可在设置中自定义）
    ├── SteinsGate\
    │   ├── steam_appid.txt
    │   └── game.exe
    ├── Witcher3\
    │   └── bin\x64\witcher3.exe
    └── ...
```

2. 在每台客户端上将 NAS 挂载到任意盘符（如 `Z:\`）
3. 运行 `Z:\GameLibrary\GameLibrary.exe`
4. 点击 **Scan** 开始扫描

### 设置

点击侧边栏底部的齿轮进入设置页：

| 卡片 | 内容 |
|------|------|
| Game Directories | 添加 / 删除游戏目录，为目录设置标签 |
| Scanning | 子目录扫描深度（1–10） |
| Language | 刮削元数据的语言偏好（Steam / VNDB） |
| File Watcher | 开关与去抖延迟 |
| Logging | 日志级别，以及日志写入本机目录还是 NAS |
| Metadata Sources | 数据源开关、优先级排序、API Key |
| About | 本机名与当前日志目录 |

### 游戏识别规则

| 识别方式 | 说明 |
|----------|------|
| 启动器文件 | `.exe` / `.com` / `.bat` / `.cmd` / `.ps1` / `.lnk` |
| `steam_appid.txt` | 优先识别，向上查找最多 2 层父目录（不越过库根目录） |
| Steam ACF 清单 | 位于 `steamapps\common\` 下时解析 `appmanifest_*.acf`，获得 AppID 与标题 |
| `.gamemanager\` | 已有元数据的目录直接复用，不再重复识别 |
| 过滤规则 | `unins*`、`UnityCrashHandler*`、`crashreport`、`vcredist`、`DXSETUP` 等工具程序不作为启动器 |

**库根目录始终会被递归扫描。** 即使游戏库根目录本身包含启动器文件，其下的游戏也不会被忽略。

已识别的游戏会把元数据写入该游戏目录下的 `.gamemanager\`：

```
游戏目录\
  .gamemanager\
    gameinfo.json      游戏元数据
    covers\            封面（由内置 HTTP 服务提供）
```

`gameinfo.json` 不包含机器相关信息（挂载盘符不会写入），因此多端共用同一份文件不会互相覆盖。

## 构建

### 环境要求

- Windows
- [Go](https://go.dev/dl/) 1.23+
- [Node.js](https://nodejs.org/) 24（见 `frontend/.nvmrc`）
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 开发模式

```bash
wails dev
```

### 生产构建

```bash
wails build
```

产物位于 `build/bin/GameLibrary.exe`，单文件，可直接部署。

### 运行测试

```bash
# 后端。必须显式指定包：frontend/node_modules 里恰好带有一个无关的 Go 包，
# 使用 ./... 会把它一并扫进来。
go test ./internal/... .

# 竞态检测。Windows 下需要 C 编译器（mingw-w64）；CI 在 Linux 上执行此项。
go test -race ./internal/... .

# 覆盖率
go test -cover ./internal/... .

# 前端
cd frontend
npm run typecheck
npm run lint
npm run test
```

## 项目结构

```
GameLibrary/
├── main.go                  # Wails 入口、启动诊断与装配
├── app.go                   # App 结构、生命周期、运行时状态重建
├── scrape_api.go            # 扫描 / 刮削 / 队列 API
├── library_api.go           # 库查询、用户字段、设置辅助 API
├── startup_marker.go        # 启动失败的事后诊断
├── internal/
│   ├── config/              # 配置模型、迁移、路径归一化
│   ├── fsutil/              # 路径解析、BOM 容错与原子写入
│   ├── game/                # 游戏元数据模型与持久化
│   ├── library/             # 并发安全的游戏缓存
│   ├── logger/              # 结构化日志（按天归档、目录回退）
│   ├── mediaserve/          # 封面 HTTP 服务
│   ├── platform/            # 注册表、Shell 与启动器抽象
│   ├── scanner/             # 目录扫描与游戏识别
│   ├── scraper/             # 刮削流水线（各源并发）与数据源
│   ├── taskqueue/           # 工作线程池队列
│   └── watcher/             # 文件系统监听
├── frontend/
│   ├── wailsjs/             # Wails 生成的绑定（提交到 Git）
│   └── src/
│       ├── api/             # 后端调用的唯一出口与事件订阅
│       ├── lib/             # 纯逻辑：筛选、平台元数据、封面 URL、队列、格式化
│       ├── hooks/           # useScrape
│       └── components/      # Sidebar / GameCard / GameDetail / ContextMenu /
│                            # QueuePanel / Settings
├── testdata/                # 测试用模拟游戏目录
├── DESIGN.md                # 设计文档
├── CHANGELOG.md             # 变更日志
└── wails.json
```

## 文档

- [DESIGN.md](./DESIGN.md) — 架构、模块设计、数据格式与项目约定
- [CHANGELOG.md](./CHANGELOG.md) — 版本变更，含 0.8.0 重构的完整说明

## 开发说明

本项目在 DeepSeek Harness 辅助下维护。提交前请确认以下检查全部通过：

```bash
gofmt -l $(git ls-files '*.go')        # 应为空
go build ./internal/... .              # 见下方说明
go vet ./internal/... .
go test ./internal/... .               # 竞态检测见 CI
cd frontend
npx tsc -b --noEmit
npm run lint
npm run test
```

Go 命令必须**显式写出包模式**（`./internal/... .`）而不是 `./...`：`frontend/node_modules` 里恰好带有一个无关的 Go 包，`./...` 会把它一并扫进来。

## License

MIT
