import SwiftUI

// The menu-bar panel is for the quick things: drop a file in, see that it is
// moving, and get back one of the last few. Everything else, finding a file,
// choosing where it goes, the drive's settings, is in the window.
struct MenuBarView: View {
    @ObservedObject var model: Model
    @StateObject private var drop = WindowDropState()

    var body: some View {
        VStack(spacing: 0) {
            header
            if model.configured {
                VStack(spacing: 12) {
                    dropZone
                    ActivityStrip(model: model)
                        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    recent
                }
                .padding(.horizontal, 14)
                .padding(.bottom, 14)
            } else {
                setup
            }
        }
        .frame(width: 340, height: 420, alignment: .top)
        .glassScrim()
        .noticeAlert(model)
    }

    private var header: some View {
        HStack(spacing: 10) {
            Brand.ant().frame(width: 28, height: 15)
            VStack(alignment: .leading, spacing: 1) {
                Text("taasant").font(.headline).foregroundStyle(Brand.text)
                HStack(spacing: 5) {
                    StatusDot(on: model.mounted)
                    Text(model.mounted ? "Drive connected" : "Drive off")
                        .font(.caption2).foregroundStyle(Brand.muted)
                }
            }
            Spacer()
            Button {
                MenuBar.shared.close()
                FilesWindow.shared.show()
            } label: {
                Label("Open", systemImage: "arrow.up.forward.app")
            }
            .controlSize(.small)
            .help("Open the taasant window")

            Menu {
                if model.mounted {
                    Button("Open drive in Finder") { model.openDrive() }
                    Button("Eject drive") { model.eject() }
                } else if model.configured {
                    Button("Connect drive") { model.connect() }
                }
                Toggle("Start at login", isOn: Binding(
                    get: { model.autostart },
                    set: { model.setAutostart($0) }
                ))
                .disabled(!model.configured || model.autostartBusy)
                Divider()
                Button("Quit taasant") { model.quit() }
            } label: {
                Image(systemName: "ellipsis.circle").foregroundStyle(Brand.muted)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .padding(.horizontal, 14)
        .frame(height: 54)
    }

    private var dropZone: some View {
        Button { MenuBar.shared.pick() } label: {
            VStack(spacing: 6) {
                Image(systemName: drop.targeted ? "arrow.down.circle.fill" : "arrow.up.circle")
                    .font(.system(size: 28, weight: .light))
                    .foregroundStyle(Brand.accent)
                Text(drop.targeted ? "Release to store" : "Drop files to store")
                    .font(.callout.weight(.medium))
                    .foregroundStyle(Brand.text)
                Text(model.places.count > 1 ? "or click to choose  ·  to \(model.standard)" : "or click to choose")
                    .font(.caption)
                    .foregroundStyle(Brand.muted)
            }
            .frame(maxWidth: .infinity, minHeight: 112)
            .background(RoundedRectangle(cornerRadius: 14, style: .continuous)
                .fill(drop.targeted ? Brand.accent.opacity(0.16) : Color.white.opacity(0.05)))
            .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous)
                .strokeBorder(Brand.accent.opacity(drop.targeted ? 0.9 : 0.35), style: StrokeStyle(lineWidth: 1, dash: [5, 4])))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onDrop(of: [.fileURL], isTargeted: $drop.targeted) { model.drop($0) }
    }

    @ViewBuilder private var recent: some View {
        if model.hiddenInMenuBar {
            note("File names are hidden", "Turn them back on in Settings.")
        } else if let problem = model.listError, model.files.isEmpty {
            note("Your files could not be read", problem)
        } else if model.files.isEmpty {
            note("Nothing stored yet", "Files you store show up here.")
        } else {
            VStack(spacing: 4) {
                HStack {
                    Text("Recent").font(.caption.weight(.semibold)).foregroundStyle(Brand.muted).textCase(.uppercase)
                    Spacer()
                    Button("See all") {
                        MenuBar.shared.close()
                        FilesWindow.shared.show()
                    }
                    .buttonStyle(.link)
                    .font(.caption)
                }
                .padding(.horizontal, 8)
                ForEach(model.files.prefix(4)) { file in
                    RecentRow(file: file, download: { model.quickDownload($0) })
                }
                Spacer(minLength: 0)
            }
        }
    }

    private func note(_ title: String, _ text: String) -> some View {
        VStack(spacing: 4) {
            Spacer()
            Text(title).font(.callout).foregroundStyle(Brand.text)
            Text(text).font(.caption).foregroundStyle(Brand.muted).multilineTextAlignment(.center)
            Spacer()
        }
        .frame(maxWidth: .infinity)
    }

    private var setup: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "externaldrive.badge.plus").font(.system(size: 32)).foregroundStyle(Brand.accent)
            Text("Set up taasant").font(.headline).foregroundStyle(Brand.text)
            Text("Choose where your files go.").font(.callout).foregroundStyle(Brand.muted)
            Button("Open setup") { model.setUp() }
                .controlSize(.large)
            Spacer()
        }
        .frame(maxWidth: .infinity)
        .padding(.bottom, 20)
    }
}
