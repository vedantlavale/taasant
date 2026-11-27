import SwiftUI

// One place for a message: a real alert, not a paragraph in the list.
extension View {
    func noticeAlert(_ model: Model) -> some View {
        alert(model.notice?.title ?? "taasant", isPresented: Binding(
            get: { model.notice != nil },
            set: { if !$0 { model.notice = nil } }
        )) {
            Button("OK", role: .cancel) { model.notice = nil }
        } message: {
            Text(model.notice?.text ?? "")
        }
    }
}

// The strip under the menu-bar header. It shows the same work as the
// Transfers section without making the person change tabs to find it.
struct ActivityStrip: View {
    @ObservedObject var model: Model

    var body: some View {
        if let activity = model.activity {
            VStack(spacing: 5) {
                HStack(spacing: 6) {
                    Image(systemName: activity.symbol)
                        .foregroundStyle(Brand.accent)
                    Text(activity.title)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Spacer(minLength: 4)
                    if !activity.detail.isEmpty {
                        Text(activity.detail).monospacedDigit()
                    }
                }
                .font(.caption)
                .foregroundStyle(Brand.text)

                if let fraction = activity.fraction {
                    ProgressView(value: fraction)
                        .progressViewStyle(.linear)
                        .tint(Brand.accent)
                } else {
                    ProgressView()
                        .progressViewStyle(.linear)
                        .tint(Brand.accent)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color.white.opacity(0.08))
        }
    }
}

// The title of a screen, with whatever belongs at its right end.
struct SectionHeader<Trailing: View>: View {
    let title: String
    var subtitle: String? = nil
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(alignment: .center) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.title3.weight(.semibold)).foregroundStyle(Brand.text)
                if let subtitle { Text(subtitle).font(.caption).foregroundStyle(Brand.muted) }
            }
            Spacer()
            trailing
        }
        .padding(.horizontal, 20)
        .padding(.top, 18)
        .padding(.bottom, 12)
    }
}

extension SectionHeader where Trailing == EmptyView {
    init(title: String, subtitle: String? = nil) {
        self.init(title: title, subtitle: subtitle) { EmptyView() }
    }
}

struct StatusDot: View {
    let on: Bool
    var body: some View {
        Circle().fill(on ? Brand.success : Brand.muted).frame(width: 7, height: 7)
    }
}

// Whether the pointer is on a row. (@State would do, but the command line
// tools have no plugin for it, so a small object holds it.)
final class PointerState: ObservableObject {
    @Published var over = false
}

extension FileKind {
    var symbol: String {
        switch self {
        case .image: return "photo"
        case .video: return "film"
        case .audio: return "waveform"
        case .document: return "doc.text"
        case .other: return "doc"
        }
    }
}

// What a file is, as a small icon on a rounded tile.
struct KindTile: View {
    let kind: FileKind
    var size: CGFloat = 36

    var body: some View {
        Image(systemName: kind.symbol)
            .font(.system(size: size * 0.42, weight: .medium))
            .foregroundStyle(Brand.accent)
            .frame(width: size, height: size)
            .background(RoundedRectangle(cornerRadius: size * 0.28, style: .continuous).fill(Color.white.opacity(0.08)))
    }
}

// Where a file is stored, shown only when files are stored in more than one place.
struct PlaceChip: View {
    let name: String

    var body: some View {
        Text(name)
            .font(.caption2.weight(.medium))
            .foregroundStyle(Brand.muted)
            .padding(.horizontal, 7)
            .padding(.vertical, 2)
            .background(Capsule().fill(Color.white.opacity(0.1)))
    }
}

extension StoredFile {
    var details: String {
        "\(ByteCountFormatter.string(fromByteCount: size, countStyle: .file))  ·  \(uploaded.formatted(date: .abbreviated, time: .shortened))"
    }
}

// A file in the window. The two actions are dimmed until the pointer is on
// the row, and are also in its context menu.
struct FileRow: View {
    let file: StoredFile
    let showPlace: Bool
    let download: (StoredFile) -> Void
    let remove: (StoredFile) -> Void
    @StateObject private var pointer = PointerState()

    var body: some View {
        HStack(spacing: 12) {
            KindTile(kind: file.kind)
            VStack(alignment: .leading, spacing: 3) {
                Text(file.name)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .foregroundStyle(Brand.text)
                HStack(spacing: 6) {
                    Text(file.details)
                    if showPlace { PlaceChip(name: file.place) }
                }
                .font(.caption)
                .foregroundStyle(Brand.muted)
            }
            Spacer(minLength: 8)
            HStack(spacing: 6) {
                Button { download(file) } label: { Image(systemName: "arrow.down.circle") }
                    .help("Download")
                Button { remove(file) } label: { Image(systemName: "trash") }
                    .help("Delete")
                    .foregroundStyle(Brand.danger)
            }
            .buttonStyle(.borderless)
            .imageScale(.large)
            .opacity(pointer.over ? 1 : 0.45)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(Color.white.opacity(pointer.over ? 0.07 : 0)))
        .contentShape(Rectangle())
        .onHover { pointer.over = $0 }
        .contextMenu {
            Button("Download…") { download(file) }
            Button("Delete…", role: .destructive) { remove(file) }
        }
    }
}

// A file in the menu bar: just enough to see it was stored, and to get it back.
struct RecentRow: View {
    let file: StoredFile
    let download: (StoredFile) -> Void
    @StateObject private var pointer = PointerState()

    var body: some View {
        HStack(spacing: 10) {
            KindTile(kind: file.kind, size: 30)
            VStack(alignment: .leading, spacing: 2) {
                Text(file.name)
                    .font(.callout)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .foregroundStyle(Brand.text)
                Text(file.details).font(.caption2).foregroundStyle(Brand.muted).lineLimit(1)
            }
            Spacer(minLength: 4)
            Button { download(file) } label: { Image(systemName: "arrow.down.circle") }
                .buttonStyle(.borderless)
                .help("Save to Downloads")
                .opacity(pointer.over ? 1 : 0.45)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(RoundedRectangle(cornerRadius: 10, style: .continuous).fill(Color.white.opacity(pointer.over ? 0.07 : 0)))
        .contentShape(Rectangle())
        .onHover { pointer.over = $0 }
    }
}

struct JobRow: View {
    let job: Job
    var cancel: ((Job) -> Void)? = nil

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: job.kind.symbol)
                .foregroundStyle(job.kind == .delete ? Brand.danger : Brand.accent)
            VStack(alignment: .leading, spacing: 2) {
                Text(job.title).lineLimit(1).truncationMode(.middle).foregroundStyle(Brand.text)
                if let error = job.error {
                    Text(error).foregroundStyle(Brand.danger).lineLimit(2).font(.caption)
                } else if job.finished {
                    if let note = job.note {
                        Text(note).foregroundStyle(Brand.muted).lineLimit(3).font(.caption)
                    } else {
                        Text("Done").font(.caption).foregroundStyle(Brand.success)
                    }
                } else if let fraction = job.fraction {
                    ProgressView(value: fraction)
                } else {
                    ProgressView()
                }
            }
            Spacer()
            if !job.finished && job.error == nil {
                if job.fraction != nil {
                    Text(sizes(job.done, job.total)).font(.caption).foregroundStyle(Brand.muted)
                } else {
                    Text(job.kind.verb.lowercased() + "…").font(.caption).foregroundStyle(Brand.muted)
                }
                if let cancel {
                    Button { cancel(job) } label: { Image(systemName: "xmark.circle.fill") }
                        .buttonStyle(.borderless)
                        .foregroundStyle(Brand.muted)
                        .help(job.cancelled ? "Stopping…" : "Cancel")
                        .disabled(job.cancelled)
                }
            }
        }
        .padding(.vertical, 8)
        .padding(.horizontal, 20)
    }
}

struct DriveUploadRow: View {
    let upload: DriveUpload

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "arrow.up.to.line").foregroundStyle(Brand.accent)
            VStack(alignment: .leading, spacing: 2) {
                Text(upload.title).lineLimit(1).truncationMode(.middle).foregroundStyle(Brand.text)
                ProgressView(value: upload.fraction)
            }
            Spacer()
            Text(sizes(upload.done, upload.total)).font(.caption).foregroundStyle(Brand.muted)
        }
        .padding(.vertical, 8)
        .padding(.horizontal, 20)
    }
}
