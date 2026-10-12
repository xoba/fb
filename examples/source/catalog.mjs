const files = [
  { name: "notes.md", bytes: 120 },
  { name: "letter.eml", bytes: 240 },
];

export const totalBytes = files.reduce((sum, file) => sum + file.bytes, 0);
console.log(`Catalog: ${files.length} files, ${totalBytes} bytes`);
