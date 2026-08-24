# 可选服务端 API 与 WebSocket 协议

本协议用于终端无法直连币安时的行情中继，以及 iOS 后台提醒、后台 Live Activity 和后台触发记录同步。iPhone 前台、Android、华为和 macOS 均优先直连币安并在终端本地判断；直连不可用时调用行情中继。iPhone 进入后台后由服务端接管规则计算并通过 APNs 提醒。

所有时间均为 Unix 毫秒，所有价格和百分比均使用十进制字符串，禁止 JSON 浮点数参与规则计算。

## HTTP

### `POST /v1/devices/register`

匿名注册安装实例。响应：

```json
{
  "deviceId": "uuid",
  "token": "一次性返回的随机令牌",
  "expiresAt": 1789958400000
}
```

### `POST /v1/devices/refresh`

使用 `Authorization: Bearer <token>` 滚动续期。

### `GET /v1/contracts`

返回可交易的 U 本位永续合约及币安价格精度。

### `PUT /v1/subscriptions`

提交本设备需要的去重合约列表，最多 50 个。

```json
{ "symbols": ["BTCUSDT", "ETHUSDT"] }
```

### `PUT /v1/ios/rules`

iOS 原子提交完整规则快照、规则版本、`monitoringEnabled` 和方向状态；服务端只保存用于后台接管的最小副本。旧版本返回 `409 stale_rule_version`。

### `GET /v1/ios/rules`

读取服务端最新规则快照。iPhone 回到前台时先合并服务端产生的上涨/下跌触发状态，再提交新版本，避免前后台交接造成重复提醒。

### `POST /v1/ios/lease`

iOS 前台每 10 秒续租，租约有效期 25 秒。服务端仅在租约超时后判断规则。

### `PUT /v1/ios/push-token`

保存普通 APNs device token 及 `sandbox`/`production` 环境。

### `PUT /v1/ios/live-activities`

保存 Live Activity push token、合约和过期时间；同一活动 ID 重复提交会更新 token。

### `DELETE /v1/ios/live-activities/{activityId}`

结束活动时删除对应 token。

### `GET /v1/ios/events?after=<eventId>`

拉取最多保留 30 天的未确认后台触发事件。

### `POST /v1/ios/events/{eventId}/ack`

终端写入本地历史后确认事件。事件采用至少一次投递语义，客户端按确定性事件 ID 去重。

## WebSocket `GET /v1/stream`

此接口同时作为所有终端无法直连币安时的行情中继。终端切换前先调用 `PUT /v1/subscriptions`；连接断开后重新连接并再次发送 `resume`，服务端按 `lastEventTime` 补发内存中尚存的一小时价格。

握手使用 Bearer 设备令牌。客户端首条消息：

```json
{
  "type": "resume",
  "lastEventTime": {
    "BTCUSDT": 1787363070000
  }
}
```

服务端消息：

```json
{
  "type": "price",
  "symbol": "BTCUSDT",
  "price": "64231.50",
  "eventTime": 1787363070000,
  "replay": false
}
```

```json
{
  "type": "status",
  "symbol": "BTCUSDT",
  "state": "warming_up",
  "reason": "history_gap"
}
```

```json
{
  "type": "ready",
  "serverTime": 1787363070123
}
```

连接超过 60 秒未收到任何有效价格时，终端发送“监控已中断”系统通知；恢复后再通知一次。单个合约 30 秒无新成交则标记 `stale`，没有新价格时自然不会执行规则判断。

## 币安上游

服务端通过 `wss://fstream.binance.com/ws` 动态订阅所有有效终端配置的 `<symbol>@trade`，忽略价格或数量为 0 的非成交事件，再按币安事件时间合并为每秒最后一笔。该公开行情接口无需 API Key；币安当前不按请求收费，但仍受官方连接和订阅限制约束。
