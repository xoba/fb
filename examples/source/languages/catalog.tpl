{* Catalog listing: one row per file, with its size. *}
<ul class="catalog">
{foreach $files as $file}
  <li>{$file.name|escape}: {$file.bytes} bytes{if $file.bytes > 200} (large){/if}</li>
{foreachelse}
  <li>No files.</li>
{/foreach}
</ul>
<p>Total: {$total} bytes</p>
