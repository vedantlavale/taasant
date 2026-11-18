import AppKit
import ServiceManagement
import SwiftUI

struct StoredFile: Identifiable, Hashable, Decodable {
    let name: String
    let id: String
    let size: Int64
    let uploaded: Date
    // Where the file is stored. Files from before there were several places
    // have none written down, and are in Telegram.
    let backend: String?

    var place: String { backend ?? "telegram" }
}

// A place files can be stored, as the settings file lists it.
struct Place: Identifiable, Hashable {
    let name: String
    let type: String
    // What tells two places of one kind apart: a bucket and its server.
    let detail: String

    var id: String { name }

    var label: String {
        switch type {
        case "telegram": return "Telegram"
        case "s3": return "S3 bucket"
        default: return type
        }
    }

    // The places and the default one, read from the settings file. The keys
    // and tokens in it are never read into the app.
    static func load(from url: URL) -> (places: [Place], standard: String) {
        guard let data = try? Data(contentsOf: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return ([], "") }
        var found: [Place] = []
        if let backends = json["backends"] as? [String: [String: Any]] {
            for (name, fields) in backends.sorted(by: { $0.key < $1.key }) {
                let type = fields["type"] as? String ?? ""
                let host = (fields["endpoint"] as? String).flatMap { URL(string: $0)?.host } ?? ""
                let bucket = fields["bucket"] as? String ?? ""
                found.append(Place(name: name, type: type, detail: type == "s3" ? "\(bucket) on \(host)" : "a bot of your own"))
            }
        } else if json["token"] != nil {
            // Settings from before there were several places.
            found.append(Place(name: "telegram", type: "telegram", detail: "a bot of your own"))
        }
        let standard = json["default"] as? String ?? (found.first?.name ?? "")
        // The default comes first, as it is the one that matters.
        found.sort { ($0.name == standard ? 0 : 1, $0.name) < ($1.name == standard ? 0 : 1, $1.name) }
        return (found, standard)
    }
}

enum FileKind: String {
    case image, video, audio, document, other
}

extension StoredFile {
    // The filter bar groups files by what they are, so a screenshot and a
    // video can be found without searching for their names.
    var kind: FileKind {
        switch (name as NSString).pathExtension.lowercased() {
        case "png", "jpg", "jpeg", "gif", "heic", "heif", "webp", "tif", "tiff", "bmp", "svg", "ico":
            return .image
        case "mp4", "mov", "m4v", "avi", "mkv", "webm", "mpg", "mpeg":
            return .video
        case "mp3", "m4a", "wav", "aif", "aiff", "flac", "ogg", "opus":
            return .audio
        case "pdf", "doc", "docx", "txt", "md", "csv", "xls", "xlsx", "ppt", "pptx",
             "pages", "numbers", "key", "rtf", "html", "htm", "json", "xml", "epub":
            return .document
        default:
            return .other
        }
    }
}

// A message for the person using the app. It is shown as an alert, never as
// a paragraph pushed into the file list.
struct Notice: Identifiable {
    let id = UUID()
    let title: String
    let text: String
}

// What Finder is copying to the drive, as the drive reports it.
struct DriveUpload: Decodable, Equatable {
    let files: [String]
    let done: Int64
    let total: Int64

    var fraction: Double? { total > 0 ? min(Double(done) / Double(total), 1) : nil }

    var title: String { files.count == 1 ? files[0] : "\(files.count) files" }
}

func sizes(_ done: Int64, _ total: Int64) -> String {
    let format = { (n: Int64) in ByteCountFormatter.string(fromByteCount: n, countStyle: .file) }
    return total > 0 ? "\(format(done)) of \(format(total))" : format(done)
}

struct Job: Identifiable {
    enum Kind: String {
        case upload, download, delete

        var symbol: String {
            switch self {
            case .upload: return "arrow.up"
            case .download: return "arrow.down"
            case .delete: return "trash"
            }
        }

        var verb: String {
            switch self {
            case .upload: return "Uploading"
            case .download: return "Downloading"
            case .delete: return "Deleting"
            }
        }

        var failed: String {
            switch self {
            case .upload: return "Upload failed"
            case .download: return "Download failed"
            case .delete: return "Delete failed"
            }
        }
    }

    let id = UUID()
    let kind: Kind
    let title: String
    let arguments: [String]
    var fraction: Double?
    var done: Int64 = 0
    var total: Int64 = 0
    var error: String?
    var note: String?
    var finished = false
    var cancelled = false
    var reveal: URL?
}

// What is moving now, worded for the strip under the menu-bar header.
struct Activity {
    let title: String
    let symbol: String
    let detail: String
    let fraction: Double?
}

private let driveFolder = "/Volumes/taasant"
private let agentName = "com.taasant.drive"
private let home = FileManager.default.homeDirectoryForCurrentUser

@MainActor
final class Model: ObservableObject {
    static let shared = Model()

    @Published var files: [StoredFile] = []
    @Published var jobs: [Job] = []
    @Published var mounted = false
    @Published var autostart = false
    @Published var configured = true
    @Published var places: [Place] = []
    @Published var standard = ""
    @Published var autostartBusy = false
    // Why the list of files could not be read, when it could not.
    @Published var listError: String?
    @Published var notice: Notice?
    @Published var driveUpload: DriveUpload?
    // A temporary view setting: hiding files here changes nothing on disk or
    // in Telegram, and the window keeps showing them.
    @Published var hiddenInMenuBar = false
    // The words for where new files go, in the places that talk about it.
    var standardLabel: String {
        standard.isEmpty || places.count <= 1 ? (places.first?.label ?? "your place") : standard
    }
    var windowOpen = false
    var popoverOpen = false

    private var working = false
    private var running: [UUID: CLI.Handle] = [:]

    // Where taasant keeps things. TAAS_CONFIG and TAAS_INDEX move them, for
    // the program and so for the app, which has to look in the same places.
    private static func path(_ variable: String, _ name: String) -> URL {
        if let custom = ProcessInfo.processInfo.environment[variable], !custom.isEmpty {
            return URL(fileURLWithPath: custom)
        }
        return home.appendingPathComponent("Library/Application Support/taas/" + name)
    }
    private let config = Model.path("TAAS_CONFIG", "config.json")
    private let index = Model.path("TAAS_INDEX", "index.json")
    private var progress: URL { URL(fileURLWithPath: index.path + ".progress") }

    var settingsPath: String { config.path }
    var indexFile: String { index.path }
    var version: String { Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev" }

    // The one thing worth showing above the files: the job that is running,
    // or a copy into the drive when Finder is the one moving it.
    var activity: Activity? {
        if let job = jobs.first(where: { !$0.finished && $0.error == nil }) {
            return Activity(
                title: "\(job.kind.verb) \(job.title)",
                symbol: job.kind.symbol,
                detail: job.fraction.map { "\(Int($0 * 100))%" }
                    ?? (job.total > 0 ? sizes(job.done, job.total) : "starting…"),
                fraction: job.fraction
            )
        }
        if let upload = driveUpload {
            return Activity(
                title: "Copying \(upload.title) to the drive",
                symbol: "arrow.up.to.line",
                detail: upload.fraction.map { "\(Int($0 * 100))%" } ?? "",
                fraction: upload.fraction
            )
        }
        return nil
    }

    init() {
        configured = FileManager.default.fileExists(atPath: config.path)
        (places, standard) = Place.load(from: config)
        // Once a second, so a copy in Finder is followed closely.
        Task {
            while true {
                readProgress()
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        }
        Task {
            await tick()
            if configured && !mounted { connect() }
            repointAutostart()
            while true {
                try? await Task.sleep(nanoseconds: 3_000_000_000)
                await tick()
            }
        }
    }

    private var plist: URL { home.appendingPathComponent("Library/LaunchAgents/\(agentName).plist") }

    // tick looks at everything the screen shows. The list of files is only
    // read while the window or the popover is open.
    func tick() async {
        // A @Published value tells every screen to draw again when it is set,
        // even to what it already was, so only a change is set.
        let exists = { FileManager.default.fileExists(atPath: $0) }
        if configured != exists(config.path) { configured.toggle() }
        let loaded = Place.load(from: config)
        if places != loaded.places { places = loaded.places }
        if standard != loaded.standard { standard = loaded.standard }
        if mounted != exists(driveFolder) { mounted.toggle() }
        if autostart != exists(plist.path) { autostart.toggle() }
        if windowOpen || popoverOpen { await reload() }
    }

    // The drive writes this file while Finder is copying to it. One that has
    // not changed for a while was left behind by a drive that stopped.
    private func readProgress() {
        let modified = (try? FileManager.default.attributesOfItem(atPath: progress.path))?[.modificationDate] as? Date
        guard let modified, Date().timeIntervalSince(modified) < 30,
              let data = try? Data(contentsOf: progress),
              let found = try? JSONDecoder().decode(DriveUpload.self, from: data)
        else {
            if driveUpload != nil { driveUpload = nil }
            return
        }
        if driveUpload != found { driveUpload = found }
    }

    // Go writes the time with fractions of a second, which the plain
    // formatter does not read.
    nonisolated private static func date(_ text: String) -> Date {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: text) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: text) ?? .distantPast
    }

    func reload() async {
        let text: String
        do { text = try await CLI.run(["list", "--json"]) } catch {
            listError = error.localizedDescription
            return
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            return Self.date(text)
        }
        do {
            let found = try decoder.decode([StoredFile].self, from: Data(text.utf8)).sorted { $0.uploaded > $1.uploaded }
            if found != files { files = found }
            if listError != nil { listError = nil }
        } catch {
            listError = "The list of files could not be read."
        }
    }

    // MARK: the drive

    func connect() {
        notice = nil
        if autostart {
            // The login job owns the drive, so it is the one asked to start.
            Process.launchedProcess(launchPath: "/bin/launchctl", arguments: ["kickstart", "gui/\(getuid())/\(agentName)"])
        } else {
            do { try CLI.start(["mount", "background"]) } catch {
                notice = Notice(title: "Could not start the drive", text: error.localizedDescription)
                return
            }
        }
        Task {
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            await tick()
            if !mounted && notice == nil {
                notice = Notice(title: "The drive did not start",
                                text: "Finder did not get the drive. The log in Settings may say why, for instance that another program has port 8738.")
            }
        }
    }

    func eject() {
        Task {
            do {
                _ = try await run("/sbin/umount", [driveFolder])
                mounted = false
            } catch {
                notice = Notice(title: "The drive is busy", text: "A file is open or still being saved.")
            }
        }
    }

    func openDrive() { NSWorkspace.shared.open(URL(fileURLWithPath: driveFolder)) }

    func setAutostart(_ on: Bool) {
        guard !autostartBusy else { return }
        autostartBusy = true
        Task {
            defer { autostartBusy = false }
            do {
                _ = try await CLI.run(on ? ["autostart"] : ["autostart", "off"])
                if on { try? SMAppService.mainApp.register() } else { try? await SMAppService.mainApp.unregister() }
            } catch {
                notice = Notice(title: "Start at login", text: error.localizedDescription)
            }
            await tick()
        }
    }

    // If the app was moved, the login job still points at the old copy of
    // taasant. Starting it again points it at this one.
    private func repointAutostart() {
        guard autostart, configured, let text = try? String(contentsOf: plist, encoding: .utf8) else { return }
        // The plist is XML, so an apostrophe or an ampersand in the path is
        // written as an entity; without this a path such as Bob's Apps never
        // matched, and the drive was restarted at every launch.
        let escaped = [("&", "&amp;"), ("<", "&lt;"), (">", "&gt;"), ("\"", "&#34;"), ("'", "&#39;")]
            .reduce(CLI.binary.path) { path, pair in path.replacingOccurrences(of: pair.0, with: pair.1) }
        guard !text.contains(escaped) else { return }
        setAutostart(true)
    }

    // Setting up asks questions, so it needs a terminal. With a place already,
    // it adds another one.
    func setUp() {
        let script = FileManager.default.temporaryDirectory.appendingPathComponent("taasant-setup.command")
        // A path can hold an apostrophe, as in /Applications/Bob's Apps/.
        let quoted = "'" + CLI.binary.path.replacingOccurrences(of: "'", with: "'\\''") + "'"
        let command = configured ? "\(quoted) setup" : quoted
        // Terminal does not inherit this app's environment, and setup must write where the app reads.
        let env = ["TAAS_CONFIG", "TAAS_INDEX"].compactMap { name in
            ProcessInfo.processInfo.environment[name].map { "export \(name)='" + $0.replacingOccurrences(of: "'", with: "'\\''") + "'\n" }
        }.joined()
        try? "#!/bin/sh\n\(env)exec \(command)\n".write(to: script, atomically: true, encoding: .utf8)
        try? FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: script.path)
        NSWorkspace.shared.open(script)
    }

    private func run(_ path: String, _ arguments: [String]) async throws -> Void {
        try await withCheckedThrowingContinuation { continuation in
            let process = Process()
            process.executableURL = URL(fileURLWithPath: path)
            process.arguments = arguments
            process.terminationHandler = { finished in
                if finished.terminationStatus == 0 {
                    continuation.resume()
                } else {
                    continuation.resume(throwing: CLIError(text: "failed"))
                }
            }
            do { try process.run() } catch { continuation.resume(throwing: error) }
        }
    }

    // MARK: files

    // place is where the files go, or the default one when it is nil.
    func upload(_ urls: [URL], to place: String? = nil) {
        for url in urls {
            let target = place.map { ["--to", $0] } ?? []
            enqueue(Job(kind: .upload, title: url.lastPathComponent, arguments: ["upload"] + target + [url.path, "--json"]))
        }
    }

    func drop(_ providers: [NSItemProvider]) -> Bool {
        for provider in providers {
            _ = provider.loadObject(ofClass: URL.self) { url, _ in
                if let url { Task { @MainActor in self.upload([url]) } }
            }
        }
        return true
    }

    // The popover has no room for a folder panel: the file goes to Downloads
    // and is shown in Finder when it is there.
    func quickDownload(_ file: StoredFile) {
        let downloads = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask)[0]
        let name = (file.name as NSString).lastPathComponent
        var output = downloads.appendingPathComponent(name)
        var n = 2
        while FileManager.default.fileExists(atPath: output.path) {
            let base = (name as NSString).deletingPathExtension, ext = (name as NSString).pathExtension
            output = downloads.appendingPathComponent(ext.isEmpty ? "\(base) \(n)" : "\(base) \(n).\(ext)")
            n += 1
        }
        enqueue(Job(kind: .download, title: file.name, arguments: ["download", file.id, output.path, "--json"], reveal: output))
    }

    func download(_ file: StoredFile) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.canCreateDirectories = true
        panel.prompt = "Save here"
        guard panel.runModal() == .OK, let folder = panel.url else { return }
        let output = folder.appendingPathComponent((file.name as NSString).lastPathComponent)
        enqueue(Job(kind: .download, title: file.name, arguments: ["download", file.id, output.path, "--json"]))
    }

    // A delete is a job like any other, so it gets a bar and a place in the
    // menu-bar progress while Telegram removes every message.
    func delete(_ file: StoredFile) {
        enqueue(Job(kind: .delete, title: file.name, arguments: ["delete", file.id, "--json"]))
    }

    func clearFinished() { jobs.removeAll { $0.finished || $0.error != nil } }

    // A job that has not started is simply dropped. One that is running is
    // sent a Ctrl-C, and the row says it was cancelled once it has stopped.
    func cancel(_ job: Job) {
        guard let i = jobs.firstIndex(where: { $0.id == job.id }) else { return }
        if let handle = running[job.id] {
            jobs[i].cancelled = true
            handle.cancel()
        } else {
            jobs.remove(at: i)
        }
    }

    var isBusy: Bool { jobs.contains { !$0.finished && $0.error == nil } || driveUpload != nil }

    // Quitting stops what is moving, so a person who is in the middle of it is asked.
    func quit() {
        guard isBusy else { NSApp.terminate(nil); return }
        let alert = NSAlert()
        alert.messageText = "Quit taasant?"
        alert.informativeText = "Files that are being stored or saved right now will stop, and the ones waiting will not be sent."
        alert.addButton(withTitle: "Keep going")
        alert.addButton(withTitle: "Quit")
        NSApp.activate(ignoringOtherApps: true)
        guard alert.runModal() == .alertSecondButtonReturn else { return }
        running.values.forEach { $0.cancel() }
        NSApp.terminate(nil)
    }

    // One file at a time: two uploads would only share the same connection.
    private func enqueue(_ job: Job) {
        jobs.append(job)
        guard !working else { return }
        working = true
        Task {
            while let next = jobs.first(where: { !$0.finished && $0.error == nil }) {
                await work(next)
            }
            working = false
        }
    }

    private func work(_ job: Job) async {
        func change(_ edit: (inout Job) -> Void) {
            if let i = jobs.firstIndex(where: { $0.id == job.id }) { edit(&jobs[i]) }
        }
        let handle = CLI.Handle()
        running[job.id] = handle
        defer { running[job.id] = nil }
        do {
            let output = try await CLI.run(job.arguments, handle: handle) { line in
                guard let data = line.data(using: .utf8),
                      let numbers = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let done = numbers["done"] as? Double, let total = numbers["total"] as? Double, total > 0
                else { return }
                Task { @MainActor in change { $0.fraction = done / total; $0.done = Int64(done); $0.total = Int64(total) } }
            }
            change { $0.finished = true; $0.fraction = 1 }
            if let reveal = job.reveal { NSWorkspace.shared.activateFileViewerSelecting([reveal]) }
            await reload()
            // Telegram keeps messages older than 48 hours. The file has gone
            // from the drive, but the note is shown in an alert and kept on
            // the row in Transfers.
            if job.kind == .delete, let note = Self.note(in: output) {
                change { $0.note = note }
                notice = Notice(title: "Deleted \(job.title)", text: note)
            }
        } catch {
            if jobs.first(where: { $0.id == job.id })?.cancelled == true {
                change { $0.error = "Cancelled" }
                return
            }
            change { $0.error = error.localizedDescription }
            notice = Notice(title: job.kind.failed, text: error.localizedDescription)
        }
    }

    // A delete prints progress lines and then one last object with the result.
    private static func note(in output: String) -> String? {
        for line in output.split(separator: "\n").reversed() {
            guard let data = String(line).data(using: .utf8),
                  let result = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let note = result["note"] as? String else { continue }
            return note
        }
        return nil
    }
}
