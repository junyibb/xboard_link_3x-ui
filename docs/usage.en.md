# Usage Guide

This guide explains how to deploy `xboard_link_3x-ui` and use it to synchronize Xboard users to 3x-ui, then report 3x-ui traffic usage back to Xboard.

## 1. Requirements

Your server should have:

- 3x-ui installed and running
- At least one 3x-ui inbound already created
- A matching node created in Xboard
- Xboard `server_token` configured
- Network access from the node server to Xboard

It is recommended to run this bridge on the same server as 3x-ui. In that setup, `xui.base_url` can point to `127.0.0.1`.

## 2. Configure 3x-ui

### Create An Inbound

Create the actual proxy inbound in the 3x-ui panel first. For example, for VLESS Reality:

- Protocol: VLESS
- Transport: RAW
- Security: Reality
- Port: for example `12345`
- SNI, dest, Short ID, public key, and other Reality parameters: use your real inbound settings

After creating the inbound, note its inbound ID. You will use it in `sync.inbound_ids`.

### Create An API Token

Create an API Token in the 3x-ui settings page and keep it for the bridge config.

If your 3x-ui panel uses a secret base path, include that path in `xui.base_url`.

For example, if the panel URL is:

```text
http://1.2.3.4:2053/panel-secret
```

then configure:

```json
"base_url": "http://127.0.0.1:2053/panel-secret"
```

## 3. Configure Xboard

Create a node in Xboard that matches the 3x-ui inbound.

The node address should be the domain or IP used by clients, for example:

```text
node.example.com
```

The connection port should be the 3x-ui inbound proxy port, for example:

```text
12345
```

Do not use the 3x-ui panel login/API port as the Xboard node connection port.

Transport mapping:

```text
3x-ui RAW         -> Xboard TCP
3x-ui WebSocket   -> Xboard Websocket
3x-ui gRPC        -> Xboard gRPC
3x-ui mKCP        -> Xboard mKCP
3x-ui HTTPUpgrade -> Xboard HttpUpgrade
3x-ui XHTTP       -> Xboard xHTTP
```

Reality, TLS, SNI, public key, Short ID, and related settings must also match the 3x-ui inbound.

## 4. Deploy The Bridge

Upload the binary to your server, for example:

```bash
/usr/local/bin/xboard_link_3x-ui
```

Make it executable:

```bash
chmod +x /usr/local/bin/xboard_link_3x-ui
```

Create the config directory:

```bash
mkdir -p /etc/xboard_link_3x-ui
```

Copy the example config:

```bash
cp config.example.json /etc/xboard_link_3x-ui/config.json
```

Edit the config:

```bash
nano /etc/xboard_link_3x-ui/config.json
```

Example:

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
    "username": "",
    "password": "",
    "two_factor_code": "",
    "timeout_seconds": 20,
    "insecure_tls": false
  },
  "sync": {
    "inbound_ids": [1],
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

Create the state directory:

```bash
mkdir -p /var/lib/xboard_link_3x-ui
```

## 5. Test Once

Run a one-shot sync:

```bash
/usr/local/bin/xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json -once
```

Successful output looks like:

```text
sync users done: desired=1 created=1 updated=0 attached=0 detached=0 deleted=0
traffic report done: users=0
```

Meaning:

- `desired`: available users returned by Xboard
- `created`: users newly created in 3x-ui
- `updated`: users updated in 3x-ui
- `attached`: inbound attachments added
- `detached`: inbound attachments removed
- `deleted`: stale managed users deleted
- `traffic report done`: traffic report result

## 6. Run Continuously

### Temporary Background Run

```bash
nohup /usr/local/bin/xboard_link_3x-ui -config /etc/xboard_link_3x-ui/config.json > /var/log/xboard_link_3x-ui.log 2>&1 &
tail -f /var/log/xboard_link_3x-ui.log
```

### systemd

Create the service file:

```bash
nano /etc/systemd/system/xboard_link_3x-ui.service
```

Content:

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

Start it:

```bash
systemctl daemon-reload
systemctl enable --now xboard_link_3x-ui
```

View logs:

```bash
journalctl -u xboard_link_3x-ui -f
```

## 7. Upgrade

Stop the service:

```bash
systemctl stop xboard_link_3x-ui
```

Replace the binary:

```bash
cp xboard_link_3x-ui-linux-amd64 /usr/local/bin/xboard_link_3x-ui
chmod +x /usr/local/bin/xboard_link_3x-ui
```

Start again:

```bash
systemctl start xboard_link_3x-ui
```

## 8. Troubleshooting

### 3x-ui Returns 404

Check whether `xui.base_url` includes the 3x-ui panel base path.

You can verify it with curl:

```bash
curl -i -H "Authorization: Bearer YOUR_3X_UI_API_TOKEN" \
"http://127.0.0.1:2053/YOUR_PANEL_BASE_PATH/panel/api/inbounds/list"
```

A `200` response means the URL is correct.

### Authentication Failed

Check:

- `xui.api_token` is correct
- The 3x-ui API Token is enabled
- `xboard.token` matches the Xboard server token

### Users Sync But Clients Cannot Connect

Check that the Xboard node settings match the 3x-ui inbound:

- Node address
- Proxy port
- Protocol
- Transport
- Reality/TLS settings
- Public key
- Short ID
- SNI

### Traffic Is Not Reported

The first run establishes a traffic baseline. After clients generate real proxy traffic, the next report cycle will push the increment to Xboard.

## 9. Security Notes

- Do not commit real `config.json`
- Do not commit real tokens, panel paths, or private keys
- Keep only placeholders in `config.example.json`
- Reality private keys should stay on your 3x-ui server
- Use a strong password and a secret base path for the 3x-ui panel
