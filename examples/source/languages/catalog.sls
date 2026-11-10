# Install the catalog tool and publish a label for each catalog entry.
{% set files = {'notes.md': 120, 'letter.eml': 240} %}

catalog-pkg:
  pkg.installed:
    - name: catalog

{% for name, size in files.items() %}
/srv/catalog/{{ name }}:
  file.managed:
    - source: salt://catalog/files/{{ name }}
    - mode: '0644'
    - require:
      - pkg: catalog-pkg

/srv/catalog/{{ name }}.label:
  file.managed:
    - contents: "{{ name }}: {{ size }} bytes"
{% endfor %}
