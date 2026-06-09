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
    setStats(statsData || {});

    // FIX: Set limit to 1 since we only need the metadata total, 
    // and extract the .total property instead of .length
    const alertsRes = await apiFetch('/api/alerts?limit=1');
    const alertsData = await alertsRes.json();
    setAlertsCount(alertsData?.total || 0); 
    
  } catch (err) {
    console.error("Failed to fetch dashboard data:", err);
    setStats({});
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

  // --- THE FIX: Map to the ACTUAL JSON structure from Go ---
  
  // 1. Calculate Total Logs by summing all values in count_by_level
  const levelCounts = stats?.count_by_level || {};
  const totalLogs = Object.values(levelCounts).reduce((sum, val) => sum + val, 0);
  
  // 2. Calculate Total Errors by summing ERROR and FATAL
  const totalErrors = (levelCounts.ERROR || 0) + (levelCounts.FATAL || 0);
  
  // 3. Map the logs_per_hour array for the chart
  const rawVolume = stats?.logs_per_hour || [];
  
  let chartData = rawVolume.map(d => {
    try {
      return { time: format(parseISO(d.hour), 'HH:mm'), logs: d.count || 0 };
    } catch (e) {
      return { time: '00:00', logs: 0 };
    }
  });

  // If backend aggregated empty but we have total logs, show a "Current" bar
  if (chartData.length === 0 && totalLogs > 0) {
    chartData = [{ time: format(new Date(), 'HH:mm'), logs: totalLogs }];
  }

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
    // THE FIX: Changed from flex column to a 2-column Grid (350px left, the rest on the right)
    <div style={{ display: 'grid', gridTemplateColumns: '350px 1fr', gap: '2rem', alignItems: 'stretch' }}>
      
      {/* LEFT PANEL: Stat Cards (Stacked vertically now) */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
        <StatCard 
          icon={<FileText size={20} color="var(--green)" />} 
          title="Total Logs (24h)" 
          value={totalLogs.toLocaleString()} 
          borderColor="var(--green)" 
        />
        <StatCard 
          icon={<Activity size={20} color="var(--red)" />} 
          title="Total Errors (24h)" 
          value={totalErrors.toLocaleString()} 
          borderColor="var(--red)" 
        />
        <StatCard 
          icon={<AlertTriangle size={20} color="var(--amber)" />} 
          title="Alerts Fired (24h)" 
          value={alertsCount.toLocaleString()} 
          borderColor="var(--amber)" 
        />
      </div>

      {/* RIGHT PANEL: The Chart */}
      <div className="card" style={{ padding: '1.5rem', display: 'flex', flexDirection: 'column', minHeight: '400px' }}>
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