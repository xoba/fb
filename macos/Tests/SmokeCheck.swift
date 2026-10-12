import AppKit
import Foundation
import WebKit

@MainActor
final class SmokeCheck: NSObject, WKNavigationDelegate {
    let webView: WKWebView
    let handler: FBSchemeHandler
    let fixtures: URL
    private let pandoc: String
    private let window: NSWindow
    private var navigation: CheckedContinuation<Void, any Error>?
    private var timeout: Task<Void, Never>?
    private var passed: [String] = []

    init(fixtures: URL, pandoc: String) throws {
        self.fixtures = fixtures
        self.pandoc = pandoc
        handler = FBSchemeHandler(engine: try FBEngine(pandoc: pandoc))
        let config = WKWebViewConfiguration()
        handler.configure(config)
        webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 1000, height: 800), configuration: config)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1000, height: 800),
                          styleMask: [.titled], backing: .buffered, defer: false)
        super.init()
        webView.navigationDelegate = self
        window.contentView = webView
        window.orderBack(nil)
    }

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping @MainActor (WKNavigationActionPolicy) -> Void) {
        if action.targetFrame?.isMainFrame == true, let url = action.request.url {
            handler.prepareNavigation(to: url)
        }
        decisionHandler(.allow)
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) { complete(nil) }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: any Error) { complete(error) }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: any Error) {
        if (error as NSError).code != NSURLErrorCancelled { complete(error) }
    }

    private func complete(_ error: (any Error)?) {
        timeout?.cancel()
        timeout = nil
        let pending = navigation
        navigation = nil
        if let error { pending?.resume(throwing: error) } else { pending?.resume() }
    }

    func load(_ name: String, query: String? = nil, raw: Bool = false,
              queryItems: [URLQueryItem] = []) async throws {
        var url = FBLocation.webURL(fileURL: fixtures.appendingPathComponent(name))
        if query != nil || raw || !queryItems.isEmpty {
            var components = URLComponents(url: url, resolvingAgainstBaseURL: false)!
            components.queryItems = queryItems
            if let query { components.queryItems?.append(URLQueryItem(name: "q", value: query)) }
            if raw { components.queryItems?.append(URLQueryItem(name: "raw", value: "1")) }
            url = components.url!
        }
        try await navigate(name) {
            handler.prepareNavigation(to: url)
            webView.load(URLRequest(url: url))
        }
    }

    private func navigate(_ label: String, action: () -> Void) async throws {
        try await withCheckedThrowingContinuation { continuation in
            navigation = continuation
            timeout = Task { [weak self] in
                try? await Task.sleep(for: .seconds(25))
                guard !Task.isCancelled else { return }
                self?.complete(FBError.message("Navigation timed out: \(label)"))
            }
            action()
        }
    }

    private func followEmailPart(_ id: String) async throws {
        let literal = String(decoding: try JSONEncoder().encode(id), as: UTF8.self)
        let selector = "[...document.querySelectorAll('.eml-tree a')].find(a => new URL(a.href).searchParams.get('eml-view') === \(literal))"
        guard try await webView.evaluateJavaScript("\(selector) !== undefined") as? Bool == true else {
            throw FBError.message("No MIME tree link for part \(id)")
        }
        try await navigate("MIME part \(id)") {
            Task {
                do { _ = try await webView.evaluateJavaScript("\(selector).click(); void 0") }
                catch { complete(error) }
            }
        }
    }

    func submitQuery(_ query: String) async throws {
        try await withCheckedThrowingContinuation { continuation in
            navigation = continuation
            timeout = Task { [weak self] in
                try? await Task.sleep(for: .seconds(25))
                guard !Task.isCancelled else { return }
                self?.complete(FBError.message("SQL form navigation timed out"))
            }
            let literal = String(decoding: try! JSONEncoder().encode(query), as: UTF8.self)
            Task {
                do {
                    _ = try await webView.evaluateJavaScript("document.querySelector('input[name=q]').value = \(literal); document.querySelector('form').requestSubmit(); void 0")
                } catch { complete(error) }
            }
        }
    }

    func check(_ label: String, _ script: String) async throws {
        let result = try await webView.evaluateJavaScript(script)
        guard result as? Bool == true else {
            let body = try? await webView.evaluateJavaScript("document.body.innerText.slice(0, 1200)")
            throw FBError.message("\(label) failed: \(String(describing: result)); body: \(String(describing: body))")
        }
        passed.append(label)
        print("PASS \(label)")
    }

    func waitFor(_ label: String, _ predicate: String) async throws {
        for _ in 0..<100 {
            if (try? await webView.evaluateJavaScript(predicate)) as? Bool == true {
                passed.append(label)
                print("PASS \(label)")
                return
            }
            try await Task.sleep(for: .milliseconds(100))
        }
        let details = try? await webView.evaluateJavaScript("JSON.stringify({error:window.fbSmokeError,audio:(()=>{let a=document.querySelector('audio');return a?{src:a.currentSrc,state:a.readyState,network:a.networkState,error:a.error?.message}:null})()})")
        throw FBError.message("Timed out waiting for \(label): \(String(describing: details))")
    }

    func run() async throws {
        let pasteboard = NSPasteboard(name: .init("fb-smoke-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        let originalURLs = [fixtures.appendingPathComponent("sample.md"), fixtures.appendingPathComponent("nested", isDirectory: true)]
        guard pasteboard.writeObjects(originalURLs.map { $0 as NSURL }),
              FileDropWebView.fileURLs(from: pasteboard).map(\.path) == originalURLs.map(\.path) else {
            throw FBError.message("Native mixed file/directory drop lost original URLs")
        }
        passed.append("native mixed file/folder drop URLs")
        print("PASS native mixed file/folder drop URLs")
        try await load("")
        try await check("directory listing", "document.body.innerText.includes('sample.md') && document.body.innerText.includes('nested')")
        try await load("sample.md")
        try await check("bundled Pandoc markdown", "document.querySelector('h1')?.textContent === 'Embedded engine'")
        try await waitFor("relative image", "[...document.images].some(i => i.naturalWidth === 1)")
        try await waitFor("bundled MathJax", "document.querySelector('mjx-container') !== null")
        let relative = try await webView.evaluateJavaScript("document.querySelector('a[href*=\"nested/next\"]').href") as! String
        guard URL(string: relative)?.scheme == "fb" else { throw FBError.message("Relative link escaped custom scheme") }
        try await load("nested/next.md")
        try await check("relative document", "document.querySelector('h1')?.textContent === 'Relative navigation works'")
        try await load("spaced & café #1.md")
        try await check("Unicode and reserved filename characters", "document.querySelector('h1')?.textContent === 'Encoded filename works'")
        try await load("100%.md")
        try await check("literal percent filename", "document.querySelector('h1')?.textContent === 'Percent filename works'")
        try await load("example.go")
        try await check("source highlighting", "document.body.innerText.includes('func main()') && document.querySelector('pre') !== null")
        try await load("sample.csv")
        try await submitQuery("select sum(amount) as total from t")
        try await check("CSV SQL form submission", "document.body.innerText.includes('total') && document.body.innerText.includes('30')")
        try await load("sample.db")
        try await check("SQLite table listing", "document.body.innerText.includes('records')")
        _ = try await webView.evaluateJavaScript("fetch(location.pathname + '?stat=records').then(r => r.json()).then(v => window.fbSmokeStats = v).catch(e => window.fbSmokeError = String(e)); void 0")
        try await waitFor("SQLite statistics fetch", "window.fbSmokeStats?.rows === 2")
        try await load("sample.db", query: "select sum(amount) as total from records")
        try await check("SQLite SQL query", "document.body.innerText.includes('total') && document.body.innerText.includes('30')")
        try await load("sample.zip/inside.md")
        try await check("archive member rendering", "document.querySelector('h1')?.textContent === 'Archive rendering works'")
        try await load("message.eml")
        try await check("email body and decoded headers", "document.querySelector('h1')?.textContent === 'Café MIME browser' && document.body.innerText.includes('sender@example.com') && document.querySelector('#eml-body')?.textContent.includes('MIME email body')")
        try await check("hierarchical MIME tree preserves alternatives and attachments", "['1.1', '1.1.1', '1.1.1.1', '1.1.1.2', '1.1.2', '1.2'].every(id => [...document.querySelectorAll('.eml-tree a')].some(a => new URL(a.href).searchParams.get('eml-view') === id)) && document.body.innerText.includes('café.bin')")
        try await check("email HTML blocks active content and remote images", "window.fbUnsafeEmailExecuted === undefined && document.querySelector('#eml-body script, #eml-body style, #eml-body [onerror]') === null && [...document.querySelectorAll('#eml-body img')].every(i => i.src.startsWith('fb://local/') && new URL(i.src).searchParams.has('eml-part')) && document.querySelector('#eml-body a[href^=\"file:\"]') === null")
        try await waitFor("email embedded CID image", "[...document.querySelectorAll('#eml-body img')].some(i => i.naturalWidth === 1)")
        _ = try await webView.evaluateJavaScript("fetch(location.pathname + '?eml-part=1.2').then(r => r.arrayBuffer()).then(v => window.fbSmokeAttachment = [...new Uint8Array(v)].join(',')).catch(e => window.fbSmokeError = String(e)); void 0")
        try await waitFor("email attachment decoded bytes through native bridge", "window.fbSmokeAttachment === '0,1,2,127,128,255'")
        try await followEmailPart("1.1.1.1")
        try await check("email plain-text MIME part navigation", "new URL(location.href).searchParams.get('eml-view') === '1.1.1.1' && document.querySelector('#eml-body')?.textContent.includes('Plain MIME alternative: café.')")
        guard webView.canGoBack else { throw FBError.message("MIME part navigation did not enter browser history") }
        try await navigate("back to email") { webView.goBack() }
        try await check("email MIME navigation browser history", "!new URL(location.href).searchParams.has('eml-view') && document.querySelector('#eml-body')?.textContent.includes('MIME email body')")
        try await load("sample.zip/message.eml")
        try await check("email inside archive", "document.querySelector('h1')?.textContent === 'Café MIME browser' && document.querySelector('.eml-tree') !== null")
        try await waitFor("email CID image inside archive", "[...document.querySelectorAll('#eml-body img')].some(i => i.naturalWidth === 1)")
        try await load("", queryItems: [URLQueryItem(name: "view", value: "inbox")])
        try await check("directory email inbox", "document.body.innerText.includes('Café MIME browser') && document.body.innerText.includes('sender@example.com') && [...document.querySelectorAll('a')].some(a => a.href.includes('message.eml'))")
        try await load("raw.html")
        do {
            _ = try await webView.evaluateJavaScript("window.fbUnsafeScriptExecuted")
            throw FBError.message("Raw HTML unexpectedly allowed JavaScript execution")
        } catch {
            guard (error as NSError).userInfo["WKJavaScriptExceptionMessage"] as? String ==
                    "Cannot execute JavaScript in this document" else { throw error }
            passed.append("raw HTML blocks JavaScript")
            print("PASS raw HTML blocks JavaScript")
        }
        try await load("sample.wav")
        try await waitFor("audio metadata", "Number.isFinite(document.querySelector('audio')?.duration)")
        try await load("sample.pdf")
        passed.append("PDF navigation")
        print("PASS PDF navigation")
        try await load("payload.txt~", raw: true)
        try await check("raw text backup stays inert", "document.body.innerText.includes('<script>') && window.fbUnsafeScriptExecuted === undefined")
        let navigationCheck = try BrowserNavigationCheck(fixtures: fixtures, pandoc: pandoc)
        passed.append(contentsOf: try await navigationCheck.run())
        let output: [String: Any] = ["passed": passed, "count": passed.count]
        let json = try JSONSerialization.data(withJSONObject: output, options: [.prettyPrinted, .sortedKeys])
        print(String(decoding: json, as: UTF8.self))
    }
}

/// Exercise BrowserModel's state with actual WebKit navigation objects and callbacks.
/// Cancelling a response is how native download handling leaves the current document visible.
@MainActor
private final class BrowserNavigationCheck: NSObject, WKNavigationDelegate {
    private let model: BrowserModel
    private let engine: FBEngine
    private let webView: WKWebView
    private let window: NSWindow
    private let fixtures: URL
    private let preferencesName: String
    private let preferences: UserDefaults
    private var cancelNextResponse = false
    private var finished: [WKNavigation] = []
    private var failures: [NSError] = []

    init(fixtures: URL, pandoc: String) throws {
        self.fixtures = fixtures
        preferencesName = "fb-smoke-zoom-\(UUID().uuidString)"
        guard let preferences = UserDefaults(suiteName: preferencesName) else {
            throw FBError.message("Could not create isolated zoom preferences")
        }
        self.preferences = preferences
        engine = try FBEngine(pandoc: pandoc)
        model = BrowserModel(fileURL: fixtures.appendingPathComponent("sample.md"), engine: engine,
                             preferences: preferences)
        guard let webView = model.webView else { throw FBError.message(model.errorMessage ?? "No browser view") }
        self.webView = webView
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1000, height: 800),
                          styleMask: [.titled], backing: .buffered, defer: false)
        super.init()
        webView.navigationDelegate = self
        window.contentView = webView
        window.orderBack(nil)
    }

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping @MainActor (WKNavigationActionPolicy) -> Void) {
        model.webView(webView, decidePolicyFor: action, decisionHandler: decisionHandler)
    }

    func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse,
                 decisionHandler: @escaping @MainActor (WKNavigationResponsePolicy) -> Void) {
        if cancelNextResponse {
            cancelNextResponse = false
            decisionHandler(.cancel)
        } else { model.webView(webView, decidePolicyFor: response, decisionHandler: decisionHandler) }
    }

    func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
        model.webView(webView, didStartProvisionalNavigation: navigation)
    }
    func webView(_ webView: WKWebView, didCommit navigation: WKNavigation!) {
        model.webView(webView, didCommit: navigation)
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        model.webView(webView, didFinish: navigation)
        if let navigation { finished.append(navigation) }
    }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        failures.append(error as NSError)
        model.webView(webView, didFailProvisionalNavigation: navigation, withError: error)
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        failures.append(error as NSError)
        model.webView(webView, didFail: navigation, withError: error)
    }

    private func waitUntil(_ label: String, _ predicate: () -> Bool) async throws {
        for _ in 0..<150 {
            if predicate() { return }
            try await Task.sleep(for: .milliseconds(100))
        }
        throw FBError.message("Timed out waiting for \(label); banner: \(model.errorMessage ?? "none")")
    }

    func run() async throws -> [String] {
        defer {
            webView.stopLoading()
            (webView.configuration.urlSchemeHandler(forURLScheme: "fb") as? FBSchemeHandler)?.cancelAll()
            window.orderOut(nil)
            preferences.removePersistentDomain(forName: preferencesName)
        }
        var passed: [String] = []
        guard webView.pageZoom == 1 else { throw FBError.message("Fresh preferences did not default to actual size") }
        model.start()
        try await waitUntil("initial model navigation") { finished.count == 1 && !model.isLoading }
        guard webView.pageZoom == 1 else { throw FBError.message("Initial navigation changed the default zoom") }
        passed.append("fresh zoom defaults to actual size")
        model.zoom(0.2)
        cancelNextResponse = true
        model.open(fixtures.appendingPathComponent("nested/next.md"))
        try await waitUntil("real policy interruption") {
            failures.contains { $0.domain == "WebKitErrorDomain" && $0.code == 102 }
        }
        let heading = try await webView.evaluateJavaScript("document.querySelector('h1')?.textContent") as? String
        guard model.errorMessage == nil, !model.isLoading, heading == "Embedded engine" else {
            throw FBError.message("A policy cancellation raised a false banner over the rendered document: \(model.errorMessage ?? "none")")
        }
        passed.append("policy interruption keeps rendered document without false error")

        model.open(fixtures.appendingPathComponent("nested/next.md"))
        try await waitUntil("replacement navigation") { finished.count == 2 && !model.isLoading }
        let previous = finished.last!
        model.reload()
        // Deliver a late failure for a real previous navigation after a same-URL reload.
        // Comparing URLs alone would incorrectly attach this failure to the new load.
        model.webView(webView, didFailProvisionalNavigation: previous, withError: URLError(.cannotOpenFile))
        guard model.isLoading, model.errorMessage == nil else {
            throw FBError.message("A stale navigation failure changed the replacement load's state")
        }
        try await waitUntil("same-URL reload") { finished.count == 3 && !model.isLoading }
        guard model.errorMessage == nil else { throw FBError.message("A stale navigation left an error banner") }
        passed.append("same-URL reload ignores stale navigation failure")
        guard abs(webView.pageZoom - 1.2) < 0.0001,
              abs(preferences.double(forKey: "pageZoom") - 1.2) < 0.0001 else {
            throw FBError.message("Zoom did not persist through navigation and reload")
        }
        passed.append("zoom survives navigation and reload")

        // A separate model and preferences instance exercise the same restoration
        // path used by a new window or the next app launch, before its first load.
        guard let reopenedPreferences = UserDefaults(suiteName: preferencesName) else {
            throw FBError.message("Could not reopen zoom preferences")
        }
        let reopened = BrowserModel(fileURL: fixtures.appendingPathComponent("sample.md"), engine: engine,
                                    preferences: reopenedPreferences)
        guard let reopenedView = reopened.webView, abs(reopenedView.pageZoom - 1.2) < 0.0001 else {
            throw FBError.message("A new browser did not restore the saved zoom before loading")
        }
        window.contentView = reopenedView
        defer {
            reopenedView.stopLoading()
            (reopenedView.configuration.urlSchemeHandler(forURLScheme: "fb") as? FBSchemeHandler)?.cancelAll()
        }
        reopened.start()
        try await waitUntil("restored zoom navigation") { !reopened.isLoading }
        let reopenedHeading = try await reopenedView.evaluateJavaScript("document.querySelector('h1')?.textContent") as? String
        guard reopened.errorMessage == nil, reopenedHeading == "Embedded engine",
              abs(reopenedView.pageZoom - 1.2) < 0.0001 else {
            throw FBError.message("Restored zoom did not survive the new browser's first navigation")
        }
        passed.append("new browser restores saved zoom before its first document")
        reopened.resetZoom()
        let afterReset = BrowserModel(fileURL: fixtures, engine: engine, preferences: preferences)
        guard reopenedView.pageZoom == 1, preferences.double(forKey: "pageZoom") == 1,
              afterReset.webView?.pageZoom == 1, abs(webView.pageZoom - 1.2) < 0.0001 else {
            throw FBError.message("Actual Size did not persist for new browsers while preserving existing windows")
        }
        passed.append("actual size persists without changing another open window")
        window.contentView = webView

        // Remove this test-only engine to cause a real transport failure, rather than
        // an ordinary HTTP error page. The active navigation must still show an error.
        FBDestroy(engine.handle)
        model.reload()
        try await waitUntil("genuine transport error") { model.errorMessage != nil && !model.isLoading }
        guard model.errorMessage != "Frame load interrupted" else { throw FBError.message("Wrong real failure surfaced") }
        passed.append("genuine current navigation failure remains visible")
        for label in passed { print("PASS \(label)") }
        return passed
    }
}

@main
enum SmokeMain {
    @MainActor static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        guard CommandLine.arguments.count == 3 else {
            fputs("Usage: SmokeCheck FIXTURES PANDOC\n", stderr)
            exit(2)
        }
        Task { @MainActor in
            do {
                let check = try SmokeCheck(fixtures: URL(fileURLWithPath: CommandLine.arguments[1]),
                                           pandoc: CommandLine.arguments[2])
                try await check.run()
                exit(0)
            } catch {
                fputs("FAIL \(error.localizedDescription) \((error as NSError).userInfo)\n", stderr)
                exit(1)
            }
        }
        app.run()
    }
}
