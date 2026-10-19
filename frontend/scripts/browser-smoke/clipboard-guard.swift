import AppKit
import Foundation

let board = NSPasteboard.general
func snapshot() -> [[NSPasteboard.PasteboardType: Data]] {
    (board.pasteboardItems ?? []).map { item in
        Dictionary(uniqueKeysWithValues: item.types.map { type in
            guard let data = item.data(forType: type) else {
                fputs("Unreadable clipboard format\n", stderr)
                exit(1)
            }
            return (type, data)
        })
    }
}
let saved = snapshot()
print("saved")
fflush(stdout)
_ = readLine()
let items = saved.map { values in
    let item = NSPasteboardItem()
    for (type, data) in values {
        item.setData(data, forType: type)
    }
    return item
}
board.clearContents()
if !items.isEmpty {
    board.writeObjects(items)
}
let restored = snapshot() == saved
print(restored ? "restored" : "restore failed")
exit(restored ? 0 : 1)
