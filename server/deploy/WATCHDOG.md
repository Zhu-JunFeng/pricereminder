# 行情与代理巡检

`price-watchdog` 使用 systemd 每 10 分钟检查一次。本地锁避免手工运行与定时运行重叠；程序 7 分 30 秒超时并预留 20 秒回退，systemd 的启动上限为 8 分钟。

## 判定与恢复

- `/healthz` 新增 `activeSubscriptions`，来自与行情订阅相同的有效设备订阅查询。数据库查询失败返回 503；旧字段在成功响应中保留。
- 有订阅且行情事件超过 60 秒未更新，10 秒后复查；无订阅不检查行情年龄。REST 连续三次失败（每次 8 秒）也触发修复。
- 先验证服务、健康接口及 Mihomo 手动选择组；基础设施检查失败不重启服务，也不轮换代理。
- 从 `pqjc` 获取真实节点，最多 8 路并发 Binance 测速，要求 HTTP 200，按延迟排序，最多实际尝试 5 个。测速不能证明 GET 响应或行情可用；候选必须通过 REST 时间数据和独立 BTCUSDT WebSocket 三条新事件验证。
- 只修改“节点选择”；保留 `profile.store-selected: true`。仅关闭 `ss` 确认属于 `pricereminder.service` MainPID 的 Binance 代理连接。其他进程连接不变。
- 验证行情在 60 秒内恢复并观察 30 秒，再复查 REST。全部失败恢复原选择；仅在本轮曾请求行情重连时才再次重连，避免打断一直正常的原连接。
- 每个节点连续两次候选验证失败，冷却 30 分钟。所有候选均冷却时允许重试。状态文件损坏时明确失败，不覆盖；敏感信息不写入日志。
- 当前检查的是服务端订阅总体事件进度，不能保证每个稀疏成交合约均有新成交，也不能远程判断休眠 Mac 的界面状态。

## 安装与查看

构建 `server/cmd/price-watchdog` 为 Linux amd64 二进制，安装至 `/opt/pricereminder/price-watchdog`。同时更新主服务以提供新的健康字段，替换前备份二进制并验证代理能访问 Binance 合约接口。将两个 unit 文件安装到 `/etc/systemd/system/`。

```sh
systemctl daemon-reload
/opt/pricereminder/price-watchdog --check-only
systemctl start pricereminder-watchdog.service
systemctl enable --now pricereminder-watchdog.timer
systemctl list-timers pricereminder-watchdog.timer
journalctl -u pricereminder-watchdog.service --since today --no-pager
```

默认本地接口为健康端口 18443、代理端口 7890、控制端口 9090。可用命令行参数覆盖地址；若控制器设置密钥，通过权限 600 的 `/etc/pricereminder-watchdog.env` 提供 `MIHOMO_CONTROLLER_SECRET`。不要把订阅地址或密钥提交到仓库。

状态保存在 `/var/lib/pricereminder-watchdog/state.json`。`--check-only` 不切换节点、不修改冷却状态（仍获取本地运行锁）。异常退出码为 1，日志包含原因；无外部通知。

## 停用与回退

```sh
systemctl disable --now pricereminder-watchdog.timer
systemctl stop pricereminder-watchdog.service
```

停止正在运行的巡检会发送 SIGTERM，程序尝试在 20 秒内恢复本轮原节点。日志中的 `rolled back` 用于确认结果。关闭定时器不会自动改变上一次成功选择的节点。

需要恢复主服务时，将本次部署前保存的 `pricereminder.backup-watchdog-*` 二进制复制回 `/opt/pricereminder/pricereminder`，再重启主服务并检查健康与行情时间。

2026-10-27 部署预检发现本机 UDP DNS 给 `fapi.binance.com` 返回异常地址，而经代理的加密 DNS 返回正常 AWS 地址。部署时单独为 `fapi.binance.com`、`fstream.binance.com` 设置了经 `PROXY` 的 Cloudflare/Google DoH，配置校验、热加载后清除旧 DNS 缓存。原配置备份为 `/etc/mihomo/config.yaml.backup-watchdog-20261027-111052`；此修正是部署操作，巡检程序不会自动改 DNS 配置。

若需撤销 DNS 修正，恢复该备份，先运行 `mihomo -t` 验证再热加载；旧解析可能再次影响行情。更早的 `/etc/mihomo/config.yaml.backup-20261027-104430` 还会恢复自动选择组，不应当作仅撤销本次部署的备份。

控制器测速与指定代理 DNS 的用法参考 [Mihomo 控制器实现](https://github.com/MetaCubeX/mihomo/blob/Meta/hub/route/proxies.go) 和 [DNS 配置文档](https://wiki.metacubex.one/config/dns/)。

## 2026-10-27 部署验收

- 主服务备份：`/opt/pricereminder/pricereminder.backup-watchdog-20261027-111257`。
- 部署主服务 SHA-256：`b6f6485c11d05da41783ebe4bdcc5712ab24cce328aee777808db03527af13d2`。
- 巡检二进制 SHA-256：`54b892032bc813d8f438f5ce7d46416fe565f4a91e04b8a271e0d099fcff4531`，与本地构建一致。
- `go test ./...`、新增 watchdog/api 竞态检查、`go vet ./...`、systemd unit 校验通过。需专门环境变量的原有 E2E 测试未运行；未向生产注入测试提醒。
- 11:14 的只读巡检发现一次 REST 错误，重试恢复，正确判定无需切换。
- 11:15:57 首轮正式巡检遇到真实连续 REST 失败，自动切至日本东京 04；独立 WebSocket 验证通过，仅关闭价格服务的 1 条连接，主服务行情恢复并观察 30 秒，11:17:52 再次通过 REST 检查，正常退出。整个过程约 115 秒。
- 同期本机 ETHUSDT、BTCUSDT、SNDKUSDT 行情缓存持续推进；本地缓存约每 15 秒落盘，文件年龄不能直接当作网络延迟。
- 11:20:00 定时器自主触发，11:20:02 检查正常退出：行情年龄 75 毫秒、REST 首次成功，未切换节点。定时器为 enabled，下一次执行 11:30:00。
