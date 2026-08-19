// src/config/api.js
export const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8090';
export const WS_BASE  = import.meta.env.VITE_WS_URL  || 'ws://localhost:8090';
export const API_KEY  = import.meta.env.VITE_API_KEY || 'dev-key';

export const apiFetch = (path, opts = {}) => {
  const isPostOrPut = opts.method === 'POST' || opts.method === 'PUT';
  const headers = {
    'X-API-Key': API_KEY,
    ...opts.headers,
  };

  if (isPostOrPut) {
    headers['Content-Type'] = 'application/json';
  }

  return fetch(`${API_BASE}${path}`, {
    ...opts,
    headers,
  });
};