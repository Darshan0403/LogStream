// src/components/Sidebar.jsx
import { NavLink } from 'react-router-dom';
import { Terminal, Activity, Bell, ShieldAlert, LayoutDashboard } from 'lucide-react';

export default function Sidebar() {
  return (
    <aside style={{
      width: '240px',
      backgroundColor: 'var(--surface)',
      borderRight: '1px solid var(--border)',
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      padding: '1.5rem 1rem',
      position: 'relative',
      zIndex: 10 
    }}>
      <div style={{ marginBottom: '2.5rem', padding: '0 0.5rem' }}>
        <h1 style={{ 
          fontFamily: 'var(--font-mono)', 
          color: 'var(--green)', 
          fontSize: '1.25rem', 
          letterSpacing: '2px',
          fontWeight: 700
        }}>
          LOGSTREAM
        </h1>
      </div>

      <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', flex: 1 }}>
        <SidebarLink to="/logs" icon={<Terminal size={18} />} label="Logs" />
        <SidebarLink to="/tail" icon={<Activity size={18} />} label="Live Tail" />
        
        {/* Split the Alerting module into two distinct pages */}
        <SidebarLink to="/rules" icon={<ShieldAlert size={18} />} label="Alert Rules" />
        <SidebarLink to="/alerts" icon={<Bell size={18} />} label="Alert History" />
        
        <SidebarLink to="/dashboard" icon={<LayoutDashboard size={18} />} label="Dashboard" />
      </nav>

      <div style={{ 
        marginTop: 'auto', 
        display: 'flex', 
        alignItems: 'center', 
        gap: '0.75rem', 
        padding: '0.75rem', 
        fontSize: '0.85rem', 
        color: 'var(--text-secondary)',
        borderTop: '1px solid var(--border)'
      }}>
        <div style={{ width: '8px', height: '8px', borderRadius: '50%', backgroundColor: 'var(--green)', boxShadow: '0 0 8px var(--green)' }}></div>
        System Online
      </div>
    </aside>
  );
}

function SidebarLink({ to, icon, label }) {
  return (
    <NavLink
      to={to}
      style={({ isActive }) => ({
        display: 'flex', alignItems: 'center', gap: '0.75rem',
        padding: '0.75rem 1rem', borderRadius: 'var(--radius)',
        color: isActive ? 'var(--text-primary)' : 'var(--text-secondary)',
        backgroundColor: isActive ? 'var(--surface-hover)' : 'transparent',
        borderLeft: isActive ? '2px solid var(--green)' : '2px solid transparent',
        textDecoration: 'none', transition: 'all 0.2s ease',
        fontFamily: 'var(--font-sans)'
      })}
    >
      {icon}
      <span style={{ fontWeight: 500 }}>{label}</span>
    </NavLink>
  );
}