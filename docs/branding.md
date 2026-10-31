# PriceReminder 品牌与本机构建

所有客户端显示名称为 PriceReminder，应用标识、数据目录及通知标识保持不变。

主图形为 `apple/Brand/AppIcon.svg`：深蓝背景、青绿价格折线，不含文字。Apple 的 Mac 图标使用透明留边与圆角，iOS 使用不透明满版背景；Android 使用安全区内的前景矢量；Windows EXE、窗口与托盘共用多尺寸 ICO。

重新生成资源：

```sh
npm install --prefix /tmp/pricereminder-icon-tools sharp
NODE_PATH=/tmp/pricereminder-icon-tools/node_modules node apple/Brand/generate-icons.cjs
```

本地 Mac 构建使用 `bash apple/build-mac.sh`，产物放在临时 `.noindex` 目录。正式安装只保留 `/Applications/PriceReminder.app`，不要把备用应用放进应用程序目录或普通工程目录，避免 Spotlight 将历史构建、iOS 模拟器产物和 Harness 验收工具列为重复应用。

Xcode 工程由 `apple/project.yml` 生成；更改产品名时同步生成工程、plist 和发布工作流。版本字段必须引用构建变量，确保 GitHub 发布包的版本与标签一致。
