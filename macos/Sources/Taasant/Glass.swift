import AppKit
import SwiftUI

// The look the menu-bar panel and the window share: clear glass for the edge,
// the rim and the colour of what is behind, over a blur that keeps that from
// competing with the text. The app is drawn on top in light text.
enum Glass {
    // wrap puts a hosted view on glass. radius is the corner of the glass, 0 when
    // the window already cuts the corners.
    @MainActor static func wrap(_ content: some View, radius: CGFloat, strongBlur: Bool = false) -> NSView {
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
            // one only blurs it, and clear alone shows a page of code through
            // the text, so a blur sits under it and a tint over it.
            glass.style = .clear
            glass.tintColor = NSColor.black.withAlphaComponent(0.3)
            let container = NSView()
            let underlay = NSVisualEffectView()
            underlay.appearance = NSAppearance(named: .darkAqua)
            underlay.material = .hudWindow
            underlay.blendingMode = .behindWindow
            underlay.state = .active
            underlay.alphaValue = strongBlur ? 1 : 0.85
            underlay.wantsLayer = true
            underlay.layer?.cornerRadius = radius
            underlay.layer?.masksToBounds = true

            // The window sits over other windows more often than the little
            // panel does, so it gets a second layer: regular glass blurs the
            // backdrop before the clear glass refracts what is left.
            var layers: [NSView] = [underlay]
            if strongBlur {
                let blur = NSGlassEffectView()
                blur.appearance = NSAppearance(named: .darkAqua)
                blur.cornerRadius = radius
                blur.style = .regular
                blur.tintColor = NSColor.black.withAlphaComponent(0.45)
                layers.append(blur)
            }
            layers.append(glass)
            for v in layers {
                v.translatesAutoresizingMaskIntoConstraints = false
                container.addSubview(v)
                NSLayoutConstraint.activate([
                    v.leadingAnchor.constraint(equalTo: container.leadingAnchor),
                    v.trailingAnchor.constraint(equalTo: container.trailingAnchor),
                    v.topAnchor.constraint(equalTo: container.topAnchor),
                    v.bottomAnchor.constraint(equalTo: container.bottomAnchor),
                ])
            }
            return container
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
    func glassScrim() -> some View { background(Color.black.opacity(0.2)) }
    // The list sits on a darker pane than the sidebar, so reading it never
    // competes with whatever is behind the window.
    func glassPane() -> some View { background(Color.black.opacity(0.3)) }
}
