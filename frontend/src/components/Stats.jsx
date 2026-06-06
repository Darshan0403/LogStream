// src/components/Stats.jsx
import { useState, useEffect } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from 'recharts';
import { Activity, AlertTriangle, FileText } from 'lucide-react';
import { apiFetch } from '../config/api';
import { format, parseISO } from 'date-fns';

export default function Stats() {
  const [stats, setStats] = useState(null);
  const [alertsCount, setAlertsCount] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const statsRes = await apiFetch('/api/logs/stats');
        const statsData = await statsRes.json();
        setStats(statsData || {}); // Fallback to empty object

        const alertsRes = await apiFetch('/api/alerts?limit=200');
        const alertsData = await alertsRes.json();
        setAlertsCount(alertsData?.length || 0);
      } catch (err) {
        console.error("Failed to fetch dashboard data:", err);
        setStats({}); // Prevent crash on network error
      } finally {
        setLoading(false);
      }
    };

    fetchData();
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

  if (loading || !stats) {
    return <div style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-secondary)' }}>Aggregating telemetry...</div>;
  }

  // BULLETPROOF: Safe mapping even if volume_by_hour is missing
  const chartData = (stats?.volume_by_hour || []).map(d => {
    try {
      return {
        time: format(parseISO(d.hour), 'HH:mm'),
        logs: d.count || 0
      };
    } catch (e) {
      return { time: '00:00', logs: 0 };
    }
  });

  const CustomTooltip = ({ active, payload, label }) => {
    if (active && payload && payload.length) {
      return (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border-bright)', padding: '0.5rem 1rem', borderRadius: 'var(--radius)', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
          <p style={{ color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>{label}</p>
          <p style={{ color: 'var(--green)', fontWeight: 600 }}>{payload[0].value} logs</p>
        </div>
      );
    }
    return null;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
      
      {/* BULLETPROOF: Safely extract numbers with fallbacks */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))', gap: '1.5rem' }}>
        <StatCard 
          icon={<FileText size={20} color="var(--green)" />} 
          title="Total Logs (24h)" 
          value={(stats?.total_logs || 0).toLocaleString()} 
          borderColor="var(--green)" 
        />
        <StatCard 
          icon={<Activity size={20} color="var(--red)" />} 
          title="Total Errors (24h)" 
          value={(stats?.total_errors || 0).toLocaleString()} 
          borderColor="var(--red)" 
        />
        <StatCard 
          icon={<AlertTriangle size={20} color="var(--amber)" />} 
          title="Alerts Fired (24h)" 
          value={alertsCount} 
          borderColor="var(--amber)" 
        />
      </div>

      <div className="card" style={{ height: '400px', padding: '1.5rem', display: 'flex', flexDirection: 'column' }}>
        <h3 style={{ fontFamily: 'var(--font-mono)', fontSize: '0.9rem', color: 'var(--text-secondary)', marginBottom: '1.5rem', textTransform: 'uppercase', letterSpacing: '1px' }}>
          Ingestion Volume (Last 24 Hours)
        </h3>
        <div style={{ flex: 1, minHeight: 0 }}>
          {chartData.length === 0 ? (
             <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)' }}>
               No telemetry data available for this period.
             </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={chartData} margin={{ top: 0, right: 0, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border-bright)" vertical={false} />
                <XAxis dataKey="time" stroke="var(--text-secondary)" fontSize={12} tickLine={false} axisLine={false} fontFamily="var(--font-mono)" />
                <YAxis stroke="var(--text-secondary)" fontSize={12} tickLine={false} axisLine={false} fontFamily="var(--font-mono)" />
                <Tooltip cursor={{ fill: 'var(--surface-hover)' }} content={<CustomTooltip />} />
                <Bar dataKey="logs" fill="var(--green)" radius={[4, 4, 0, 0]} maxBarSize={40} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

    </div>
  );
}

function StatCard({ icon, title, value, borderColor }) {
  return (
    <div className="card" style={{ display: 'flex', alignItems: 'center', gap: '1rem', borderLeft: `3px solid ${borderColor}` }}>
      <div style={{ padding: '0.75rem', background: 'rgba(255,255,255,0.03)', borderRadius: 'var(--radius-lg)' }}>
        {icon}
      </div>
      <div>
        <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: '0.25rem' }}>{title}</div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.75rem', fontWeight: 700, color: 'var(--text-primary)' }}>{value}</div>
      </div>
    </div>
  );
}