import AppKit
import SwiftUI

final class FilesUI: ObservableObject {
    @Published var search = ""
    @Published var asked: StoredFile?
}

struct FilesSection: View {
    @ObservedObject var model: Model
    let compact: Bool
    @StateObject private var ui = FilesUI()

    private var shown: [StoredFile] {
        ui.search.isEmpty ? model.files : model.files.filter { $0.name.localizedCaseInsensitiveContains(ui.search) }
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                if !compact {
                    TextField("Search", text: $ui.search).textFieldStyle(.roundedBorder)
                }
                Spacer()
                Button("Upload…") { chooseFiles() }
            }
            .padding(12)

            if compact {
                dropZone
            }

            if compact && model.hiddenInMenuBar {
                Spacer()
                VStack(spacing: 10) {
                    Image(systemName: "eye.slash").font(.title2).foregroundStyle(Brand.muted)
                    Text("Files are hidden").foregroundStyle(Brand.text)
                    Text("Only this view is hidden. Nothing was changed in Telegram.")
                        .font(.caption).foregroundStyle(Brand.muted)
                        .multilineTextAlignment(.center)
                    Button("Show files") { model.hiddenInMenuBar = false }
                }
                .padding()
                Spacer()
            } else if shown.isEmpty {
                Spacer()
                Text(model.files.isEmpty ? "No files yet. Drop some here." : "Nothing matches.")
                    .foregroundStyle(Brand.muted)
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(shown) { file in
                            FileRow(file: file, compact: compact,
                                    download: { compact ? model.quickDownload($0) : model.download($0) },
                                    remove: { ui.asked = $0 })
                        }
                    }
                }
            }

            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(Brand.danger)
                    .padding(.horizontal, 12).padding(.bottom, 8)
            }
        }
        .confirmationDialog("Delete \(ui.asked?.name ?? "")?", isPresented: Binding(
            get: { ui.asked != nil },
            set: { if !$0 { ui.asked = nil } }
        )) {
            Button("Delete", role: .destructive) {
                if let file = ui.asked { model.delete(file) }
            }
        } message: {
            Text("It is taken off the drive. Telegram keeps its encrypted messages if the file is older than 48 hours.")
        }
    }

    private var dropZone: some View {
        VStack(spacing: 4) {
            Image(systemName: "arrow.up.circle").foregroundStyle(Brand.accent)
            Text("Drop files here or tap to choose").font(.caption2).foregroundStyle(Brand.muted)
        }
        .frame(maxWidth: .infinity, minHeight: 62)
        .background(RoundedRectangle(cornerRadius: 6).strokeBorder(Brand.accent.opacity(0.5), style: StrokeStyle(lineWidth: 1, dash: [4])))
        .padding(.horizontal, 12)
        .contentShape(Rectangle())
        .onTapGesture {
            if compact { MenuBar.shared.pick() } else { chooseFiles() }
        }
    }

    private func chooseFiles() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        if panel.runModal() == .OK { model.upload(panel.urls) }
    }
}

struct TransfersSection: View {
    @ObservedObject var model: Model

    private var hasFinished: Bool {
        model.jobs.contains(where: { $0.finished || $0.error != nil })
    }

    var body: some View {
        VStack(spacing: 0) {
            if hasFinished {
                SectionHeader(title: "Transfers", subtitle: "What is moving now",
                              action: { model.jobs.removeAll { $0.finished || $0.error != nil } },
                              actionTitle: "Clear")
            } else {
                SectionHeader(title: "Transfers", subtitle: "What is moving now")
            }

            ScrollView {
                LazyVStack(spacing: 0) {
                    if let upload = model.driveUpload { DriveUploadRow(upload: upload) }
                    ForEach(model.jobs) { JobRow(job: $0) }
                }
            }
            if model.jobs.isEmpty && model.driveUpload == nil {
                Spacer()
                Text("Nothing is transferring.").foregroundStyle(Brand.muted)
                Spacer()
            }
            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(Brand.danger).padding(12)
            }
        }
    }
}

struct DriveSection: View {
    @ObservedObject var model: Model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionHeader(title: "Drive", subtitle: "Your files as a disk in Finder")

            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    StatusDot(on: model.mounted)
                    Text(model.mounted ? "Connected at /Volumes/taasant" : "Not connected")
                        .foregroundStyle(Brand.text)
                }
                .font(.subheadline)

                HStack {
                    if model.mounted {
                        Button("Open drive") { model.openDrive() }
                        Button("Eject") { model.eject() }
                    } else {
                        Button("Connect drive") { model.connect() }
                    }
                    Toggle("Start at login", isOn: Binding(
                        get: { model.autostart },
                        set: { model.setAutostart($0) }
                    ))
                    .disabled(!model.configured)
                }

                if let upload = model.driveUpload {
                    DriveUploadRow(upload: upload)
                }

                Text("A copy into the drive finishes when the upload to Telegram finishes. The menu-bar icon shows the real progress.")
                    .font(.caption).foregroundStyle(Brand.muted)
                Text("Drag files in to store them. Drag out or open to get them back.")
                    .font(.caption).foregroundStyle(Brand.muted)

                if let message = model.message {
                    Text(message).font(.caption).foregroundStyle(Brand.danger)
                }
            }
            .padding(.horizontal, 14)
            Spacer()
        }
    }
}

struct SettingsSection: View {
    @ObservedObject var model: Model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionHeader(title: "Settings", subtitle: "This Mac, not your Telegram chat")

            VStack(alignment: .leading, spacing: 14) {
                if !model.configured {
                    Button("Open setup") { model.setUp() }
                }
                Toggle("Hide files in the menu bar", isOn: $model.hiddenInMenuBar)
                    .foregroundStyle(Brand.text)

                VStack(alignment: .leading, spacing: 4) {
                    Text("Settings").font(.caption).foregroundStyle(Brand.muted)
                    Text(model.settingsPath).font(.caption2).foregroundStyle(Brand.text)
                        .textSelection(.enabled)
                    Text("Index").font(.caption).foregroundStyle(Brand.muted).padding(.top, 4)
                    Text(model.indexFile).font(.caption2).foregroundStyle(Brand.text)
                        .textSelection(.enabled)
                }

                Button("Open log") {
                    let log = FileManager.default.homeDirectoryForCurrentUser
                        .appendingPathComponent("Library/Logs/taasant.log")
                    NSWorkspace.shared.open(log)
                }
                .disabled(!FileManager.default.fileExists(atPath: NSHomeDirectory() + "/Library/Logs/taasant.log"))

                Text("taasant \(model.version)").font(.caption).foregroundStyle(Brand.muted)
                Spacer()
            }
            .padding(.horizontal, 14)
        }
    }
}
