# Telegram 自动签到程序

[English](README.md) | [中文](#中文)

基于 Go 语言开发的 Telegram 自动签到程序，支持多账号管理、灵活的任务调度和多种签到方式。

## 功能特性

- **多账号支持** - 同时管理多个 Telegram 账号
- **灵活登录方式** - 支持手机号登录或二维码登录，支持两步验证
- **多种签到方式** - 文本消息签到或按钮点击签到
- **并发执行** - 高性能工作池架构
- **灵活调度** - 支持 Cron 表达式和间隔时间调度
- **代理支持** - 支持 SOCKS5 代理配置
- **会话持久化** - 首次登录后自动管理会话
- **完整日志** - 主日志和独立任务日志
- **Docker 支持** - 提供官方多架构 Docker 镜像
- **国际化** - 支持英文和中文界面

## 快速开始

### 方式一：Docker（推荐）

```bash
# 拉取最新镜像
docker pull ghcr.io/bamzest/telegram-auto-checkin:latest

# 创建并编辑配置文件
cp config.yaml config.local.yaml
# 编辑 config.local.yaml 填入您的配置

# 运行容器
docker run --rm -it \
  -v $(pwd)/session:/app/session \
  -v $(pwd)/config.local.yaml:/app/config.yaml:ro \
  -v $(pwd)/log:/app/log \
  ghcr.io/bamzest/telegram-auto-checkin:latest

# 或使用 docker-compose
docker-compose up -d
```

### 方式二：预编译二进制

从 [GitHub Releases](https://github.com/bamzest/telegram-auto-checkin/releases) 下载最新版本。

```bash
# 解压文件
tar -xzf telegram-auto-checkin_*.tar.gz

# 配置应用
cp config.yaml config.local.yaml
# 编辑 config.local.yaml 填入您的配置

# 运行程序
./telegram-auto-checkin --config config.local.yaml
```

### 方式三：从源码构建

```bash
# 要求：Go 1.20 或更高版本
git clone https://github.com/bamzest/telegram-auto-checkin.git
cd telegram-auto-checkin

# 构建二进制文件
go build -o telegram-auto-checkin .

# 运行应用
./telegram-auto-checkin
```

## 配置说明

所有配置选项都在 [config.yaml](config.yaml) 中有详细说明。主要配置项包括：

### 基础设置

- **语言**：设置 `language: "zh"` 使用中文，或 `"en"` 使用英文
- **代理**：可选的 SOCKS5 代理地址（例如 `127.0.0.1:1080`）
- **应用凭证**：从 https://my.telegram.org/apps 获取
  - `app_id`：您的 Telegram API ID
  - `app_hash`：您的 Telegram API Hash

### 账号配置

支持配置多个账号，每个账号可独立设置：

```yaml
accounts:
  - name: "main"
    phone: "+8612345678900"     # 手机号登录
    password: "your_2fa_pass"   # 可选：两步验证密码
    tasks:
      - name: "daily_checkin"
        enabled: true
        target: "@botusername"
        method: "message"       # 或 "button"
        payload: "/checkin"
        schedule: "0 8 * * *"   # Cron 表达式
```

### 登录方式

**手机号登录**：
- 在配置中填写 `phone`
- 根据提示输入验证码
- 如开启两步验证，可设置 `password` 字段

**二维码登录**：
- 将 `phone` 留空
- 扫描终端显示的 `tg://login?token=...` 链接
- 在移动设备上确认登录

### 任务调度

任务支持灵活的调度选项：

- **Cron 表达式**：`"0 8 * * *"`（每天早上 8 点）
- **间隔语法**：`"@every 12h"`（每 12 小时）
- **启动时运行**：设置 `run_on_start: true` 立即执行

## 配置优先级

1. 环境变量（最高优先级）
2. 环境特定配置文件：`config.{APP_ENV}.yaml`
3. 主配置文件：`config.yaml`

## 日志系统

- **主日志**：`log/app.log`
- **任务日志**：`log/tasks/{账号}/{任务}_{时间戳}.log`
- 可在 `config.yaml` 中配置日志目录和格式

## 开发

### 项目结构

```
telegram-auto-checker/
├── main.go                 # 应用入口
├── config.yaml             # 配置模板
├── internal/               # 内部包
│   ├── client/            # Telegram 客户端封装
│   ├── config/            # 配置管理
│   ├── executor/          # 任务执行引擎
│   ├── scheduler/         # 任务调度
│   ├── logger/            # 日志工具
│   └── i18n/              # 国际化
├── locales/               # 翻译文件
│   ├── en.yaml           # 英文翻译
│   └── zh.yaml           # 中文翻译
└── session/              # 会话存储（自动生成）
```

### 构建

```bash
# 安装依赖
go mod tidy

# 为当前平台构建
go build -o telegram-auto-checkin .

# 为特定平台构建
GOOS=linux GOARCH=amd64 go build -o telegram-auto-checkin .
```

## Docker 使用

### 使用 Docker Compose

创建 `docker-compose.yml` 文件：

```yaml
version: '3'
services:
  telegram-auto-checkin:
    image: ghcr.io/bamzest/telegram-auto-checkin:latest
    container_name: telegram-auto-checkin
    volumes:
      - ./session:/app/session
      - ./config.local.yaml:/app/config.yaml:ro
      - ./log:/app/log
    restart: unless-stopped
```

运行：
```bash
docker-compose up -d
```

### 多架构支持

Docker 镜像支持以下架构：
- `linux/amd64`（x86_64）
- `linux/arm64`（ARM 64位）
- `linux/arm/v7`（ARM 32位）

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 作者

**bamzest**

## 免责声明

本工具仅用于教育和自动化目的。用户需自行负责遵守 Telegram 服务条款。作者不对任何滥用或违规行为承担责任。

