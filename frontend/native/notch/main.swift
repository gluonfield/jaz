import AppKit
import Intents
import UserNotifications

@main
struct Notch {
    static func main() async {
        // Jaz's Info.plist would otherwise check this helper in as a foreground app, briefly adding a Dock tile.
        (CFBundleGetInfoDictionary(CFBundleGetMainBundle()) as NSDictionary as? NSMutableDictionary)?["LSBackgroundOnly"] = true
        let arguments = Array(CommandLine.arguments.dropFirst())
        switch arguments.first {
        case "status":
            emit(await status())
        case "request-focus":
            _ = await INFocusStatusCenter.default.requestAuthorization()
            emit(["focus": focus()])
        case "announce" where arguments.count == 2:
            NSAccessibility.post(element: NSApplication.shared, notification: .announcementRequested, userInfo: [.announcement: arguments[1], .priority: NSAccessibilityPriorityLevel.high.rawValue])
            try? await Task.sleep(for: .seconds(1))
        default:
            exit(64)
        }
    }
}

func emit(_ value: [String: Any]) {
    print(String(decoding: try! JSONSerialization.data(withJSONObject: value), as: UTF8.self))
}

func status() async -> [String: Any] {
    var settings: UNNotificationSettings?
    if Bundle.main.bundleURL.pathExtension == "app" {
        settings = await UNUserNotificationCenter.current().notificationSettings()
    }
    return [
        "model": model(),
        "screens": NSScreen.screens.map { screen in
            [
                "id": screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")]!,
                "notch": notch(left: screen.auxiliaryTopLeftArea, right: screen.auxiliaryTopRightArea),
            ]
        },
        "focus": focus(),
        "previews": previews(settings?.showPreviewsSetting),
        "notifications": notifications(settings?.authorizationStatus),
    ]
}

func model() -> String {
    var size = 0
    sysctlbyname("hw.model", nil, &size, nil, 0)
    var model = [UInt8](repeating: 0, count: size)
    sysctlbyname("hw.model", &model, &size, nil, 0)
    return String(decoding: model.prefix { $0 != 0 }, as: UTF8.self)
}

func notch(left: NSRect?, right: NSRect?) -> Any {
    guard let left, let right, !left.isEmpty, !right.isEmpty else {
        return NSNull()
    }
    return ["x": left.width, "width": right.minX - left.maxX, "height": left.height]
}

func focus() -> String {
    let center = INFocusStatusCenter.default
    guard center.authorizationStatus == .authorized, let focused = center.focusStatus.isFocused else {
        return "unknown"
    }
    return focused ? "on" : "off"
}

func previews(_ setting: UNShowPreviewsSetting?) -> String {
    switch setting {
    case .always: "always"
    case .whenAuthenticated: "when_unlocked"
    case .never: "never"
    default: "unknown"
    }
}

func notifications(_ status: UNAuthorizationStatus?) -> String {
    switch status {
    case .authorized: "authorized"
    case .provisional: "provisional"
    case .denied: "denied"
    case .notDetermined: "not_determined"
    default: "unknown"
    }
}
