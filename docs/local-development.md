# 本地开发

## 服务端

```bash
cd server
docker compose up -d
cp .env.example .env
# 修改 TOKEN_PEPPER 后加载环境变量
set -a
source .env
set +a
go test ./...
go run ./cmd/pricereminder
```

默认只连接币安官方 U 本位接口。需要的环境变量记录在 `server/.env.example`；环境变量只用于显式配置，不会触发备用数据源或自动降级。

本机若无法直连币安，可显式让 Go 的官方币安 HTTP/WebSocket 请求经过本地代理：

```bash
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
NO_PROXY=127.0.0.1,localhost \
go run ./cmd/pricereminder
```

真实行情端到端测试会注册临时设备、写入本地数据库并等待 BTC 成交价：

```bash
PRICE_REMINDER_E2E_URL=http://127.0.0.1:18443 \
PRICE_REMINDER_E2E_DATABASE_URL='postgres://pricereminder:pricereminder@127.0.0.1:55432/pricereminder?sslmode=disable' \
go test -count=1 -v ./integration
```

## Apple

当前工程目标为 iOS 17.2+ 与 macOS 14+。macOS 与 iPhone 前台直接连接币安；iPhone 后台提醒需要可公网访问的 HTTPS 服务端。生成并打开工程：

```bash
cd apple
xcodegen generate
open PriceReminder.xcodeproj
```

工程默认使用已部署的 `https://keyflow.zcn.world/price-reminder`。如需连接另一套 PriceReminder 服务，可在构建时显式覆盖：

```bash
xcodebuild -project PriceReminder.xcodeproj -scheme PriceReminderiOS \
  PRICE_REMINDER_SERVER_URL='https://your-price-reminder-host.example' build
```

服务端需要配置 `APNS_KEY_ID`、`APNS_TEAM_ID`、`APNS_PRIVATE_KEY` 和 `APNS_BUNDLE_ID`。Push Notifications 不支持 Personal Team；真机后台通知必须使用已加入 Apple Developer Program、且为 `world.zcn.pricereminder` 启用 Push Notifications 的开发者团队。服务地址未配置时，App 会明确显示“后台监控不可用”，不会使用其他地址。

## Android

安装 Android SDK 与 JDK 17 后：

```bash
cd android
./gradlew testDebugUnitTest assembleDebug
```

APK 不依赖 GMS/HMS，适用于 Android 10+ 与仍支持 Android APK 的华为设备。

Android Debug 与 Release 优先连接币安公开 HTTPS/WebSocket；直连失败时使用 `BuildConfig.PRICE_REMINDER_SERVER_URL` 指定的服务端。当前部署地址为 `https://keyflow.zcn.world/price-reminder`。
