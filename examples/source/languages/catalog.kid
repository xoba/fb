<?xml version="1.0" encoding="utf-8"?>
<!-- Lists the entries of an in-memory file catalog. -->
<?python
files = [("notes.md", 120), ("letter.eml", 240)]
?>
<html xmlns="http://www.w3.org/1999/xhtml"
      xmlns:py="http://purl.org/kid/ns#">
  <head>
    <title>File catalog</title>
  </head>
  <body>
    <ul py:if="files">
      <li py:for="name, size in files">${name}: ${size} bytes</li>
    </ul>
    <p py:content="'%d files' % len(files)">No files</p>
  </body>
</html>
