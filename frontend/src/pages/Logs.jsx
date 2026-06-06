// src/pages/Logs.jsx
import { useState, useEffect, useCallback } from 'react';
import SearchBar from '../components/SearchBar';
import LogTable from '../components/LogTable';
import { apiFetch } from '../config/api';

export default function Logs() {
  const [logs, setLogs] = useState([]);
  const [services, setServices] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({ q: '', service: '', level: '', from: '', to: '' });
  const [offset, setOffset] = useState(0);
  const LIMIT = 50;

  // Fetch unique service names for the dropdown
  useEffect(() => {
    const fetchServices = async () => {
      try {
        const res = await apiFetch('/api/services');
        if (res.ok) {
          const data = await res.json();
          setServices(data || []);
        }
      } catch (err) {
        console.error("Failed to fetch services", err);
      }
    };
    fetchServices();
  }, []);

  // Fetch logs from Go Backend
  const fetchLogs = useCallback(async (currentFilters, currentOffset) => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (currentFilters.q) params.set('q', currentFilters.q);
      if (currentFilters.service) params.set('service', currentFilters.service);
      if (currentFilters.level) params.set('level', currentFilters.level);
      
      if (currentFilters.from) params.set('from', new Date(currentFilters.from).toISOString());
      if (currentFilters.to) params.set('to', new Date(currentFilters.to).toISOString());
      
      params.set('limit', LIMIT.toString());
      params.set('offset', currentOffset.toString());
      
      const res = await apiFetch(`/api/logs?${params.toString()}`);
      if (res.ok) {
        const data = await res.json();
        // If offset is 0, replace logs. If > 0, append to existing list.
        setLogs(currentOffset === 0 ? data.logs : (prev) => [...prev, ...data.logs]);
        setTotal(data.total);
      }
    } catch (err) {
      console.error("Failed to fetch logs", err);
    } finally {
      setLoading(false);
    }
  }, []);

  // Re-run fetch when filters change
  useEffect(() => {
    setOffset(0);
    fetchLogs(filters, 0);
  }, [filters, fetchLogs]);

  const handleSearch = (newFilters) => {
    setFilters(newFilters);
  };

  const loadMore = () => {
    const nextOffset = offset + LIMIT;
    setOffset(nextOffset);
    fetchLogs(filters, nextOffset);
  };

  return (
    <div style={{ padding: '2rem', maxWidth: '1600px', margin: '0 auto' }}>
      <h1 style={{ fontFamily: 'var(--font-mono)', marginBottom: '1.5rem', color: 'var(--text-primary)' }}>
        {'>_'} Log Explorer
      </h1>
      
      <SearchBar onSearch={handleSearch} services={services} />
      
      <LogTable logs={logs} loading={loading} />

      {/* Pagination Footer */}
      {logs.length > 0 && logs.length < total && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '1.5rem' }}>
          <div style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
            Showing {logs.length} of {total} logs
          </div>
          <button 
            onClick={loadMore} 
            disabled={loading}
            style={{
              background: 'var(--surface)',
              border: '1px solid var(--border-bright)',
              color: 'var(--text-primary)',
              padding: '0.5rem 1rem',
              borderRadius: 'var(--radius)',
              cursor: loading ? 'not-allowed' : 'pointer'
            }}>
            {loading ? 'Loading...' : 'Load More'}
          </button>
        </div>
      )}
    </div>
  );
}