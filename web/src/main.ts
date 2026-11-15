import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

const target = document.getElementById('app')
if (!target) throw new Error('Target element #app not found')

const app = mount(App, {
  target,
})

// Register service worker for PWA installability
if ('serviceWorker' in navigator && !import.meta.env.DEV) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.warn('Service worker registration failed:', err);
    });
  });
} else if ('serviceWorker' in navigator) {
  // Also register in dev mode if sw.js is available
  navigator.serviceWorker.register('/sw.js').catch(() => {});
}

export default app
