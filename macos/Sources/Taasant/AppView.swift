import SwiftUI

// The full window: a sidebar and one section of the app, on the same glass as
// the menu-bar panel. The glass runs under the title bar, so the window's
// three buttons float on it; the content stays below them by itself.
struct AppView: View {
    @ObservedObject var model: Model
    @StateObject private var sidebar = SidebarState()
    @StateObject private var drop = WindowDropState()

    var body: some View {
        HStack(spacing: 0) {
            Sidebar(model: model, state: sidebar, compact: false)
            Divider().background(Brand.line)

            VStack(spacing: 0) {
                ActivityStrip(model: model)
                Group {
                    switch sidebar.section {
                    case .files: FilesSection(model: model, compact: false)
                    case .transfers: TransfersSection(model: model)
                    case .drive: DriveSection(model: model)
                    case .settings: SettingsSection(model: model)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .glassScrim()
        .onDrop(of: [.fileURL], isTargeted: $drop.targeted) { model.drop($0) }
        .overlay {
            if drop.targeted {
                RoundedRectangle(cornerRadius: 8)
                    .strokeBorder(Brand.accent, style: StrokeStyle(lineWidth: 3, dash: [8]))
                    .padding(6)
            }
        }
        .noticeAlert(model)
    }
}

final class WindowDropState: ObservableObject {
    @Published var targeted = false
}
