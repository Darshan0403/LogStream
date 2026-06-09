// src/pages/Logs.jsx
import { useState, useEffect, useCallback } from 'react';
import SearchBar from '../components/SearchBar';
import LogTable from '../components/LogTable';
import { apiFetch } from '../config/api';
import { ChevronLeft, ChevronRight } from 'lucide-react';

export default function Logs() {
  const [logs, setLogs] = useState([]);
  const [services, setServices] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [searchTime, setSearchTime] = useState(null); // ms
  const [filters, setFilters] = useState({ q: '', service: '', level: '', from: '', to: '' });
  
  // --- PAGINATION STATE ---
  const [currentPage, setCurrentPage] = useState(1);
  const LIMIT = 50;

  useEffect(() => {
    const fetchServices = async () => {
      try {
        const res = await apiFetch('/api/services');
        if (res.ok) setServices(await res.json() || []);
      } catch (err) {
        console.error("Failed to fetch services", err);
      }
    };
    fetchServices();
  }, []);

  // Fetch logs based on current page
  const fetchLogs = useCallback(async (currentFilters, page) => {
    setLoading(true);
    const t0 = performance.now();
    try {
      const params = new URLSearchParams();
      if (currentFilters.q) params.set('q', currentFilters.q);
      if (currentFilters.service) params.set('service', currentFilters.service);
      if (currentFilters.level) params.set('level', currentFilters.level);
      if (currentFilters.from) params.set('from', new Date(currentFilters.from).toISOString());
      if (currentFilters.to) params.set('to', new Date(currentFilters.to).toISOString());
      
      const offset = (page - 1) * LIMIT;
      params.set('limit', LIMIT.toString());
      params.set('offset', offset.toString());
      
      const res = await apiFetch(`/api/logs?${params.toString()}`);
      if (res.ok) {
        const data = await res.json();
        setLogs(data.logs || []);
        setTotal(data.total || 0);
        setSearchTime(Math.round(performance.now() - t0));
      }
    } catch (err) {
      console.error("Failed to fetch logs", err);
    } finally {
      setLoading(false);
    }
  }, []);

  // --- ONE SINGLE EFFECT TO RULE THEM ALL ---
  // Whenever filters or currentPage change, just fetch the logs.
  useEffect(() => {
    fetchLogs(filters, currentPage);
  }, [filters, currentPage, fetchLogs]);

  // When a user types a new search or changes a dropdown, 
  // update the filters AND force the page back to 1.
  const handleSearch = (newFilters) => {
    setFilters(newFilters);
    setCurrentPage(1); 
  };

  const totalPages = Math.ceil(total / LIMIT);

  return (
    <div style={{ padding: '2rem', maxWidth: '1600px', margin: '0 auto' }}>
      <h1 style={{ fontFamily: 'var(--font-mono)', marginBottom: '1.5rem', color: 'var(--text-primary)' }}>
        {'>_'} Log Explorer
      </h1>
      
      <SearchBar onSearch={handleSearch} services={services} searchTime={searchTime} total={total} />
      
      <LogTable logs={logs} loading={loading} />

      {/* --- TRUE PAGINATION FOOTER --- */}
      {total > 0 && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '1.5rem', background: 'var(--surface)', padding: '1rem 1.5rem', borderRadius: 'var(--radius)', border: '1px solid var(--border)' }}>
          <div style={{ color: 'var(--text-secondary)', fontSize: '0.85rem', fontFamily: 'var(--font-sans)' }}>
            Showing <strong>{((currentPage - 1) * LIMIT) + 1}</strong> to <strong>{Math.min(currentPage * LIMIT, total)}</strong> of <strong>{total.toLocaleString()}</strong> logs
          </div>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
            <button 
              onClick={() => setCurrentPage(p => Math.max(1, p - 1))} 
              disabled={currentPage === 1 || loading}
              style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', background: 'transparent', border: '1px solid var(--border-bright)', color: currentPage === 1 ? 'var(--text-dim)' : 'var(--text-primary)', padding: '0.4rem 0.75rem', borderRadius: 'var(--radius)', cursor: currentPage === 1 || loading ? 'not-allowed' : 'pointer' }}
            >
              <ChevronLeft size={16} /> Prev
            </button>
            
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
              Page <span style={{ color: 'var(--text-primary)' }}>{currentPage}</span> of {totalPages || 1}
            </span>

            <button 
              onClick={() => setCurrentPage(p => Math.min(totalPages, p + 1))} 
              disabled={currentPage === totalPages || loading}
              style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', background: 'transparent', border: '1px solid var(--border-bright)', color: currentPage === totalPages ? 'var(--text-dim)' : 'var(--text-primary)', padding: '0.4rem 0.75rem', borderRadius: 'var(--radius)', cursor: currentPage === totalPages || loading ? 'not-allowed' : 'pointer' }}
            >
              Next <ChevronRight size={16} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}