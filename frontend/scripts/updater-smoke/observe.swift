import AppKit
import CoreGraphics
import Foundation

let root = URL(fileURLWithPath: CommandLine.arguments[1])
let files = FileManager.default
let first = NSWorkspace.shared.frontmostApplication?.processIdentifier
var samples = 0
var visibleWindows = 0
var frontmostChanges = 0
try Data().write(to: root.appendingPathComponent("os-ready"))
let deadline = Date().addingTimeInterval(45)
while !files.fileExists(atPath: root.appendingPathComponent("os-stop").path) && Date() < deadline {
    samples += 1
    if NSWorkspace.shared.frontmostApplication?.processIdentifier != first {
        frontmostChanges += 1
    }
    if let pidText = try? String(contentsOf: root.appendingPathComponent("spawned-pid"), encoding: .utf8),
       let pid = Int(pidText),
       let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] {
        visibleWindows += windows.filter { ($0[kCGWindowOwnerPID as String] as? Int) == pid }.count
    }
    Thread.sleep(forTimeInterval: 0.01)
}
let report: [String: Any] = ["samples": samples, "visibleWindows": visibleWindows, "frontmostChanges": frontmostChanges]
try JSONSerialization.data(withJSONObject: report, options: [.prettyPrinted]).write(to: root.appendingPathComponent("os-report.json"))
