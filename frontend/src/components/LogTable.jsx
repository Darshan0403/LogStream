// src/components/LogTable.jsx
import { useState } from 'react';
import { format } from 'date-fns';
import { ChevronRight } from 'lucide-react';

export default function LogTable({ logs, loading }) {
  if (loading && logs.length === 0) {
    return (
      <div className="card" style={{ textAlign: 'center', padding: '4rem', color: 'var(--text-secondary)' }}>
        <span style={{ fontFamily: 'var(--font-mono)' }}>Loading logs...</span>
      </div>
    );
  }

  if (!loading && logs.length === 0) {
    return (
      <div className="card" style={{ textAlign: 'center', padding: '4rem', color: 'var(--text-secondary)' }}>
        <p style={{ fontFamily: 'var(--font-mono)' }}>{'>_'} No logs found. Try adjusting your filters.</p>
      </div>
    );
  }

  return (
    <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)' }}>
              <th style={{ padding: '0.75rem 1rem', width: '30px' }}></th>
              <th style={{ padding: '0.75rem 1rem', width: '200px' }}>Timestamp</th>
              <th style={{ padding: '0.75rem 1rem', width: '100px' }}>Level</th>
              <th style={{ padding: '0.75rem 1rem', width: '150px' }}>Service</th>
              <th style={{ padding: '0.75rem 1rem' }}>Message</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log) => (
              <LogRow key={log.id} log={log} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function LogRow({ log }) {
  const [expanded, setExpanded] = useState(false);
  const badgeClass = `badge badge-${log.level.toLowerCase()}`;

  return (
    <>
      <tr 
        onClick={() => setExpanded(!expanded)}
        style={{ 
          borderBottom: '1px solid var(--border)', 
          cursor: 'pointer',
          background: expanded ? 'var(--surface-hover)' : 'transparent',
          transition: 'background 0.2s ease'
        }}
        onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'}
        onMouseLeave={(e) => e.currentTarget.style.background = expanded ? 'var(--surface-hover)' : 'transparent'}
      >
        <td style={{ padding: '0.75rem 1rem', color: 'var(--text-dim)' }}>
          <ChevronRight 
            size={14} 
            style={{ 
              transition: 'transform 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
              transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)'
            }} 
          />
        </td>
        <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)' }}>
          {format(new Date(log.timestamp), 'MMM dd HH:mm:ss.SSS')}
        </td>
        <td style={{ padding: '0.75rem 1rem' }}>
          <span className={badgeClass}>{log.level}</span>
        </td>
        <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)' }}>
          {log.service}
        </td>
        <td style={{ 
          padding: '0.75rem 1rem', 
          whiteSpace: 'nowrap', 
          overflow: 'hidden', 
          textOverflow: 'ellipsis', 
          maxWidth: '400px' 
        }}>
          {log.message}
        </td>
      </tr>
      
      {/* The Grid 0fr/1fr trick enables buttery smooth slide-down animations 
        without needing to hardcode pixel heights. 
      */}
      <tr>
        <td colSpan={5} style={{ padding: 0 }}>
          <div style={{ 
            display: 'grid', 
            gridTemplateRows: expanded ? '1fr' : '0fr',
            transition: 'grid-template-rows 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
            background: 'var(--bg)',
            borderBottom: expanded ? '1px solid var(--border)' : 'none'
          }}>
            <div style={{ overflow: 'hidden' }}>
              {/* Opacity and padding animate alongside the height expansion */}
              <div style={{ 
                padding: expanded ? '1.5rem' : '0 1.5rem',
                opacity: expanded ? 1 : 0,
                transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                display: 'flex', 
                flexDirection: 'column', 
                gap: '1rem' 
              }}>
                <div>
                  <strong style={{ color: 'var(--text-secondary)', fontSize: '0.8rem', textTransform: 'uppercase' }}>Full Message</strong>
                  <div style={{ marginTop: '0.5rem', color: 'var(--text-primary)', whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'var(--font-sans)' }}>
                    {log.message}
                  </div>
                </div>
                <div>
                  <strong style={{ color: 'var(--text-secondary)', fontSize: '0.8rem', textTransform: 'uppercase' }}>Metadata JSON</strong>
                  <pre style={{ marginTop: '0.5rem', padding: '1rem', background: 'var(--surface)', borderRadius: 'var(--radius)', border: '1px solid var(--border-bright)', overflowX: 'auto', color: 'var(--cyan)' }}>
                    {JSON.stringify(log.metadata, null, 2)}
                  </pre>
                </div>
              </div>
            </div>
          </div>
        </td>
      </tr>
    </>
  );
}