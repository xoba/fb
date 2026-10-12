<script setup>
const files = ["notes.md", "letter.eml"];
</script>

<template>
  <section aria-label="Example files">
    <h1>Catalog</h1>
    <ul><li v-for="name in files" :key="name">{{ name }}</li></ul>
  </section>
</template>

<style scoped>
li { margin-block: 0.5rem; }
</style>
