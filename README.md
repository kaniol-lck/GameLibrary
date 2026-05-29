# GameLibrary

> This project is generated with AI assistance (opencode).

跨机器、便携式的游戏库管理器。将管理程序与游戏文件一同放在 NAS 网络挂载路径中，通过相对路径管理游戏库，支持自动识别、多端时长统计与云存档同步。

## 技术栈

| 层 | 技术 |
|----|------|
| 运行时 | [Wails v2](https://wails.io/) (Go + WebView2) |
| 后端 | Go 1.23+ |
| 前端 | React 18 + TypeScript + Vite |
| 数据存储 | 纯文件 (JSON)，免数据库 |
| 刮削源 | Steam API / VNDB API / DLsite |

## 功能

- **游戏自动识别** — 递归扫描目录，识别 `.exe` 文件与 `steam_appid.txt`，自动生成元数据
- **便携部署** — 单 `.exe` 文件放入 NAS，相对路径寻址，任意挂载盘符通用
- **多端管理** — 各客户端运行同一份程序，时长统计、存档状态均文件化存储于 NAS
- **云存档** — 符号链接 / 文件同步两种模式，启动前拉取、关闭后推送
- **元数据刮削** — Steam / VNDB / DLsite 多源刮削（规划中）
- **游戏分类** — 按类型 / 标签自动分类，侧边栏筛选

## 使用方法

### 部署

1. 将 `GameLibrary.exe` 放到 NAS 的游戏库根目录，结构如下：

```
Z:\GameLibrary\
├── GameLibrary.exe          ← 管理程序
├── config.json              ← 首次运行自动生成
└── Games\                   ← 游戏目录（可在设置中自定义）
    ├── SteinsGate\
    │   ├── steam_appid.txt
    │   └── game.exe
    ├── Witcher3\
    │   └── bin\x64\witcher3.exe
    └── ...
```

2. 在每台客户端上挂载 NAS 到任意盘符（如 `Z:\`）
3. 运行 `Z:\GameLibrary\GameLibrary.exe`
4. 点击 **Scan Games** 开始扫描游戏库

### 设置

点击左下角齿轮图标进入设置页面，可配置：

- **Machine Name** — 区分不同客户端
- **Game Directories** — 添加 / 删除游戏目录（相对路径）
- **Max Scan Depth** — 子目录扫描深度
- **Language** — 简体中文 / English / 日本語
- **Metadata Sources** — VNDB / DLsite 开关

### 游戏识别规则

| 识别方式 | 说明 |
|----------|------|
| `steam_appid.txt` | 优先识别，向上查找 3 层父目录 |
| `.exe` 文件 | 直接包含 `.exe` 的目录即为游戏目录 |
| `.gameinfo.json` | 已有元数据的目录跳过重复扫描 |
| `unins*.exe` | 自动过滤卸载程序 |

## 构建

### 环境要求

- [Go](https://go.dev/dl/) 1.23+
- [Node.js](https://nodejs.org/) 18+ (含 npm)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

```bash
# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 安装前端依赖
cd frontend && npm install && cd ..
```

### 开发模式

```bash
wails dev
```

启动热重载开发服务器，前端修改即时生效。

### 生产构建

```bash
wails build
```

编译产物位于 `build/bin/GameLibrary.exe`，单文件，可直接部署。

### 运行测试

```bash
go test ./...
```

## 项目结构

```
GameLibrary/
├── main.go                # Wails 入口
├── app.go                 # 后端 API（前端可调用的方法）
├── config.go              # 配置文件读写
├── gameinfo.go            # 游戏元数据模型
├── scanner.go             # 游戏目录扫描器
├── scanner_test.go        # 单元测试
├── testdata/              # 测试用模拟游戏目录
├── frontend/
│   └── src/
│       ├── App.tsx         # 主界面（侧边栏 + 内容区）
│       ├── App.css         # 全局样式
│       └── components/
│           ├── Sidebar.tsx  # 可折叠侧边栏导航
│           ├── GameCard.tsx # 游戏卡片组件
│           └── Settings.tsx # 设置页面
└── wails.json             # Wails 项目配置
```

## License

MIT
