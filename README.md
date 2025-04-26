# Notify

GitHub仓库变更通知服务，支持将GitHub仓库的更新发送到DingTalk和Telegram。

[English Document](README_en.md)

## 功能特点

- 监控指定GitHub仓库的变更
- 支持监控多个仓库
- 可选择性监控特定分支和路径
- 支持DingTalk和Telegram通知渠道
- 自定义通知模板
- 灵活的调度配置
- 智能管理钉钉消息频率限制

## 快速开始

### 使用Docker

1. 准备配置文件
   
   将项目中的`config/config.example.yaml`复制为`config.yaml`并根据需要修改配置。

2. 使用Docker运行

   ```bash
   docker run -v /path/to/config.yaml:/app/config/config.yaml starcatmeow/notify:latest
   ```

### 从源码构建

1. 克隆仓库

   ```bash
   git clone https://github.com/yourusername/notify.git
   cd notify
   ```

2. 构建项目

   ```bash
   make release
   ```

3. 准备配置文件

   ```bash
   cp config/config.example.yaml config/config.yaml
   # 编辑配置文件
   ```

4. 运行服务

   ```bash
   ./release/notify/notify-linux-amd64
   ```

## 配置说明

配置文件使用YAML格式，包含以下主要部分：

### GitHub配置

```yaml
github:
  token: "your-github-token"  # GitHub个人访问令牌
  repos:
    - owner: "example-owner"  # 仓库拥有者
      name: "example-repo"    # 仓库名称
      branch: "main"          # 分支名称（可选）
      paths:                  # 监控的路径（可选）
        - "docs/"
        - "src/"
  watch_starred: false        # 是否监控关注的仓库
  watch_organizations: false  # 是否监控组织仓库
```

### 通知配置

```yaml
notifications:
  dingtalk:
    webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=your-token"
    secret: "your-secret"
  
  telegram:
    bot_token: "your-bot-token"
    chat_id: "your-chat-id"
```

### 通知模板和调度

```yaml
template: |
  ## {{ .Repository.FullName }} 有更新！
  # 模板内容...

# 定时运行配置
schedule:
  # 是否启用定时运行（作为守护进程）
  enabled: true
  # 检查间隔，支持的格式: 10s, 1m, 1h, 24h
  interval: "6h"
```

## 钉钉消息限流机制

钉钉机器人存在发送消息频率限制：

- 每个机器人每分钟最多发送20条消息
- 超过限制后，该机器人会被限流10分钟

本工具已实现智能限流管理:

- 自动分批发送通知，避免触发限流
- 批次间自动添加适当延迟
- 遇到限流时自动等待冷却期
- 提供清晰的限流提示和解决建议

当您监控大量仓库或有大量更新时，建议：

1. 增加定时任务的检查间隔（schedule.interval）
2. 减少单次监控的仓库数量
3. 细分多个通知实例，使用不同的钉钉机器人

## 许可证

MIT