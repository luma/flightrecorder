import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "./auth/AuthContext";
import { ProjectScopeProvider } from "./hooks/useProjectScope";
import App from "./App";
import "./index.css";

const queryClient = new QueryClient();

function removeLegacyServiceWorker() {
  if (!("serviceWorker" in navigator)) {
    return;
  }

  const cleanupKey = "flightrecorder:legacy-service-worker-cleanup-v1";
  try {
    if (window.localStorage.getItem(cleanupKey) === "done") {
      return;
    }
  } catch (_error) {
    // Continue without persistence; cleanup is still useful for this page load.
  }

  // Flightrecorder no longer uses a service worker. This migration removes the
  // previous Workbox app shell so stale deploy-era caches cannot control loads.
  const unregisterWorkers = navigator.serviceWorker
    .getRegistrations()
    .then((registrations) =>
      Promise.all(registrations.map((registration) => registration.unregister())),
    );

  const clearCaches =
    "caches" in window
      ? window.caches
          .keys()
          .then((keys) => Promise.all(keys.map((key) => window.caches.delete(key))))
      : Promise.resolve();

  Promise.all([unregisterWorkers, clearCaches])
    .then(() => {
      try {
        window.localStorage.setItem(cleanupKey, "done");
      } catch (_error) {
        // Persistence is best-effort; rendering must not depend on storage APIs.
      }
    })
    .catch(() => {
      // Cleanup is best-effort; rendering must not depend on service worker APIs.
    });
}

removeLegacyServiceWorker();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ProjectScopeProvider>
        <BrowserRouter>
          <AuthProvider>
            <App />
          </AuthProvider>
        </BrowserRouter>
      </ProjectScopeProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
