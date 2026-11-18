import SwiftUI
import UniformTypeIdentifiers

// An object and not @State, as in FilesView.
final class PopoverState: ObservableObject {
    @Published var targeted = false
    @Published var hovered: String?
    @Published var asked: String?
}

struct PopoverView: View {
    @ObservedObject var model: Model
    @StateObject private var view = PopoverState()

    private var active: [Job] { model.jobs.filter { !$0.finished && $0.error == nil } }

    var body: some View {
        VStack(spacing: 0) {
            header
            if model.configured {
                dropZone
                recent
            } else {
                setup
            }
        }
        .frame(width: 340, height: 440)
    }

    private var header: some View {
        HStack(spacing: 6) {
            Text("taasant").font(.headline)
            Circle().fill(model.mounted ? Color.green : Color.secondary).frame(width: 7, height: 7)
                .help(model.mounted ? "Drive connected" : "Drive not connected")
            Spacer()
            Menu {
                if model.mounted {
                    Button("Open drive") { model.openDrive() }
                    Button("Eject") { model.eject() }
                } else if model.configured {
                    Button("Connect drive") { model.connect() }
                }
                Toggle("Start at login", isOn: Binding(get: { model.autostart }, set: { model.setAutostart($0) }))
                    .disabled(!model.configured)
                Divider()
                Button("Show all files…") {
                    MenuBar.shared.close()
                    FilesWindow.shared.show()
                }
                Divider()
                Button("Quit taasant") { NSApp.terminate(nil) }
            } label: {
                Image(systemName: "gearshape")
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .padding(.horizontal, 12)
        .padding(.top, 12)
        .padding(.bottom, 8)
    }

    private var setup: some View {
        VStack(spacing: 12) {
            Spacer()
            Text("Set up taasant").font(.title3)
            Text("It needs a Telegram bot and a chat to keep your files in. A terminal window asks for them once.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
            Button("Open setup") { model.setUp() }.keyboardShortcut(.defaultAction)
            Spacer()
        }
        .padding(24)
    }

    private var dropZone: some View {
        VStack(spacing: 6) {
            if let job = active.first(where: { $0.kind == .upload }) {
                let uploads = model.jobs.filter { $0.kind == .upload }
                let at = (uploads.firstIndex { $0.id == job.id } ?? 0) + 1
                ProgressView(value: job.fraction).padding(.horizontal, 24)
                Text("Uploading \(at) of \(uploads.count)").font(.caption)
                Text(job.title).font(.caption2).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                    .padding(.horizontal, 12)
            } else {
                Image(systemName: "arrow.up.circle").font(.title2)
                Text("Drop files here or click to select").font(.caption).foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: 90)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.secondary.opacity(view.targeted ? 0.25 : 0.1)))
        .overlay(RoundedRectangle(cornerRadius: 8)
            .strokeBorder(Color.accentColor, style: StrokeStyle(lineWidth: 1.5, dash: [5]))
            .opacity(view.targeted ? 1 : 0))
        .contentShape(Rectangle())
        .onTapGesture { MenuBar.shared.pick() }
        .onDrop(of: [.fileURL], isTargeted: $view.targeted) { model.drop($0) }
        .padding(.horizontal, 12)
    }

    private var recent: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Recent uploads").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button { Task { await model.reload() } } label: { Image(systemName: "arrow.clockwise") }
                    .buttonStyle(.borderless)
                    .help("Refresh")
            }
            .padding(.horizontal, 14)
            .padding(.top, 12)
            .padding(.bottom, 4)
            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(.red).padding(.horizontal, 14)
            }
            ScrollView {
                LazyVStack(spacing: 0) {
                    if let upload = model.driveUpload { driveRow(upload) }
                    ForEach(model.jobs) { job in jobRow(job) }
                    ForEach(model.files.prefix(30)) { file in row(file) }
                    if model.files.isEmpty && model.jobs.isEmpty && model.driveUpload == nil {
                        Text("No files yet").foregroundStyle(.secondary).padding(.top, 40)
                    }
                }
                .padding(.horizontal, 6)
            }
            if model.jobs.contains(where: { $0.finished || $0.error != nil }) {
                Button("Clear finished") { model.clearFinished() }
                    .buttonStyle(.link).font(.caption).padding(6)
            }
        }
    }

    // MARK: rows

    private func icon(_ name: String) -> some View {
        let type = UTType(filenameExtension: (name as NSString).pathExtension) ?? .data
        return Image(nsImage: NSWorkspace.shared.icon(for: type)).resizable().frame(width: 28, height: 28)
    }

    private func row(_ file: StoredFile) -> some View {
        HStack(spacing: 8) {
            icon(file.name)
            VStack(alignment: .leading, spacing: 1) {
                Text(file.name).font(.callout).lineLimit(1).truncationMode(.middle)
                Text("\(ByteCountFormatter.string(fromByteCount: file.size, countStyle: .file))  ·  \(file.uploaded.formatted(.relative(presentation: .numeric, unitsStyle: .abbreviated)))")
                    .font(.caption2).foregroundStyle(.secondary)
            }
            Spacer()
            if view.asked == file.id {
                Text("Delete?").font(.caption)
                Button("Cancel") { view.asked = nil }
                Button("Delete") {
                    view.asked = nil
                    model.delete(file)
                }
                .foregroundStyle(.red)
            } else {
                // Always present, so keyboard and accessibility users can
                // reach them. Hover only makes them stand out.
                Button { model.quickDownload(file) } label: { Image(systemName: "arrow.down.circle") }
                    .help("Download to Downloads")
                    .opacity(view.hovered == file.id ? 1 : 0.55)
                Button { view.asked = file.id } label: { Image(systemName: "trash") }
                    .help("Delete")
                    .opacity(view.hovered == file.id ? 1 : 0.55)
            }
        }
        .buttonStyle(.borderless)
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(RoundedRectangle(cornerRadius: 6).fill(Color.secondary.opacity(view.hovered == file.id ? 0.12 : 0)))
        .onHover { inside in
            if inside { view.hovered = file.id } else if view.hovered == file.id { view.hovered = nil }
        }
    }

    private func driveRow(_ upload: DriveUpload) -> some View {
        progressRow(name: upload.title, kind: "Copying to the drive", fraction: upload.fraction,
                    detail: sizes(upload.done, upload.total), failed: nil)
    }

    private func jobRow(_ job: Job) -> some View {
        progressRow(name: job.title, kind: job.kind == .upload ? "Uploading" : "Downloading", fraction: job.fraction,
                    detail: job.finished ? nil : sizes(job.done, job.total), failed: job.error, finished: job.finished)
    }

    private func progressRow(name: String, kind: String, fraction: Double?, detail: String?, failed: String?,
                             finished: Bool = false) -> some View {
        HStack(spacing: 8) {
            icon(name)
            VStack(alignment: .leading, spacing: 2) {
                Text(name).font(.callout).lineLimit(1).truncationMode(.middle)
                if let failed {
                    Text(failed).font(.caption2).foregroundStyle(.red).lineLimit(2)
                } else if finished {
                    Text("Done").font(.caption2).foregroundStyle(.secondary)
                } else {
                    ProgressView(value: fraction)
                    Text("\(kind)  ·  \(detail ?? "")").font(.caption2).foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 0)
            if finished { Image(systemName: "checkmark").foregroundStyle(.green) }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
    }
}
