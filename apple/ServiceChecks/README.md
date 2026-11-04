# 认证协调器回归检查

在 Mac 上执行 `bash apple/ServiceChecks/run.sh`。

直接编译生产 `ServerConnectionService.swift`，使用模拟 PriceCore API 和钥匙串实现，不进行网络请求，不访问或修改真实钥匙串。

覆盖并发请求只认证一次、网络失败不重复读取凭据、钥匙串拒绝后不反复读取或新建设备、主动授权后恢复。该检查不替代实际签名/钥匙串 ACL 与菜单栏 UI 验收。
