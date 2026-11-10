# mieru-ui

[mieru](https://github.com/enfein/mieru) 代理服务端（`mita`）的 Web 管理面板，参考 [3x-ui](https://github.com/MHSanaei/3x-ui) 设计。

- 单个 Go 二进制文件，前端内嵌，不依赖任何 CDN（国内打开不受影响）
- 数据保存在一个 JSON 文件中，无需数据库
- 支持 Linux amd64 / arm64，需要 systemd

> ⚠️ 本项目尚未在真实服务器上完整验证（目前只用模拟的 mita 测试过）。首次部署请留意安装输出和面板「概览」页的「上次应用」状态，遇到报错欢迎反馈。

---

## 目录

- [功能](#功能)
- [安装](#安装)
- [首次使用](#首次使用)
- [面板功能说明](#面板功能说明)
- [客户端连接](#客户端连接)
- [命令行](#命令行)
- [卸载](#卸载)
- [安全建议](#安全建议)
- [常见问题](#常见问题)
- [工作原理](#工作原理)
- [开发与构建](#开发与构建)

---

## 功能

| 模块 | 说明 |
| --- | --- |
| 概览 | CPU / 内存 / 磁盘 / 实时网速 / 连接数；mita 启动、停止、重启、重新应用配置；查看 `mita get users`、`mita get metrics` |
| 用户管理 | 添加、编辑、删除、启用/禁用、批量添加；流量配额（滑动周期）；到期时间（到期自动禁用）；是否允许访问服务器内网/回环地址 |
| 分享 | `mierus://` 链接 + 二维码；mieru 客户端 `client.json`；mihomo（Clash.Meta）配置 |
| 端口与协议 | 多个 TCP/UDP 单端口或端口范围，自动检查重叠 |
| 防火墙 | ufw / firewalld 状态查看；关键端口检查和一键放行；添加/删除规则；启用/关闭（带防锁死保护） |
| 高级设置 | MTU、日志级别、DNS 双栈策略、多路复用、握手模式；`trafficPattern`（流量混淆）、`egress`（出站代理，如 WARP）JSON 编辑；配置预览 |
| 面板设置 | 监听 IP、端口、随机路径、HTTPS 证书、分享地址、管理员账号、备份/恢复、重启面板 |
| 日志 | 通过 journald 查看 mita 和面板日志 |

---

## 安装

### 方式一：在线安装

在服务器上以 root 用户执行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/bidabrain/mieru_ui/main/install.sh)
```

脚本会从本仓库的 [Releases](https://github.com/bidabrain/mieru_ui/releases) 下载面板，所以需要至少发布过一个版本。发布方法：推送一个 `v` 开头的 tag，GitHub Actions 会自动编译并发布 Release。

```bash
git tag v0.1.0
```

```bash
git push origin v0.1.0
```

如果你 fork 了本项目，可以用 `MIERU_UI_REPO=你的用户名/仓库名` 指定从自己的仓库下载。

### 方式二：本地编译后上传

在装有 Go 1.24+ 的机器上编译：

```bash
bash build.sh
```

会在 `dist/` 下生成 `mieru-ui-linux-amd64`、`mieru-ui-linux-arm64` 和对应的 `.tar.gz`。把对应架构的二进制和 `install.sh` 上传到服务器的同一个目录，然后执行：

```bash
bash install.sh local
```

### 安装脚本做了什么

1. 从 mieru 官方 GitHub Releases 下载最新的 `mita`（deb 或 rpm）并安装。如果已经装了 mita 会跳过。
2. 把面板安装到 `/usr/local/mieru-ui`，并注册 `mieru-ui` systemd 服务（开机自启）。
3. **首次安装**时随机生成用户名、密码、面板端口和访问路径，在最后打印出来，请务必保存。
4. 如果本机 ufw / firewalld 已启用，自动放行面板端口。

**升级**：重新运行安装命令即可。已有的账号、用户和配置会保留。

可用的环境变量：

| 变量 | 作用 | 默认值 |
| --- | --- | --- |
| `MIERU_UI_REPO` | 下载面板的 GitHub 仓库 | `bidabrain/mieru_ui` |
| `MIERU_UI_PORT` | 首次安装时的面板端口 | 10000-60000 随机 |
| `MITA_VERSION` | 指定 mita 版本，如 `v3.12.0` | 最新版 |

---

## 首次使用

1. **打开面板**：用安装完成时打印的地址登录，形如 `http://服务器IP:端口/随机路径/`。打不开时请检查云服务商安全组是否放行了面板端口。
2. **修改密码（可选）**：在「面板设置」中修改管理员账号。如果看到黄色提示「仍在使用默认账号 admin/admin」，必须立即修改。
3. **设置分享地址**：在「面板设置 → 分享地址」中填写服务器的域名或公网 IP，生成的分享链接会使用这个地址。
4. **配置端口**：在「端口与协议」中添加 mieru 的监听端口，默认是 `2012/TCP`。可以同时开 TCP 和 UDP，也可以用端口范围。
5. **放行端口**：进入「防火墙」，点「一键放行面板与 mieru 端口」，确认所有关键端口都显示「已放行」。**云服务商安全组也要放行同样的端口**。
6. **添加用户**：在「用户管理」中点「添加用户」。用户名留空会自动生成，密码默认随机。
7. **启动 mita**：回到「概览」，点「启动」，状态应变为「运行中」，「上次应用」显示「成功」。
8. **分享给客户端**：在「用户管理」中点击用户的「分享」，扫码、复制链接或下载配置文件。

---

## 面板功能说明

### 用户管理

- **流量配额**：格式是「N 天内最多 X GB」，按滑动窗口计算，由 mita 自己执行。可以设置多条，例如「1 天 10 GB」加「30 天 200 GB」。不设置即不限流量。
- **到期时间**：到期后面板会自动禁用该用户并通知 mita，每分钟检查一次。
- **启用开关**：禁用的用户会从 mita 中删除，重新启用后会再加回去。
- **内网/回环访问**：默认禁止用户通过代理访问服务器所在的内网和 `127.0.0.1`，一般不需要打开。
- **批量添加**：指定数量、前缀、有效天数和配额，一次生成多个随机用户。

每次修改用户，面板都会自动把配置应用到 mita，不需要手动重启。

### 端口与协议

- 支持单端口（如 `2012`）和端口范围（如 `3000-3010`）。
- 协议可选 TCP 或 UDP。同一协议的端口不能重叠。
- 客户端会使用所有配置的端口。端口范围可以分散流量，但安全组和防火墙需要放行整个范围。

### 防火墙

支持 **ufw**（Ubuntu/Debian）和 **firewalld**（CentOS/RHEL/Rocky 等）。

- **状态**：显示是否启用、默认策略、SSH 端口（从 sshd 配置读取）、全部规则以及原始输出。
- **关键端口检查**：自动列出 SSH、面板端口和所有 mieru 端口，标出是否已放行，可以单独或一键放行。
- **添加规则**：输入端口或范围，选择 TCP、UDP 或两者都要。
- **删除规则**：如果某条规则是 SSH 或面板端口唯一的放行规则，删除前会额外警告。
- **启用防火墙**：启用前会自动放行 SSH 端口和面板端口，避免把自己锁在服务器外。
- 如果系统只有 iptables，页面只能查看。Ubuntu/Debian 上可以在页面里一键安装 ufw。

> 云服务商的「安全组」和系统防火墙是两层独立的防火墙。面板只能管理系统防火墙，安全组需要在云服务商后台设置。

### 高级设置

| 选项 | 说明 |
| --- | --- |
| MTU | 1280-1400，只对 UDP 传输生效，默认 1400 |
| 日志级别 | 默认 INFO。排查问题时可改为 DEBUG，日志量会很大 |
| DNS 双栈策略 | 服务器解析目标域名时 IPv4/IPv6 的优先级 |
| 多路复用 | 写入分享链接和客户端配置，由客户端使用 |
| 握手模式 | 写入分享链接和客户端配置。`HANDSHAKE_NO_WAIT` 可以降低首包延迟 |
| trafficPattern | 流量模式，用来打乱包长、分片和随机数特征。点「填入示例」可以得到一个模板 |
| egress | 出站代理，例如把流量转给本机 WARP 的 SOCKS5 端口，并按规则分流 |

「预览 mita 配置」可以同时看到面板生成的配置和 mita 实际在用的配置（`mita describe config`），方便排查问题。

> `trafficPattern` 和 `egress` 的字段请以 mieru 官方文档为准。字段写错时，mita 会拒绝应用，报错会显示在「概览 → 上次应用」中。

### 面板设置

- **监听 IP**：留空表示监听所有地址。填 `127.0.0.1` 后只能通过 SSH 隧道访问，最安全。
- **端口、路径、证书**：修改后点「重启面板」才会生效，之后用新地址访问。
- **TLS 证书**：填写证书和私钥文件的路径后，面板会改用 HTTPS。
- **备份/恢复**：备份文件包含所有用户和服务端配置，不包含面板账号。恢复会覆盖当前的用户和配置。

---

## 客户端连接

在「用户管理 → 分享」中有三种格式：

### 1. mierus:// 链接或二维码

适用于支持该格式的 mieru 客户端。使用官方命令行客户端时：

```bash
mieru import config 'mierus://...'
```

```bash
mieru start
```

### 2. mieru 客户端配置文件

下载 `client.json` 后执行：

```bash
mieru apply config client.json
```

```bash
mieru start
```

本地会开启 SOCKS5 端口 `1080` 和 HTTP 代理端口 `8080`。

### 3. mihomo / Clash.Meta

复制生成的内容，粘贴到配置文件的 `proxies:` 部分。每个端口会生成一个节点。需要 mihomo 1.18.5 或更高版本；使用 UDP 传输需要更新的版本。Clash Verge Rev、FlClash 等基于 mihomo 内核的客户端都可以使用。

---

## 命令行

```bash
mieru-ui setting -show
```

查看当前的面板用户名、端口、路径和数据文件位置。

```bash
mieru-ui setting -username admin -password '新密码'
```

重置面板账号，适合忘记密码时使用。

```bash
mieru-ui setting -port 54321 -basepath /my-secret-path/
```

修改面板端口和路径。

```bash
mieru-ui setting -listen 127.0.0.1
```

只监听本机，之后通过 SSH 隧道访问。

```bash
mieru-ui version
```

修改设置后需要重启面板：

```bash
systemctl restart mieru-ui
```

服务管理：

```bash
systemctl status mieru-ui
```

```bash
journalctl -u mieru-ui -f
```

```bash
journalctl -u mita -f
```

---

## 卸载

### 只卸载面板（保留数据和 mita）

```bash
bash install.sh uninstall
```

- 删除 `mieru-ui` 服务、`/usr/local/mieru-ui` 和 `/usr/bin/mieru-ui`
- **保留**面板数据 `/etc/mieru-ui`，重装后可以继续使用
- **保留** mita，代理服务不受影响，用户仍可连接

### 彻底卸载（面板、数据和 mita 全部删除）

```bash
bash install.sh purge
```

需要输入 `yes` 确认。会依次：

1. 卸载面板（同上）
2. 停止 mieru 代理，卸载 mita 软件包（`apt purge` 或 `rpm -e`）
3. 删除 mita 配置 `/etc/mita` 和面板数据 `/etc/mieru-ui`

如果在脚本中使用、不想交互确认：

```bash
bash install.sh purge -y
```

> purge 无法撤销，所有用户会立即断开。执行前建议在「面板设置」中下载备份。
>
> 两种卸载都**不会改动防火墙规则**。如需关闭之前放行的端口，请用 `ufw delete ...` 或 `firewall-cmd --remove-port=...` 手动删除，并检查云服务商的安全组。

---

## 安全建议

- **不要用明文 HTTP 暴露面板**。推荐的方式有两种：
  - 配置 TLS 证书，在「面板设置」中填写证书和私钥路径；
  - 让面板只监听 `127.0.0.1`，通过 SSH 隧道访问：

    ```bash
    ssh -L 2053:127.0.0.1:面板端口 root@服务器IP
    ```

    然后在本地浏览器打开 `http://127.0.0.1:2053/路径/`。
- 保持随机的面板路径，并使用强密码。
- 同一 IP 登录失败 5 次会被锁定 5 分钟。
- 定期下载备份。

---

## 常见问题

**面板打不开？**

1. 先确认面板在运行：`systemctl status mieru-ui`。
2. 检查云服务商安全组是否放行了面板端口。
3. 运行 `mieru-ui setting -show`，确认端口和路径正确。注意路径末尾有 `/`。

**客户端连不上？**

1. 「概览」中 mita 状态是否为「运行中」，「上次应用」是否成功？
2. 「防火墙 → 关键端口检查」中 mieru 端口是否都已放行？云服务商安全组也放行了吗？
3. 「面板设置 → 分享地址」是否填写了正确的公网 IP 或域名？
4. 用户是否启用、是否已到期、是否超出流量配额？
5. 查看 mita 日志：`journalctl -u mita -n 100`。

**「上次应用」显示失败？**

把报错复制下来，常见原因有：

- mita 未安装或路径不对：在「面板设置 → mita 路径」中检查。
- trafficPattern 或 egress 的 JSON 字段不被 mita 接受。
- 没有任何用户：mita 可能拒绝启动，先添加至少一个用户。

**忘记面板密码？**

在服务器上执行：

```bash
mieru-ui setting -username admin -password '新密码'
```

```bash
systemctl restart mieru-ui
```

**IP 被墙了怎么办？**

换协议或端口通常解决不了 IP 被封的问题。需要更换服务器 IP，或在前面加一层中转。mieru 的作用是降低协议被识别的概率，它不能防止 IP 被封。

---

## 工作原理

面板不直接修改 mita 的内部配置文件，所有操作都通过 mita 官方命令行完成：

1. 把面板中的用户、端口和高级设置生成一份完整的 mita 服务端配置，写入 `/etc/mieru-ui/mita-server.json`。
2. 执行 `mita apply config <文件>`。mita 会按用户名合并用户，并整体替换端口配置。
3. 对已删除、已禁用或已到期的用户执行 `mita delete user <名字>`。
4. 如果 mita 正在运行，执行 `mita reload` 让新配置生效。

界面上的每次修改都会自动执行这个流程。另外，面板每次启动时会重新应用一次配置，每分钟检查一次用户是否到期。

文件位置：

| 路径 | 内容 |
| --- | --- |
| `/usr/local/mieru-ui/mieru-ui` | 面板程序 |
| `/etc/mieru-ui/mieru-ui.json` | 面板数据（账号、用户、配置） |
| `/etc/mieru-ui/mita-server.json` | 最近一次生成的 mita 配置 |
| `/etc/systemd/system/mieru-ui.service` | systemd 服务 |

---

## 开发与构建

需要 Go 1.24 或更高版本。

本地运行。数据会写到 `./data`；在 Windows 上不设置 `MIERU_UI_DATA` 时也默认使用 `./data`：

```bash
MIERU_UI_DATA=./data go run .
```

然后打开 http://127.0.0.1:2053/ ，默认账号是 `admin` / `admin`。在没有安装 mita 的机器上，界面可以正常使用，只是应用配置时会提示找不到 mita。系统状态和防火墙功能只在 Linux 上可用。

运行测试：

```bash
go test ./...
```

交叉编译 Linux 版本，输出到 `dist/`：

```bash
bash build.sh
```

发布新版本：推送 `v*` tag，GitHub Actions（`.github/workflows/release.yml`）会自动编译并上传 Release。

### 目录结构

```
main.go              命令行入口（run / setting / version）
config/              版本号与路径
database/            JSON 文件存储、管理员密码哈希（PBKDF2）
service/             mita 命令封装、配置生成与应用、分享链接、系统状态、防火墙、到期任务
web/                 HTTP 服务、登录会话、API
web/assets/          前端（原生 JS，编译时嵌入二进制）
install.sh           安装 / 升级 / 卸载脚本
build.sh             交叉编译脚本
```

---

## License

MIT
