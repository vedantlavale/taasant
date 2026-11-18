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
    var glass = false

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
            .background(glass ? Color.white.opacity(0.08) : Brand.raised)
        }
    }
}

struct SectionHeader: View {
    let title: String
    var subtitle: String? = nil
    var action: (() -> Void)? = nil
    var actionTitle: String = "Refresh"

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.headline).foregroundStyle(Brand.text)
                if let subtitle { Text(subtitle).font(.caption).foregroundStyle(Brand.muted) }
            }
            Spacer()
            if let action {
                Button(actionTitle, action: action).buttonStyle(.link).font(.caption)
            }
        }
        .padding(.horizontal, 14)
        .padding(.top, 14)
        .padding(.bottom, 8)
    }
}

struct StatusDot: View {
    let on: Bool
    var body: some View {
        Circle().fill(on ? Brand.success : Brand.muted).frame(width: 7, height: 7)
    }
}

struct FileRow: View {
    let file: StoredFile
    let compact: Bool
    let download: (StoredFile) -> Void
    let remove: (StoredFile) -> Void

    var body: some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(file.name)
                    .font(compact ? .callout : .body)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .foregroundStyle(Brand.text)
                Text("\(ByteCountFormatter.string(fromByteCount: file.size, countStyle: .file))  ·  \(file.uploaded.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption2)
                    .foregroundStyle(Brand.muted)
            }
            Spacer()
            Button { download(file) } label: { Image(systemName: "arrow.down.circle") }
                .help("Download")
            Button { remove(file) } label: { Image(systemName: "trash") }
                .help("Delete")
                .foregroundStyle(Brand.danger)
        }
        .buttonStyle(.borderless)
        .padding(.vertical, compact ? 5 : 7)
        .padding(.horizontal, 8)
        .contentShape(Rectangle())
    }
}

struct JobRow: View {
    let job: Job

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
            }
        }
        .padding(.vertical, 5)
        .padding(.horizontal, 8)
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
        .padding(.vertical, 5)
        .padding(.horizontal, 8)
    }
}
