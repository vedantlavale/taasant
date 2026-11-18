import SwiftUI

enum AppSection: String, CaseIterable, Identifiable {
    case files, transfers, drive, settings

    var id: String { rawValue }

    var title: String {
        switch self {
        case .files: return "Files"
        case .transfers: return "Transfers"
        case .drive: return "Drive"
        case .settings: return "Settings"
        }
    }

    var symbol: String {
        switch self {
        case .files: return "folder"
        case .transfers: return "arrow.triangle.2.circlepath"
        case .drive: return "externaldrive"
        case .settings: return "gearshape"
        }
    }
}

final class SidebarState: ObservableObject {
    @Published var section: AppSection = .files
}

// One sidebar for the window and the menu bar. The compact form is an icon
// rail, so the popover still has room for the files themselves.
struct Sidebar: View {
    @ObservedObject var model: Model
    @ObservedObject var state: SidebarState
    let compact: Bool

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Brand.ant().frame(width: compact ? 26 : 34, height: compact ? 14 : 18)
                if !compact {
                    Text("taasant").font(.headline).foregroundStyle(Brand.text)
                }
            }
            .frame(maxWidth: .infinity, minHeight: compact ? 46 : 62)
            .padding(.horizontal, compact ? 6 : 14)

            Divider().background(Brand.line)

            VStack(spacing: 3) {
                ForEach(AppSection.allCases) { section in
                    Button { state.section = section } label: {
                        HStack(spacing: compact ? 0 : 10) {
                            Image(systemName: section.symbol)
                                .frame(width: compact ? 24 : 20)
                            if !compact {
                                Text(section.title)
                                Spacer()
                            }
                        }
                        .padding(.vertical, 8)
                        .padding(.horizontal, compact ? 0 : 10)
                        .frame(maxWidth: .infinity, alignment: compact ? .center : .leading)
                        .background(state.section == section ? Brand.raised : Color.clear)
                        .foregroundStyle(state.section == section ? Brand.accent : Brand.muted)
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .buttonStyle(.plain)
                    .help(section.title)
                }
            }
            .padding(compact ? 6 : 10)

            Spacer()

            if compact {
                Button { model.hiddenInMenuBar.toggle() } label: {
                    Image(systemName: model.hiddenInMenuBar ? "eye.slash" : "eye")
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(.plain)
                .foregroundStyle(Brand.muted)
                .help(model.hiddenInMenuBar ? "Show files in the menu bar" : "Hide files in the menu bar")
                .padding(.bottom, 8)
            } else {
                VStack(alignment: .leading, spacing: 3) {
                    Text(model.configured ? "Ready" : "Set up needed")
                        .font(.caption)
                        .foregroundStyle(model.configured ? Brand.success : Brand.danger)
                    Text("version \(model.version)")
                        .font(.caption2)
                        .foregroundStyle(Brand.muted)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(14)
            }
        }
        .frame(width: compact ? 52 : 176)
        .background(Brand.surface)
    }
}
