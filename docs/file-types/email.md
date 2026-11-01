# Email

fb reads saved email messages, including their attachments, without an email account or any
change to the message.

**Extensions:** `.eml`

## What you see

A message opens like a folder with a README. A tree of its MIME parts comes first, followed by
the readable message.

The tree keeps the message's structure: multipart containers, plain-text and HTML
alternatives, inline images, attachments, and forwarded messages. Click a part to inspect it,
and use Back to return to the message.

Headers and the original source are available in disclosure sections, and **Save original
.eml** gets you the complete file.

### Attachments

Attachments open in the same viewers as other files: CSV files become queryable tables, PDFs
display, images get the image viewer, documents render, and archives open as folders. **Save
this part** saves a decoded attachment as a separate file.

### Inbox view

A folder containing `.eml` files offers an **Inbox** view with sender, subject, and date
columns and a search field. Each row opens the same message view, and **Files** returns to the
normal listing. Moving to another folder goes back to the normal listing.

### What is decoded and what is removed

fb decodes transfer encodings, encoded headers, and declared character sets.

HTML in message bodies is sanitized: scripts, forms, message styling, and remote images are
removed. Removing remote images means that opening a message cannot tell its sender you read
it. Images embedded in the message still display without any network request.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The original file |
| `?view=inbox` | On a folder: opens the Inbox view |
| `#L12` | Shows the numbered source of the message, with a **rendered** link back. See [Linking](../linking.md#links-to-lines) |

## Limits and fallbacks

| Limit | Value |
|-------|-------|
| Message size | 32 MiB |
| Levels of nested parts | 16 |
| Parts in a message | 256 |
| Preview of a body or the source | 2 MiB |
| Messages per Inbox page | 500, grouped by filename |
| Headers read for the Inbox | 64 KiB per message |

Sorting by date and searching apply to the current Inbox page. Unsupported or malformed content
reports a warning or falls back to the message source, and the original bytes always remain
available. Messages inside ZIP and TAR archives work the same way.

## Try it

- [mime-tour.eml](../../examples/mime-tour.eml), a message using most of these features
- [mime-tour-reply.eml](../../examples/mime-tour-reply.eml)
- [The two messages in an Inbox](../../examples/?view=inbox)

[All file types](index.md) · [fb Help](../index.md)
