// src/config/api.js
export const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8090';
export const WS_BASE  = import.meta.env.VITE_WS_URL  || 'ws://localhost:8090';
export const API_KEY  = import.meta.env.VITE_API_KEY || 'dev-key';

export const apiFetch = (path, opts = {}) =>
  fetch(`${API_BASE}${path}`, {
    ...opts,
    headers: {
      'X-API-Key': API_KEY,
      'Content-Type': 'application/json',
      ...opts.headers,
    },
  });