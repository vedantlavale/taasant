import SwiftUI

@main
struct TaasantApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @ObservedObject private var model = Model.shared

    var body: some Scene {
        MenuBarExtra {
            MenuContent(model: model)
        } label: {
            Image(systemName: model.mounted ? "externaldrive.fill" : "externaldrive")
            // The percent is shown next to the icon while Finder copies.
            if let fraction = model.driveUpload?.fraction { Text("\(Int(fraction * 100))%") }
        }
    }
}

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

struct MenuContent: View {
    @ObservedObject var model: Model

    var body: some View {
        Text(model.mounted ? "Drive connected" : "Drive not connected")
        if let upload = model.driveUpload {
            Text("Copying \(upload.title): \(sizes(upload.done, upload.total))")
        }
        if let message = model.message { Text(message) }
        if model.mounted {
            Button("Open drive") { model.openDrive() }
            Button("Eject") { model.eject() }
        } else if model.configured {
            Button("Connect drive") { model.connect() }
        }
        Divider()
        Button("Show files…") { FilesWindow.shared.show() }
        Toggle("Start at login", isOn: Binding(get: { model.autostart }, set: { model.setAutostart($0) }))
            .disabled(!model.configured)
        Divider()
        Button("Quit taasant") { NSApp.terminate(nil) }
    }
}
