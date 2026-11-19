import AppKit
import Combine
import SwiftUI

// The menu bar icon is made by hand and not with MenuBarExtra: that one has no
// way to take a drop on the icon, and its menu cannot hold a drop zone.
@MainActor
final class MenuBar: NSObject {
    static let shared = MenuBar()

    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private var panel: GlassPanel!
    private var watching: AnyCancellable?

    func start() {
        let model = Model.shared
        panel = GlassPanel(model: model)
        panel.onClose = { Model.shared.popoverOpen = false }

        guard let button = item.button else { return }
        // The overlay below takes the mouse, so this is for the keyboard and
        // for VoiceOver, which press the button itself.
        button.target = self
        button.action = #selector(togglePanel)
        button.setAccessibilityLabel("taasant")
        let target = DropTarget(frame: button.bounds)
        target.autoresizingMask = [.width, .height]
        target.clicked = { [weak self] in self?.toggle() }
        target.entered = { [weak self] in
            button.isHighlighted = true
            self?.show()
        }
        target.left = { button.isHighlighted = false }
        target.dropped = { urls in
            button.isHighlighted = false
            model.upload(urls)
        }
        button.addSubview(target)

        // The model says when anything changed, and the icon follows.
        watching = model.objectWillChange.sink { [weak self] in
            DispatchQueue.main.async { self?.refresh() }
        }
        refresh()
    }

    @objc private func togglePanel() { toggle() }

    func toggle() { panel.isVisible ? close() : show() }

    func show() {
        guard !panel.isVisible, let button = item.button else { return }
        let model = Model.shared
        model.popoverOpen = true
        Task { await model.reload() }
        NSApp.activate(ignoringOtherApps: true)
        panel.show(below: button)
    }

    func close() { panel.close() }

    // A file picker takes the focus from the panel, which would close it. The
    // panel floats above menus, which is above the picker, so it steps down
    // while the picker is open and the picker is not hidden behind it.
    func pick() {
        let chooser = NSOpenPanel()
        chooser.allowsMultipleSelection = true
        panel.closesOnFocusLoss = false
        panel.level = .normal
        NSApp.activate(ignoringOtherApps: true)
        let chose = chooser.runModal() == .OK
        panel.level = .popUpMenu
        panel.closesOnFocusLoss = true
        panel.makeKey()
        if chose { Model.shared.upload(chooser.urls) }
    }

    // The ant is the site's mark, drawn as a template so macOS colours it to
    // match the menu bar. A small dot under it means the drive is connected.
    private func icon(_ connected: Bool) -> NSImage? {
        let name = connected ? "ant-menu-on" : "ant-menu"
        guard let url = Bundle.main.url(forResource: name, withExtension: "png"),
              let image = NSImage(contentsOf: url) else { return nil }
        image.isTemplate = true
        image.size = NSSize(width: 22, height: 12)
        return image
    }

    // The percent is shown next to the icon while a file moves.
    private func refresh() {
        let model = Model.shared
        guard let button = item.button else { return }
        if let image = icon(model.mounted) {
            button.image = image
        } else {
            button.image = NSImage(systemSymbolName: model.mounted ? "externaldrive.fill" : "externaldrive",
                                   accessibilityDescription: "taasant")
        }
        if let activity = model.activity {
            button.imagePosition = .imageLeading
            button.title = activity.fraction.map { " \(Int($0 * 100))%" } ?? " …"
            button.toolTip = activity.title
        } else {
            button.imagePosition = .imageOnly
            button.title = ""
            button.toolTip = nil
        }
    }
}

// It sits over the icon: a status item's button cannot be a drop target, so
// this one takes the drops and the clicks.
final class DropTarget: NSView {
    var clicked: () -> Void = {}
    var entered: () -> Void = {}
    var left: () -> Void = {}
    var dropped: ([URL]) -> Void = { _ in }

    override init(frame: NSRect) {
        super.init(frame: frame)
        registerForDraggedTypes([.fileURL])
    }

    required init?(coder: NSCoder) { fatalError() }

    override func mouseDown(with event: NSEvent) { clicked() }

    override func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
        entered()
        return .copy
    }

    override func draggingExited(_ sender: NSDraggingInfo?) { left() }

    override func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
        let urls = sender.draggingPasteboard.readObjects(forClasses: [NSURL.self],
                                                         options: [.urlReadingFileURLsOnly: true]) as? [URL] ?? []
        dropped(urls)
        return !urls.isEmpty
    }
}
