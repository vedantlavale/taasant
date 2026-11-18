import AppKit
import SwiftUI

// The colours come from the site's dark theme. They are used for surfaces and
// accents only; status colours stay with their own meaning.
enum Brand {
    static func color(_ hex: String) -> Color {
        let digits = hex.hasPrefix("#") ? String(hex.dropFirst()) : hex
        let value = UInt32(digits, radix: 16) ?? 0
        return Color(
            red: Double((value >> 16) & 0xff) / 255,
            green: Double((value >> 8) & 0xff) / 255,
            blue: Double(value & 0xff) / 255
        )
    }

    static let canvas = color("0f0f13")
    static let surface = color("17171d")
    static let raised = color("1d1d38")
    static let line = color("25252d")
    static let text = color("ececf1")
    static let muted = color("a9a9b6")
    static let accent = color("a3a8fb")
    static let danger = color("f98c80")
    static let success = color("7bd88f")

    // The site's pixel ant, shipped as a PNG so the app uses the same mark.
    static func antImage(_ name: String = "ant") -> NSImage? {
        guard let url = Bundle.main.url(forResource: name, withExtension: "png") else { return nil }
        return NSImage(contentsOf: url)
    }

    static func ant(_ name: String = "ant") -> some View {
        Group {
            if let image = antImage(name) {
                Image(nsImage: image).resizable().aspectRatio(contentMode: .fit)
            } else {
                Image(systemName: "ant")
            }
        }
    }
}
