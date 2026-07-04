(function () {
  if (!("serviceWorker" in navigator)) {
    return;
  }

  window.addEventListener("load", function () {
    var reloadKey = "flightrecorder:legacy-service-worker-cleanup";
    var shouldReload = false;

    try {
      if (window.sessionStorage.getItem(reloadKey) === "done") {
        return;
      }
      window.sessionStorage.setItem(reloadKey, "done");
      shouldReload = true;
    } catch (_error) {
      shouldReload = false;
    }

    var unregisterWorkers = navigator.serviceWorker
      .getRegistrations()
      .then(function (registrations) {
        return Promise.all(
          registrations.map(function (registration) {
            return registration.unregister();
          }),
        );
      });

    var clearCaches =
      "caches" in window
        ? window.caches.keys().then(function (keys) {
            return Promise.all(
              keys.map(function (key) {
                return window.caches.delete(key);
              }),
            );
          })
        : Promise.resolve();

    Promise.allSettled([unregisterWorkers, clearCaches]).then(function () {
      if (shouldReload) {
        window.location.reload();
      }
    });
  });
})();
