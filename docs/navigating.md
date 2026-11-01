# Navigating

Every window shows one page at a time, with a toolbar across the top and the usual
menus. The menu commands act on the frontmost window.

## The toolbar

| Control | What it does |
|---------|--------------|
| Back and Forward | Move through this window's history |
| Folder | Opens files or folders |
| Path | Shows where you are. You can select and copy it |
| Reload | Reads the file again and redraws the page |
| Reveal in Finder | Shows the current file in a Finder window |

fb always reads your current files, so **Reload** picks up any changes you have made
since the page opened. Going Back also shows a fresh copy of the page and returns you to
your place on it.

## Finding text

Choose **Edit > Find…** (⌘F) to search the current page. The search field replaces the
path in the toolbar. Press Return or ⌘G for the next match, ⇧⌘G for the previous one, and
Esc to close it. Matches ignore case and wrap around at the end of the page.

An icon beside the field tells you when there are no matches. To search the help pages
themselves, use the search field in the **Help** menu.

## Zooming

**View > Zoom In** and **Zoom Out** change the size of the current window's page, from
50% to 300%. **Actual Size** (⌘0) returns to 100%.

fb remembers the last level you chose for new windows and future launches. Windows that
are already open keep their own zoom until you change it there.

## Printing

Choose **File > Print…** (⌘P) to print the rendered page, including content below the
visible part of the window. Markdown keeps its headings, tables, images, and math, and
long documents flow onto as many pages as they need. To make a PDF, choose **Save as
PDF** in the print dialog.

## Saving a copy

**File > Save a Copy…** (⇧⌘S) saves the original file to a location you choose. You get
the original bytes rather than the rendered page, so saving a Markdown file gives you the
Markdown. It can replace an existing file at the destination, after the usual macOS
confirmation.

This is the only command in fb that writes a file you asked for. See
[Privacy and security](privacy-and-security.md).

## Rendered and raw views

fb renders files for reading, but the original is always one click away: rendered pages
have a **raw** link in their top-right corner. Adding `?raw=1` to a link does the same
thing, which is useful when you want one document to link to another's source.

When fb cannot display a file at all, it offers to save a copy instead.

## Previewing links

Hover over a link to see its destination at the bottom of the window, including any
`#anchor`. Local destinations show as paths relative to the current page when that is
shorter, and links within the same page show just the `#anchor`.

## Related pages

- [Keyboard shortcuts](keyboard-shortcuts.md)
- [Linking to files and lines](linking.md)
- [Back to fb Help](index.md)
