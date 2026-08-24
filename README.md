# 币价提醒

跨端币安 U 本位永续合约价格提醒工具。

## 已确认的产品边界

- 数据源：币安 U 本位合约公开 `<symbol>@trade` 实时流，最新成交价，无 API Key，当前不按请求收费。
- 价格连接：Apple、Android/华为与 Windows 端优先直连币安；直连不可用时自动切换到服务端转发的同一币安官方行情，并自动重连。
- 监控启动：每次打开 App 默认开始监控，用户仍可在 App 内手动停止或重新开始。
- 规则：当前价相对 `N` 分钟前价格的滚动变化，`N=1..60`，`X=0.1..100%`。
- 触发：上涨、下跌方向独立；达到阈值即触发，回到阈值内立即重新武装。
- 存储：终端只保留最近 1 小时价格；触发历史在终端保留 30 天。
- Android/华为/macOS：终端本地判断并发送系统通知；通知未授权时行情和规则计算仍继续。
- Windows：通知区域托盘程序在本地判断并发送系统通知；关闭窗口后继续监控，选择“退出”才停止。
- iOS：前台直连币安并本地判断；进入后台后由配置的服务端继续计算规则，并通过 APNs 提醒。
- 系统展示：前台与 macOS 菜单栏每秒更新；iOS App 活跃时灵动岛最多每 15 秒更新一次。

## 工程结构

```text
server/             Go 行情中继、断线补发与 iOS 后台能力
apple/              iOS、Live Activity、macOS 菜单栏与共享 Swift 核心
android/            Android/华为通用 APK，Compose 与前台服务
windows/            Windows 10/11 原生托盘程序（.NET 8 WinForms）
shared/fixtures/    三端规则引擎一致性样例
docs/               接口、架构与本地验证说明
```

## 本地环境

服务端需要 Go 1.24+。Apple 真机构建需要 Xcode；Android APK 构建需要 Android SDK、JDK 17 与 Gradle Wrapper。详细步骤见 `docs/local-development.md`。

Windows 版本由 GitHub Actions 在 Windows 环境生成自包含 ZIP；无需预装 .NET。启动后显示管理窗口并驻留通知区域，窗口关闭后继续运行。
