// src/pages/Alerts.jsx
import { useState, useEffect, useCallback, useRef } from 'react';
import { Bell, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';
import { apiFetch } from '../config/api';
import SearchBar from '../components/SearchBar';

// --- Sleek Custom Dropdown ---
function CustomSelect({ value, onChange, options, placeholder }) {
  const [isOpen, setIsOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const handleClickOutside = (e) => {
      if (ref.current && !ref.current.contains(e.target)) setIsOpen(false);
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const selectedLabel = options.find(o => o.value === value)?.label || placeholder;

  return (
    <div ref={ref} style={{ position: 'relative', minWidth: '220px' }}>
      <div 
        onClick={() => setIsOpen(!isOpen)}
        style={{
          background: 'var(--surface)',
          border: `1px solid ${isOpen ? 'var(--green)' : 'var(--border-bright)'}`,
          borderRadius: 'var(--radius)',
          padding: '0.6rem 1rem',
          fontSize: '0.85rem',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          color: value ? 'var(--text-primary)' : 'var(--text-secondary)',
          transition: 'border-color 0.2s ease',
          fontFamily: 'var(--font-sans)'
        }}
      >
        <span style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', paddingRight: '1rem' }}>
          {selectedLabel}
        </span>
        <ChevronDown size={16} style={{ flexShrink: 0, transition: 'transform 0.2s ease', transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)' }} />
      </div>

      <div style={{
        position: 'absolute', top: '100%', left: 0, right: 0, marginTop: '0.25rem',
        background: 'var(--surface)', border: '1px solid var(--border-bright)',
        borderRadius: 'var(--radius)', zIndex: 50, overflow: 'hidden',
        opacity: isOpen ? 1 : 0, transform: isOpen ? 'translateY(0)' : 'translateY(-10px)',
        pointerEvents: isOpen ? 'auto' : 'none', transition: 'all 0.2s ease',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)'
      }}>
        {options.map((opt) => (
          <div
            key={opt.value}
            onClick={() => { onChange(opt.value); setIsOpen(false); }}
            style={{
              padding: '0.6rem 1rem', fontSize: '0.85rem', cursor: 'pointer',
              color: value === opt.value ? 'var(--green)' : 'var(--text-primary)',
              background: value === opt.value ? 'rgba(34, 197, 94, 0.1)' : 'transparent',
              transition: 'background 0.1s',
              fontFamily: 'var(--font-sans)'
            }}
            onMouseEnter={(e) => e.target.style.background = 'var(--surface-hover)'}
            onMouseLeave={(e) => e.target.style.background = value === opt.value ? 'rgba(34, 197, 94, 0.1)' : 'transparent'}
          >
            {opt.label}
          </div>
        ))}
      </div>
    </div>
  );
}

export default function Alerts() {
  const [alerts, setAlerts] = useState([]);
  const [total, setTotal] = useState(0);
  const [rules, setRules] = useState([]);
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searchTime, setSearchTime] = useState(null);
  
  // Active Filters & Pagination State
  const [selectedRuleId, setSelectedRuleId] = useState('');
  const [filters, setFilters] = useState({ q: '', service: '', level: '', from: '', to: '' });
  const [currentPage, setCurrentPage] = useState(1);
  const LIMIT = 50;

  // Fetch Rules and Services for the dropdowns
  useEffect(() => {
    apiFetch('/api/rules').then(res => res.json()).then(data => setRules(data || []));
    apiFetch('/api/services').then(res => res.json()).then(data => setServices(data || []));
  }, []);

  const fetchAlertHistory = useCallback(async (currentFilters, ruleId, page) => {
    setLoading(true);
    const t0 = performance.now();
    try {
      const params = new URLSearchParams();
      const offset = (page - 1) * LIMIT;
      
      params.set('limit', LIMIT.toString());
      params.set('offset', offset.toString());
      
      if (ruleId) params.set('rule_id', ruleId);
      if (currentFilters.q) params.set('q', currentFilters.q);
      if (currentFilters.service) params.set('service', currentFilters.service);
      if (currentFilters.level) params.set('level', currentFilters.level);
      if (currentFilters.from) params.set('from', new Date(currentFilters.from).toISOString());
      if (currentFilters.to) params.set('to', new Date(currentFilters.to).toISOString());
      
      const res = await apiFetch(`/api/alerts?${params.toString()}`);
      if (res.ok) {
        const data = await res.json();
        setAlerts(data?.alerts || []);
        setTotal(data?.total || 0);
        setSearchTime(Math.round(performance.now() - t0));
      }
    } catch (err) {
      console.error("Failed to fetch alert history:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  // --- SINGLE EFFECT FOR FETCHING ---
  useEffect(() => {
    fetchAlertHistory(filters, selectedRuleId, currentPage);
  }, [filters, selectedRuleId, currentPage, fetchAlertHistory]);

  // Auto-refresh ONLY if on Page 1 (don't shift data while reading history)
  useEffect(() => {
    if (currentPage !== 1) return;
    const interval = setInterval(() => {
      fetchAlertHistory(filters, selectedRuleId, 1);
    }, 15000);
    return () => clearInterval(interval);
  }, [filters, selectedRuleId, currentPage, fetchAlertHistory]);

  const handleSearch = (newFilters) => {
    setFilters(newFilters);
    setCurrentPage(1); // Reset to page 1 on new search
  };

  const handleRuleChange = (newRuleId) => {
    setSelectedRuleId(newRuleId);
    setCurrentPage(1); // Reset to page 1 on rule change
  };

  const ruleOptions = [
    { value: '', label: 'All Rules (Unfiltered)' },
    ...rules.map(r => ({ value: r.id, label: r.name }))
  ];

  const totalPages = Math.ceil(total / LIMIT);

  return (
    <div style={{ padding: '2rem', maxWidth: '1400px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '2rem' }}>
      
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h1 style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-primary)', margin: 0, display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
          <Bell size={24} color="var(--amber)" /> Alert History
        </h1>
        
        <CustomSelect 
          value={selectedRuleId} 
          onChange={handleRuleChange}
          options={ruleOptions}
          placeholder="All Rules (Unfiltered)"
        />
      </div>

      <SearchBar onSearch={handleSearch} services={services} searchTime={searchTime} total={total} />

      {/* Full Width History Table */}
      <div className="card" style={{ padding: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column', minHeight: '600px' }}>
        <div style={{ overflowX: 'auto', flex: 1 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)', position: 'sticky', top: 0, background: 'var(--surface)', zIndex: 10 }}>
                <th style={{ padding: '1rem 1.5rem', width: '150px' }}>Fired At</th>
                <th style={{ padding: '1rem 1.5rem', width: '200px' }}>Rule Triggered</th>
                <th style={{ padding: '1rem 1.5rem', width: '120px' }}>Level</th>
                <th style={{ padding: '1rem 1.5rem', width: '150px' }}>Service</th>
                <th style={{ padding: '1rem 1.5rem' }}>Raw Message</th>
              </tr>
            </thead>
            <tbody>
              {loading && alerts.length === 0 ? (
                <tr><td colSpan={5} style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-dim)' }}>Loading history...</td></tr>
              ) : alerts.length === 0 ? (
                <tr><td colSpan={5} style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-dim)' }}>No alerts match your current filters.</td></tr>
              ) : (
                alerts.map(alert => (
                  <tr key={alert.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', animation: 'fadeInUp 0.3s ease' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-secondary)', whiteSpace: 'nowrap' }}>
                      {formatDistanceToNow(new Date(alert.fired_at), { addSuffix: true })}
                    </td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-primary)', fontWeight: 600 }}>{alert.rule_name}</td>
                    <td style={{ padding: '1rem 1.5rem' }}><span className={`badge badge-${alert.level.toLowerCase()}`}>{alert.level}</span></td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-secondary)' }}>{alert.service}</td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-dim)' }}>{alert.message}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* --- TRUE PAGINATION FOOTER --- */}
      {total > 0 && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '0.5rem', background: 'var(--surface)', padding: '1rem 1.5rem', borderRadius: 'var(--radius)', border: '1px solid var(--border)' }}>
          <div style={{ color: 'var(--text-secondary)', fontSize: '0.85rem', fontFamily: 'var(--font-sans)' }}>
            Showing <strong>{((currentPage - 1) * LIMIT) + 1}</strong> to <strong>{Math.min(currentPage * LIMIT, total)}</strong> of <strong>{total.toLocaleString()}</strong> alerts
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