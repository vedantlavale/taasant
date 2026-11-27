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
            Sidebar(model: model, state: sidebar)
            Divider().background(Brand.line)

            VStack(spacing: 0) {
                ActivityStrip(model: model)
                if let problem = model.listError { problemBanner(problem) }
                Group {
                    switch sidebar.section {
                    case .files: FilesSection(model: model)
                    case .transfers: TransfersSection(model: model)
                    case .drive: DriveSection(model: model)
                    case .settings: SettingsSection(model: model)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .glassPane()
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

extension AppView {
    // The list could not be read. The files that are shown are the last ones
    // that could, and saying so beats a list that looks empty.
    fileprivate func problemBanner(_ text: String) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Brand.danger)
            Text(text).font(.caption).foregroundStyle(Brand.text).lineLimit(2)
            Spacer()
            Button("Try again") { Task { await model.reload() } }.controlSize(.small)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 8)
        .background(Brand.danger.opacity(0.14))
    }
}

final class WindowDropState: ObservableObject {
    @Published var targeted = false
}
