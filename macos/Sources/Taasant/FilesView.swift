import SwiftUI
import UniformTypeIdentifiers

// What only this window needs. It is an object, and not @State, so that the
// app also builds with just the command line tools, which cannot expand that
// macro.
final class ViewState: ObservableObject {
    @Published var search = ""
    @Published var targeted = false
    @Published var asked: StoredFile?
}

struct FilesView: View {
    @ObservedObject var model: Model
    @StateObject private var view = ViewState()

    private var shown: [StoredFile] {
        view.search.isEmpty ? model.files : model.files.filter { $0.name.localizedCaseInsensitiveContains(view.search) }
    }

    var body: some View {
        Group {
            if model.configured { list } else { setup }
        }
    }

    private var setup: some View {
        VStack(spacing: 12) {
            Text("Set up taasant").font(.title2)
            Text("It needs a Telegram bot and a chat to keep your files in. A terminal window asks for them once.")
                .multilineTextAlignment(.center)
            Button("Open setup") { model.setUp() }.keyboardShortcut(.defaultAction)
        }
        .padding(32)
    }

    private var list: some View {
        VStack(spacing: 0) {
            HStack {
                TextField("Search", text: $view.search).textFieldStyle(.roundedBorder)
                Button("Upload…") { chooseFiles() }
            }
            .padding(10)
            if shown.isEmpty {
                Spacer()
                Text(model.files.isEmpty ? "No files yet. Drop some here." : "Nothing matches.")
                    .foregroundStyle(.secondary)
                Spacer()
            } else {
                List(shown) { file in row(file) }
            }
            if !model.jobs.isEmpty || model.driveUpload != nil { jobs }
            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(.red).padding(.horizontal, 10)
            }
        }
        .overlay { if view.targeted { RoundedRectangle(cornerRadius: 8).stroke(Color.accentColor, lineWidth: 3).padding(4) } }
        .onDrop(of: [.fileURL], isTargeted: $view.targeted) { model.drop($0) }
        .confirmationDialog("Delete \(view.asked?.name ?? "")?", isPresented: Binding(get: { view.asked != nil }, set: { if !$0 { view.asked = nil } })) {
            Button("Delete", role: .destructive) { if let file = view.asked { model.delete(file) } }
        } message: {
            Text("It is taken off the drive. Telegram keeps its encrypted messages if the file is older than 48 hours.")
        }
    }

    private func row(_ file: StoredFile) -> some View {
        HStack {
            VStack(alignment: .leading) {
                Text(file.name).lineLimit(1).truncationMode(.middle)
                Text("\(ByteCountFormatter.string(fromByteCount: file.size, countStyle: .file))  ·  \(file.uploaded.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Button { model.download(file) } label: { Image(systemName: "arrow.down.circle") }
                .help("Download")
            Button { view.asked = file } label: { Image(systemName: "trash") }
                .help("Delete")
        }
        .buttonStyle(.borderless)
    }

    private var jobs: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let upload = model.driveUpload {
                HStack {
                    Image(systemName: "arrow.up")
                    Text("\(upload.title) (copying to the drive)").lineLimit(1).truncationMode(.middle)
                    Spacer()
                    Text(sizes(upload.done, upload.total)).font(.caption).foregroundStyle(.secondary)
                    ProgressView(value: upload.fraction).frame(width: 120)
                }
            }
            ForEach(model.jobs) { job in
                HStack {
                    Image(systemName: job.kind == .upload ? "arrow.up" : "arrow.down")
                    Text(job.title).lineLimit(1).truncationMode(.middle)
                    Spacer()
                    if let error = job.error {
                        Text(error).foregroundStyle(.red).lineLimit(2).font(.caption)
                    } else if job.finished {
                        Image(systemName: "checkmark").foregroundStyle(.green)
                    } else {
                        Text(sizes(job.done, job.total)).font(.caption).foregroundStyle(.secondary)
                        ProgressView(value: job.fraction).frame(width: 120)
                    }
                }
            }
            if model.jobs.contains(where: { $0.finished || $0.error != nil }) {
                Button("Clear") { model.jobs.removeAll { $0.finished || $0.error != nil } }
                    .buttonStyle(.link).font(.caption)
            }
        }
        .padding(10)
        .background(.bar)
    }

    private func chooseFiles() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        if panel.runModal() == .OK { model.upload(panel.urls) }
    }
}
