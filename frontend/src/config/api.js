// src/config/api.js
//
// In production the frontend is served by nginx, which reverse-proxies /api and
// /ws to the Go server AND injects the X-API-Key header server-side. The browser
// bundle therefore ships NO API credentials (C4).
//
// For `vite dev` against a standalone server you can set VITE_API_URL / VITE_WS_URL
// (and optionally VITE_API_KEY) in a local .env — these are only used in that mode.
const DEV_API_KEY = import.meta.env.VITE_API_KEY || '';

export const API_BASE = import.meta.env.VITE_API_URL || '';

export const WS_BASE =
  import.meta.env.VITE_WS_URL ||
  `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`;

export const apiFetch = (path, opts = {}) => {
  const isPostOrPut = opts.method === 'POST' || opts.method === 'PUT';
  const headers = { ...opts.headers };

  // Only attach a key when explicitly configured for local dev. In production
  // nginx adds it and the browser never sees the real value.
  if (DEV_API_KEY) {
    headers['X-API-Key'] = DEV_API_KEY;
  }

  if (isPostOrPut) {
    headers['Content-Type'] = 'application/json';
  }

  return fetch(`${API_BASE}${path}`, {
    ...opts,
    headers,
  });
};
