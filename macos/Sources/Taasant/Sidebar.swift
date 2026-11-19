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

// The window's sidebar: the four sections, and under them how things stand.
struct Sidebar: View {
    @ObservedObject var model: Model
    @ObservedObject var state: SidebarState

    private var activeJobs: Int { model.jobs.filter { !$0.finished && $0.error == nil }.count + (model.driveUpload == nil ? 0 : 1) }

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Brand.ant().frame(width: 34, height: 18)
                Text("taasant").font(.headline).foregroundStyle(Brand.text)
            }
            .frame(maxWidth: .infinity, minHeight: 56, alignment: .leading)
            .padding(.horizontal, 16)

            VStack(spacing: 3) {
                ForEach(AppSection.allCases) { section in
                    Button { state.section = section } label: {
                        HStack(spacing: 10) {
                            Image(systemName: section.symbol).frame(width: 20)
                            Text(section.title)
                            Spacer()
                            if section == .transfers && activeJobs > 0 {
                                Text("\(activeJobs)")
                                    .font(.caption2.weight(.semibold))
                                    .foregroundStyle(Brand.canvas)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 1)
                                    .background(Capsule().fill(Brand.accent))
                            }
                        }
                        .padding(.vertical, 8)
                        .padding(.horizontal, 10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(state.section == section ? Color.white.opacity(0.14) : Color.clear)
                        .foregroundStyle(state.section == section ? Brand.accent : Brand.muted)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 10)

            Spacer()

            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    StatusDot(on: model.mounted)
                    Text(model.mounted ? "Drive connected" : "Drive off")
                }
                .font(.caption)
                .foregroundStyle(Brand.muted)
                Text(model.configured ? "version \(model.version)" : "Set up needed")
                    .font(.caption2)
                    .foregroundStyle(model.configured ? Brand.muted : Brand.danger)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(16)
        }
        .frame(width: 184)
    }
}
