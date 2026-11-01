# Images

fb shows an image scaled to the window, with its technical details beneath it.

**Extensions:** `.jpg` `.jpeg` `.png` `.gif` `.webp` `.bmp` `.tif` `.tiff` `.heic` `.heif`

## What you see

The image scaled up or down to fit the window with its proportions kept, whatever its size. Beneath
it is a readout of file size, dimensions and megapixels, the type from the file's actual content,
and its modification time.

Photos also show their full EXIF details, with photography fields written naturally: `f/7.1`,
`85 mm`, `1/200 s`.

### Maps

A photo with GPS coordinates gets a map below the details with a dot at its location, along with
links to OpenStreetMap and Apple Maps. **This is the one feature that needs a connection:** the map
is built from OpenStreetMap's tiles, which are requested when you view the photo. See
[Privacy and security](../privacy-and-security.md).

### HEIC and TIFF

WebKit cannot display every HEIC and TIFF file, so fb converts them to JPEG with macOS's built-in
`sips`. The EXIF details and map survive this, and the conversion is cached for each file.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The image file itself |

Images are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Images referenced from a document, such as `<img>` tags in Markdown, get the image file itself, so
they keep working. If a file cannot be converted, fb falls back to the original.

## Try it

- [color-grid.png](../../examples/formats/color-grid.png)
- [color-grid.jpg](../../examples/formats/color-grid.jpg)
- [color-grid.jpeg](../../examples/formats/color-grid.jpeg)
- [color-grid.gif](../../examples/formats/color-grid.gif)
- [color-grid.webp](../../examples/formats/color-grid.webp)
- [color-grid.bmp](../../examples/formats/color-grid.bmp)
- [color-grid.tif](../../examples/formats/color-grid.tif)
- [color-grid.tiff](../../examples/formats/color-grid.tiff)
- [color-grid.heic](../../examples/formats/color-grid.heic)
- [color-grid.heif](../../examples/formats/color-grid.heif)

[All file types](index.md) · [fb Help](../index.md)
