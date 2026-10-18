<script setup>const renderedAt=useState('renderedAt',()=>new Date().toISOString())</script>
<template><main><h1>Actual Nuxt SSR on My Hosting</h1><p>Rendered on the Windows server at {{ renderedAt }}</p></main></template>
