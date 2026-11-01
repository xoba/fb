import AppKit
import Foundation

/// fb's documentation is Markdown shipped inside the app and opened in fb itself, so
/// its links can reach any file, line, or sample that fb can show. A system Help Book
/// could not do this: it would open in a separate viewer that renders only its own HTML.
enum FBHelp {
    /// Nil outside an .app, as in the smoke test, whose fixtures must not count as bundled.
    private static var appBundle: URL? {
        let bundle = Bundle.main.bundleURL
        return bundle.pathExtension == "app" ? bundle : nil
    }

    private static var resourcesRoot: URL {
        Bundle.main.bundleURL.appendingPathComponent("Contents/Resources", isDirectory: true)
    }

    static var docsRoot: URL { resourcesRoot.appendingPathComponent("docs", isDirectory: true) }

    static var noticesURL: URL { resourcesRoot.appendingPathComponent("Dependencies/NOTICE.md") }

    static func url(forPage page: String, fragment: String? = nil) -> URL {
        FBLocation.fileURL(path: docsRoot.appendingPathComponent(page).path, fragment: fragment)
    }

    /// Whether a location is inside the app, rather than one of the user's own files.
    static func isBundled(_ url: URL) -> Bool {
        guard let appBundle else { return false }
        return contains(url.path, in: appBundle.path)
    }

    /// The toolbar's text for a bundled page, which reads better than a path into the app.
    static func displayPath(forPath path: String) -> String? {
        guard appBundle != nil else { return nil }
        guard let relative = relativePath(path, to: docsRoot) ?? relativePath(path, to: resourcesRoot) else { return nil }
        return "fb Help › " + relative
    }

    /// Help pages take their title from their first heading. Other bundled files, such
    /// as samples, keep their filenames.
    static func windowTitle(forPath path: String) -> String? {
        guard appBundle != nil, let relative = relativePath(path, to: docsRoot) else { return nil }
        guard let title = FBHelpIndex.shared.title(ofPage: relative), title != "fb Help" else { return "fb Help" }
        return "fb Help: " + title
    }

    private static func relativePath(_ path: String, to folder: URL) -> String? {
        guard contains(path, in: folder.path) else { return nil }
        // Compare through the same resolution `contains` uses, then drop the root.
        let resolved = URL(fileURLWithPath: path).resolvingSymlinksInPath().path
        let root = folder.resolvingSymlinksInPath().path
        return String(resolved.dropFirst(root.count)).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }

    static func contains(_ path: String, in folder: String) -> Bool {
        let resolved = URL(fileURLWithPath: path).resolvingSymlinksInPath().path
        let root = URL(fileURLWithPath: folder).resolvingSymlinksInPath().path
        return resolved == root || resolved.hasPrefix(root.hasSuffix("/") ? root : root + "/")
    }
}

/// One searchable place in the documentation: a page, or a section of one.
final class FBHelpTopic: NSObject, Sendable {
    let page: String
    let pageTitle: String
    let heading: String?
    let anchor: String?
    fileprivate let titleKey: String
    fileprivate let bodyKey: String

    init(page: String, pageTitle: String, heading: String?, anchor: String?, body: String) {
        self.page = page
        self.pageTitle = pageTitle
        self.heading = heading
        self.anchor = anchor
        titleKey = FBHelpIndex.searchKey(pageTitle + " " + (heading ?? ""))
        bodyKey = FBHelpIndex.searchKey(body)
    }
}

/// A small in-memory index of the bundled Markdown. There are few enough pages to
/// read on first use, and nothing has to be generated or kept in step at build time.
final class FBHelpIndex: Sendable {
    static let shared = FBHelpIndex(root: FBHelp.docsRoot)

    let topics: [FBHelpTopic]
    private let titles: [String: String]

    init(root: URL) {
        var topics: [FBHelpTopic] = []
        var titles: [String: String] = [:]
        let rootPath = root.resolvingSymlinksInPath().path
        let enumerator = FileManager.default.enumerator(at: root.resolvingSymlinksInPath(), includingPropertiesForKeys: nil)
        var pages: [(relative: String, url: URL)] = []
        while let url = enumerator?.nextObject() as? URL {
            guard url.pathExtension.lowercased() == "md" else { continue }
            let relative = String(url.resolvingSymlinksInPath().path.dropFirst(rootPath.count))
                .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            pages.append((relative, url))
        }
        // Stable results: the overview pages come before the deeper reference pages.
        pages.sort { ($0.relative.filter { $0 == "/" }.count, $0.relative) < ($1.relative.filter { $0 == "/" }.count, $1.relative) }
        for page in pages {
            guard let text = try? String(contentsOf: page.url, encoding: .utf8) else { continue }
            let parsed = Self.topics(in: text, page: page.relative)
            topics.append(contentsOf: parsed)
            if let title = parsed.first?.pageTitle { titles[page.relative] = title }
        }
        self.topics = topics
        self.titles = titles
    }

    func title(ofPage page: String) -> String? { titles[page] }

    /// Every word of the query must appear. Topics whose titles match come first, so
    /// that searching "parquet" lists the Parquet page ahead of pages that mention it.
    func search(_ query: String, limit: Int) -> [FBHelpTopic] {
        let words = Self.searchKey(query).split(separator: " ").map(String.init)
        guard !words.isEmpty, limit > 0 else { return [] }
        var inTitle: [FBHelpTopic] = []
        var inBody: [FBHelpTopic] = []
        for topic in topics {
            if words.allSatisfy(topic.titleKey.contains) {
                inTitle.append(topic)
            } else if words.allSatisfy({ topic.titleKey.contains($0) || topic.bodyKey.contains($0) }) {
                inBody.append(topic)
            }
        }
        return Array((inTitle + inBody).prefix(limit))
    }

    fileprivate static func searchKey(_ text: String) -> String {
        text.folding(options: [.caseInsensitive, .diacriticInsensitive], locale: nil)
    }

    /// Splits a page at its ## and ### headings. Lines inside fenced code blocks are
    /// not headings, even when they begin with a # like a shell comment.
    static func topics(in text: String, page: String) -> [FBHelpTopic] {
        var pageTitle: String?
        var sections: [(heading: String?, anchor: String?, body: [String])] = [(nil, nil, [])]
        var usedAnchors: [String: Int] = [:]
        var fence: String?
        for line in text.split(separator: "\n", omittingEmptySubsequences: false).map(String.init) {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if let marker = fence {
                if trimmed.hasPrefix(marker) { fence = nil }
            } else if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                fence = String(trimmed.prefix(3))
            } else if let (level, heading) = parseHeading(line) {
                if level == 1 {
                    if pageTitle == nil { pageTitle = heading }
                } else if level <= 3 {
                    var anchor = gfmAnchor(heading)
                    // Pandoc numbers repeated headings the way GitHub does.
                    let seen = usedAnchors[anchor, default: 0]
                    usedAnchors[anchor] = seen + 1
                    if seen > 0 { anchor += "-\(seen)" }
                    sections.append((heading, anchor, []))
                    continue
                }
            }
            sections[sections.count - 1].body.append(line)
        }
        let title = pageTitle ?? (page as NSString).lastPathComponent
        return sections.map {
            FBHelpTopic(page: page, pageTitle: title, heading: $0.heading, anchor: $0.anchor,
                        body: $0.body.joined(separator: " "))
        }
    }

    private static func parseHeading(_ line: String) -> (Int, String)? {
        let level = line.prefix { $0 == "#" }.count
        guard (1...6).contains(level), line.dropFirst(level).first == " " else { return nil }
        let text = line.dropFirst(level).trimmingCharacters(in: .whitespaces)
        return text.isEmpty ? nil : (level, plainHeading(text))
    }

    /// Headings show as plain text in the menu, without Markdown's code markers.
    private static func plainHeading(_ heading: String) -> String {
        heading.replacingOccurrences(of: "`", with: "")
    }

    /// The identifier Pandoc's gfm_auto_identifiers gives a heading, matching GitHub's
    /// rule: lowercase, drop punctuation, and turn spaces into hyphens.
    static func gfmAnchor(_ heading: String) -> String {
        var anchor = ""
        for character in heading.lowercased() {
            if character == " " {
                anchor.append("-")
            } else if character.isLetter || character.isNumber || character == "-" || character == "_" {
                anchor.append(character)
            }
        }
        return anchor
    }
}
