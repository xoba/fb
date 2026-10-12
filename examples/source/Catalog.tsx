type Entry = { name: string; bytes: number };

const files: Entry[] = [
  { name: "notes.md", bytes: 120 },
  { name: "letter.eml", bytes: 240 },
];

export default function Catalog() {
  return <ul>{files.map(file => (
    <li key={file.name}>{file.name}: {file.bytes} bytes</li>
  ))}</ul>;
}
