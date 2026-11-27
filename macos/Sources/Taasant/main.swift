import AppKit

// No SwiftUI App: its one scene would have to be a window or Settings, and
// SwiftUI opens that on its own. The icon and the windows are made by hand.
let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
