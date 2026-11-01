import AppKit
import SwiftUI

@main
struct FBApp: App {
    @NSApplicationDelegateAdaptor(FBAppDelegate.self) private var appDelegate
    @FocusedValue(\.browser) private var browser

    var body: some Scene {
        WindowGroup("fb", for: BrowserWindowLocation.self) { location in
            BrowserView(location: location.wrappedValue)
        }
        .defaultSize(width: 1120, height: 820)
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("About File Browser") { Self.showAbout() }
            }
            CommandGroup(after: .newItem) {
                Button("Open…") { ApplicationCoordinator.shared.chooseFiles(in: browser) }
                    .keyboardShortcut("o")
                Button("Save a Copy…") { browser?.saveOriginal() }
                    .keyboardShortcut("s", modifiers: [.command, .shift]).disabled(browser == nil)
                Button("Reveal in Finder") { browser?.reveal() }
                    .keyboardShortcut("r", modifiers: [.command, .shift]).disabled(browser == nil)
            }
            CommandGroup(replacing: .printItem) {
                Button("Print…") { browser?.printDocument() }
                    .keyboardShortcut("p").disabled(browser?.canPrint != true)
            }
            CommandGroup(after: .textEditing) {
                Button("Find…") { browser?.showFind() }
                    .keyboardShortcut("f").disabled(browser == nil)
                Button("Find Next") { browser?.find() }.keyboardShortcut("g").disabled(browser == nil)
                Button("Find Previous") { browser?.find(backwards: true) }
                    .keyboardShortcut("g", modifiers: [.command, .shift]).disabled(browser == nil)
            }
            CommandGroup(after: .toolbar) {
                Button("Reload") { browser?.reload() }.keyboardShortcut("r").disabled(browser == nil)
                Button("Back") { browser?.goBack() }.keyboardShortcut("[").disabled(browser?.canGoBack != true)
                Button("Forward") { browser?.goForward() }.keyboardShortcut("]").disabled(browser?.canGoForward != true)
                Divider()
                Button("Zoom In") { browser?.zoom(0.1) }.keyboardShortcut("+").disabled(browser == nil)
                Button("Zoom Out") { browser?.zoom(-0.1) }.keyboardShortcut("-").disabled(browser == nil)
                Button("Actual Size") { browser?.resetZoom() }.keyboardShortcut("0").disabled(browser == nil)
            }
            CommandGroup(replacing: .help) {
                Button("fb Help") { ApplicationCoordinator.shared.openHelp() }
                    .keyboardShortcut("?", modifiers: .command)
                Divider()
                Button("File Types") { ApplicationCoordinator.shared.openHelp("file-types/index.md") }
                Button("Linking to Files and Lines") { ApplicationCoordinator.shared.openHelp("linking.md") }
                Button("Keyboard Shortcuts") { ApplicationCoordinator.shared.openHelp("keyboard-shortcuts.md") }
                Button("Troubleshooting") { ApplicationCoordinator.shared.openHelp("troubleshooting.md") }
                Divider()
                Button("Third-Party Notices") { ApplicationCoordinator.shared.openNotices() }
            }
        }
    }

    private static func showAbout() {
        let bundle = Bundle.main
        let version = bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.1.0"
        let build = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "1"
        let revision = bundle.object(forInfoDictionaryKey: "FBBuildRevision") as? String ?? ""
        var details = "By Michael Raskin Andrews LLC\n\nBrowse local files and folders.\n"
        if let url = bundle.url(forResource: "pandoc", withExtension: "json", subdirectory: "Dependencies"),
           let data = try? Data(contentsOf: url),
           let manifest = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let pandocVersion = manifest["version"] as? String {
            details += "\nPandoc \(pandocVersion)\n"
        }
        if !revision.isEmpty { details += "\nSource \(revision)" }
        let paragraph = NSMutableParagraphStyle()
        paragraph.alignment = .center
        var options: [NSApplication.AboutPanelOptionKey: Any] = [
            .applicationName: "File Browser",
            .applicationVersion: version,
            .version: build,
            .credits: NSAttributedString(string: details, attributes: [
                .font: NSFont.systemFont(ofSize: NSFont.smallSystemFontSize),
                .foregroundColor: NSColor.secondaryLabelColor,
                .paragraphStyle: paragraph
            ])
        ]
        if let icon = NSImage(contentsOf: bundle.bundleURL.appendingPathComponent("Contents/Resources/AppIcon.icns")) {
            options[.applicationIcon] = icon
        }
        NSApp.orderFrontStandardAboutPanel(options: options)
    }
}

@MainActor
final class ApplicationCoordinator {
    static let shared = ApplicationCoordinator()
    private weak var initialBrowser: BrowserModel?
    private var openWindow: ((BrowserWindowLocation) -> Void)?
    private var pendingURLs: [URL] = []
    private var useInitialBrowser = true
    private var browsers: [WeakBrowser] = []

    private struct WeakBrowser { weak var model: BrowserModel? }

    func register(model: BrowserModel, openWindow: @escaping (BrowserWindowLocation) -> Void) {
        if self.openWindow == nil { initialBrowser = model }
        self.openWindow = openWindow
        browsers.removeAll { $0.model == nil }
        browsers.append(WeakBrowser(model: model))
        if !pendingURLs.isEmpty {
            let urls = pendingURLs
            pendingURLs.removeAll()
            openExternal(urls)
        }
    }

    func openExternal(_ urls: [URL]) {
        let urls = urls.filter(\.isFileURL)
        guard !urls.isEmpty else { return }
        guard let openWindow else { pendingURLs.append(contentsOf: urls); return }
        if useInitialBrowser, let initialBrowser {
            useInitialBrowser = false
            initialBrowser.open(urls)
        } else {
            for url in urls { openWindow(BrowserWindowLocation(fileURL: url)) }
        }
        NSApp.activate()
    }

    /// Shows a page of the bundled documentation, reusing a window already showing
    /// help so that repeated visits do not accumulate windows.
    func openHelp(_ page: String = "index.md", fragment: String? = nil) {
        show(FBHelp.url(forPage: page, fragment: fragment))
    }

    func openNotices() { show(FBHelp.noticesURL) }

    private func show(_ url: URL) {
        guard FileManager.default.fileExists(atPath: url.path) else {
            NSSound.beep()
            return
        }
        if let existing = browsers.lazy.compactMap(\.model).first(where: { $0.isShowingHelp && $0.webView?.window != nil }) {
            existing.open(url)
            existing.webView?.window?.deminiaturize(nil)
            existing.webView?.window?.makeKeyAndOrderFront(nil)
        } else {
            openWindow?(BrowserWindowLocation(fileURL: url))
        }
        NSApp.activate()
    }

    func chooseFiles(in browser: BrowserModel?) {
        if let browser { browser.chooseFiles(); return }
        let panel = NSOpenPanel()
        panel.title = "Open in fb"
        panel.prompt = "Open"
        panel.canChooseFiles = true
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = true
        if panel.runModal() == .OK { openExternal(panel.urls) }
    }

    func reopenWindow() {
        let path = UserDefaults.standard.string(forKey: "lastOpenedPath") ?? NSHomeDirectory()
        openWindow?(BrowserWindowLocation(path: path))
    }
}

@MainActor
final class FBAppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // A local reinstall can retain Launch Services' previous placeholder.
        // Use the current bundle's artwork for the Dock and standard About panel.
        if let icon = NSImage(contentsOf: Bundle.main.bundleURL.appendingPathComponent("Contents/Resources/AppIcon.icns")) {
            NSApp.applicationIconImage = icon
        }
        NSApp.registerUserInterfaceItemSearchHandler(FBHelpSearch.shared)
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        ApplicationCoordinator.shared.openExternal(urls)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        guard !hasVisibleWindows else { return true }
        if let window = sender.windows.first(where: { $0.canBecomeMain && !($0 is NSPanel) }) {
            window.deminiaturize(nil)
            window.makeKeyAndOrderFront(nil)
        } else { ApplicationCoordinator.shared.reopenWindow() }
        return false
    }
}

/// Feeds topics into the search field AppKit puts at the top of the Help menu.
final class FBHelpSearch: NSObject, NSUserInterfaceItemSearching, Sendable {
    static let shared = FBHelpSearch()

    func searchForItems(withSearch searchString: String, resultLimit: Int,
                        matchedItemHandler handleMatchedItems: @escaping ([Any]) -> Void) {
        handleMatchedItems(FBHelpIndex.shared.search(searchString, limit: resultLimit))
    }

    func localizedTitles(forItem item: Any) -> [String] {
        guard let topic = item as? FBHelpTopic else { return [] }
        return [topic.pageTitle, topic.heading].compactMap { $0 }
    }

    func performAction(forItem item: Any) {
        guard let topic = item as? FBHelpTopic else { return }
        let page = topic.page, anchor = topic.anchor
        Task { @MainActor in
            ApplicationCoordinator.shared.openHelp(page, fragment: anchor)
        }
    }
}
