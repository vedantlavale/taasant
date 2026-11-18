import AppKit
import SwiftUI

// The menu-bar panel gets the system's glass: a blurred, tinted pane that
// picks up the wallpaper. The window keeps the site's black. Older Macs get
// the older vibrant material instead.
final class GlassPopoverController: NSViewController {
    private let model: Model

    init(model: Model) {
        self.model = model
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func loadView() {
        let hosted = NSHostingView(rootView: PopoverView(model: model, glass: true))
        hosted.appearance = NSAppearance(named: .darkAqua)
        hosted.wantsLayer = true
        hosted.layer?.backgroundColor = NSColor.clear.cgColor

        if #available(macOS 26.0, *) {
            let glass = NSGlassEffectView()
            glass.appearance = NSAppearance(named: .darkAqua)
            glass.contentView = hosted
            glass.cornerRadius = 14
            glass.style = .regular
            // A little dark tint keeps the light text readable on any wallpaper.
            glass.tintColor = NSColor.black.withAlphaComponent(0.15)
            if #available(macOS 27.0, *) { glass.effectIsInteractive = true }
            view = glass
        } else {
            let blur = NSVisualEffectView()
            blur.appearance = NSAppearance(named: .darkAqua)
            blur.material = .hudWindow
            blur.blendingMode = .behindWindow
            blur.state = .active
            hosted.translatesAutoresizingMaskIntoConstraints = false
            blur.addSubview(hosted)
            NSLayoutConstraint.activate([
                hosted.leadingAnchor.constraint(equalTo: blur.leadingAnchor),
                hosted.trailingAnchor.constraint(equalTo: blur.trailingAnchor),
                hosted.topAnchor.constraint(equalTo: blur.topAnchor),
                hosted.bottomAnchor.constraint(equalTo: blur.bottomAnchor),
            ])
            view = blur
        }
    }
}
