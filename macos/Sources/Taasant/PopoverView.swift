import SwiftUI

// The popover uses the same four sections as the window, in a compact form.
struct PopoverView: View {
    @ObservedObject var model: Model
    @StateObject private var sidebar = SidebarState()

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider().background(Brand.line)
            ActivityStrip(model: model)
            HStack(spacing: 0) {
                Sidebar(model: model, state: sidebar, compact: true)
                Divider().background(Brand.line)

                Group {
                    switch sidebar.section {
                    case .files: FilesSection(model: model, compact: true)
                    case .transfers: TransfersSection(model: model)
                    case .drive: DriveSection(model: model)
                    case .settings: SettingsSection(model: model)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .glassPane()
            }
        }
        .glassScrim()
        .frame(width: 400, height: 440)
        .noticeAlert(model)
    }

    private var header: some View {
        HStack(spacing: 8) {
            Brand.ant().frame(width: 28, height: 15)
            Text("taasant").font(.headline).foregroundStyle(Brand.text)
            StatusDot(on: model.mounted)
                .help(model.mounted ? "Drive connected" : "Drive not connected")
            Spacer()

            Menu {
                if model.mounted {
                    Button("Open drive") { model.openDrive() }
                    Button("Eject") { model.eject() }
                } else if model.configured {
                    Button("Connect drive") { model.connect() }
                }
                Toggle("Start at login", isOn: Binding(
                    get: { model.autostart },
                    set: { model.setAutostart($0) }
                ))
                .disabled(!model.configured)
                Divider()
                Button("Show all files…") {
                    MenuBar.shared.close()
                    FilesWindow.shared.show()
                }
                Divider()
                Button("Quit taasant") { NSApp.terminate(nil) }
            } label: {
                Image(systemName: "ellipsis.circle")
                    .foregroundStyle(Brand.muted)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .padding(.horizontal, 12)
        .frame(height: 46)
    }
}
