// src/pages/Alerts.jsx
import { useState, useEffect } from 'react';
import { Bell, ShieldAlert, Activity, CheckCircle, XCircle } from 'lucide-react';
import { format } from 'date-fns';
import { apiFetch } from '../config/api';

export default function Alerts() {
  const [rules, setRules] = useState([]);
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch both Rules and Alert History concurrently
        const [rulesRes, alertsRes] = await Promise.all([
          apiFetch('/api/rules'),
          apiFetch('/api/alerts?limit=50')
        ]);
        
        if (rulesRes.ok) {
          const rData = await rulesRes.json();
          setRules(rData || []);
        }
        
        if (alertsRes.ok) {
          const aData = await alertsRes.json();
          setAlerts(aData || []);
        }
      } catch (err) {
        console.error("Failed to fetch alerts data:", err);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  if (loading) {
    return <div style={{ padding: '2rem', fontFamily: 'var(--font-mono)', color: 'var(--text-secondary)' }}>Loading alert configuration...</div>;
  }

  return (
    <div style={{ padding: '2rem', maxWidth: '1600px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '2rem' }}>
      
      <h1 style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-primary)', margin: 0, display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
        <Bell size={24} color="var(--amber)" /> Alerting Engine
      </h1>

      {/* Two-Panel Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))', gap: '1.5rem' }}>
        
        {/* PANEL 1: Rules List */}
        <div className="card" style={{ padding: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: '1rem 1.5rem', borderBottom: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <ShieldAlert size={18} color="var(--cyan)" />
            <h2 style={{ fontFamily: 'var(--font-mono)', fontSize: '1rem', margin: 0 }}>Active Rules</h2>
          </div>
          
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Name</th>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Pattern</th>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Level</th>
                  <th style={{ padding: '0.75rem 1.5rem', textAlign: 'center' }}>Status</th>
                </tr>
              </thead>
              <tbody>
                {rules.length === 0 ? (
                  <tr>
                    <td colSpan={4} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>No rules configured.</td>
                  </tr>
                ) : (
                  rules.map(rule => (
                    <tr key={rule.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-primary)' }}>{rule.name}</td>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-secondary)' }}>{rule.pattern}</td>
                      <td style={{ padding: '0.75rem 1.5rem' }}>
                        {rule.level_filter ? <span className={`badge badge-${rule.level_filter.toLowerCase()}`}>{rule.level_filter}</span> : <span style={{ color: 'var(--text-dim)' }}>*</span>}
                      </td>
                      <td style={{ padding: '0.75rem 1.5rem', textAlign: 'center' }}>
                        {rule.is_active ? <CheckCircle size={16} color="var(--green)" /> : <XCircle size={16} color="var(--text-dim)" />}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>

        {/* PANEL 2: Alert History */}
        <div className="card" style={{ padding: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column', gridColumn: 'span 2' }}>
          <div style={{ padding: '1rem 1.5rem', borderBottom: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <Activity size={18} color="var(--amber)" />
            <h2 style={{ fontFamily: 'var(--font-mono)', fontSize: '1rem', margin: 0 }}>Trigger History</h2>
          </div>
          
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Fired At</th>
                  <th style={{ padding: '0.75rem 1rem' }}>Rule</th>
                  <th style={{ padding: '0.75rem 1rem' }}>Service</th>
                  <th style={{ padding: '0.75rem 1.5rem' }}>Triggered By</th>
                </tr>
              </thead>
              <tbody>
                {alerts.length === 0 ? (
                  <tr>
                    <td colSpan={4} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>No alerts have been triggered recently.</td>
                  </tr>
                ) : (
                  alerts.map(alert => (
                    <tr key={alert.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-secondary)' }}>
                        {format(new Date(alert.fired_at), 'MMM dd HH:mm:ss')}
                      </td>
                      <td style={{ padding: '0.75rem 1rem', color: 'var(--text-primary)', fontWeight: 600 }}>{alert.rule_name}</td>
                      <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)' }}>{alert.service}</td>
                      <td style={{ padding: '0.75rem 1.5rem', color: 'var(--text-dim)', maxWidth: '400px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
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