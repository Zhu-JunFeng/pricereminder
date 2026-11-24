# Mac 授权与发布签名

## 用户授权

应用自动获取行情、重连及加载走势图时禁止弹出钥匙串窗口。失败结果在当前进程内缓存；只有用户点击授权才允许系统交互。多个入口共享同一次授权任务，成功后复用内存中的已认证客户端。取消或拒绝后不会自动重新询问，用户仍可手动重试。

授权入口旁的小字会提示：在 macOS 系统窗口输入本机登录密码并选择“始终允许”。系统“允许”仅对单次操作生效，应用无法更名或改变系统按钮语义。应用不保存用户的登录密码，不放宽钥匙串访问控制，不删除旧凭据。

## 正式发布

工作流调用 `scripts/package-macos.sh`，使用 Developer ID Application 证书、Hardened Runtime、可信时间戳和原有权限文件签名；DMG 公证通过后装订票据并验证。固定使用相同开发者团队、应用标识和签名身份，减少后续升级导致的信任失效。首次从临时签名升级到正式签名仍可能需要一次授权，不能保证系统永不提示。

仓库管理员需在 GitHub Actions 的 Repository secrets 配置：

| Secret | 内容 |
| --- | --- |
| `MACOS_CERTIFICATE_P12_BASE64` | 包含证书及私钥的 Developer ID Application P12 的 Base64 |
| `MACOS_CERTIFICATE_PASSWORD` | P12 导出密码 |
| `MACOS_SIGNING_IDENTITY` | 完整证书名 `Developer ID Application: … (TEAMID)` |
| `MACOS_NOTARY_KEY_P8` | 有公证权限的 App Store Connect API 私钥全文 |
| `MACOS_NOTARY_KEY_ID` | API Key ID |
| `MACOS_NOTARY_ISSUER_ID` | API Issuer ID |

密钥只在 CI 临时目录和专用钥匙串使用，结束时清除。不修改默认钥匙串或用户已有凭据。证书尚未配置时保留原有临时签名发布并输出明确警告；配置不完整、签名或公证失败时发布失败，禁止悄悄降级临时签名。不要将 P12、P8 或密码提交到仓库。

## 图表标注

图表下方以小字显示数据日期与单根周期，周期来自服务端 `intervalMS`。数字货币显示“15 分钟 K 线”（目前接口周期），黄金显示“1 分钟分时”，避免把分时线误认为 K 线。整张图覆盖当天已有数据，并非仅最近 15 分钟。
