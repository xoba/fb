const files = ["notes.md", "letter.eml"];

function catalogLabel(name) {
  return `Preview: ${name}`;
}

console.log(files.map(catalogLabel).join("\n"));
