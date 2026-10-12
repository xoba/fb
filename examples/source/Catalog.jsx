export default function Catalog() {
  const files = ["notes.md", "letter.eml"];
  return (
    <section aria-label="Example files">
      <h1>Catalog</h1>
      <ul>{files.map(name => <li key={name}>{name}</li>)}</ul>
    </section>
  );
}
