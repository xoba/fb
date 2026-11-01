# Video

fb shows video in an inline player scaled to the window.

**Extensions:** `.mov` `.mp4` `.m4v` `.webm`

## What you see

A player that fits the window whatever the size of the movie. Beneath it are the file size, format,
and modification time. Duration and dimensions fill in once WebKit has loaded the movie's details.

Going Back to a player page keeps your place in the video.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The video file itself |

Video files are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Playback depends on the codecs WebKit supports. If a file will not play, **File > Save a Copy…**
still gives you the file.

## Try it

- [test-pattern.mp4](../../examples/formats/test-pattern.mp4)
- [test-pattern.mov](../../examples/formats/test-pattern.mov)
- [test-pattern.m4v](../../examples/formats/test-pattern.m4v)
- [test-pattern.webm](../../examples/formats/test-pattern.webm)

[All file types](index.md) · [fb Help](../index.md)
