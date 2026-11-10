<%args>
title = "Example folder"
</%args>
% # Render an in-memory file catalog as a list.
% files = [("notes.md", 120), ("letter.eml", 240)]
<h1><% title %></h1>
<ul>
% for name, size in files:
  <li><% name %>: <% size %> bytes</li>
% # end for
</ul>
<p>Total: <% sum([size for name, size in files]) %> bytes</p>
