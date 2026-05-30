# Xboard 对接 3x-ui 开发文档

## 目标

本项目新增一个独立 Go 服务 `xboard_link_3x-ui`，部署在 3x-ui 所在 Linux 服务器上运行。服务通过 Xboard 的 UniProxy 节点接口拉取可用用户，再通过 3x-ui Panel API 创建、更新、删除 3x-ui 客户端，并周期性把 3x-ui 记录到的用户流量增量回传给 Xboard。

这样可以实现类似 `xboard_link_3x-ui` 的核心能力：

- Xboard 作为主控面板，负责套餐、用户、到期、封禁和流量扣减。
- 3x-ui 作为落地节点，负责 Xray 入站配置和真实代理服务。
- 本服务作为桥接器，负责用户同步和流量回传。

## 已确认接口

### Xboard UniProxy

Xboard 服务端接口来自公开 Xboard 代码：

- `GET /api/v1/server/UniProxy/user`
- `GET /api/v1/server/UniProxy/config`
- `POST /api/v1/server/UniProxy/push`

所有请求带查询参数：

- `token`: Xboard 后台的 `server_token`
- `node_id`: Xboard 节点 ID 或 code
- `node_type`: 节点协议类型，例如 `vless`、`vmess`、`trojan`、`shadowsocks`

用户接口返回形态：

```json
{
  "users": [
    {
      "id": 1,
      "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "speed_limit": 0,
      "device_limit": 0
    }
  ]
}
```

流量回传接口接受以用户 ID 为 key 的对象，值为 `[upload_delta, download_delta]`：

```json
{
  "1": [1048576, 2097152]
}
```

### 3x-ui Panel API

当前目录提供的 `openapi.json` 显示 3x-ui 支持 Bearer Token 或登录 Cookie 鉴权。本服务优先使用 Bearer Token。

本服务使用的 3x-ui API：

- `GET /panel/api/clients/list`
- `POST /panel/api/clients/add`
- `POST /panel/api/clients/update/{email}`
- `POST /panel/api/clients/del/{email}?keepTraffic=0`
- `POST /panel/api/clients/{email}/attach`
- `POST /panel/api/clients/{email}/detach`
- `GET /panel/api/clients/traffic/{email}`

## 同步规则

### 用户标识

3x-ui 的客户端唯一键是 `email`。Xboard UniProxy 用户接口只返回用户 ID 和 UUID，不返回邮箱，因此桥接器生成稳定 email：

```text
<email_prefix>-<node_type>-<node_id>-<user_id>
```

默认示例：

```text
xboard-vless-1-10001
```

这个 email 只作为 3x-ui 内部标识，不影响 Xboard 订阅链接里使用的 UUID。

### 密钥映射

为了兼容不同 3x-ui 协议字段，本服务会同时写入：

- `uuid`: Xboard 用户 UUID
- `id`: Xboard 用户 UUID
- `password`: Xboard 用户 UUID

VLESS/VMess 通常使用 UUID；Trojan/Shadowsocks 可使用 password 字段。多余字段由 3x-ui 忽略或保存在客户端记录中。

### 配额与到期

Xboard UniProxy `/user` 只返回当前可用用户，不返回总流量和到期时间。因此 3x-ui 客户端默认设置为：

- `totalGB: 0`
- `expiryTime: 0`
- `enable: true`

用户是否可用由 Xboard 决定：当用户套餐耗尽、到期或封禁后，Xboard `/user` 不再返回该用户；桥接器随后会从 3x-ui 删除该托管客户端。

### 入站绑定

3x-ui 的真实入站由管理员在 3x-ui 面板中提前创建，桥接器只负责把用户绑定到配置文件中的 `xui.node_id` 或 `xui.node_ids`。旧版 `sync.inbound_ids` 仍保留兼容，但不推荐新配置使用。

如果以后需要根据 Xboard `/config` 自动创建入站，可在第二阶段增加入站模板生成逻辑。

### 流量回传

3x-ui 返回的是每个客户端累计 `up/down`。Xboard 需要增量流量。因此桥接器在本地 `state_file` 中保存上次已推送的计数：

1. 读取 3x-ui 当前累计流量。
2. 与本地状态比较，计算正向增量。
3. 推送 `{user_id: [up_delta, down_delta]}` 到 Xboard。
4. 推送成功后更新本地状态。

如果 3x-ui 侧计数被重置，桥接器会把当前值当成新的基线，避免重复扣费。

Xboard 用户列表的 ETag 只在进程内缓存，用于减少运行期间的重复传输；服务重启后会强制完整拉取一次用户列表，保证能修复 3x-ui 侧可能出现的手动改动。

## 配置

复制 `config.example.json` 为生产配置，例如：

```bash
cp config.example.json /etc/xboard_link_3x-ui/config.json
```

关键配置：

- `xboard.base_url`: Xboard 站点地址，例如 `https://panel.example.com`
- `xboard.token`: Xboard 后台 server token
- `xboard.node_id`: Xboard 节点 ID 或 code
- `xboard.node_type`: Xboard 节点类型
- `xui.base_url`: 3x-ui 面板地址
- `xui.api_token`: 3x-ui 设置里的 API Token
- `xui.node_id`: 要绑定的单个 3x-ui 入站 ID
- `xui.node_ids`: 要绑定的多个 3x-ui 入站 ID
- `sync.inbound_ids`: 旧版兼容字段
- `sync.delete_stale`: 是否删除不再由 Xboard 返回的托管用户
- `sync.state_file`: 流量状态文件路径

## 运行

开发环境：

```bash
go run ./cmd/xboard_link_3x-ui -config config.example.json -once
```

Linux 构建：

```bash
GOOS=linux GOARCH=amd64 go build -o xboard_link_3x-ui ./cmd/xboard_link_3x-ui
```

后台运行：

```bash
./xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json
```

## systemd 示例

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

## 第一阶段范围

已纳入第一版：

- 拉取 Xboard 可用用户
- 同步新增用户到 3x-ui
- 更新已存在用户 UUID、启用状态、IP 限制
- 绑定/解绑配置中的 3x-ui 入站
- 删除 Xboard 不再返回的托管用户
- 读取 3x-ui 流量并增量回传 Xboard
- 本地状态文件防重复上报

暂不纳入第一版：

- 自动创建/修改 3x-ui 入站配置
- 自动生成 3x-ui Reality/TLS/WS 参数
- 上报在线 IP 列表和机器负载
- 多 Xboard 节点复用同一进程

这些能力可以在第二阶段基于当前同步器继续扩展。
