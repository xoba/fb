package main

// Directory and email pages share the same compact path navigation. The root
// crumb already supplies the first slash; later components need separators.
const browserBreadcrumbsDefine = `{{define "breadcrumbs"}}{{range $i, $c := .}}{{if gt $i 1}}<span class="sep">/</span>{{end}}{{if $c.Href}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{else}}<span class="file" aria-current="page">{{$c.Name}}</span>{{end}}{{end}}{{end}}`

const browserBreadcrumbsCSS = `
  nav.breadcrumbs { margin-bottom: 1rem; font-size: 1.1rem; line-height: normal; overflow-wrap: anywhere; }
  nav.breadcrumbs a, nav.breadcrumbs .file { font-weight: 600; }
  nav.breadcrumbs a.raw { font-weight: 400; }
  nav.breadcrumbs .sep { color: #57606a; }
`
