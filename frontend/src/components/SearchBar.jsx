// src/components/SearchBar.jsx
import { useState, useEffect, useRef } from 'react';
import { Search, X, ChevronDown } from 'lucide-react';

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
    <div ref={ref} style={{ position: 'relative', flex: 1, minWidth: '160px' }}>
      <div 
        onClick={() => setIsOpen(!isOpen)}
        style={{
          background: 'var(--surface)',
          border: `1px solid ${isOpen ? 'var(--green)' : 'var(--border-bright)'}`,
          borderRadius: 'var(--radius)',
          padding: '0.75rem 1rem', // Made slightly chunkier to match date picker
          fontSize: '0.95rem',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          transition: 'all 0.2s ease',
          color: value ? 'var(--text-primary)' : 'var(--text-secondary)'
        }}
      >
        {selectedLabel}
        <ChevronDown 
          size={18} 
          style={{ 
            transition: 'transform 0.2s ease', 
            transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)' 
          }} 
        />
      </div>

      <div style={{
        position: 'absolute',
        top: '100%',
        left: 0,
        right: 0,
        marginTop: '0.25rem',
        background: 'var(--surface)',
        border: '1px solid var(--border-bright)',
        borderRadius: 'var(--radius)',
        zIndex: 50,
        overflow: 'hidden',
        opacity: isOpen ? 1 : 0,
        transform: isOpen ? 'translateY(0)' : 'translateY(-10px)',
        pointerEvents: isOpen ? 'auto' : 'none',
        transition: 'all 0.2s ease',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)'
      }}>
        {options.map((opt) => (
          <div
            key={opt.value}
            onClick={() => { onChange(opt.value); setIsOpen(false); }}
            style={{
              padding: '0.75rem 1rem',
              fontSize: '0.9rem',
              cursor: 'pointer',
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

export default function SearchBar({ onSearch, services = [], searchTime = null, total = 0 }) {
  const [q, setQ] = useState('');
  const [service, setService] = useState('');
  const [level, setLevel] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  
  const searchInputRef = useRef(null);

  useEffect(() => {
    const handleKeyDown = (e) => {
      // If the user presses '/' and isn't already typing in an input field
      if (e.key === '/' && document.activeElement.tagName !== 'INPUT') {
        e.preventDefault(); // Stop the '/' from being typed
        
        // Small timeout ensures the browser drops the keystroke before focusing
        setTimeout(() => {
          searchInputRef.current?.focus();
        }, 10);
      }
    };
    
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => {
      onSearch({ q, service, level, from, to });
    }, 300);
    return () => clearTimeout(timer);
  }, [q, service, level, from, to]);

  const handleClear = () => {
    setQ(''); setService(''); setLevel(''); setFrom(''); setTo('');
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
    <div className="card" style={{ marginBottom: '1.5rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', background: 'var(--bg)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', padding: '0.75rem 1rem', transition: 'border-color 0.2s', ...((q.length > 0) ? {borderColor: 'var(--green)'} : {}) }}>
        <Search size={20} color="var(--text-secondary)" />
        <input 
          ref={searchInputRef}
          type="text" 
          placeholder='Search logs... (Press "/" to focus)' 
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{ flex: 1, border: 'none', background: 'transparent', outline: 'none', fontSize: '1rem', fontFamily: 'var(--font-mono)' }}
        />
        {q && <X size={18} style={{ cursor: 'pointer', color: 'var(--text-secondary)' }} onClick={() => setQ('')} />}
      </div>

      {/* Search timing — only shown after first result */}
      {searchTime !== null && (
        <div style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '0.75rem',
          color: 'var(--text-dim)',
          paddingLeft: '0.25rem',
          marginTop: '-0.25rem',
        }}>
          <span style={{ color: 'var(--green)', opacity: 0.7 }}>›</span>
          {' '}{total.toLocaleString()} result{total !== 1 ? 's' : ''}{' '}
          <span style={{ color: 'var(--text-dim)' }}>in {searchTime}ms</span>
        </div>
      )}
      
      <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
        <CustomSelect value={service} onChange={setService} options={serviceOptions} placeholder="All Services" />
        <CustomSelect value={level} onChange={setLevel} options={levelOptions} placeholder="All Levels" />

        {/* Larger, Chunkier Date Pickers */}
        <input 
          type="datetime-local" 
          value={from} 
          onChange={(e) => setFrom(e.target.value)} 
          style={{ 
            flex: 1, 
            minWidth: '220px', 
            colorScheme: 'dark',
            padding: '0.75rem 1rem',
            fontSize: '0.95rem',
            color: from ? 'var(--text-primary)' : 'var(--text-secondary)',
            background: 'var(--surface)',
            border: '1px solid var(--border-bright)',
            borderRadius: 'var(--radius)',
            outline: 'none',
            transition: 'border-color 0.2s',
            cursor: 'pointer'
          }} 
        />
        <input 
          type="datetime-local" 
          value={to} 
          onChange={(e) => setTo(e.target.value)} 
          style={{ 
            flex: 1, 
            minWidth: '220px', 
            colorScheme: 'dark',
            padding: '0.75rem 1rem',
            fontSize: '0.95rem',
            color: to ? 'var(--text-primary)' : 'var(--text-secondary)',
            background: 'var(--surface)',
            border: '1px solid var(--border-bright)',
            borderRadius: 'var(--radius)',
            outline: 'none',
            transition: 'border-color 0.2s',
            cursor: 'pointer'
          }} 
        />

        <button 
          onClick={handleClear} 
          style={{ 
            background: 'transparent', 
            border: '1px solid var(--border-bright)', 
            color: 'var(--text-secondary)', 
            padding: '0.75rem 1.5rem', 
            fontSize: '0.95rem',
            borderRadius: 'var(--radius)', 
            cursor: 'pointer', 
            transition: 'all 0.2s ease'
          }}
          onMouseEnter={(e) => { e.target.style.color = 'var(--text-primary)'; e.target.style.borderColor = 'var(--text-secondary)'; }}
          onMouseLeave={(e) => { e.target.style.color = 'var(--text-secondary)'; e.target.style.borderColor = 'var(--border-bright)'; }}
        >
          Clear Filters
        </button>
      </div>
    </div>
  );
}