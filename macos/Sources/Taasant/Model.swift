import AppKit
import ServiceManagement
import SwiftUI

struct StoredFile: Identifiable, Hashable, Decodable {
    let name: String
    let id: String
    let size: Int64
    let uploaded: Date
}

// What Finder is copying to the drive, as the drive reports it.
struct DriveUpload: Decodable {
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
    enum Kind { case upload, download }
    let id = UUID()
    let kind: Kind
    let title: String
    let arguments: [String]
    var fraction: Double?
    var done: Int64 = 0
    var total: Int64 = 0
    var error: String?
    var finished = false
    var reveal: URL?
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
    @Published var message: String?
    @Published var driveUpload: DriveUpload?
    var windowOpen = false
    var popoverOpen = false

    private var working = false

    private let progress = home.appendingPathComponent("Library/Application Support/taas/index.json.progress")
    private let config = home.appendingPathComponent("Library/Application Support/taas/config.json")

    init() {
        configured = FileManager.default.fileExists(atPath: config.path)
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
        configured = FileManager.default.fileExists(atPath: config.path)
        mounted = FileManager.default.fileExists(atPath: driveFolder)
        autostart = FileManager.default.fileExists(atPath: plist.path)
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
            driveUpload = nil
            return
        }
        driveUpload = found
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
        guard let text = try? await CLI.run(["list", "--json"]) else { return }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            return Self.date(text)
        }
        if let found = try? decoder.decode([StoredFile].self, from: Data(text.utf8)) {
            files = found.sorted { $0.uploaded > $1.uploaded }
        }
    }

    // MARK: the drive

    func connect() {
        message = nil
        if autostart {
            // The login job owns the drive, so it is the one asked to start.
            Process.launchedProcess(launchPath: "/bin/launchctl", arguments: ["kickstart", "gui/\(getuid())/\(agentName)"])
        } else {
            do { try CLI.start(["mount", "background"]) } catch { message = error.localizedDescription }
        }
    }

    func eject() {
        Task {
            do {
                _ = try await run("/sbin/umount", [driveFolder])
                mounted = false
            } catch {
                message = "The drive is busy: a file is open or still being saved."
            }
        }
    }

    func openDrive() { NSWorkspace.shared.open(URL(fileURLWithPath: driveFolder)) }

    func setAutostart(_ on: Bool) {
        Task {
            do {
                _ = try await CLI.run(on ? ["autostart"] : ["autostart", "off"])
                if on { try? SMAppService.mainApp.register() } else { try? await SMAppService.mainApp.unregister() }
            } catch {
                message = error.localizedDescription
            }
            await tick()
        }
    }

    // If the app was moved, the login job still points at the old copy of
    // taasant. Starting it again points it at this one.
    private func repointAutostart() {
        guard autostart, let text = try? String(contentsOf: plist, encoding: .utf8),
              !text.contains(CLI.binary.path) else { return }
        setAutostart(true)
    }

    // Setting up asks questions, so it needs a terminal.
    func setUp() {
        let script = FileManager.default.temporaryDirectory.appendingPathComponent("taasant-setup.command")
        try? "#!/bin/sh\nexec '\(CLI.binary.path)'\n".write(to: script, atomically: true, encoding: .utf8)
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

    func upload(_ urls: [URL]) {
        for url in urls {
            // The ID is what the program is told, never the name.
            enqueue(Job(kind: .upload, title: url.lastPathComponent, arguments: ["upload", url.path, "--json"]))
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

    func delete(_ file: StoredFile) {
        Task {
            do {
                _ = try await CLI.run(["delete", file.id, "--json"])
                await reload()
            } catch {
                message = error.localizedDescription
            }
        }
    }

    func clearFinished() { jobs.removeAll { $0.finished } }

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
        do {
            _ = try await CLI.run(job.arguments) { line in
                guard let data = line.data(using: .utf8),
                      let numbers = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let done = numbers["done"] as? Double, let total = numbers["total"] as? Double, total > 0
                else { return }
                Task { @MainActor in change { $0.fraction = done / total; $0.done = Int64(done); $0.total = Int64(total) } }
            }
            change { $0.finished = true; $0.fraction = 1 }
            if let reveal = job.reveal { NSWorkspace.shared.activateFileViewerSelecting([reveal]) }
            await reload()
        } catch {
            change { $0.error = error.localizedDescription }
        }
    }
}
