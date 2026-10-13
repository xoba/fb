#!/usr/bin/env python3
"""Generate disposable, nonprivate documents for the WebKit integration check."""
import base64
import json
from pathlib import Path
import sqlite3
import sys
import wave
import zipfile

root = Path(sys.argv[1])
root.mkdir(parents=True, exist_ok=True)
(root / "nested").mkdir(exist_ok=True)
(root / "sample.md").write_text(
    "# Embedded engine\n\nOriginal file, rendered with bundled Pandoc.\n\n"
    "[Relative link](nested/next.md)\n\n![Pixel](pixel.png)\n\n"
    "[**Hover local**](spaced%20%26%20caf%C3%A9%20%231.md?raw=1#somewhere)\n\n"
    "[External reference](https://example.com/reference?from=fb#details)\n\n"
    "$$x^2 + y^2 = z^2$$\n"
)
(root / "print-mark.svg").write_text(
    '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="60" viewBox="0 0 240 60">'
    '<rect width="240" height="60" rx="8" fill="#1268be"/>'
    '<circle cx="30" cy="30" r="16" fill="#fff"/>'
    '<path d="M65 20h150M65 40h105" stroke="#fff" stroke-width="6"/>'
    '</svg>'
)
print_sections = [
    "# Printable Markdown\n\n**Print start marker.** This document checks formatted printing.\n\n"
    "![A blue document illustration](print-mark.svg)\n\n"
    "| Region | Amount |\n| :--- | ---: |\n| North | 120 |\n| South | 240 |\n\n"
    "```python\nprint(\"formatted code\")\n```\n\n"
    "$$x^2 + y^2 = z^2$$\n"
]
for section in range(1, 25):
    print_sections.append(
        f"## Section {section}\n\n"
        "A printed document should include the complete rendered content, including paragraphs "
        "that begin below the visible window. Headings, tables, illustrations, and code remain "
        "readable across page boundaries. This paragraph provides enough content to exercise "
        "normal pagination without requiring any remote assets or changing the original file.\n"
    )
print_sections.append(
    "| Tail region | Amount |\n| :--- | ---: |\n| Final row | 360 |\n\n"
    "```python\nprint(\"final formatted code\")\n```\n\n"
    "## Print end marker\n\nThe complete document reaches the final printed page.\n"
)
(root / "print.md").write_text("\n".join(print_sections))
(root / "nested/next.md").write_text("# Relative navigation works\n")
(root / "spaced & café #1.md").write_text("# Encoded filename works\n")
(root / "100%.md").write_text("# Percent filename works\n")
(root / "example.go").write_text("package main\n\nfunc main() {}\n")
(root / "sample.csv").write_text("name,amount\nalpha,10\nbeta,20\n")
(root / "pixel.png").write_bytes(base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j53sAAAAASUVORK5CYII="
))
# A minimal valid PDF, without depending on a PDF generator installation.
objects = [
    b"<< /Type /Catalog /Pages 2 0 R >>",
    b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
    b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
]
content = b"BT /F1 20 Tf 30 250 Td (Embedded PDF) Tj ET"
objects.append(b"<< /Length " + str(len(content)).encode() + b" >>\nstream\n" + content + b"\nendstream")
pdf = bytearray(b"%PDF-1.4\n")
offsets = [0]
for i, obj in enumerate(objects, 1):
    offsets.append(len(pdf))
    pdf.extend(f"{i} 0 obj\n".encode() + obj + b"\nendobj\n")
xref = len(pdf)
pdf.extend(f"xref\n0 {len(offsets)}\n0000000000 65535 f \n".encode())
for offset in offsets[1:]:
    pdf.extend(f"{offset:010d} 00000 n \n".encode())
pdf.extend(f"trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode())
(root / "sample.pdf").write_bytes(pdf)


# Reuse the same document bytes inside the MIME message and on disk.
(root / "note.md").write_text("# Attached document\n\nMarkdown inside an email.\n")
with zipfile.ZipFile(root / "bundle.zip", "w") as archive:
    archive.writestr("inside.md", "# Attached archive document\n")

# Nested related/alternative parts make the structure meaningful to browse,
# while the unsafe HTML verifies that previewing local mail stays inert.
email = "\r\n".join([
    "From: Example Sender <sender@example.com>",
    "To: Example Reader <reader@example.com>",
    "Subject: =?UTF-8?B?Q2Fmw6kgTUlNRSBicm93c2Vy?=",
    "Date: Mon, 12 Oct 2026 10:00:00 -0400",
    "MIME-Version: 1.0",
    'Content-Type: multipart/mixed; boundary="fb-mixed"',
    "", "--fb-mixed",
    'Content-Type: multipart/related; boundary="fb-related"',
    "", "--fb-related",
    'Content-Type: multipart/alternative; boundary="fb-alternative"',
    "", "--fb-alternative",
    "Content-Type: text/plain; charset=UTF-8",
    "Content-Transfer-Encoding: quoted-printable",
    "", "Plain MIME alternative: caf=C3=A9.",
    "--fb-alternative",
    "Content-Type: text/html; charset=UTF-8",
    "Content-Transfer-Encoding: quoted-printable",
    "",
    '<p>Readable <strong>MIME email body</strong>: caf=C3=A9.</p>',
    '<img src=3D"cid:pixel@example.com" alt=3D"Embedded pixel">',
    '<img src=3D"https://example.com/tracker.png" alt=3D"Remote tracker">',
    '<a href=3D"https://example.com/message">External message link</a>',
    '<a href=3D"file:///etc/passwd">Unsafe local link</a>',
    '<img src=3D"/etc/passwd" onerror=3D"window.fbUnsafeEmailExecuted=3Dtrue">',
    '<script>window.fbUnsafeEmailExecuted=3Dtrue</script>',
    '<style>body { display: none }</style>',
    "--fb-alternative--",
    "--fb-related",
    "Content-Type: image/png",
    "Content-ID: <pixel@example.com>",
    'Content-Disposition: inline; filename="pixel.png"',
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode((root / "pixel.png").read_bytes()).decode("ascii"),
    "--fb-related--",
    "--fb-mixed",
    "Content-Type: application/octet-stream",
    "Content-Disposition: attachment; filename*=UTF-8''caf%C3%A9.bin",
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode(bytes([0, 1, 2, 127, 128, 255])).decode("ascii"),
    "--fb-mixed",
    'Content-Type: text/csv; charset=UTF-8',
    'Content-Disposition: attachment; filename="sample.csv"',
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode((root / "sample.csv").read_bytes()).decode("ascii"),
    "--fb-mixed",
    "Content-Type: application/pdf",
    'Content-Disposition: attachment; filename="sample.pdf"',
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode((root / "sample.pdf").read_bytes()).decode("ascii"),
    "--fb-mixed",
    "Content-Type: text/markdown; charset=UTF-8",
    'Content-Disposition: attachment; filename="note.md"',
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode((root / "note.md").read_bytes()).decode("ascii"),
    "--fb-mixed",
    "Content-Type: application/zip",
    'Content-Disposition: attachment; filename="bundle.zip"',
    "Content-Transfer-Encoding: base64", "",
    base64.b64encode((root / "bundle.zip").read_bytes()).decode("ascii"),
    "--fb-mixed--", "",
])
(root / "message.eml").write_bytes(email.encode("ascii"))
(root / "raw.html").write_text(
    "<!doctype html><title>Sandbox check</title><p>Raw HTML still displays</p>"
    '<a id="external" href="https://example.com/plain-html#details">External reference</a>'
    "<script>window.fbUnsafeScriptExecuted = true</script>"
)
(root / "payload.txt~").write_text(
    "<!doctype html><p>Backup content stays plain text</p>"
    "<script>window.fbUnsafeScriptExecuted = true</script>"
)
db = sqlite3.connect(root / "sample.db")
db.execute("create table if not exists records (name text, amount integer)")
db.execute("delete from records")
db.executemany("insert into records values (?, ?)", [("alpha", 10), ("beta", 20)])
db.commit()
db.close()
with zipfile.ZipFile(root / "sample.zip", "w") as archive:
    archive.writestr("inside.md", "# Archive rendering works\n")
    archive.writestr("message.eml", email.encode("ascii"))
with wave.open(str(root / "sample.wav"), "wb") as audio:
    audio.setnchannels(1)
    audio.setsampwidth(2)
    audio.setframerate(8000)
    audio.writeframes(b"\0" * 16000)


(root / "mailbox/emails").mkdir(parents=True, exist_ok=True)
(root / "mailbox/emails/message.eml").write_bytes((root / "message.eml").read_bytes())
(root / "mailbox/notes.txt").write_text("Normal folder contents remain visible.\n")

print(json.dumps({"fixtures": str(root), "files": len(list(root.rglob("*")))}))
