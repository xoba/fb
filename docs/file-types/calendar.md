# Calendars

fb shows calendar files as readable event cards, rather than handing them to Calendar.

**Extensions:** `.ics`

## What you see

This covers meeting invitations, cancellations, replies, exported calendars, and subscribed
feeds. Each event gets a card showing:

- **When,** in your local time, with the event's own time zone alongside when it differs. A
  Windows-style zone name macOS does not know, such as `Eastern Standard Time`, shows the time
  as written.
- **How it repeats,** in words: "every 2 weeks on Tuesday and Thursday, 10 times", with any
  exceptions listed.
- **Where,** with URLs linked.
- **Who:** the organizer, and each attendee with their response.
- The description, links, and reminders.

Badges show the status (tentative, cancelled, private) and what kind of file it is (invitation,
cancellation, reply). Every property of the event follows in a collapsed table, and the file's
source in a collapsed section at the end, so nothing in the file is out of reach.

A file with more than ten events starts collapsed, in date order, as an agenda. To-dos, journal
entries, and free or busy blocks get cards of their own.

fb reads calendars with a tolerant parser: what it cannot read, it skips rather than failing.

## Options

| Option | Effect |
|--------|--------|
| `#L12` | Opens the Source section and scrolls to that line. See [Linking](../linking.md#links-to-lines) |
| `?raw=1` | The original file, as does the **raw** link |

## Limits and fallbacks

Files over 2 MiB display as plain text. A file with no calendar in it shows as written, with
line numbers.

## Try it

[sample.ics](../../examples/documents/sample.ics)

[All file types](index.md) · [fb Help](../index.md)
