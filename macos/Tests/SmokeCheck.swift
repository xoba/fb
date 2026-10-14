import AppKit
import Foundation
import PDFKit
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
    private var responseMIMEType: String?
    private var responseCanDisplay = false

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

    func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse,
                 decisionHandler: @escaping @MainActor (WKNavigationResponsePolicy) -> Void) {
        if response.isForMainFrame {
            responseMIMEType = response.response.mimeType
            responseCanDisplay = response.canShowMIMEType
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
        responseMIMEType = nil
        responseCanDisplay = false
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
        let selector = "[...document.querySelectorAll('.eml-tree a')].find(a => a.dataset.emlId === \(literal))"
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
        try await load("example.asm")
        try await check("assembly source highlighting", "document.querySelector('#L1') !== null && document.body.innerText.includes('entry:') && [...document.querySelectorAll('pre span')].some(s => s.textContent === '#1.25e-06') && document.querySelector('a[href=\"example.asm?raw=1\"]') !== null")
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
        try await check("hierarchical MIME tree preserves alternatives and attachments", "['1.1', '1.1.1', '1.1.1.1', '1.1.1.2', '1.1.2', '1.2', '1.3', '1.4', '1.5', '1.6'].every(id => [...document.querySelectorAll('.eml-tree a')].some(a => a.dataset.emlId === id)) && document.body.innerText.includes('café.bin')")
        try await check("email HTML blocks active content and remote images", "window.fbUnsafeEmailExecuted === undefined && document.querySelector('#eml-body script, #eml-body style, #eml-body [onerror]') === null && [...document.querySelectorAll('#eml-body img')].every(i => i.src.startsWith('fb://local/') && new URL(i.src).searchParams.has('eml-part')) && document.querySelector('#eml-body a[href^=\"file:\"]') === null")
        try await waitFor("email embedded CID image", "[...document.querySelectorAll('#eml-body img')].some(i => i.naturalWidth === 1)")
        _ = try await webView.evaluateJavaScript("fetch(location.pathname + '?eml-part=1.2').then(r => r.arrayBuffer()).then(v => window.fbSmokeAttachment = [...new Uint8Array(v)].join(',')).catch(e => window.fbSmokeError = String(e)); void 0")
        try await waitFor("email attachment decoded bytes through native bridge", "window.fbSmokeAttachment === '0,1,2,127,128,255'")
        try await followEmailPart("1.1.1.1")
        try await check("email plain-text MIME part navigation", "new URL(location.href).searchParams.get('eml-view') === '1.1.1.1' && document.querySelector('#eml-body')?.textContent.includes('Plain MIME alternative: café.')")
        guard webView.canGoBack else { throw FBError.message("MIME part navigation did not enter browser history") }
        try await navigate("back to email") { webView.goBack() }
        try await check("email MIME navigation browser history", "!new URL(location.href).searchParams.has('eml-view') && document.querySelector('#eml-body')?.textContent.includes('MIME email body')")
        try await followEmailPart("1.3")
        try await check("email CSV attachment uses the table viewer", "location.pathname.endsWith('/message.eml/1.3/sample.csv') && document.querySelector('table') !== null && document.body.innerText.includes('alpha') && document.body.innerText.includes('20')")
        try await submitQuery("select sum(amount) as total from t")
        try await check("email CSV attachment supports SQL queries", "document.body.innerText.includes('total') && document.body.innerText.includes('30')")
        try await load("message.eml")
        try await followEmailPart("1.1.2")
        try await waitFor("email image attachment uses the image viewer", "location.pathname.endsWith('/message.eml/1.1.2/pixel.png') && document.querySelector('img.subject')?.naturalWidth === 1")
        try await navigate("back from attached image") { webView.goBack() }
        try await check("back from attachment restores the email", "document.querySelector('#eml-body')?.textContent.includes('MIME email body')")
        try await followEmailPart("1.4")
        guard responseMIMEType == "application/pdf", responseCanDisplay,
              webView.url?.path.hasSuffix("/message.eml/1.4/sample.pdf") == true else {
            throw FBError.message("Attached PDF did not navigate to WebKit's PDF viewer")
        }
        passed.append("email PDF attachment displays in WebKit")
        print("PASS email PDF attachment displays in WebKit")
        try await navigate("back from attached PDF") { webView.goBack() }
        try await check("back from attached PDF restores the email", "document.querySelector('#eml-body')?.textContent.includes('MIME email body')")
        try await followEmailPart("1.5")
        try await check("email Markdown attachment uses bundled Pandoc", "document.querySelector('h1')?.textContent === 'Attached document'")
        try await load("message.eml")
        try await followEmailPart("1.6")
        try await check("email archive attachment uses directory browsing", "document.querySelector('table.listing') !== null && document.body.innerText.includes('inside.md')")
        try await load("message.eml/1.6/bundle.zip/inside.md")
        try await check("email archive attachment member uses bundled Pandoc", "document.querySelector('h1')?.textContent === 'Attached archive document'")
        try await load("sample.zip/message.eml")
        try await check("email inside archive", "document.querySelector('h1')?.textContent === 'Café MIME browser' && document.querySelector('.eml-tree') !== null")
        try await waitFor("email CID image inside archive", "[...document.querySelectorAll('#eml-body img')].some(i => i.naturalWidth === 1)")
        try await load("", queryItems: [URLQueryItem(name: "view", value: "inbox")])
        try await check("directory email inbox", "document.body.innerText.includes('Café MIME browser') && document.body.innerText.includes('sender@example.com') && [...document.querySelectorAll('a')].some(a => a.href.includes('message.eml'))")
        try await load("mailbox/emails/", queryItems: [URLQueryItem(name: "view", value: "inbox")])
        try await navigate("leave inbox for parent folder") {
            Task {
                do { _ = try await webView.evaluateJavaScript("[...document.querySelectorAll('.folders a')].find(a => a.textContent === '../').click(); void 0") }
                catch { complete(error) }
            }
        }
        try await check("leaving inbox restores normal file browsing", "location.pathname.endsWith('/mailbox/') && !new URL(location.href).searchParams.has('view') && document.querySelector('table.listing') !== null && document.body.innerText.includes('notes.txt') && !document.body.innerText.includes('0 messages')")
        try await load("mailbox/", queryItems: [URLQueryItem(name: "view", value: "inbox")])
        try await check("stale inbox link to non-mail folder recovers", "!new URL(location.href).searchParams.has('view') && document.querySelector('table.listing') !== null && document.body.innerText.includes('notes.txt')")
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
    private var printCompletion: CheckedContinuation<Bool, any Error>?

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

    private func hover(_ selector: String) async throws {
        let literal = String(decoding: try JSONEncoder().encode(selector), as: UTF8.self)
        let script = "(() => { const r = document.querySelector(\(literal)).getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()"
        let point: [Double] = try await withCheckedThrowingContinuation { continuation in
            webView.evaluateJavaScript(script, in: nil, in: .defaultClient) { result in
                switch result {
                case .success(let value):
                    guard let point = value as? [Double], point.count == 2 else {
                        continuation.resume(throwing: FBError.message("No coordinates for the hovered link"))
                        return
                    }
                    continuation.resume(returning: point)
                case .failure(let error): continuation.resume(throwing: error)
                }
            }
        }
        let x = point[0] * webView.pageZoom
        let y = point[1] * webView.pageZoom
        let location = webView.convert(NSPoint(x: x, y: webView.isFlipped ? y : webView.bounds.height - y), to: nil)
        guard let event = NSEvent.mouseEvent(with: .mouseMoved, location: location, modifierFlags: [],
                                            timestamp: ProcessInfo.processInfo.systemUptime,
                                            windowNumber: window.windowNumber, context: nil,
                                            eventNumber: 0, clickCount: 0, pressure: 0) else {
            throw FBError.message("Could not create a native hover event")
        }
        webView.mouseMoved(with: event)
    }

    private func checkLinkPreview() async throws {
        let originalURL = webView.url
        try await hover("a[href*=\"raw=1\"] strong")
        let localDestination = "spaced & café #1.md?raw=1#somewhere"
        try await waitUntil("local link preview") { model.hoveredLink == localDestination }
        guard let exit = NSEvent.enterExitEvent(with: .mouseExited, location: .zero, modifierFlags: [],
                                               timestamp: ProcessInfo.processInfo.systemUptime,
                                               windowNumber: window.windowNumber, context: nil,
                                               eventNumber: 0, trackingNumber: 0, userData: nil) else {
            throw FBError.message("Could not create a native mouse-exit event")
        }
        webView.mouseExited(with: exit)
        try await waitUntil("link preview clears after pointer leaves") { model.hoveredLink == nil }
        try await hover("a[href^=\"https:\"]")
        try await waitUntil("external link preview") { model.hoveredLink == "https://example.com/reference?from=fb#details" }
        guard webView.url == originalURL else { throw FBError.message("Hovering navigated away from the current document") }
    }

    private func checkHTMLLinkPreview() async throws {
        let finishCount = finished.count
        model.open(fixtures.appendingPathComponent("raw.html"))
        try await waitUntil("plain HTML for link preview") { finished.count > finishCount && !model.isLoading }
        try await hover("#external")
        try await waitUntil("link preview in sandboxed HTML at saved zoom") {
            model.hoveredLink == "https://example.com/plain-html#details"
        }
    }

    @objc nonisolated private func printOperationDidRun(_ operation: NSPrintOperation, success: Bool,
                                                        contextInfo: UnsafeMutableRawPointer?) {
        Task { @MainActor [weak self] in
            let continuation = self?.printCompletion
            self?.printCompletion = nil
            continuation?.resume(returning: success)
        }
    }

    private func checkPrinting() async throws {
        let finishCount = finished.count
        model.open(fixtures.appendingPathComponent("print.md"))
        try await waitUntil("print document navigation") { finished.count > finishCount && !model.isLoading }
        let ready = "document.querySelector('mjx-container') !== null && [...document.images].every(i => i.complete && i.naturalWidth > 0)"
        var rendered = false
        for _ in 0..<150 {
            if try await webView.evaluateJavaScript(ready) as? Bool == true {
                rendered = true
                break
            }
            try await Task.sleep(for: .milliseconds(100))
        }
        guard rendered else { throw FBError.message("Print document image or math did not render") }
        guard try await webView.evaluateJavaScript("document.querySelector('#print-end-marker').getBoundingClientRect().top > innerHeight") as? Bool == true else {
            throw FBError.message("Print fixture did not extend beyond the viewport")
        }
        let originalURL = webView.url
        let originalZoom = webView.pageZoom
        let outputURL = fixtures.deletingLastPathComponent().appendingPathComponent("printed-markdown.pdf")
        if FileManager.default.fileExists(atPath: outputURL.path) { try FileManager.default.removeItem(at: outputURL) }
        // Save directly to a test PDF. This exercises AppKit's paginated print
        // operation without showing a dialog or sending a job to a printer.
        let printInfo = NSPrintInfo(dictionary: [.jobSavingURL: outputURL])
        printInfo.jobDisposition = .save
        printInfo.paperSize = NSSize(width: 612, height: 792)
        printInfo.leftMargin = 36
        printInfo.rightMargin = 36
        printInfo.topMargin = 36
        printInfo.bottomMargin = 36
        guard let operation = model.makePrintOperation(printInfo: printInfo) else {
            throw FBError.message("The loaded document did not provide a print operation")
        }
        guard operation.printInfo.jobDisposition == .save,
              operation.printInfo.dictionary()[NSPrintInfo.AttributeKey.jobSavingURL] as? URL == outputURL else {
            throw FBError.message("Print test requires saving to its PDF destination")
        }
        operation.showsPrintPanel = false
        operation.showsProgressPanel = false
        // WebKit computes page ranges asynchronously. Use the same modal
        // execution path as the app, so its printing thread can await that work.
        var printTimeout: Task<Void, Never>?
        defer { printTimeout?.cancel() }
        let printed = try await withCheckedThrowingContinuation { continuation in
            printCompletion = continuation
            printTimeout = Task { [weak self] in
                try? await Task.sleep(for: .seconds(45))
                guard !Task.isCancelled else { return }
                let pending = self?.printCompletion
                self?.printCompletion = nil
                pending?.resume(throwing: FBError.message("Timed out saving the paginated print PDF"))
            }
            operation.runModal(for: window, delegate: self,
                               didRun: #selector(printOperationDidRun(_:success:contextInfo:)), contextInfo: nil)
        }
        guard printed, let pdf = PDFDocument(url: outputURL), pdf.pageCount > 1,
              pdf.page(at: 0)?.string?.contains("Printable Markdown") == true,
              pdf.page(at: pdf.pageCount - 1)?.string?.contains("Print end marker") == true,
              pdf.string?.contains("North") == true,
              pdf.string?.contains("Final row") == true,
              pdf.string?.contains("final formatted code") == true else {
            throw FBError.message("Printing did not produce a complete, paginated Markdown PDF")
        }
        guard webView.url == originalURL, webView.pageZoom == originalZoom,
              model.errorMessage == nil else {
            throw FBError.message("Printing changed the current document or zoom")
        }
        for index in 0..<pdf.pageCount {
            guard let page = pdf.page(at: index),
                  let text = page.selection(for: NSRange(location: 0, length: page.numberOfCharacters)) else {
                throw FBError.message("Printed page \(index + 1) has no selectable text")
            }
            let bounds = text.bounds(for: page)
            let paper = page.bounds(for: .mediaBox)
            // Font selection rectangles can extend slightly beyond the glyphs.
            let tolerance: CGFloat = 6
            guard bounds.minY >= printInfo.bottomMargin - tolerance,
                  bounds.maxY <= paper.height - printInfo.topMargin + tolerance else {
                throw FBError.message("Printed page \(index + 1) escapes the requested margins: \(bounds)")
            }
        }
        print("Printed Markdown PDF: \(outputURL.path) (\(pdf.pageCount) pages)")
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
        try await checkLinkPreview()
        passed.append("hover previews local and external destinations without navigating")
        model.zoom(0.2)
        cancelNextResponse = true
        model.open(fixtures.appendingPathComponent("nested/next.md"))
        guard model.hoveredLink == nil else { throw FBError.message("Navigation retained a stale link preview") }
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

        try await checkPrinting()
        passed.append("formatted printing includes every page without changing the document or zoom")
        try await checkHTMLLinkPreview()
        passed.append("hover preview works on plain HTML with page scripts disabled")
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
        setbuf(stdout, nil)
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
