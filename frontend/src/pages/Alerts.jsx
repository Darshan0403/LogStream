// src/pages/Alerts.jsx
import { useState, useEffect, useCallback } from 'react';
import { Bell, Activity } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';
import { apiFetch } from '../config/api';
import AlertRules from '../components/AlertRules';

export default function Alerts() {
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [selectedRuleId, setSelectedRuleId] = useState(null);
  const [toast, setToast] = useState(null);

  // Lightweight Toast Manager
  const showToast = useCallback((message, type = "success") => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const fetchAlertHistory = useCallback(async () => {
    try {
      const url = selectedRuleId 
        ? `/api/alerts?limit=50&rule_id=${selectedRuleId}` 
        : `/api/alerts?limit=50`;
      
      const res = await apiFetch(url);
      if (res.ok) {
        setAlerts(await res.json() || []);
      }
    } catch (err) {
      console.error("Failed to fetch alert history:", err);
    } finally {
      setLoading(false);
    }
  }, [selectedRuleId]);

  // Initial fetch and dependency on selectedRuleId
  useEffect(() => {
    setLoading(true);
    fetchAlertHistory();
  }, [fetchAlertHistory]);

  // Auto-refresh history every 15 seconds
  useEffect(() => {
    const interval = setInterval(fetchAlertHistory, 15000);
    return () => clearInterval(interval);
  }, [fetchAlertHistory]);

  return (
    <div style={{ padding: '2rem', maxWidth: '1600px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '2rem', position: 'relative' }}>
      
      {/* --- TOAST NOTIFICATION --- */}
      {toast && (
        <div style={{
          position: 'fixed', bottom: '2rem', right: '2rem', zIndex: 100,
          background: 'var(--surface)',
          border: `1px solid ${toast.type === 'success' ? 'var(--green)' : 'var(--red)'}`,
          color: 'var(--text-primary)',
          padding: '1rem 1.5rem',
          borderRadius: 'var(--radius)',
          fontFamily: 'var(--font-sans)',
          fontWeight: 500,
          boxShadow: '0 8px 16px rgba(0,0,0,0.5)',
          animation: 'fadeInUp 0.3s ease forwards'
        }}>
          {toast.message}
        </div>
      )}

      <h1 style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-primary)', margin: 0, display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
        <Bell size={24} color="var(--amber)" /> Alerting Engine
      </h1>

      {/* --- TWO-PANEL RESPONSIVE LAYOUT --- */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(450px, 1fr))', gap: '1.5rem', alignItems: 'start' }}>
        
        {/* PANEL 1: Rules CRUD (Left Panel) */}
        <div style={{ height: 'calc(100vh - 150px)', position: 'sticky', top: '2rem' }}>
          <AlertRules 
            onRuleSelect={setSelectedRuleId} 
            selectedRuleId={selectedRuleId} 
            showToast={showToast}
          />
        </div>

        {/* PANEL 2: Alert History (Right Panel) */}
        <div className="card" style={{ padding: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column', height: 'calc(100vh - 150px)' }}>
          <div style={{ padding: '1rem 1.5rem', borderBottom: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <Activity size={18} color="var(--amber)" />
              <h2 style={{ fontFamily: 'var(--font-mono)', fontSize: '1rem', margin: 0 }}>
                Alert History {selectedRuleId && <span style={{ color: 'var(--cyan)' }}>(Filtered)</span>}
              </h2>
            </div>
            {selectedRuleId && (
              <button onClick={() => setSelectedRuleId(null)} style={{ background: 'transparent', border: '1px solid var(--border-bright)', color: 'var(--text-secondary)', padding: '0.2rem 0.5rem', borderRadius: 'var(--radius)', cursor: 'pointer', fontSize: '0.75rem' }}>
                Clear Filter
              </button>
            )}
          </div>
          
          <div style={{ overflowY: 'auto', flex: 1 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)', position: 'sticky', top: 0, background: 'var(--surface)' }}>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Fired At</th>
                  <th style={{ padding: '0.75rem 1rem' }}>Rule</th>
                  <th style={{ padding: '0.75rem 1rem' }}>Target</th>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Message</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr><td colSpan={4} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>Loading history...</td></tr>
                ) : alerts.length === 0 ? (
                  <tr><td colSpan={4} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>No alerts fired in the last 24 hours. Your systems are quiet.</td></tr>
                ) : (
                  alerts.map(alert => (
                    <tr key={alert.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', animation: 'fadeInUp 0.3s ease' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-secondary)', whiteSpace: 'nowrap' }}>
                        {formatDistanceToNow(new Date(alert.fired_at), { addSuffix: true })}
                      </td>
                      <td style={{ padding: '0.75rem 1rem', color: 'var(--text-primary)', fontWeight: 600 }}>{alert.rule_name}</td>
                      <td style={{ padding: '0.75rem 1rem' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
                          <span className={`badge badge-${alert.level.toLowerCase()}`} style={{ alignSelf: 'flex-start' }}>{alert.level}</span>
                          <span style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>{alert.service}</span>
                        </div>
                      </td>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-dim)', maxWidth: '300px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        {alert.message}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>

      </div>
    </div>
  );
}