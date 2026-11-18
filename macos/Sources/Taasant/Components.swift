import SwiftUI

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
            Image(systemName: job.kind == .upload ? "arrow.up" : "arrow.down")
                .foregroundStyle(Brand.accent)
            VStack(alignment: .leading, spacing: 2) {
                Text(job.title).lineLimit(1).truncationMode(.middle).foregroundStyle(Brand.text)
                if let error = job.error {
                    Text(error).foregroundStyle(Brand.danger).lineLimit(2).font(.caption)
                } else if job.finished {
                    Text("Done").font(.caption).foregroundStyle(Brand.success)
                } else {
                    ProgressView(value: job.fraction)
                }
            }
            Spacer()
            if !job.finished && job.error == nil {
                Text(sizes(job.done, job.total)).font(.caption).foregroundStyle(Brand.muted)
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
