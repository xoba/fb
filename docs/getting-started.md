# Getting started

fb requires an Apple Silicon Mac running macOS 26 or later. Everything it needs,
including its document converter, is inside the app.

## Opening files and folders

Any of these opens a file or folder in fb:

- Choose **File > Open** (⌘O), or click the folder button in the toolbar.
- Drag items from Finder onto a fb window. The first item opens in that window, and any
  others open in their own windows.
- In Finder, choose **Open With > fb** from the Control-click menu.
- Open from the terminal, as described [below](#opening-from-the-terminal).

Opening or dragging a file only changes what you are viewing. The file stays where it
is: nothing is imported, moved, or uploaded.

## Windows

Each window has its own history, like a browser tab. **File > New fb Window** opens another
at your last location, and links opened in a new window keep any anchor they name.

fb reopens your most recent location the next time you start it. Help pages do not count
as a location, so reading them does not change where fb starts.

## Following links

Click a file or folder to open it in the same window. Use the toolbar's Back and Forward
buttons, ⌘\[ and ⌘\], or a two-finger swipe to move through your history.

Links to websites and email addresses open in your default browser and mail app. To see
where a link goes before you click it, hover over it: the destination appears at the
bottom of the window.

See [Navigating](navigating.md) for everything else in the toolbar and menus.

## Opening from the terminal

macOS's `open` command sends files and folders to the app:

```sh
open -b com.xoba.fb.app -- README.md
open -b com.xoba.fb.app -- .
open -b com.xoba.fb.app -- "My notes.md" /path/to/folder
```

Relative paths work, and several items open in separate windows. Quote paths that contain
spaces.

For a shorter command in zsh, add this function to `~/.zshrc`:

```sh
fb() {
  if [ "$#" -eq 0 ]; then
    set -- .
  fi
  /usr/bin/open -b com.xoba.fb.app -- "$@"
}
```

Then `fb README.md` opens a file, and `fb` alone opens the current folder. The app has no
public URL scheme: its `fb://` addresses are private to its own windows.

## Installing and updating

Install with [Homebrew](https://brew.sh), or download the app from
[GitHub Releases](https://github.com/xoba/fb/releases/latest) and move it to your
Applications folder. Homebrew is optional.

```sh
brew install --cask xoba/tap/fb
brew upgrade --cask xoba/tap/fb
```

## Next steps

- [Navigating](navigating.md)
- [All file types](file-types/index.md)
- [Back to fb Help](index.md)
