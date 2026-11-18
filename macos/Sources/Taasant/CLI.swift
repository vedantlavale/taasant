import Foundation

struct CLIError: LocalizedError {
    let text: String
    var errorDescription: String? { text }
}

// Capture collects what the program prints from the threads that read its pipes.
final class Capture: @unchecked Sendable {
    private let lock = NSLock()
    private var printed = Data(), complained = Data(), pending = Data()

    // heard keeps the output and returns the lines that are complete.
    func heard(_ chunk: Data) -> [String] {
        lock.lock()
        defer { lock.unlock() }
        printed.append(chunk)
        pending.append(chunk)
        var lines: [String] = []
        while let newline = pending.firstIndex(of: 10) {
            lines.append(String(decoding: pending[..<newline], as: UTF8.self))
            pending.removeSubrange(...newline)
        }
        return lines
    }

    func complain(_ chunk: Data) {
        lock.lock()
        complained.append(chunk)
        lock.unlock()
    }

    func text() -> (output: String, problem: String) {
        lock.lock()
        defer { lock.unlock() }
        return (String(decoding: printed, as: UTF8.self),
                String(decoding: complained, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines))
    }
}

// The app does no work of its own: everything goes through the taasant
// program that sits inside the app, so the app and the terminal always agree.
enum CLI {
    static var binary: URL {
        if let bundled = Bundle.main.url(forResource: "taasant", withExtension: nil) {
            return bundled
        }
        // Run from "swift run" while developing: use the one on this computer.
        for path in ["/opt/homebrew/bin/taasant", "/usr/local/bin/taasant"] {
            if FileManager.default.isExecutableFile(atPath: path) { return URL(fileURLWithPath: path) }
        }
        return URL(fileURLWithPath: "/usr/local/bin/taasant")
    }

    // Handle lets a caller stop a run that is going on. It sends the same
    // Ctrl-C a terminal would, so taasant cleans up: an upload that is
    // cancelled gives its parts back instead of leaving them in the bucket.
    final class Handle: @unchecked Sendable {
        private let lock = NSLock()
        private var process: Process?
        private var cancelled = false

        fileprivate func attach(_ started: Process) {
            lock.lock()
            process = started
            let stop = cancelled
            lock.unlock()
            if stop { started.interrupt() }
        }

        func cancel() {
            lock.lock()
            cancelled = true
            let running = process
            lock.unlock()
            running?.interrupt()
        }
    }

    // run starts taasant and returns what it printed. onLine hears each line
    // as it comes, which is how progress arrives.
    static func run(_ arguments: [String], handle: Handle? = nil, onLine: @escaping (String) -> Void = { _ in }) async throws -> String {
        try await withCheckedThrowingContinuation { continuation in
            let process = Process()
            process.executableURL = binary
            process.arguments = arguments
            let out = Pipe(), err = Pipe()
            process.standardOutput = out
            process.standardError = err

            let capture = Capture()
            out.fileHandleForReading.readabilityHandler = { handle in
                for line in capture.heard(handle.availableData) { onLine(line) }
            }
            err.fileHandleForReading.readabilityHandler = { handle in
                capture.complain(handle.availableData)
            }
            process.terminationHandler = { finished in
                out.fileHandleForReading.readabilityHandler = nil
                err.fileHandleForReading.readabilityHandler = nil
                _ = capture.heard(out.fileHandleForReading.readDataToEndOfFile())
                capture.complain(err.fileHandleForReading.readDataToEndOfFile())
                let (result, problem) = capture.text()
                if finished.terminationStatus == 0 {
                    continuation.resume(returning: result)
                } else {
                    let text = problem.isEmpty ? "taasant stopped with code \(finished.terminationStatus)" : problem
                    continuation.resume(throwing: CLIError(text: text.replacingOccurrences(of: "error: ", with: "")))
                }
            }
            do {
                try process.run()
                handle?.attach(process)
            } catch {
                continuation.resume(throwing: CLIError(text: "taasant could not be started: \(error.localizedDescription)"))
            }
        }
    }

    // start launches taasant and leaves it running, as the drive needs.
    static func start(_ arguments: [String]) throws {
        let process = Process()
        process.executableURL = binary
        process.arguments = arguments
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        try process.run()
    }
}
