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
- [Android 客户端](#android-客户端)
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
| 分享 | `mierus://` 链接 + 二维码；mieru 客户端 `client.json`；mihomo（Clash.Meta）完整配置或仅节点 |
| mihomo 配置 | 定制 mihomo 完整配置模板：绕过大陆 / GFW 列表 / 全局三种分流模式，广告拦截，指定服务强制代理，自定义直连/代理/拦截域名和高级规则，代理组、DNS、规则数据源；实时预览。分享时按模板实时生成 |
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
2. 下载面板并**校验 SHA256**（与 Release 中的 `SHA256SUMS` 比对，不一致则中止），安装到 `/usr/local/mieru-ui`，注册 `mieru-ui` systemd 服务（开机自启）。
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
2. **修改密码（可选）**：在「面板设置」中修改管理员账号，密码至少 8 位。如果不是用安装脚本安装、而是直接运行程序，首次启动会生成随机密码并写入 `/etc/mieru-ui/initial-credentials.txt`（仅 root 可读），修改密码后该文件会自动删除。
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

### mihomo 配置

在这里定制一份 mihomo（Clash.Meta）完整配置的模板。保存后，「用户管理 → 分享 → mihomo」里的配置会按模板**实时生成**：每次打开分享都使用当前的模板、端口和该用户的账号密码，无需手动同步。

- **分流模式**
  - 绕过大陆（推荐）：`GEOSITE,cn` 和 `GEOIP,CN` 直连，其余走代理。
  - GFW 列表：只有 `GEOSITE,gfw` 走代理，其余直连。
  - 全局代理：除直连规则外全部走代理。
- **规则选项**：局域网直连、广告拦截（`category-ads-all`）、Apple 和微软国内服务直连；可以勾选 Google、YouTube、Telegram、AI 服务（ChatGPT / Claude / Gemini 等）、GitHub、Netflix 等服务强制走代理。
- **自定义规则**：直连、代理、拦截域名各一个列表，每行一个，按后缀匹配。另有「高级规则」，每行一条 `类型,值,DIRECT|REJECT|PROXY[,no-resolve]`，`PROXY` 代表主代理组。
- **规则优先级**（从高到低）：高级规则 → 拦截域名 → 直连域名 → 代理域名 → 局域网 → 广告 → 强制代理的服务 → Apple/微软 → 分流模式。
- **代理组**：主代理组（手动选择）、自动选择（url-test）、故障转移（fallback）。每个 mieru 端口生成一个节点。
- **DNS**：fake-ip / redir-host、国内主 DNS、境外备用 DNS、fake-ip 过滤列表。默认值适合国内使用。
- **规则数据**：GeoSite / GeoIP 数据默认从 jsDelivr 镜像下载（国内可访问），可设置自动更新间隔。
- 右侧**实时预览**可以选择任意用户，查看按当前表单（包括未保存的修改）生成的配置。
- 模板会包含在面板备份里。

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

默认生成**完整配置**（按「mihomo 配置」页面的模板），复制或下载 `config.yaml` 后，可直接导入 Clash Verge Rev、FlClash 等基于 mihomo 内核的客户端。

如果你已有自己的配置，只想加节点，勾选「只显示节点（proxies）」，把内容粘贴到配置文件的 `proxies:` 部分即可。

需要 mihomo 1.18.5 或更高版本才支持 mieru；使用 UDP 传输需要更新的版本。

---

## Android 客户端

`android/` 目录是一个独立的 mieru 安卓客户端（Mieru，图标取「見える」之意：一只眼睛站在「儿」上，即「見」字的本义）。

- **只需要 `mierus://` 分享链接**：扫面板分享页的二维码、从剪贴板粘贴，或直接点开链接即可导入；也支持面板导出的 mieru 客户端 JSON
- **分流（参考 husi）**：绕过大陆 / GFW 列表 / 全局 / 直连四种模式；绕过局域网、拦截广告、Apple/微软国内直连、流量嗅探；自定义规则（域名后缀、域名、关键词、GEOSITE、GEOIP、IP 段、端口 → 代理 / 直连 / 拦截，可排序）；分应用代理；直连 / 远程 DNS、Fake-IP
- **内核**：mihomo（其 mieru 出站直接使用 mieru 官方 Go 库，App 内升级到 mieru 最新版），以 VpnService + TUN（gVisor）接管全局流量；配置由 App 根据节点和分流设置实时生成，用户无需接触 Clash 配置
- **界面**：Material 3 Expressive + 磨砂玻璃（Android 12+ 实时背景模糊），支持深色模式和动态取色
- 节点测速、实时网速与流量、运行日志、快捷设置磁贴、本地 HTTP/SOCKS5 端口（默认 `127.0.0.1:17890`）
- **应用内更新**：检查本仓库 `android-v*` Release，下载后直接覆盖安装

### 下载安装

在 [Releases](https://github.com/bidabrain/mieru_ui/releases) 中找到 `Mieru Android x.y.z`（注意不是标记为 Latest 的面板版本），大多数手机下载 `mieru-android-x.y.z-arm64-v8a.apk`，不确定就下 `universal`。要求 Android 8.0+。

首次连接时会依次请求：通知权限（显示网速，可拒绝）、附近设备 / 本地网络权限（Android 17+，仅在需要访问局域网地址时用到，可拒绝）、VPN 权限（必须允许）。

> 所有 APK 都用仓库里固定的调试签名（`android/app/debug.keystore`，公开的）签名，所以新版本总能覆盖安装。代价是任何人都能用同一个签名打包——请只从本仓库 Release 或 App 内更新下载。

### 发布新版本

推送 `android-v1.2.3` 形式的 tag，`.github/workflows/android.yml` 会编译 Go 内核（gomobile）和 APK，并发布 Release（不会占用 Latest，面板安装脚本不受影响）。

### 本地编译

需要 Go、JDK 17+、Android SDK（含 NDK）：

```bash
bash android/core/build.sh
```

```bash
cd android && ./gradlew assembleRelease
```

内核测试：`cd android/core && go test -tags cmfa,with_gvisor ./...`

Android 部分链接了 mihomo（GPL-3.0），因此 `android/` 目录以 GPL-3.0 发布，见 `android/LICENSE`。

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
mieru-ui setting -mita /usr/bin/mita
```

指定 mita 程序路径。出于安全考虑，这一项只能在服务器上用命令行修改，网页上是只读的。

```bash
mieru-ui setting -logout-all
```

让所有已登录的会话失效（重启面板后生效）。网页上「面板设置 → 退出所有设备」可以立即生效。

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
- 怀疑密码或 cookie 泄露时：修改密码，或点「面板设置 → 退出所有设备」，所有已登录会话会立即失效。
- 定期下载备份。

### 面板已有的安全措施

本面板参考了 x-ui / 3x-ui 过去出现过的安全问题，例如默认密码、更新下载不校验证书、通过面板设置写文件导致 root 代码执行、改密码后旧会话仍有效、CSRF 等，做了以下防护：

| 措施 | 说明 |
| --- | --- |
| 无默认密码 | 首次运行随机生成密码，不存在通用的 admin/admin |
| 安装校验 | 全程 HTTPS 下载（不跳过证书校验），面板文件还会校验 SHA256 |
| 网页不能改执行路径 | 面板会以 root 身份执行 mita，mita 路径只能在服务器命令行修改，即使面板密码泄露也无法让它执行任意程序 |
| 备份恢复受限 | 恢复只接受用户和 mieru 配置，全部经过校验，不涉及任何文件路径 |
| 会话安全 | 签名 cookie（HttpOnly、SameSite=Strict，启用 HTTPS 时加 Secure）；修改密码或「退出所有设备」后旧会话立即失效 |
| CSRF 防护 | 所有修改操作都要求自定义请求头，跨站请求无法伪造 |
| XSS 防护 | 前端对所有输出做转义，并设置严格的 Content-Security-Policy，只允许运行面板自带的脚本 |
| 防爆破 | 密码使用 PBKDF2（21 万次迭代）存储，常量时间比较；同一 IP 连续失败 5 次锁定 5 分钟 |
| 命令注入防护 | 调用 mita / ufw / firewall-cmd 时不经过 shell，端口、协议、用户名都经过严格格式校验 |
| 无公开接口 | 除登录外所有接口都需要登录，没有订阅链接之类的免登录入口 |
| 文件权限 | 数据目录 700，数据和生成的配置文件 600 |
| 连接超时 | 设置了读写和空闲超时，缓解慢速连接攻击；启用 HTTPS 时最低 TLS 1.2 |

> 仍然存在的风险：面板默认是明文 HTTP；面板必须以 root 运行，才能管理 mita 和防火墙。所以面板账号本身就等同于服务器的高权限，请务必按上面的建议保护好它。

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

- mita 未安装或路径不对：在「面板设置」中查看当前 mita 路径，需要修改时在服务器上执行 `mieru-ui setting -mita /path/to/mita`，然后重启面板。
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

然后打开 http://127.0.0.1:2053/ ，用户名是 `admin`，密码在 `./data/initial-credentials.txt` 里。在没有安装 mita 的机器上，界面可以正常使用，只是应用配置时会提示找不到 mita。系统状态和防火墙功能只在 Linux 上可用。

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
android/core/        Android 内核（Go，gomobile）：链接解析、mihomo 配置生成、启停 TUN
android/app/         Android App（Kotlin + Jetpack Compose）
```

---

## License

面板：MIT。Android 客户端（`android/`）：GPL-3.0。
