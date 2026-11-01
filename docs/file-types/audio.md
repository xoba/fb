# Audio

fb shows audio in an inline player, with the file's tags and cover art.

**Extensions:** `.mp3` `.m4a` `.aac` `.flac` `.ogg` `.wav`

## What you see

A player, with the file size, format, and modification time beneath it. Any tags in the file appear
too: title, artist, album, year, genre, track, and composer. Embedded cover art displays, and the
duration fills in once WebKit has loaded the file.

Going Back to a player page keeps your place.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The audio file itself |

Audio files are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Playback depends on the codecs WebKit supports. If a file will not play, its details still appear
and **File > Save a Copy…** still gives you the file.

Cover art over 2 MiB is not displayed, and tags are not read from audio files over 30 MiB stored
inside an archive.

## Try it

- [quiet-tone.mp3](../../examples/formats/quiet-tone.mp3)
- [quiet-tone.m4a](../../examples/formats/quiet-tone.m4a)
- [quiet-tone.aac](../../examples/formats/quiet-tone.aac)
- [quiet-tone.flac](../../examples/formats/quiet-tone.flac)
- [quiet-tone.ogg](../../examples/formats/quiet-tone.ogg)
- [quiet-tone.wav](../../examples/formats/quiet-tone.wav)

[All file types](index.md) · [fb Help](../index.md)
