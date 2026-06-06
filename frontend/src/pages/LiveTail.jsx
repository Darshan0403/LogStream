// src/pages/LiveTail.jsx
import { useState, useEffect, useRef, useCallback } from 'react';
import { Play, Pause, Activity, ArrowUp, ChevronDown } from 'lucide-react';
import { format } from 'date-fns';
import { WS_BASE, API_KEY, apiFetch } from '../config/api';

// --- Reusable Sleek Dropdown ---
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
    <div ref={ref} style={{ position: 'relative', minWidth: '160px' }}>
      <div 
        onClick={() => setIsOpen(!isOpen)}
        style={{
          background: 'var(--bg)',
          border: `1px solid ${isOpen ? 'var(--green)' : 'var(--border-bright)'}`,
          borderRadius: 'var(--radius)',
          padding: '0.6rem 1rem',
          fontSize: '0.9rem',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          color: value ? 'var(--text-primary)' : 'var(--text-secondary)',
          transition: 'border-color 0.2s ease'
        }}
      >
        {selectedLabel}
        <ChevronDown size={16} style={{ transition: 'transform 0.2s ease', transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)' }} />
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
              padding: '0.6rem 1rem', fontSize: '0.9rem', cursor: 'pointer',
              color: value === opt.value ? 'var(--green)' : 'var(--text-primary)',
              background: value === opt.value ? 'rgba(34, 197, 94, 0.1)' : 'transparent',
              transition: 'background 0.1s'
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

// --- Main LiveTail Component ---
export default function LiveTail() {
  const [logs, setLogs] = useState([]);
  const [isLive, setIsLive] = useState(true);
  const [status, setStatus] = useState('connecting'); // connecting, live, paused, error
  const [service, setService] = useState('');
  const [level, setLevel] = useState('');
  const [services, setServices] = useState([]);
  
  const [isScrolled, setIsScrolled] = useState(false);
  const [missedLogs, setMissedLogs] = useState(0);
  
  const wsRef = useRef(null);
  const containerRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);

  // 1. Fetch available services for the filter dropdown
  useEffect(() => {
    apiFetch('/api/services')
      .then(res => res.json())
      .then(data => setServices(data || []))
      .catch(console.error);
  }, []);

  // 2. Bulletproof WebSocket Connection Manager
  const connectWS = useCallback(() => {
    // Clear any pending reconnects to prevent pile-ups
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }

    // FIX 1: Pass 1000 to cleanly close old connections
    if (wsRef.current) {
      wsRef.current.close(1000);
    }

    if (!isLive) {
      setStatus('paused');
      return;
    }

    setStatus('connecting');
    const wsUrl = `${WS_BASE}/ws/tail?key=${API_KEY}&service=${service}&level=${level}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => setStatus('live');
    
    ws.onmessage = (e) => {
      const batch = JSON.parse(e.data);
      setLogs(prev => {
        const newLogs = [...batch, ...prev].slice(0, 500); // Cap at 500 logs in memory
        return newLogs;
      });

      if (containerRef.current?.scrollTop > 50) {
        setMissedLogs(m => m + batch.length);
      } else if (containerRef.current) {
        containerRef.current.scrollTop = 0;
      }
    };

    ws.onerror = () => setStatus('error');
    
    ws.onclose = (e) => {
      // FIX 2: If this event belongs to an old, dead socket, ignore it completely!
      if (wsRef.current !== ws) return;

      if (isLive && e.code !== 1000) {
        setStatus('error');
        reconnectTimeoutRef.current = setTimeout(connectWS, 3000);
      }
    };
  }, [isLive, service, level]);

  useEffect(() => {
    connectWS();
    return () => {
      if (wsRef.current) wsRef.current.close(1000);
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
    };
  }, [connectWS]);

  // 3. Scroll position tracker
  const handleScroll = (e) => {
    const scrolled = e.target.scrollTop > 50;
    setIsScrolled(scrolled);
    if (!scrolled) setMissedLogs(0);
  };

  const scrollToTop = () => {
    if (containerRef.current) {
      containerRef.current.scrollTo({ top: 0, behavior: 'smooth' });
      setMissedLogs(0);
    }
  };

  const serviceOptions = [{ value: '', label: 'All Services' }, ...services.map(s => ({ value: s, label: s }))];
  const levelOptions = [
    { value: '', label: 'All Levels' },
    { value: 'DEBUG', label: 'DEBUG' },
    { value: 'INFO', label: 'INFO' },
    { value: 'WARN', label: 'WARN' },
    { value: 'ERROR', label: 'ERROR' },
    { value: 'FATAL', label: 'FATAL' },
  ];

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', padding: '2rem', maxWidth: '1600px', margin: '0 auto' }}>
      
      <style>{`
        @keyframes pulse {
          0% { box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.4); }
          70% { box-shadow: 0 0 0 10px rgba(34, 197, 94, 0); }
          100% { box-shadow: 0 0 0 0 rgba(34, 197, 94, 0); }
        }
        .live-dot {
          width: 10px; height: 10px; border-radius: 50%;
          background-color: var(--green);
          animation: pulse 2s infinite;
        }
        .paused-dot {
          width: 10px; height: 10px; border-radius: 50%;
          background-color: var(--text-dim);
        }
        .error-dot {
          width: 10px; height: 10px; border-radius: 50%;
          background-color: var(--amber);
        }
      `}</style>

      <div className="card" style={{ marginBottom: '1.5rem', display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '1rem 1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '1.5rem' }}>
          <h1 style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-primary)', margin: 0, fontSize: '1.5rem', display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <Activity size={24} color="var(--cyan)" /> Live Tail
          </h1>

          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', padding: '0.5rem 1rem', background: 'var(--bg)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--border)' }}>
            <div className={status === 'live' ? 'live-dot' : status === 'error' ? 'error-dot' : 'paused-dot'} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.85rem', fontWeight: 600, color: status === 'live' ? 'var(--green)' : 'var(--text-secondary)' }}>
              {status.toUpperCase()}
            </span>
          </div>
        </div>

        <div style={{ display: 'flex', gap: '1rem' }}>
          <CustomSelect value={service} onChange={setService} options={serviceOptions} placeholder="All Services" />
          <CustomSelect value={level} onChange={setLevel} options={levelOptions} placeholder="All Levels" />
          
          <button 
            onClick={() => setIsLive(!isLive)}
            style={{
              display: 'flex', alignItems: 'center', gap: '0.5rem',
              background: isLive ? 'rgba(239,68,68,0.1)' : 'rgba(34,197,94,0.1)',
              border: `1px solid ${isLive ? 'rgba(239,68,68,0.3)' : 'rgba(34,197,94,0.3)'}`,
              color: isLive ? 'var(--red)' : 'var(--green)',
              padding: '0 1.25rem', borderRadius: 'var(--radius)',
              cursor: 'pointer', fontFamily: 'var(--font-sans)', fontWeight: 600,
              transition: 'all 0.2s ease'
            }}>
            {isLive ? <><Pause size={16}/> Pause</> : <><Play size={16}/> Stream</>}
          </button>
        </div>
      </div>

      <div className="card" style={{ flex: 1, position: 'relative', overflow: 'hidden', padding: 0, display: 'flex', flexDirection: 'column' }}>
        
        {missedLogs > 0 && (
          <button 
            onClick={scrollToTop}
            style={{
              position: 'absolute', top: '1.5rem', left: '50%', transform: 'translateX(-50%)',
              background: 'var(--green)', color: '#000', border: 'none',
              padding: '0.5rem 1.5rem', borderRadius: '20px', fontFamily: 'var(--font-mono)',
              fontSize: '0.85rem', fontWeight: 600, cursor: 'pointer', zIndex: 10,
              display: 'flex', alignItems: 'center', gap: '0.5rem',
              boxShadow: '0 4px 12px rgba(34, 197, 94, 0.3)',
              transition: 'transform 0.2s ease'
            }}
            onMouseEnter={(e) => e.currentTarget.style.transform = 'translateX(-50%) scale(1.05)'}
            onMouseLeave={(e) => e.currentTarget.style.transform = 'translateX(-50%) scale(1)'}
          >
            <ArrowUp size={16} /> {missedLogs} New Logs Available
          </button>
        )}

        <div 
          ref={containerRef}
          onScroll={handleScroll}
          style={{ flex: 1, overflowY: 'auto', padding: '1rem 0', scrollBehavior: 'smooth' }}
        >
          {logs.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '4rem', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)' }}>
              Waiting for incoming logs...
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
              <tbody>
                {logs.map((log, i) => (
                  <tr key={`${log.id}-${i}`} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', transition: 'background 0.2s' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                    <td style={{ padding: '0.5rem 1.5rem', width: '180px', color: 'var(--text-secondary)' }}>
                      {format(new Date(log.timestamp), 'HH:mm:ss.SSS')}
                    </td>
                    <td style={{ padding: '0.5rem 1rem', width: '100px' }}>
                      <span className={`badge badge-${log.level.toLowerCase()}`}>{log.level}</span>
                    </td>
                    <td style={{ padding: '0.5rem 1rem', width: '150px', color: 'var(--text-secondary)' }}>
                      {log.service}
                    </td>
                    <td style={{ padding: '0.5rem 1.5rem', color: 'var(--text-primary)', wordBreak: 'break-word' }}>
                      {log.message}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}