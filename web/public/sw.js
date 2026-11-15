// Minimal service worker for PWA installability criteria
const CACHE_NAME = 'deploystackr-v1';

self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (event) => {
  // Pass-through fetch handler satisfying PWA offline requirement
  event.respondWith(
    fetch(event.request).catch(() => caches.match(event.request))
  );
});
