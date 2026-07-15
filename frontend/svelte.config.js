import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

export default {
  // vitePreprocess handles <script lang="ts">. Svelte 5 ships it with the Vite
  // plugin, so the separate svelte-preprocess dependency is no longer needed.
  preprocess: vitePreprocess(),
}
