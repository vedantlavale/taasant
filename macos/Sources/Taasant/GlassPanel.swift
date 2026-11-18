import AppKit
import SwiftUI

// The menu-bar panel. It is a window of its own and not an NSPopover: a
// popover draws its own frosted backing, and glass over that is only more
// blur. A clear window lets the glass see the real wallpaper and windows
// behind it, bend them at its edge and catch light on its rim.
@MainActor
final class GlassPanel: NSPanel {
    // Whether a click elsewhere closes the panel. It is off while a file
    // picker is open, which takes the focus.
    var closesOnFocusLoss = true
    var onClose: () -> Void = {}

    private static let size = NSSize(width: 400, height: 440)
    private static let radius: CGFloat = 28

    init(model: Model) {
        super.init(contentRect: NSRect(origin: .zero, size: Self.size),
                   styleMask: [.borderless, .nonactivatingPanel],
                   backing: .buffered, defer: false)
        isOpaque = false
        backgroundColor = .clear
        hasShadow = true
        level = .popUpMenu
        collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        isReleasedWhenClosed = false
        appearance = NSAppearance(named: .darkAqua)
        contentView = Self.glass(around: model)

        NotificationCenter.default.addObserver(forName: NSWindow.didResignKeyNotification, object: self, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated {
                guard let self, self.closesOnFocusLoss, self.attachedSheet == nil else { return }
                self.close()
            }
        }
    }

    override var canBecomeKey: Bool { true }

    // Esc closes it, as it does a popover.
    override func cancelOperation(_ sender: Any?) { close() }

    // It opens just under the icon, and stays inside the screen.
    func show(below button: NSStatusBarButton) {
        guard let window = button.window else { return }
        let icon = window.convertToScreen(button.convert(button.bounds, to: nil))
        let visible = (window.screen ?? NSScreen.main)?.visibleFrame ?? .zero
        let x = min(max(icon.midX - Self.size.width / 2, visible.minX + 8), visible.maxX - Self.size.width - 8)
        setFrame(NSRect(origin: NSPoint(x: x, y: icon.minY - Self.size.height - 6), size: Self.size), display: false)
        alphaValue = 0
        makeKeyAndOrderFront(nil)
        NSAnimationContext.runAnimationGroup { $0.duration = 0.12; animator().alphaValue = 1 }
        // The shadow follows the glass's outline, which is only known once it has been drawn.
        DispatchQueue.main.async { self.invalidateShadow() }
    }

    override func close() {
        guard isVisible else { return }
        NSAnimationContext.runAnimationGroup({ $0.duration = 0.1; animator().alphaValue = 0 }) {
            MainActor.assumeIsolated {
                self.orderOut(nil)
                self.onClose()
            }
        }
    }

    private static func glass(around model: Model) -> NSView {
        let hosted = NSHostingView(rootView: PopoverView(model: model, glass: true))
        hosted.appearance = NSAppearance(named: .darkAqua)
        hosted.wantsLayer = true
        hosted.layer?.backgroundColor = NSColor.clear.cgColor

        if #available(macOS 26.0, *) {
            let glass = NSGlassEffectView()
            glass.appearance = NSAppearance(named: .darkAqua)
            glass.contentView = hosted
            glass.cornerRadius = radius
            // Clear glass is the one that bends what is behind it. The regular
            // one only blurs it. A tint and the dark layer in PopoverView keep
            // the text readable.
            glass.style = .clear
            glass.tintColor = NSColor.black.withAlphaComponent(0.45)
            return glass
        }

        // Older Macs have no glass, only the vibrant material.
        let blur = NSVisualEffectView()
        blur.appearance = NSAppearance(named: .darkAqua)
        blur.material = .hudWindow
        blur.blendingMode = .behindWindow
        blur.state = .active
        blur.wantsLayer = true
        blur.layer?.cornerRadius = radius
        blur.layer?.masksToBounds = true
        hosted.translatesAutoresizingMaskIntoConstraints = false
        blur.addSubview(hosted)
        NSLayoutConstraint.activate([
            hosted.leadingAnchor.constraint(equalTo: blur.leadingAnchor),
            hosted.trailingAnchor.constraint(equalTo: blur.trailingAnchor),
            hosted.topAnchor.constraint(equalTo: blur.topAnchor),
            hosted.bottomAnchor.constraint(equalTo: blur.bottomAnchor),
        ])
        return blur
    }
}
