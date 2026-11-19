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
    @StateObject private var ui = FilesUI()

    private var shown: [StoredFile] {
        var found = model.files.filter { ui.filter.matches($0) }
        if !ui.search.isEmpty {
            found = found.filter { $0.name.localizedCaseInsensitiveContains(ui.search) }
        }
        return ui.sort.apply(to: found)
    }

    // The place of a file is only worth showing once there is a choice of places.
    private var showPlace: Bool { Set(model.files.map(\.place)).count > 1 || model.places.count > 1 }

    var body: some View {
        VStack(spacing: 0) {
            SectionHeader(title: "Files", subtitle: subtitle) { uploadMenu }

            if !model.configured {
                setupPrompt
            } else if model.files.isEmpty {
                emptyState
            } else {
                controls
                    .padding(.horizontal, 20)
                    .padding(.bottom, 10)
                list
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

    private var subtitle: String {
        guard model.configured else { return "Nothing is set up yet" }
        let count = model.files.count
        return count == 1 ? "1 file" : "\(count) files"
    }

    // Upload goes to the default place. With more places, the arrow beside it
    // lists the others.
    private var uploadMenu: some View {
        Menu {
            if model.places.count > 1 {
                ForEach(model.places) { place in
                    Button("Upload to \(place.name)…") { chooseFiles(to: place.name) }
                }
            }
        } label: {
            Label("Upload", systemImage: "arrow.up")
        } primaryAction: {
            chooseFiles(to: nil)
        }
        .menuStyle(.borderedButton)
        .fixedSize()
        .disabled(!model.configured)
    }

    private var controls: some View {
        VStack(spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").foregroundStyle(Brand.muted)
                TextField("Search", text: $ui.search).textFieldStyle(.plain)
                if !ui.search.isEmpty {
                    Button { ui.search = "" } label: { Image(systemName: "xmark.circle.fill") }
                        .buttonStyle(.plain)
                        .foregroundStyle(Brand.muted)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .background(RoundedRectangle(cornerRadius: 9, style: .continuous).fill(Color.white.opacity(0.08)))

            HStack(spacing: 14) {
                menu(ui.filter.title, symbol: "line.3.horizontal.decrease.circle", help: "Show one kind of file") {
                    ForEach(FileFilter.allCases) { filter in
                        Button { ui.filter = filter } label: {
                            if ui.filter == filter { Label(filter.title, systemImage: "checkmark") } else { Text(filter.title) }
                        }
                    }
                }
                menu(ui.sort.title, symbol: "arrow.up.arrow.down", help: "Order the files") {
                    ForEach(FileSort.allCases) { sort in
                        Button { ui.sort = sort } label: {
                            if ui.sort == sort { Label(sort.title, systemImage: "checkmark") } else { Text(sort.title) }
                        }
                    }
                }
                Spacer(minLength: 0)
                Button { Task { await model.reload() } } label: { Image(systemName: "arrow.clockwise") }
                    .buttonStyle(.borderless)
                    .foregroundStyle(Brand.muted)
                    .help("Refresh")
            }
        }
    }

    private func menu<Items: View>(_ title: String, symbol: String, help: String, @ViewBuilder items: () -> Items) -> some View {
        Menu { items() } label: { Label(title, systemImage: symbol).font(.callout) }
            .menuStyle(.borderlessButton)
            .fixedSize()
            .help(help)
    }

    private var list: some View {
        Group {
            if shown.isEmpty {
                VStack { Spacer(); Text("Nothing matches.").foregroundStyle(Brand.muted); Spacer() }
            } else {
                ScrollView {
                    LazyVStack(spacing: 2) {
                        ForEach(shown) { file in
                            FileRow(file: file, showPlace: showPlace,
                                    download: { model.download($0) },
                                    remove: { ui.asked = $0 })
                        }
                    }
                    .padding(.horizontal, 8)
                    .padding(.bottom, 12)
                }
            }
        }
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "tray.and.arrow.up").font(.system(size: 34)).foregroundStyle(Brand.accent)
            Text("No files yet").font(.title3.weight(.semibold)).foregroundStyle(Brand.text)
            Text("Drop files anywhere in this window, or choose them.")
                .font(.callout).foregroundStyle(Brand.muted)
            Button("Choose files…") { chooseFiles(to: nil) }
                .controlSize(.large)
            Spacer()
        }
        .frame(maxWidth: .infinity)
    }

    private var setupPrompt: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "externaldrive.badge.plus").font(.system(size: 34)).foregroundStyle(Brand.accent)
            Text("Set up taasant").font(.title3.weight(.semibold)).foregroundStyle(Brand.text)
            Text("Choose where your files go, and taasant makes the key that locks them.")
                .font(.callout).foregroundStyle(Brand.muted)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 320)
            Button("Open setup") { model.setUp() }
                .controlSize(.large)
                .keyboardShortcut(.defaultAction)
            Spacer()
        }
        .frame(maxWidth: .infinity)
    }

    private var deleteMessage: String {
        let name = ui.asked?.name ?? "The file"
        if ui.asked?.place == "telegram" || ui.asked?.backend == nil {
            return "“\(name)” is taken off the drive. Telegram keeps its encrypted messages if the file is older than 48 hours."
        }
        return "“\(name)” is deleted from \(ui.asked?.place ?? "its place"). This cannot be undone."
    }

    private func chooseFiles(to place: String?) {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        if panel.runModal() == .OK { model.upload(panel.urls, to: place) }
    }
}

struct TransfersSection: View {
    @ObservedObject var model: Model

    private var hasFinished: Bool {
        model.jobs.contains(where: { $0.finished || $0.error != nil })
    }

    var body: some View {
        VStack(spacing: 0) {
            SectionHeader(title: "Transfers", subtitle: "What is moving now") {
                if hasFinished {
                    Button("Clear finished") { model.clearFinished() }
                        .controlSize(.small)
                }
            }

            if model.jobs.isEmpty && model.driveUpload == nil {
                VStack(spacing: 8) {
                    Spacer()
                    Image(systemName: "arrow.triangle.2.circlepath").font(.system(size: 30)).foregroundStyle(Brand.muted)
                    Text("Nothing is transferring").foregroundStyle(Brand.muted)
                    Spacer()
                }
                .frame(maxWidth: .infinity)
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        if let upload = model.driveUpload { DriveUploadRow(upload: upload) }
                        ForEach(model.jobs) { JobRow(job: $0, cancel: { model.cancel($0) }) }
                    }
                }
            }
        }
    }
}

// A block of a settings screen: a small title and what belongs under it.
struct Card<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title).font(.caption.weight(.semibold)).foregroundStyle(Brand.muted).textCase(.uppercase)
            VStack(alignment: .leading, spacing: 12) { content }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(Color.white.opacity(0.06)))
        }
    }
}

struct DriveSection: View {
    @ObservedObject var model: Model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionHeader(title: "Drive", subtitle: "Your files as a disk in Finder")

            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    Card(title: "Status") {
                        HStack(spacing: 8) {
                            StatusDot(on: model.mounted)
                            Text(model.mounted ? "Connected at /Volumes/taasant" : "Not connected")
                                .foregroundStyle(Brand.text)
                        }
                        HStack {
                            if model.mounted {
                                Button("Open in Finder") { model.openDrive() }
                                Button("Eject") { model.eject() }
                            } else {
                                Button("Connect drive") { model.connect() }
                                    .disabled(!model.configured)
                            }
                        }
                        if let upload = model.driveUpload {
                            DriveUploadRow(upload: upload).padding(.horizontal, -20)
                        }
                    }

                    Card(title: "At login") {
                        Toggle("Connect the drive when I log in", isOn: Binding(
                            get: { model.autostart },
                            set: { model.setAutostart($0) }
                        ))
                        .disabled(!model.configured || model.autostartBusy)
                    }

                    Text("Drag files into the drive to store them, and drag them out or open them to get them back. A copy into the drive is finished when the upload to \(model.standardLabel) is, and the menu-bar icon shows how far it has got.")
                        .font(.caption).foregroundStyle(Brand.muted)
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
        }
    }
}

struct SettingsSection: View {
    @ObservedObject var model: Model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionHeader(title: "Settings", subtitle: "This Mac")

            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    Card(title: "Places for your files") {
                        if model.places.isEmpty {
                            Text("Nothing is set up yet.").foregroundStyle(Brand.muted)
                        }
                        ForEach(model.places) { place in
                            HStack(spacing: 10) {
                                Image(systemName: place.type == "telegram" ? "paperplane" : "externaldrive.connected.to.line.below")
                                    .foregroundStyle(Brand.accent)
                                    .frame(width: 22)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(place.name).foregroundStyle(Brand.text)
                                    Text("\(place.label) · \(place.detail)")
                                        .font(.caption).foregroundStyle(Brand.muted)
                                        .lineLimit(1).truncationMode(.middle)
                                }
                                Spacer()
                                if place.name == model.standard && model.places.count > 1 {
                                    Text("Default").font(.caption2.weight(.medium)).foregroundStyle(Brand.accent)
                                }
                            }
                        }
                        Button(model.configured ? "Add a place…" : "Open setup") { model.setUp() }
                        if model.configured {
                            Text("New files go to the default place. To send one somewhere else, use the arrow beside Upload.")
                                .font(.caption).foregroundStyle(Brand.muted)
                        }
                    }

                    Card(title: "Menu bar") {
                        Toggle("Show recent files", isOn: Binding(
                            get: { !model.hiddenInMenuBar },
                            set: { model.hiddenInMenuBar = !$0 }
                        ))
                        .foregroundStyle(Brand.text)
                        Text("Turn this off to keep file names out of the menu bar, for a screen share. Nothing is changed in your files.")
                            .font(.caption).foregroundStyle(Brand.muted)
                    }

                    Card(title: "On this Mac") {
                        pathRow("Settings", model.settingsPath)
                        pathRow("List of files", model.indexFile)
                        HStack {
                            Button("Open log") {
                                NSWorkspace.shared.open(FileManager.default.homeDirectoryForCurrentUser
                                    .appendingPathComponent("Library/Logs/taasant.log"))
                            }
                            .disabled(!FileManager.default.fileExists(atPath: NSHomeDirectory() + "/Library/Logs/taasant.log"))
                            Spacer()
                            Text("taasant \(model.version)").font(.caption).foregroundStyle(Brand.muted)
                        }
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
        }
    }

    private func pathRow(_ title: String, _ path: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(.caption).foregroundStyle(Brand.muted)
            HStack {
                Text(path).font(.caption).foregroundStyle(Brand.text).textSelection(.enabled).lineLimit(1).truncationMode(.middle)
                Spacer()
                Button { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)]) } label: {
                    Image(systemName: "magnifyingglass.circle")
                }
                .buttonStyle(.borderless)
                .help("Show in Finder")
            }
        }
    }
}
