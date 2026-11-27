import Foundation
import Security

private let service = "dev.wins.jaz.master-key"
private let account = "default"
private let defaultAccessGroup = "JDDZ55DT74.dev.wins.jaz"

@main
struct KeychainHelper {
    static func main() {
        // Keep this short-lived helper out of the Dock when bundled inside Jaz.app/Contents/MacOS.
        (CFBundleGetInfoDictionary(CFBundleGetMainBundle()) as NSDictionary as? NSMutableDictionary)?["LSBackgroundOnly"] = true
        let arguments = Array(CommandLine.arguments.dropFirst())
        let command = arguments.first
        let accessGroup = arguments.count >= 2 && !arguments[1].isEmpty ? arguments[1] : defaultAccessGroup
        switch command {
        case "get":
            handleGet(accessGroup: accessGroup)
        case "set":
            handleSet(accessGroup: accessGroup)
        default:
            exit(64)
        }
    }
}

private func baseQuery(accessGroup: String) -> [String: Any] {
    [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account,
        kSecAttrAccessGroup as String: accessGroup,
        kSecUseDataProtectionKeychain as String: true,
    ]
}

private func handleGet(accessGroup: String) {
    var query = baseQuery(accessGroup: accessGroup)
    query[kSecReturnData as String] = true
    query[kSecMatchLimit as String] = kSecMatchLimitOne
    var result: CFTypeRef?
    let status = SecItemCopyMatching(query as CFDictionary, &result)
    switch status {
    case errSecSuccess:
        guard let data = result as? Data, data.count == 32 else {
            emit(["status": "error", "code": Int(errSecDecode)])
            exit(1)
        }
        emit(["status": "ok", "key": data.base64EncodedString()])
    case errSecItemNotFound:
        emit(["status": "not_found"])
    case errSecMissingEntitlement:
        emit(["status": "missing_entitlement"])
    default:
        emit(["status": "error", "code": Int(status)])
        exit(1)
    }
}

private func handleSet(accessGroup: String) {
    let raw = String(decoding: FileHandle.standardInput.readDataToEndOfFile(), as: UTF8.self)
        .trimmingCharacters(in: .whitespacesAndNewlines)
    guard let keyData = Data(base64Encoded: raw), keyData.count == 32 else {
        emit(["status": "error", "code": Int(errSecParam)])
        exit(64)
    }
    var item = baseQuery(accessGroup: accessGroup)
    item[kSecValueData as String] = keyData
    item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
    let status = SecItemAdd(item as CFDictionary, nil)
    switch status {
    case errSecSuccess:
        emit(["status": "ok", "key": keyData.base64EncodedString()])
    case errSecDuplicateItem:
        handleGet(accessGroup: accessGroup)
    case errSecMissingEntitlement:
        emit(["status": "missing_entitlement"])
    default:
        emit(["status": "error", "code": Int(status)])
        exit(1)
    }
}

private func emit(_ value: [String: Any]) {
    print(String(decoding: try! JSONSerialization.data(withJSONObject: value), as: UTF8.self))
}
