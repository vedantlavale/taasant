import AppKit
import SwiftUI

// The look the menu-bar panel and the window share: clear glass that bends
// what is behind it, with the app drawn on top in light text.
enum Glass {
    // wrap puts a hosted view on glass. radius is the corner of the glass, 0 when
    // the window already cuts the corners.
    @MainActor static func wrap(_ content: some View, radius: CGFloat) -> NSView {
        let hosted = NSHostingView(rootView: content)
        hosted.appearance = NSAppearance(named: .darkAqua)
        hosted.wantsLayer = true
        hosted.layer?.backgroundColor = NSColor.clear.cgColor

        if #available(macOS 26.0, *) {
            let glass = NSGlassEffectView()
            glass.appearance = NSAppearance(named: .darkAqua)
            glass.contentView = hosted
            glass.cornerRadius = radius
            // Clear glass is the one that bends what is behind it. The regular
            // one only blurs it. A tint, and the dark layer the screens put
            // behind their content, keep the text readable.
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

extension View {
    // Clear glass shows what is behind it as it is, so a dark layer keeps the
    // light text readable over a white page too.
    func glassScrim() -> some View { background(Color.black.opacity(0.25)) }
}
