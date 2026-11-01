import Foundation

enum FBError: LocalizedError {
    case message(String)
    var errorDescription: String? {
        switch self { case .message(let message): message }
    }
}

struct FBResponseHead: Decodable, Sendable {
    let status: Int?
    let headers: [String: String]?
    let error: String?
}

struct FBRequestDescription: Encodable, Sendable {
    let url: String
    var method = "GET"
    var headers: [String: String] = [:]
    var document = false
    var bodyBase64: String?
}

/// The Go runtime and renderer live in the application process. No listener or helper server exists.
final class FBEngine: @unchecked Sendable {
    let handle: UInt64

    init(root: String = "/", pandoc: String) throws {
        struct Result: Decodable { let engine: UInt64?; let error: String? }
        guard let pointer = root.withCString({ root in pandoc.withCString { FBCreate(root, $0) } }) else {
            throw FBError.message("The document engine could not start.")
        }
        defer { FBFree(pointer) }
        let result = try JSONDecoder().decode(Result.self, from: Data(String(cString: pointer).utf8))
        guard let handle = result.engine, handle != 0 else {
            throw FBError.message(result.error ?? "The document engine could not start.")
        }
        self.handle = handle
    }

    deinit { FBDestroy(handle) }
}

/// Cancellation may arrive on the UI thread while Go is producing a response on a worker thread.
/// Only the worker calls finish; it retains the engine until all C calls have completed.
final class FBEngineRequest: @unchecked Sendable {
    private let engine: FBEngine
    private let description: FBRequestDescription
    private let lock = NSLock()
    private var handle: UInt64 = 0
    private var cancelled = false

    init(engine: FBEngine, description: FBRequestDescription) {
        self.engine = engine
        self.description = description
    }

    func begin() throws -> FBResponseHead {
        let encoded = try JSONEncoder().encode(description)
        let json = String(decoding: encoded, as: UTF8.self)
        let id: UInt64 = try lock.withLock {
            guard !cancelled else { throw CancellationError() }
            handle = json.withCString { FBStart(engine.handle, $0) }
            guard handle != 0 else { throw FBError.message("The document request could not start.") }
            return handle
        }
        guard let pointer = FBResponse(id) else { throw FBError.message("The document engine returned no response.") }
        defer { FBFree(pointer) }
        let head = try JSONDecoder().decode(FBResponseHead.self, from: Data(String(cString: pointer).utf8))
        if let error = head.error { throw FBError.message(error) }
        return head
    }

    func read() throws -> Data? {
        let id = lock.withLock { handle }
        guard id != 0 else { throw CancellationError() }
        var bytes = [UInt8](repeating: 0, count: 64 * 1024)
        let count = bytes.withUnsafeMutableBytes { FBRead(id, $0.baseAddress, Int64($0.count)) }
        guard count >= 0 else { throw FBError.message("Reading the document was interrupted.") }
        return count == 0 ? nil : Data(bytes.prefix(Int(count)))
    }

    func cancel() {
        lock.withLock {
            cancelled = true
            if handle != 0 { FBCancel(handle) }
        }
    }

    func finish() {
        lock.withLock {
            if handle != 0 { FBFinish(handle); handle = 0 }
        }
    }

    deinit { finish() }
}

enum FBLocation {
    static func webURL(fileURL: URL) -> URL {
        var components = URLComponents()
        components.scheme = "fb"
        components.host = "local"
        var path = fileURL.standardizedFileURL.path
        var isDirectory: ObjCBool = false
        if FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory), isDirectory.boolValue, !path.hasSuffix("/") {
            path += "/"
        }
        components.path = path
        components.percentEncodedFragment = fileURL.fragment(percentEncoded: true)
        return components.url!
    }

    static func fileURL(webURL: URL) -> URL? {
        guard webURL.scheme == "fb", webURL.host == "local" else { return nil }
        return URL(fileURLWithPath: webURL.path)
    }

    /// A file URL that also names an #anchor, which `webURL(fileURL:)` carries over.
    static func fileURL(path: String, fragment: String?) -> URL {
        let url = URL(fileURLWithPath: path)
        guard let fragment, var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return url }
        components.percentEncodedFragment = fragment
        return components.url ?? url
    }
}
