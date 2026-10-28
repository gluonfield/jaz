import AppKit
import CoreGraphics
import Darwin
import Foundation

let bundle = CommandLine.arguments[1] + "/"
let stopFile = CommandLine.arguments[2]
let output = CommandLine.arguments[3]
let pidFile = CommandLine.arguments[4]
let parent = getppid()
let started = Date()
var samples: [[String: Any]] = []
var seen = Set<pid_t>()

func isTarget(_ pid: pid_t) -> Bool {
    var path = [CChar](repeating: 0, count: 4096)
    return proc_pidpath(pid, &path, UInt32(path.count)) > 0 && String(cString: path).hasPrefix(bundle)
}

func sample() {
    let roots = (try? String(contentsOfFile: pidFile, encoding: .utf8))?.split(separator: "\n").compactMap { pid_t($0) } ?? []
    let pids = Set(roots.filter(isTarget))
    seen.formUnion(pids)
    guard let front = NSWorkspace.shared.frontmostApplication?.processIdentifier,
          let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else {
        fputs("Window or foreground observation unavailable\n", stderr)
        exit(1)
    }
    let owned = windows.filter {
        guard let pid = $0[kCGWindowOwnerPID as String] as? pid_t else { return false }
        return isTarget(pid)
    }.map {
        [
            "pid": $0[kCGWindowOwnerPID as String] ?? 0,
            "id": $0[kCGWindowNumber as String] ?? 0,
            "layer": $0[kCGWindowLayer as String] ?? 0,
            "bounds": $0[kCGWindowBounds as String] ?? [:],
        ]
    }
    samples.append([
        "elapsed_ms": Date().timeIntervalSince(started) * 1000,
        "foreground_pid": front,
        "target_foreground": isTarget(front),
        "on_screen_windows": owned,
    ])
}

sample()
print("ready")
fflush(stdout)
while getppid() == parent && !FileManager.default.fileExists(atPath: stopFile) {
    RunLoop.current.run(until: Date().addingTimeInterval(0.02))
    sample()
}
sample()
let visible = samples.filter { !($0["on_screen_windows"] as! [[String: Any]]).isEmpty }
let foreground = samples.filter { $0["target_foreground"] as! Bool }
let times = samples.map { $0["elapsed_ms"] as! Double }
let gaps = zip(times, times.dropFirst()).map { $1 - $0 }
let receipt: [String: Any] = [
    "method": "CGWindowListCopyWindowInfo on-screen windows and NSWorkspace foreground, proc_pidpath ownership, external sampler",
    "requested_interval_ms": 20,
    "max_gap_ms": gaps.max() ?? 0,
    "bundle_path": bundle,
    "seen_target_pids": seen.sorted(),
    "sample_count": samples.count,
    "on_screen_violation_samples": visible.count,
    "foreground_violation_samples": foreground.count,
    "samples": samples,
]
try JSONSerialization.data(withJSONObject: receipt, options: [.prettyPrinted, .sortedKeys]).write(to: URL(fileURLWithPath: output))
exit(visible.isEmpty && foreground.isEmpty ? 0 : 1)
