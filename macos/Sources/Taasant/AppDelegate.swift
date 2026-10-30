import SwiftUI

// The window is made by hand and not as a scene: a scene's window is only made
// once something asks for it, and there is no Dock icon to ask with.
@MainActor
final class FilesWindow: NSObject, NSWindowDelegate {
    static let shared = FilesWindow()
    private var window: NSWindow?

    func show() {
        let model = Model.shared
        if window == nil {
            let made = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 520, height: 440),
                                styleMask: [.titled, .closable, .miniaturizable, .resizable],
                                backing: .buffered, defer: false)
            made.title = "taasant"
            made.isReleasedWhenClosed = false
            made.minSize = NSSize(width: 440, height: 320)
            made.contentView = NSHostingView(rootView: FilesView(model: model))
            made.delegate = self
            made.center()
            window = made
        }
        model.windowOpen = true
        Task { await model.reload() }
        window?.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func windowWillClose(_ notification: Notification) { Model.shared.windowOpen = false }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    @MainActor func applicationDidFinishLaunching(_ notification: Notification) {
        MenuBar.shared.start()
        // The first time there is nothing to show but how to set up.
        if !Model.shared.configured { FilesWindow.shared.show() }
    }

    // Opening the app again, from Finder or Spotlight, is how to get the
    // window when the menu bar icon is hidden.
    @MainActor func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        FilesWindow.shared.show()
        return true
    }
}
