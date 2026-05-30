# xboard_link_3x-ui

`xboard_link_3x-ui` 是一个用 Go 编写的 Xboard 与 3x-ui 桥接服务。它部署在 3x-ui 节点服务器上运行，通过 Xboard UniProxy 接口拉取可用用户，再通过 3x-ui Panel API 同步客户端，并把 3x-ui 统计到的用户流量增量回传给 Xboard。

本项目不依赖任何未开源项目代码，只基于 Xboard 和 3x-ui 的公开 HTTP 接口重新实现同步逻辑。

## 功能

- 从 Xboard UniProxy 拉取可用用户
- 自动创建、更新、删除 3x-ui 客户端
- 自动绑定客户端到指定 3x-ui 入站
- 支持新版 3x-ui `/panel/api/clients/*` 接口
- 兼容旧版 3x-ui `/panel/api/inbounds/*` 客户端接口
- 将 3x-ui 累计流量转换为增量流量并推送回 Xboard
- 使用本地 state 文件防止重复上报流量
- 支持单次同步和常驻运行
- 支持 Linux amd64 交叉编译部署

## 工作方式

```text
Xboard  --拉取用户-->  xboard_link_3x-ui  --写入客户端-->  3x-ui
Xboard  <--回传流量--  xboard_link_3x-ui  <--读取流量--    3x-ui
```

3x-ui 的入站需要提前在面板中创建好。本服务只负责同步用户到指定入站，不会自动创建 Reality、TLS、WebSocket、gRPC 等入站配置。

## 快速开始

### 1. 准备 3x-ui 入站

先在 3x-ui 面板创建好入站，例如：

- 协议：VLESS
- 传输：TCP/RAW、WebSocket、gRPC 等
- 安全：Reality/TLS/None
- 端口：你的代理端口

记下 3x-ui 入站 ID，后面填入 `xui.node_id`。例如 3x-ui 入站列表里目标入站 ID 是 `3`，就填写 `"node_id": 3`。

### 2. 创建 3x-ui API Token

在 3x-ui 面板设置中创建 API Token。推荐使用 API Token，不推荐使用账号密码登录。

如果 3x-ui 开启了面板安全路径，`xui.base_url` 必须包含路径，例如：

```json
"base_url": "http://127.0.0.1:2053/your-secret-path"
```

### 3. 配置 Xboard 节点

在 Xboard 中添加对应节点，节点协议、端口、传输、安全参数要和 3x-ui 入站一致。

注意：

- Xboard 的节点连接端口填代理入站端口，不是 3x-ui 面板登录端口。
- VLESS Reality + 3x-ui RAW 在 Xboard 里通常对应 TCP。
- Xboard 的 `server_token` 填到本服务配置的 `xboard.token`。

### 4. 修改配置

复制示例配置：

```bash
cp config.example.json config.json
```

编辑 `config.json`：

```json
{
  "xboard": {
    "base_url": "https://your-xboard.example.com",
    "token": "CHANGE_ME_XBOARD_SERVER_TOKEN",
    "node_id": 1,
    "node_type": "vless",
    "timeout_seconds": 20
  },
  "xui": {
    "base_url": "http://127.0.0.1:2053/YOUR_PANEL_BASE_PATH",
    "api_token": "CHANGE_ME_3X_UI_API_TOKEN",
    "node_id": 1,
    "timeout_seconds": 20,
    "insecure_tls": false
  },
  "sync": {
    "inbound_ids": [],
    "interval_seconds": 60,
    "traffic_interval_seconds": 60,
    "delete_stale": true,
    "email_prefix": "xboard",
    "enable_traffic_report": true,
    "state_file": "./xboard_link_3x-ui-state.json"
  }
}
```

关键字段：

- `xboard.base_url`: Xboard 站点地址
- `xboard.token`: Xboard 后台 server token
- `xboard.node_id`: Xboard 节点 ID 或 code
- `xboard.node_type`: 节点协议，例如 `vless`、`vmess`、`trojan`
- `xui.base_url`: 3x-ui 面板地址，若有安全路径必须带上
- `xui.api_token`: 3x-ui API Token
- `xui.node_id`: 要同步到的 3x-ui 入站 ID
- `xui.node_ids`: 可选，多个 3x-ui 入站 ID，例如 `[1, 3]`
- `sync.inbound_ids`: 旧版兼容字段，不推荐新配置继续使用

### 5. 单次同步测试

```bash
./xboard_link_3x-ui -config config.json -once
```

看到类似日志即表示同步成功：

```text
sync users done: desired=1 created=1 updated=0 attached=0 detached=0 deleted=0
traffic report done: users=0
```

### 6. 常驻运行

```bash
./xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json
```

临时后台运行：

```bash
nohup ./xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json > /var/log/xboard_link_3x-ui.log 2>&1 &
tail -f /var/log/xboard_link_3x-ui.log
```

## 编译

本机编译：

```bash
go build -o xboard_link_3x-ui ./cmd/xboard_link_3x-ui
```

Linux amd64：

```bash
GOOS=linux GOARCH=amd64 go build -o xboard_link_3x-ui-linux-amd64 ./cmd/xboard_link_3x-ui
```

Windows：

```bash
GOOS=windows GOARCH=amd64 go build -o xboard_link_3x-ui.exe ./cmd/xboard_link_3x-ui
```

## systemd

示例服务文件：

```ini
[Unit]
Description=Xboard Link 3x-ui bridge
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
```

启用：

```bash
systemctl daemon-reload
systemctl enable --now xboard_link_3x-ui
journalctl -u xboard_link_3x-ui -f
```

## 常见问题

### 3x-ui 返回 404

通常是 `xui.base_url` 没有包含 3x-ui 面板安全路径。请确认面板真实访问地址，例如：

```text
http://127.0.0.1:2053/secret-path
```

然后把完整路径写入 `xui.base_url`。

### 流量回传显示 users=0

这表示当前周期没有新增流量，属于正常情况。客户端产生代理流量后，下次推送才会显示 `users=1` 或更多。

### Xboard 连接端口填什么

填 3x-ui 入站代理端口，不是 3x-ui 面板登录端口。

## 文档

- [中文使用文档](docs/usage.zh-CN.md)
- [English Usage Guide](docs/usage.en.md)
- [开发设计文档](docs/xboard-bridge-development.md)

## 许可证

MIT License
