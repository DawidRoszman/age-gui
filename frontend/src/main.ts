import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'

// Svelte 5 mounts through mount() rather than `new App()`.
const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
