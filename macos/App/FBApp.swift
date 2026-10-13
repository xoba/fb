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
                Button("About fb") { Self.showAbout() }
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
        }
    }

    private static func showAbout() {
        let bundle = Bundle.main
        let version = bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.1.0"
        let build = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "1"
        let revision = bundle.object(forInfoDictionaryKey: "FBBuildRevision") as? String ?? ""
        var details = "Browse local files and folders.\n"
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
            .applicationName: "fb",
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

    func register(model: BrowserModel, openWindow: @escaping (BrowserWindowLocation) -> Void) {
        if self.openWindow == nil { initialBrowser = model }
        self.openWindow = openWindow
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
            for url in urls { openWindow(BrowserWindowLocation(path: url.path)) }
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
