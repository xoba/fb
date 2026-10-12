interface Entry {
  name: string;
  bytes: number;
}

const files: Entry[] = [
  { name: "notes.md", bytes: 120 },
  { name: "letter.eml", bytes: 240 },
];

const totalBytes: number = files.reduce((sum, file) => sum + file.bytes, 0);
console.log(`Catalog: ${files.length} files, ${totalBytes} bytes`);
