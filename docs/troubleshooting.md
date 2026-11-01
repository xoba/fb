# Troubleshooting

## fb asks me to save the file instead of showing it

The file is a type fb and WebKit cannot display, so saving a copy is what remains. Check
[All file types](file-types/index.md) for what is supported. A supported file also falls
back to this when it cannot be read as its format, such as a damaged Word document or a
`.db` file that is not really a SQLite database.

## A large file shows as plain text or downloads

Most viewers have a size limit, past which fb shows or offers the original file. See
[Size limits](file-types/index.md#size-limits).

## A link to a line opens at the top of the file

fb cannot show lines for these files:

- It is a binary format, such as an image or a PDF, or it is not UTF-8 text.
- The link is inside the same document it points to. See
  [Limitations](linking.md#limitations).
- The link leaves out the `L`: `notes.md#42` looks for a heading rather than a line.

## Line numbers differ from my editor

fb numbers what it displays, and a few formats get reformatted first. See
[When the numbers differ from your editor](linking.md#when-the-numbers-differ-from-your-editor).

## A big source file is slow to open

Source files of up to 2 MiB open whole, and WebKit takes much longer to lay out a page as
its line count grows: a file of tens of thousands of lines can take many seconds. Files
over 2 MiB open as plain text instead, and a link to a line in one shows just the lines
around it. See [Long files](linking.md#long-files).

## A document looks wrong

fb converts documents with Pandoc, which reads the structure of a document more than its
appearance. Exact fonts, page layouts, headers, and footers from a Word file may not
carry over. To see the original, use **File > Save a Copy** and open the copy in its own
app.

Markdown YAML front matter displays as ordinary text. fb does not interpret it, because
it could otherwise inject content into the page.

## An audio or video file will not play

Playback depends on the formats WebKit can decode, and a file with an unusual codec may not
play even when its extension is supported. **File > Save a Copy** still gives you the file. See
the [audio](file-types/audio.md) and [video](file-types/video.md) pages.

## Git status does not appear

See [When annotations are missing](git.md#when-annotations-are-missing).

## A Typst file is not reformatted

fb only reformats Typst when it can find the separate `typstyle` program. See
[Typst](file-types/typst.md).

## A photo's map is blank

Maps come from OpenStreetMap, so they need a network connection. The photo and its other
details are unaffected.

## macOS asks for permission to access a folder

fb uses your own file permissions, and macOS asks before an app can first read protected
folders such as Desktop, Documents, and Downloads. You can change your answer in **System
Settings > Privacy & Security > Files and Folders**.

## The Dock shows a placeholder icon

A Dock shortcut left by an older copy of fb can point at the wrong place. Remove it from
the Dock, open fb, then Control-click its icon and choose **Options > Keep in Dock**.
This does not affect the app or your documents.

## Reporting a problem

Report problems on [GitHub](https://github.com/xoba/fb/issues). The version and source
revision are in **fb > About File Browser**.

## Related pages

- [Privacy and security](privacy-and-security.md)
- [Back to fb Help](index.md)
