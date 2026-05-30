# 使用文档

本文档说明如何从零部署 `xboard_link_3x-ui`，并完成 Xboard 到 3x-ui 的用户同步与流量回传。

## 一、准备条件

服务器上需要：

- 已安装并运行 3x-ui
- 3x-ui 中已创建好入站
- Xboard 中已添加对应节点
- Xboard 后台已配置 `server_token`
- 服务器能访问 Xboard

本服务建议和 3x-ui 部署在同一台服务器上，这样 `xui.base_url` 可以使用 `127.0.0.1`。

## 二、配置 3x-ui

### 1. 创建入站

在 3x-ui 面板中创建入站。例如 VLESS Reality：

- 协议：VLESS
- 传输：RAW
- 安全：Reality
- 端口：例如 `12345`
- SNI/dest/shortId/public key 等参数按实际需求配置

创建后记下入站 ID。这个 ID 是 3x-ui 入站列表里的 ID，不是 Xboard 的节点 ID。

### 2. 创建 API Token

在 3x-ui 设置中创建 API Token，并保存下来。

如果 3x-ui 开启了安全路径，例如面板地址是：

```text
http://1.2.3.4:2053/panel-secret
```

那么配置中的 `xui.base_url` 应写为：

```json
"base_url": "http://127.0.0.1:2053/panel-secret"
```

## 三、配置 Xboard

在 Xboard 后台添加节点。

节点地址填写用户客户端连接的域名或 IP，例如：

```text
node.example.com
```

连接端口填写 3x-ui 入站代理端口，例如：

```text
12345
```

不要填写 3x-ui 面板登录端口。

传输协议需要和 3x-ui 入站保持一致：

```text
3x-ui RAW         -> Xboard TCP
3x-ui WebSocket   -> Xboard Websocket
3x-ui gRPC        -> Xboard gRPC
3x-ui mKCP        -> Xboard mKCP
3x-ui HTTPUpgrade -> Xboard HttpUpgrade
3x-ui XHTTP       -> Xboard xHTTP
```

Reality、TLS、SNI、Public Key、Short ID 等也必须和 3x-ui 入站一致。

## 四、部署程序

上传二进制到服务器，例如：

```bash
/usr/local/bin/xboard_link_3x-ui
```

增加执行权限：

```bash
chmod +x /usr/local/bin/xboard_link_3x-ui
```

创建配置目录：

```bash
mkdir -p /etc/xboard_link_3x-ui
```

复制配置：

```bash
cp config.example.json /etc/xboard_link_3x-ui/config.json
```

编辑配置：

```bash
nano /etc/xboard_link_3x-ui/config.json
```

配置示例：

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
    "username": "",
    "password": "",
    "two_factor_code": "",
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
    "state_file": "/var/lib/xboard_link_3x-ui/state.json"
  },
  "log": {
    "level": "info"
  }
}
```

`xui.node_id` 是 3x-ui 入站 ID。例如目标入站在 3x-ui 入站列表里显示 ID 为 `3`，就填写：

```json
"node_id": 3
```

如果一个 Xboard 节点需要同时绑定多个 3x-ui 入站，可以使用：

```json
"node_ids": [1, 3]
```

为了兼容旧配置，程序仍然支持 `sync.inbound_ids`，但新配置推荐使用 `xui.node_id` 或 `xui.node_ids`。

创建状态目录：

```bash
mkdir -p /var/lib/xboard_link_3x-ui
```

## 五、测试

单次同步：

```bash
/usr/local/bin/xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json -once
```

成功日志示例：

```text
sync users done: desired=1 created=1 updated=0 attached=0 detached=0 deleted=0
traffic report done: users=0
```

含义：

- `desired`: Xboard 当前返回的可用用户数
- `created`: 本次新增到 3x-ui 的用户数
- `updated`: 本次更新的用户数
- `attached`: 本次新增绑定的入站数量
- `detached`: 本次解绑的入站数量
- `deleted`: 本次删除的不再可用用户数
- `traffic report done`: 本次流量回传结果

## 六、常驻运行

### 临时后台运行

```bash
nohup /usr/local/bin/xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json > /var/log/xboard_link_3x-ui.log 2>&1 &
tail -f /var/log/xboard_link_3x-ui.log
```

### systemd 运行

创建服务文件：

```bash
nano /etc/systemd/system/xboard_link_3x-ui.service
```

内容：

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

启动：

```bash
systemctl daemon-reload
systemctl enable --now xboard_link_3x-ui
```

查看日志：

```bash
journalctl -u xboard_link_3x-ui -f
```

## 七、升级

停止服务：

```bash
systemctl stop xboard_link_3x-ui
```

替换二进制：

```bash
cp xboard_link_3x-ui-linux-amd64 /usr/local/bin/xboard_link_3x-ui
chmod +x /usr/local/bin/xboard_link_3x-ui
```

启动服务：

```bash
systemctl start xboard_link_3x-ui
```

## 八、排错

### 3x-ui 返回 404

检查 `xui.base_url` 是否包含面板安全路径。

可以用 curl 验证：

```bash
curl -i -H "Authorization: Bearer YOUR_3X_UI_API_TOKEN" \
"http://127.0.0.1:2053/YOUR_PANEL_BASE_PATH/panel/api/inbounds/list"
```

返回 `200` 才表示路径正确。

### 认证失败

检查：

- `xui.api_token` 是否正确
- 3x-ui API Token 是否启用
- `xboard.token` 是否和 Xboard 后台 server token 一致

### 用户同步成功但客户端无法连接

检查 Xboard 节点配置是否和 3x-ui 入站一致：

- 节点地址
- 代理端口
- 协议
- 传输协议
- Reality/TLS 参数
- Public Key
- Short ID
- SNI

### 流量没有回传

第一次运行时会建立流量基线。用户真实产生代理流量后，下一个回传周期才会上报增量。

## 九、安全建议

- 不要提交真实 `config.json`
- 不要提交真实 token、面板路径、私钥
- `config.example.json` 只保留占位符
- Reality Private Key 只保存在 3x-ui 服务器，不要公开
- 建议给 3x-ui 面板设置安全路径和强密码
