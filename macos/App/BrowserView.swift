import AppKit
import SwiftUI
import WebKit

struct BrowserWindowLocation: Codable, Hashable {
    var path: String
    var id = UUID()
}

private struct BrowserFocusKey: FocusedValueKey { typealias Value = BrowserModel }
extension FocusedValues {
    var browser: BrowserModel? {
        get { self[BrowserFocusKey.self] }
        set { self[BrowserFocusKey.self] = newValue }
    }
}

struct BrowserView: View {
    @Environment(\.openWindow) private var openWindow
    @State private var model: BrowserModel

    init(location: BrowserWindowLocation?) {
        let path = location?.path ?? UserDefaults.standard.string(forKey: "lastOpenedPath") ?? NSHomeDirectory()
        _model = State(initialValue: BrowserModel(fileURL: URL(fileURLWithPath: path)))
    }

    var body: some View {
        VStack(spacing: 0) {
            if let error = model.errorMessage {
                HStack(alignment: .top) {
                    Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange)
                    Text(error).textSelection(.enabled)
                    Spacer()
                    Button("Reload") { model.reload() }
                    Button { model.errorMessage = nil } label: { Image(systemName: "xmark") }
                        .buttonStyle(.plain).help("Dismiss")
                }
                .padding().background(.background.secondary)
                Divider()
            }
            if let webView = model.webView {
                NativeWebView(webView: webView)
                    .overlay(alignment: .bottomLeading) {
                        if let destination = model.hoveredLink {
                            Text(destination)
                                .font(.system(size: 11))
                                .lineLimit(1)
                                .truncationMode(.middle)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 5)
                                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 5))
                                .frame(maxWidth: 600, alignment: .leading)
                                .padding(8)
                                .allowsHitTesting(false)
                                .accessibilityLabel("Link destination: \(destination)")
                        }
                    }
            } else {
                ContentUnavailableView("The document engine could not start", systemImage: "doc.badge.ellipsis",
                                       description: Text("Rebuild or reinstall fb with its bundled Pandoc executable."))
            }
        }
        .frame(minWidth: 640, minHeight: 420)
        .navigationTitle(model.title)
        .focusedSceneValue(\.browser, model)
        .toolbar {
            ToolbarItemGroup(placement: .navigation) {
                Button("Back", systemImage: "chevron.left") { model.goBack() }.disabled(!model.canGoBack)
                Button("Forward", systemImage: "chevron.right") { model.goForward() }.disabled(!model.canGoForward)
                Button("Open", systemImage: "folder") { model.chooseFiles() }.help("Open files or folders")
            }
            ToolbarItem(placement: .principal) {
                if model.showingFind {
                    findControls
                } else {
                    HStack(spacing: 8) {
                        if model.isLoading { ProgressView().controlSize(.small) }
                        Text(model.path).font(.callout).lineLimit(1).truncationMode(.middle)
                            .help(model.path).textSelection(.enabled)
                    }.padding(.horizontal, 12).frame(maxWidth: 600)
                }
            }
            ToolbarItemGroup(placement: .primaryAction) {
                Button("Reload", systemImage: "arrow.clockwise") { model.reload() }
                Button("Reveal in Finder", systemImage: "folder.badge.gearshape") { model.reveal() }
            }
        }
        .onAppear {
            let openWindowAction = openWindow
            model.openAdditional = { url in
                openWindowAction(value: BrowserWindowLocation(path: url.path))
            }
            ApplicationCoordinator.shared.register(model: model, openWindow: { location in
                openWindowAction(value: location)
            })
            model.start()
        }
        .onChange(of: model.findText) { model.findResult = "" }
    }

    private var findControls: some View {
        HStack(spacing: 6) {
            ToolbarFindField(text: $model.findText, focusRequest: model.findFocusRequest,
                             find: { model.find() }, dismiss: dismissFind)
                .frame(minWidth: 120, idealWidth: 200, maxWidth: 200)
            Image(systemName: "exclamationmark.circle")
                .foregroundStyle(.secondary)
                .frame(width: 14)
                .opacity(model.findResult.isEmpty ? 0 : 1)
                .accessibilityHidden(model.findResult.isEmpty)
                .accessibilityLabel("No matches")
                .help("No matches")
            Button { model.find(backwards: true) } label: {
                Image(systemName: "chevron.up")
            }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Previous match")
                .accessibilityAddTraits(.isButton)
                .help("Previous match (⇧⌘G)")
                .disabled(model.findText.isEmpty)
            Button { model.find() } label: {
                Image(systemName: "chevron.down")
            }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Next match")
                .accessibilityAddTraits(.isButton)
                .help("Next match (⌘G)")
                .disabled(model.findText.isEmpty)
            Button { dismissFind() } label: {
                Image(systemName: "xmark")
            }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Done")
                .accessibilityAddTraits(.isButton)
                .help("Close Find (Esc)")
                .keyboardShortcut(.cancelAction)
        }
        .controlSize(.small)
        .padding(.horizontal, 6)
    }

    private func dismissFind() {
        model.showingFind = false
        if let webView = model.webView {
            webView.window?.makeFirstResponder(webView)
        }
    }
}

private struct ToolbarFindField: NSViewRepresentable {
    @Binding var text: String
    let focusRequest: Int
    let find: () -> Void
    let dismiss: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> FindSearchField {
        let field = FindSearchField()
        field.placeholderString = "Find in this page"
        field.setAccessibilityLabel("Find in this page")
        field.controlSize = .small
        field.font = .systemFont(ofSize: NSFont.smallSystemFontSize)
        field.sendsWholeSearchString = true
        field.delegate = context.coordinator
        field.target = context.coordinator
        field.action = #selector(Coordinator.submit)
        return field
    }

    func updateNSView(_ field: FindSearchField, context: Context) {
        context.coordinator.parent = self
        if field.stringValue != text { field.stringValue = text }
        field.requestFocus(focusRequest)
    }

    static func dismantleNSView(_ field: FindSearchField, coordinator: Coordinator) {
        field.cancelFocusRequest()
        field.delegate = nil
        field.target = nil
    }

    final class Coordinator: NSObject, NSSearchFieldDelegate {
        var parent: ToolbarFindField
        init(_ parent: ToolbarFindField) { self.parent = parent }

        @objc func submit() { parent.find() }

        func controlTextDidChange(_ notification: Notification) {
            guard let field = notification.object as? NSSearchField else { return }
            parent.text = field.stringValue
        }

        func control(_ control: NSControl, textView: NSTextView,
                     doCommandBy commandSelector: Selector) -> Bool {
            switch commandSelector {
            case #selector(NSResponder.insertNewline(_:)):
                parent.find()
                return true
            case #selector(NSResponder.cancelOperation(_:)):
                parent.dismiss()
                return true
            default:
                return false
            }
        }
    }
}

private final class FindSearchField: NSSearchField {
    private var requestedFocus: Int?
    private var appliedFocus: Int?

    func requestFocus(_ request: Int) {
        requestedFocus = request
        focusWhenAttached()
    }

    func cancelFocusRequest() { requestedFocus = nil }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        focusWhenAttached()
    }

    private func focusWhenAttached() {
        guard let request = requestedFocus, request != appliedFocus, let window else { return }
        // A toolbar view can acquire its window before its native controls are ready.
        DispatchQueue.main.async { [weak self, weak window] in
            guard let self, let window, self.window === window,
                  self.requestedFocus == request, self.appliedFocus != request else { return }
            if window.makeFirstResponder(self) {
                self.selectText(nil)
                self.appliedFocus = request
            }
        }
    }
}

private struct NativeWebView: NSViewRepresentable {
    let webView: FileDropWebView
    func makeNSView(context: Context) -> FileDropWebView { webView }
    func updateNSView(_ nsView: FileDropWebView, context: Context) {}
    static func dismantleNSView(_ nsView: FileDropWebView, coordinator: ()) {
        nsView.clearLinkHover()
        nsView.stopLoading()
        (nsView.configuration.urlSchemeHandler(forURLScheme: "fb") as? FBSchemeHandler)?.cancelAll()
        nsView.navigationDelegate = nil
        nsView.uiDelegate = nil
    }
}
