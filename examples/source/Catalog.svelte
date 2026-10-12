<script>
  const files = ["notes.md", "letter.eml"];
</script>

<h1>Example files</h1>
<ul>
  {#each files as name}
    <li>{name}</li>
  {/each}
</ul>

<style>
  li { margin-block: 0.5rem; }
</style>
