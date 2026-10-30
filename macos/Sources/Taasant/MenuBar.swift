import AppKit
import Combine
import SwiftUI

// The menu bar icon is made by hand and not with MenuBarExtra: that one has no
// way to take a drop on the icon, and its menu cannot hold a drop zone.
@MainActor
final class MenuBar: NSObject, NSPopoverDelegate {
    static let shared = MenuBar()

    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private let popover = NSPopover()
    private var watching: AnyCancellable?

    func start() {
        let model = Model.shared
        popover.behavior = .transient
        popover.delegate = self
        popover.contentSize = NSSize(width: 340, height: 440)
        popover.contentViewController = NSHostingController(rootView: PopoverView(model: model))

        guard let button = item.button else { return }
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

    func toggle() { popover.isShown ? close() : show() }

    func show() {
        guard !popover.isShown, let button = item.button else { return }
        let model = Model.shared
        model.popoverOpen = true
        Task { await model.reload() }
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        NSApp.activate(ignoringOtherApps: true)
    }

    func close() { popover.performClose(nil) }

    func popoverDidClose(_ notification: Notification) { Model.shared.popoverOpen = false }

    // A panel would take the focus from the popover and close it.
    func pick() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        popover.behavior = .applicationDefined
        NSApp.activate(ignoringOtherApps: true)
        let chose = panel.runModal() == .OK
        popover.behavior = .transient
        if chose { Model.shared.upload(panel.urls) }
    }

    // The percent is shown next to the icon while a file moves.
    private func refresh() {
        let model = Model.shared
        guard let button = item.button else { return }
        button.image = NSImage(systemSymbolName: model.mounted ? "externaldrive.fill" : "externaldrive",
                               accessibilityDescription: "taasant")
        let fraction = model.jobs.first { !$0.finished && $0.error == nil }?.fraction ?? model.driveUpload?.fraction
        button.imagePosition = fraction == nil ? .imageOnly : .imageLeading
        button.title = fraction.map { " \(Int($0 * 100))%" } ?? ""
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
