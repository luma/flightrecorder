self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(clearLegacyCaches());
});

self.addEventListener("activate", (event) => {
  event.waitUntil(removeServiceWorkerAndReloadClients());
});

async function removeServiceWorkerAndReloadClients() {
  await clearLegacyCaches();
  await self.clients.claim();
  await self.registration.unregister();

  const clients = await self.clients.matchAll({
    includeUncontrolled: true,
    type: "window",
  });

  await Promise.all(
    clients.map((client) => {
      if ("navigate" in client) {
        return client.navigate(client.url);
      }
      return undefined;
    }),
  );
}

async function clearLegacyCaches() {
  const keys = await self.caches.keys();
  await Promise.all(keys.map((key) => self.caches.delete(key)));
}
