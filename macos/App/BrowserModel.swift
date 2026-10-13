import AppKit
import Observation
import WebKit

@MainActor
@Observable
final class BrowserModel: NSObject, WKNavigationDelegate, WKUIDelegate {
    private(set) var webView: FileDropWebView?
    private(set) var title = "fb"
    private(set) var path = ""
    private(set) var isLoading = false
    private(set) var canGoBack = false
    private(set) var canGoForward = false
    private(set) var hoveredLink: String?
    var errorMessage: String?
    var showingFind = false
    private(set) var findFocusRequest = 0
    var findText = ""
    var findResult = ""
    var openAdditional: ((URL) -> Void)?
    private var handler: FBSchemeHandler?
    private var initialURL: URL
    private var started = false
    private var activeNavigation: WKNavigation?
    private let preferences: UserDefaults
    private static let zoomPreferenceKey = "pageZoom"

    init(fileURL: URL, engine suppliedEngine: FBEngine? = nil, preferences: UserDefaults = .standard) {
        initialURL = fileURL
        self.preferences = preferences
        super.init()
        do {
            let pandoc = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers/pandoc").path
            let engine = try suppliedEngine ?? FBEngine(pandoc: pandoc)
            let handler = FBSchemeHandler(engine: engine)
            self.handler = handler
            let configuration = WKWebViewConfiguration()
            handler.configure(configuration)
            let webView = FileDropWebView(frame: .zero, configuration: configuration)
            // Restore before the first load, including windows opened by Finder or Terminal.
            webView.pageZoom = Self.validZoom(preferences.object(forKey: Self.zoomPreferenceKey) as? Double ?? 1)
            webView.navigationDelegate = self
            webView.uiDelegate = self
            webView.allowsBackForwardNavigationGestures = true
            webView.onFilesDropped = { [weak self] in self?.open($0) }
            webView.onLinkHover = { [weak self] in self?.hoveredLink = $0 }
            webView.registerForDraggedTypes([.fileURL])
            self.webView = webView
        } catch { errorMessage = error.localizedDescription }
    }

    func start() {
        guard !started else { return }
        started = true
        open(initialURL)
    }

    func open(_ urls: [URL]) {
        let files = urls.filter(\.isFileURL)
        guard let first = files.first else { return }
        open(first)
        for url in files.dropFirst() { openAdditional?(url) }
    }

    func open(_ url: URL) {
        guard url.isFileURL else { return }
        initialURL = url
        let destination = FBLocation.webURL(fileURL: url)
        errorMessage = nil
        path = url.path
        title = url.lastPathComponent.isEmpty ? "/" : url.lastPathComponent
        handler?.prepareNavigation(to: destination)
        track(webView?.load(URLRequest(url: destination)))
        NSDocumentController.shared.noteNewRecentDocumentURL(url)
        preferences.set(url.path, forKey: "lastOpenedPath")
    }

    func chooseFiles() {
        let panel = NSOpenPanel()
        panel.title = "Open in fb"
        panel.prompt = "Open"
        panel.canChooseFiles = true
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = true
        panel.directoryURL = currentFileURL?.deletingLastPathComponent()
        if let window = webView?.window {
            panel.beginSheetModal(for: window) { [weak self] response in
                if response == .OK { self?.open(panel.urls) }
            }
        } else if panel.runModal() == .OK { open(panel.urls) }
    }

    var currentFileURL: URL? { webView?.url.flatMap(FBLocation.fileURL(webURL:)) ?? initialURL }

    func goBack() { track(webView?.goBack()) }
    func goForward() { track(webView?.goForward()) }
    func reload() {
        errorMessage = nil
        if let url = webView?.url { handler?.prepareNavigation(to: url) }
        track(webView?.reload())
    }
    func reveal() { if let url = currentFileURL { NSWorkspace.shared.activateFileViewerSelecting([url]) } }
    func zoom(_ amount: Double) {
        guard let webView else { return }
        setZoom(webView.pageZoom + amount)
    }
    func resetZoom() { setZoom(1) }

    private static func validZoom(_ value: Double) -> Double {
        value.isFinite ? min(3, max(0.5, value)) : 1
    }

    private func setZoom(_ value: Double) {
        guard let webView else { return }
        let zoom = Self.validZoom(value)
        webView.pageZoom = zoom
        // Only explicit changes update the default; closing another window cannot overwrite it.
        preferences.set(zoom, forKey: Self.zoomPreferenceKey)
    }

    func showFind() {
        showingFind = true
        findFocusRequest += 1
    }

    func find(backwards: Bool = false) {
        guard let webView, !findText.isEmpty else { return }
        let configuration = WKFindConfiguration()
        configuration.backwards = backwards
        configuration.caseSensitive = false
        configuration.wraps = true
        webView.find(findText, configuration: configuration) { [weak self] result in
            self?.findResult = result.matchFound ? "" : "No matches"
        }
    }

    var canPrint: Bool { webView?.url != nil && !isLoading }

    func makePrintOperation(printInfo: NSPrintInfo? = nil) -> NSPrintOperation? {
        guard canPrint, let webView else { return nil }
        let settings = (printInfo ?? NSPrintInfo.shared).copy() as! NSPrintInfo
        settings.horizontalPagination = .fit
        settings.verticalPagination = .automatic
        settings.isVerticallyCentered = false
        // WebKit prints the rendered document, including content below the
        // viewport, with its styles, images, and native page breaks.
        let operation = webView.printOperation(with: settings)
        operation.jobTitle = title
        operation.showsPrintPanel = true
        operation.showsProgressPanel = true
        operation.printPanel.options.formUnion([.showsPaperSize, .showsOrientation, .showsScaling])
        return operation
    }

    func printDocument() {
        guard let window = webView?.window, window.attachedSheet == nil,
              let operation = makePrintOperation() else { return }
        operation.runModal(for: window, delegate: nil, didRun: nil, contextInfo: nil)
    }

    func saveOriginal() {
        guard let url = webView?.url else { return }
        var components = URLComponents(url: url, resolvingAgainstBaseURL: false)!
        var query = components.queryItems ?? []
        query.removeAll { $0.name == "raw" }
        query.append(URLQueryItem(name: "raw", value: "1"))
        components.queryItems = query
        if let source = components.url { save(source) }
    }

    private func save(_ url: URL, suggestedName: String? = nil) {
        guard let handler else { return }
        let panel = NSSavePanel()
        panel.title = "Save a Copy"
        panel.nameFieldStringValue = suggestedName ?? url.lastPathComponent
        let completion: @MainActor (NSApplication.ModalResponse) -> Void = { [weak self] response in
            guard response == .OK, let target = panel.url else { return }
            let operation = FBEngineRequest(engine: handler.engine, description: FBRequestDescription(url: url.absoluteString))
            Task {
                do {
                    try await Task.detached(priority: .userInitiated) {
                        defer { operation.finish() }
                        let head = try operation.begin()
                        guard let status = head.status, (200..<300).contains(status) else {
                            operation.cancel()
                            throw FBError.message("This file could not be saved (status \(head.status ?? 0)).")
                        }
                        // A sibling temporary file preserves an existing destination if reading fails.
                        let temporary = target.deletingLastPathComponent().appendingPathComponent(".fb-save-\(UUID().uuidString)")
                        defer { try? FileManager.default.removeItem(at: temporary) }
                        guard FileManager.default.createFile(atPath: temporary.path, contents: nil) else {
                            operation.cancel()
                            throw FBError.message("The destination folder could not be written.")
                        }
                        let output = try FileHandle(forWritingTo: temporary)
                        do {
                            while let data = try operation.read() { try output.write(contentsOf: data) }
                            try output.close()
                        } catch { try? output.close(); throw error }
                        if FileManager.default.fileExists(atPath: target.path) {
                            _ = try FileManager.default.replaceItemAt(target, withItemAt: temporary)
                        } else { try FileManager.default.moveItem(at: temporary, to: target) }
                    }.value
                } catch { self?.errorMessage = error.localizedDescription }
            }
        }
        if let window = webView?.window { panel.beginSheetModal(for: window, completionHandler: completion) }
        else { completion(panel.runModal()) }
    }

    private func updateLocation(_ webView: WKWebView) {
        canGoBack = webView.canGoBack
        canGoForward = webView.canGoForward
        guard let url = webView.url else { return }
        path = url.path
        title = webView.title.flatMap { $0.isEmpty ? nil : $0 } ?? url.lastPathComponent
        webView.window?.representedURL = FBLocation.fileURL(webURL: url)
        if let fileURL = FBLocation.fileURL(webURL: url) {
            preferences.set(fileURL.path, forKey: "lastOpenedPath")
        }
    }

    private func track(_ navigation: WKNavigation?) {
        guard let navigation else { return }
        webView?.clearLinkHover()
        activeNavigation = navigation
        isLoading = true
        errorMessage = nil
    }

    func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) { track(navigation) }
    func webView(_ webView: WKWebView, didCommit navigation: WKNavigation!) {
        guard let navigation, navigation === activeNavigation else { return }
        updateLocation(webView)
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        guard let navigation, navigation === activeNavigation else { return }
        activeNavigation = nil
        isLoading = false
        updateLocation(webView)
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        navigationFailed(navigation, error: error)
    }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        navigationFailed(navigation, error: error)
    }

    private func navigationFailed(_ navigation: WKNavigation?, error: Error) {
        // WebKit can deliver the cancelled load's failure after its replacement starts,
        // including reloads of the exact same URL. Navigation identity distinguishes them.
        guard let navigation, navigation === activeNavigation else { return }
        activeNavigation = nil
        isLoading = false
        let failure = error as NSError
        // 102 is WebKitErrorFrameLoadInterruptedByPolicyChange: our deliberate .cancel
        // decisions hand downloads/links to native UI. Neither case is a rendering error.
        if (failure.domain == NSURLErrorDomain && failure.code == NSURLErrorCancelled) ||
            (failure.domain == "WebKitErrorDomain" && failure.code == 102) {
            if let webView { updateLocation(webView) }
            return
        }
        errorMessage = error.localizedDescription
    }

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping @MainActor (WKNavigationActionPolicy) -> Void) {
        guard let url = action.request.url else { decisionHandler(.cancel); return }
        if action.targetFrame?.isMainFrame != false { self.webView?.clearLinkHover() }
        if url.scheme == "fb", url.host == "local" {
            if action.shouldPerformDownload {
                decisionHandler(.cancel)
                save(url)
            } else {
                if action.targetFrame?.isMainFrame != false { handler?.prepareNavigation(to: url) }
                decisionHandler(.allow)
            }
        } else if url.isFileURL {
            decisionHandler(.cancel)
            open(url)
        } else {
            decisionHandler(.cancel)
            if action.navigationType == .linkActivated, ["https", "http", "mailto"].contains(url.scheme ?? "") {
                NSWorkspace.shared.open(url)
            }
        }
    }

    func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse,
                 decisionHandler: @escaping @MainActor (WKNavigationResponsePolicy) -> Void) {
        if !response.canShowMIMEType, response.isForMainFrame, let url = response.response.url {
            decisionHandler(.cancel)
            save(url, suggestedName: response.response.suggestedFilename)
        } else { decisionHandler(.allow) }
    }

    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                 for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        guard let url = navigationAction.request.url else { return nil }
        if let fileURL = FBLocation.fileURL(webURL: url) { openAdditional?(fileURL) }
        else if ["https", "http", "mailto"].contains(url.scheme ?? "") { NSWorkspace.shared.open(url) }
        return nil
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        self.webView?.clearLinkHover()
        activeNavigation = nil
        isLoading = false
        errorMessage = "The document view stopped unexpectedly. Reload to reopen this file."
    }
}

@MainActor
final class FileDropWebView: WKWebView {
    var onFilesDropped: (([URL]) -> Void)?
    var onLinkHover: ((String?) -> Void)?
    private var linkTrackingArea: NSTrackingArea?
    private var linkHoverRequest = 0

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let linkTrackingArea { removeTrackingArea(linkTrackingArea) }
        let area = NSTrackingArea(rect: .zero,
                                 options: [.inVisibleRect, .mouseMoved, .mouseEnteredAndExited, .activeInKeyWindow],
                                 owner: self, userInfo: nil)
        addTrackingArea(area)
        linkTrackingArea = area
    }

    override func mouseEntered(with event: NSEvent) {
        super.mouseEntered(with: event)
        updateLinkHover(with: event)
    }

    override func mouseMoved(with event: NSEvent) {
        super.mouseMoved(with: event)
        updateLinkHover(with: event)
    }

    override func mouseExited(with event: NSEvent) {
        super.mouseExited(with: event)
        clearLinkHover()
    }

    override func scrollWheel(with event: NSEvent) {
        clearLinkHover()
        super.scrollWheel(with: event)
    }

    func clearLinkHover() {
        linkHoverRequest &+= 1
        onLinkHover?(nil)
    }

    private func updateLinkHover(with event: NSEvent) {
        let point = convert(event.locationInWindow, from: nil)
        guard bounds.contains(point), url?.scheme == "fb", url?.host == "local" else {
            clearLinkHover()
            return
        }
        let x = (point.x - bounds.minX) / pageZoom
        let y = (isFlipped ? point.y - bounds.minY : bounds.maxY - point.y) / pageZoom
        linkHoverRequest &+= 1
        let request = linkHoverRequest
        // Native pointer tracking works even when a document's sandbox blocks
        // JavaScript event handlers. This isolated evaluation only reads the
        // hovered anchor; it does not enable page scripts or change navigation.
        let script = """
        (() => {
          const link = document.elementFromPoint(\(x), \(y))?.closest('a[href], area[href]');
          if (!link) return null;
          try { return new URL(link.getAttribute('href'), document.baseURI).href; }
          catch { return null; }
        })()
        """
        evaluateJavaScript(script, in: nil, in: .defaultClient) { [weak self] result in
            guard let self, self.linkHoverRequest == request else { return }
            let address = (try? result.get()) as? String
            self.onLinkHover?(address.flatMap(Self.linkPreviewText))
        }
    }

    static func linkPreviewText(_ address: String) -> String? {
        guard !address.isEmpty, let url = URL(string: address), url.scheme != nil else { return nil }
        if url.scheme == "fb", url.host == "local",
           let components = URLComponents(url: url, resolvingAgainstBaseURL: false) {
            var text = components.path
            if let query = components.percentEncodedQuery { text += "?" + query }
            if let fragment = components.percentEncodedFragment { text += "#" + fragment }
            return text
        }
        return url.absoluteString
    }

    static func fileURLs(from pasteboard: NSPasteboard) -> [URL] {
        (pasteboard.readObjects(forClasses: [NSURL.self], options: [.urlReadingFileURLsOnly: true]) as? [URL] ?? [])
            .filter(\.isFileURL)
    }

    override func draggingEntered(_ sender: any NSDraggingInfo) -> NSDragOperation {
        Self.fileURLs(from: sender.draggingPasteboard).isEmpty ? super.draggingEntered(sender) : .copy
    }
    override func draggingUpdated(_ sender: any NSDraggingInfo) -> NSDragOperation {
        Self.fileURLs(from: sender.draggingPasteboard).isEmpty ? super.draggingUpdated(sender) : .copy
    }
    override func prepareForDragOperation(_ sender: any NSDraggingInfo) -> Bool {
        !Self.fileURLs(from: sender.draggingPasteboard).isEmpty || super.prepareForDragOperation(sender)
    }
    override func performDragOperation(_ sender: any NSDraggingInfo) -> Bool {
        let urls = Self.fileURLs(from: sender.draggingPasteboard)
        guard !urls.isEmpty else { return super.performDragOperation(sender) }
        onFilesDropped?(urls)
        return true
    }
}
