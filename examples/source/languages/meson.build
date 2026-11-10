# Build the catalog example that prints file labels.
project('catalog', 'c',
  version : '0.1.0',
  default_options : ['warning_level=3', 'c_std=c11'])

sizes = {'notes.md' : 120, 'letter.eml' : 240}
add_project_arguments('-DCATALOG_ENTRIES=@0@'.format(sizes.keys().length()), language : 'c')

exe = executable('catalog', 'catalog.c', install : true)
test('labels', exe)
