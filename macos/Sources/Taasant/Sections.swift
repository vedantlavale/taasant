import AppKit
import SwiftUI

enum FileFilter: String, CaseIterable, Identifiable {
    case all, image, video, audio, document, other

    var id: String { rawValue }

    var title: String {
        switch self {
        case .all: return "All files"
        case .image: return "Images"
        case .video: return "Video"
        case .audio: return "Audio"
        case .document: return "Documents"
        case .other: return "Other"
        }
    }

    func matches(_ file: StoredFile) -> Bool {
        self == .all || file.kind.rawValue == rawValue
    }
}

enum FileSort: String, CaseIterable, Identifiable {
    case newest, oldest, name, largest

    var id: String { rawValue }

    var title: String {
        switch self {
        case .newest: return "Newest"
        case .oldest: return "Oldest"
        case .name: return "Name"
        case .largest: return "Largest"
        }
    }

    func apply(to files: [StoredFile]) -> [StoredFile] {
        switch self {
        case .newest:
            return files.sorted {
                $0.uploaded == $1.uploaded
                    ? $0.name.localizedStandardCompare($1.name) == .orderedAscending
                    : $0.uploaded > $1.uploaded
            }
        case .oldest:
            return files.sorted {
                $0.uploaded == $1.uploaded
                    ? $0.name.localizedStandardCompare($1.name) == .orderedAscending
                    : $0.uploaded < $1.uploaded
            }
        case .name:
            return files.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        case .largest:
            return files.sorted {
                $0.size == $1.size
                    ? $0.name.localizedStandardCompare($1.name) == .orderedAscending
                    : $0.size > $1.size
            }
        }
    }
}

final class FilesUI: ObservableObject {
    @Published var search = ""
    @Published var filter: FileFilter = .all
    @Published var sort: FileSort = .newest
    @Published var asked: StoredFile?
}

struct FilesSection: View {
    @ObservedObject var model: Model
    let compact: Bool
    @StateObject private var ui = FilesUI()

    private var shown: [StoredFile] {
        var found = model.files.filter { ui.filter.matches($0) }
        if !ui.search.isEmpty {
            found = found.filter { $0.name.localizedCaseInsensitiveContains(ui.search) }
        }
        return ui.sort.apply(to: found)
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                if !compact {
                    TextField("Search", text: $ui.search).textFieldStyle(.roundedBorder)
                }
                Spacer(minLength: 0)
                Button {
                    Task { await model.reload() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(Brand.muted)
                .help("Refresh")
                if !compact {
                    Button("Upload…") { chooseFiles() }
                }
            }
            .padding(.horizontal, 12)
            .padding(.top, 12)
            .padding(.bottom, model.files.isEmpty ? 12 : 6)

            if compact {
                dropZone
            }

            if !model.files.isEmpty {
                filterBar
                    .padding(.horizontal, 12)
                    .padding(.bottom, 8)
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
        }
        .alert("Delete this file?", isPresented: Binding(
            get: { ui.asked != nil },
            set: { if !$0 { ui.asked = nil } }
        )) {
            Button("Cancel", role: .cancel) { ui.asked = nil }
            Button("Delete", role: .destructive) {
                if let file = ui.asked { model.delete(file) }
                ui.asked = nil
            }
        } message: {
            Text(deleteMessage)
        }
    }

    private var deleteMessage: String {
        let name = ui.asked?.name ?? "The file"
        return "“\(name)” is taken off the drive. Telegram keeps its encrypted messages if the file is older than 48 hours."
    }

    private var filterBar: some View {
        HStack(spacing: 12) {
            Menu {
                ForEach(FileFilter.allCases) { filter in
                    Button {
                        ui.filter = filter
                    } label: {
                        if ui.filter == filter {
                            Label(filter.title, systemImage: "checkmark")
                        } else {
                            Text(filter.title)
                        }
                    }
                }
            } label: {
                Label(ui.filter.title, systemImage: "line.3.horizontal.decrease.circle")
                    .font(.caption)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
            .help("Show one kind of file")

            Menu {
                ForEach(FileSort.allCases) { sort in
                    Button {
                        ui.sort = sort
                    } label: {
                        if ui.sort == sort {
                            Label(sort.title, systemImage: "checkmark")
                        } else {
                            Text(sort.title)
                        }
                    }
                }
            } label: {
                Label(ui.sort.title, systemImage: "arrow.up.arrow.down")
                    .font(.caption)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
            .help("Order the files")

            Spacer(minLength: 0)
            Text("\(shown.count)").font(.caption2).foregroundStyle(Brand.muted)
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
