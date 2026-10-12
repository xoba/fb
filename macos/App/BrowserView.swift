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
    @FocusState private var findFocused: Bool

    init(location: BrowserWindowLocation?) {
        let path = location?.path ?? UserDefaults.standard.string(forKey: "lastOpenedPath") ?? NSHomeDirectory()
        _model = State(initialValue: BrowserModel(fileURL: URL(fileURLWithPath: path)))
    }

    var body: some View {
        VStack(spacing: 0) {
            if model.showingFind {
                HStack {
                    TextField("Find in this page", text: $model.findText)
                        .textFieldStyle(.roundedBorder).frame(maxWidth: 280)
                        .focused($findFocused).onSubmit { model.find() }
                    Text(model.findResult).foregroundStyle(.secondary)
                    Spacer()
                    Button("Previous", systemImage: "chevron.up") { model.find(backwards: true) }
                    Button("Next", systemImage: "chevron.down") { model.find() }
                    Button("Done") { model.showingFind = false }
                }
                .padding(10)
                Divider()
            }
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
                HStack(spacing: 8) {
                    if model.isLoading { ProgressView().controlSize(.small) }
                    Text(model.path).font(.callout).lineLimit(1).truncationMode(.middle)
                        .help(model.path).textSelection(.enabled)
                }.padding(.horizontal, 12).frame(maxWidth: 600)
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
        .onChange(of: model.showingFind) { _, visible in findFocused = visible }
    }
}

private struct NativeWebView: NSViewRepresentable {
    let webView: FileDropWebView
    func makeNSView(context: Context) -> FileDropWebView { webView }
    func updateNSView(_ nsView: FileDropWebView, context: Context) {}
    static func dismantleNSView(_ nsView: FileDropWebView, coordinator: ()) {
        nsView.stopLoading()
        (nsView.configuration.urlSchemeHandler(forURLScheme: "fb") as? FBSchemeHandler)?.cancelAll()
        nsView.navigationDelegate = nil
        nsView.uiDelegate = nil
    }
}
