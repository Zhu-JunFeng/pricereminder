import Foundation
import Security

enum KeychainStore {
    private static let service = "world.zcn.pricereminder"
    private static let lock = NSLock()

    struct Failure: LocalizedError {
        let status: OSStatus
        var errorDescription: String? {
            "服务端凭据暂不可用，请到设置 → 自检点击“授权服务端连接”（钥匙串错误 \(status)）"
        }
    }

    // Ad-hoc Mac builds use the legacy login keychain. LAContext/UIFail alone
    // does not suppress legacy prompts. Serialize and restore the process flag.
    private static func access<T>(allowInteraction: Bool, _ operation: () throws -> T) throws -> T {
        lock.lock()
        defer { lock.unlock() }
        #if os(macOS)
        var previous = DarwinBoolean(true)
        let readStatus = SecKeychainGetUserInteractionAllowed(&previous)
        guard readStatus == errSecSuccess else { throw Failure(status: readStatus) }
        let status = SecKeychainSetUserInteractionAllowed(allowInteraction)
        guard status == errSecSuccess else { throw Failure(status: status) }
        defer { SecKeychainSetUserInteractionAllowed(previous.boolValue) }
        #endif
        return try operation()
    }

    private static var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: "device-token"]
    }

    static func token(allowInteraction: Bool = false) throws -> String? {
        try access(allowInteraction: allowInteraction) {
            var item = query
            item[kSecReturnData as String] = true
            item[kSecMatchLimit as String] = kSecMatchLimitOne
            var result: CFTypeRef?
            let status = SecItemCopyMatching(item as CFDictionary, &result)
            if status == errSecItemNotFound { return nil }
            guard status == errSecSuccess else { throw Failure(status: status) }
            guard let data = result as? Data, let token = String(data: data, encoding: .utf8), !token.isEmpty else {
                throw Failure(status: errSecDecode)
            }
            return token
        }
    }

    static func save(token: String, allowInteraction: Bool = false) throws {
        try access(allowInteraction: allowInteraction) {
            let data = Data(token.utf8)
            let status = SecItemUpdate(query as CFDictionary, [kSecValueData as String: data] as CFDictionary)
            if status == errSecSuccess { return }
            guard status == errSecItemNotFound else { throw Failure(status: status) }
            var item = query
            item[kSecValueData as String] = data
            item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            let added = SecItemAdd(item as CFDictionary, nil)
            guard added == errSecSuccess else { throw Failure(status: added) }
        }
    }
}
