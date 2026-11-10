## Lists the entries of an in-memory file catalog.
<%!
    files = [("notes.md", 120), ("letter.eml", 240)]
%>
<%def name="label(name, size)">${name}: ${size} bytes</%def>
<ul>
% for name, size in files:
    <li>${label(name, size)}</li>
% endfor
</ul>
<p>${len(files)} files, ${sum(size for _, size in files)} bytes</p>
