// src/components/AlertRules.jsx
import { useState, useEffect, useRef } from 'react';
import { ShieldAlert, Trash2, CheckCircle, XCircle, Plus, ChevronDown } from 'lucide-react';
import { apiFetch } from '../config/api';

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
    <div ref={ref} style={{ position: 'relative', flex: 1, minWidth: '140px' }}>
      <div 
        onClick={() => setIsOpen(!isOpen)}
        style={{
          background: 'var(--surface)',
          border: `1px solid ${isOpen ? 'var(--green)' : 'var(--border-bright)'}`,
          borderRadius: 'var(--radius)',
          padding: '0.5rem 0.75rem',
          fontSize: '0.875rem',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          color: value ? 'var(--text-primary)' : 'var(--text-secondary)',
          transition: 'border-color 0.2s ease',
          height: '100%'
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
              padding: '0.5rem 0.75rem', fontSize: '0.875rem', cursor: 'pointer',
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

// --- Main AlertRules Component ---
export default function AlertRules({ onRuleSelect, selectedRuleId, showToast }) {
  const [rules, setRules] = useState([]);
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState(null);

  // Form State
  const [name, setName] = useState('');
  const [pattern, setPattern] = useState('');
  const [level, setLevel] = useState('');
  const [service, setService] = useState('');
  const [cooldown, setCooldown] = useState(5);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    fetchRules();
    fetchServices();
  }, []);

  const fetchRules = async () => {
    try {
      const res = await apiFetch('/api/rules');
      if (res.ok) setRules(await res.json() || []);
    } catch (err) {
      showToast("Failed to fetch rules", "error");
    } finally {
      setLoading(false);
    }
  };

  const fetchServices = async () => {
    try {
      const res = await apiFetch('/api/services');
      if (res.ok) setServices(await res.json() || []);
    } catch (err) {}
  };

  const handleCreate = async (e) => {
    e.preventDefault();
    setIsSubmitting(true);
    
    const newRule = {
      name,
      pattern,
      level_filter: level || null,
      service_filter: service || null,
      cooldown_minutes: parseInt(cooldown, 10),
      is_active: true
    };

    try {
      const res = await apiFetch('/api/rules', {
        method: 'POST',
        body: JSON.stringify(newRule)
      });
      
      if (res.ok) {
        showToast("Rule created successfully", "success");
        setName(''); setPattern(''); setLevel(''); setService(''); setCooldown(5);
        fetchRules();
      } else {
        showToast("Failed to create rule", "error");
      }
    } catch (err) {
      showToast("Network error", "error");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleToggleActive = async (rule, e) => {
    e.stopPropagation();
    setRules(rules.map(r => r.id === rule.id ? { ...r, is_active: !r.is_active } : r));
    
    try {
      const res = await apiFetch(`/api/rules/${rule.id}`, {
        method: 'PUT',
        body: JSON.stringify({ ...rule, is_active: !rule.is_active })
      });
      if (res.ok) {
        showToast(`Rule ${!rule.is_active ? 'enabled' : 'disabled'}`, "success");
      } else {
        throw new Error();
      }
    } catch (err) {
      setRules(rules.map(r => r.id === rule.id ? { ...r, is_active: rule.is_active } : r));
      showToast("Failed to update rule", "error");
    }
  };

  const handleDelete = async (id, e) => {
    e.stopPropagation();
    if (deletingId !== id) {
      setDeletingId(id);
      setTimeout(() => setDeletingId(null), 3000);
      return;
    }

    try {
      const res = await apiFetch(`/api/rules/${id}`, { method: 'DELETE' });
      if (res.ok) {
        setRules(rules.filter(r => r.id !== id));
        if (selectedRuleId === id) onRuleSelect(null);
        showToast("Rule deleted", "success");
      } else {
        showToast("Failed to delete rule", "error");
      }
    } catch (err) {
      showToast("Network error", "error");
    }
    setDeletingId(null);
  };

  // Dropdown Options
  const serviceOptions = [{ value: '', label: 'Any Service' }, ...services.map(s => ({ value: s, label: s }))];
  const levelOptions = [
    { value: '', label: 'Any Level' },
    { value: 'ERROR', label: 'ERROR' },
    { value: 'FATAL', label: 'FATAL' },
    { value: 'WARN', label: 'WARN' },
    { value: 'INFO', label: 'INFO' },
  ];

  return (
    <div className="card" style={{ padding: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ padding: '1rem 1.5rem', borderBottom: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
        <ShieldAlert size={18} color="var(--cyan)" />
        <h2 style={{ fontFamily: 'var(--font-mono)', fontSize: '1rem', margin: 0 }}>Active Rules</h2>
      </div>

      <form onSubmit={handleCreate} style={{ padding: '1.5rem', borderBottom: '1px solid var(--border)', background: 'var(--bg)', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: '1rem' }}>
          <input required type="text" placeholder="Rule Name (e.g. DB Crash)" value={name} onChange={e => setName(e.target.value)} />
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
            <input required type="text" placeholder="Regex Pattern (e.g. timeout|refused)" value={pattern} onChange={e => setPattern(e.target.value)} style={{ fontFamily: 'var(--font-mono)' }} />
            <span style={{ fontSize: '0.7rem', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)' }}>Regex pattern matched against log messages</span>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '1rem' }}>
          
          {/* Replaced native selects with CustomSelect */}
          <CustomSelect value={level} onChange={setLevel} options={levelOptions} placeholder="Any Level" />
          <CustomSelect value={service} onChange={setService} options={serviceOptions} placeholder="Any Service" />
          
          <input type="number" min="1" placeholder="Cooldown (m)" value={cooldown} onChange={e => setCooldown(e.target.value)} style={{ width: '100px' }} title="Cooldown Minutes" />
          <button type="submit" disabled={isSubmitting} style={{ background: 'var(--green)', color: '#000', border: 'none', padding: '0 1.5rem', borderRadius: 'var(--radius)', cursor: isSubmitting ? 'not-allowed' : 'pointer', fontWeight: 600 }}>
            {isSubmitting ? '...' : <><Plus size={16} style={{ verticalAlign: 'middle', marginRight: '4px' }} /> Add</>}
          </button>
        </div>
      </form>

      <div style={{ overflowX: 'auto', flex: 1 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
              <th style={{ padding: '0.75rem 1rem' }}>Name</th>
              <th style={{ padding: '0.75rem 1rem' }}>Pattern</th>
              <th style={{ padding: '0.75rem 1rem' }}>Filters</th>
              <th style={{ padding: '0.75rem 1rem', width: '60px' }}>Cool</th>
              <th style={{ padding: '0.75rem 1rem', textAlign: 'center', width: '120px' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={5} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>Loading...</td></tr>
            ) : rules.length === 0 ? (
              <tr><td colSpan={5} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-dim)' }}>No alert rules configured.<span className="blink">_</span></td></tr>
            ) : (
              rules.map(rule => (
                <tr 
                  key={rule.id} 
                  onClick={() => onRuleSelect(selectedRuleId === rule.id ? null : rule.id)}
                  style={{ 
                    borderBottom: '1px solid rgba(255,255,255,0.02)', 
                    cursor: 'pointer',
                    background: selectedRuleId === rule.id ? 'var(--surface-hover)' : 'transparent',
                    borderLeft: selectedRuleId === rule.id ? '2px solid var(--cyan)' : '2px solid transparent',
                    transition: 'background 0.2s'
                  }}
                  onMouseEnter={(e) => { if(selectedRuleId !== rule.id) e.currentTarget.style.background = 'var(--surface-hover)'}}
                  onMouseLeave={(e) => { if(selectedRuleId !== rule.id) e.currentTarget.style.background = 'transparent'}}
                >
                  <td style={{ padding: '0.75rem 1rem', color: 'var(--text-primary)' }}>{rule.name}</td>
                  <td style={{ padding: '0.75rem 1rem', color: 'var(--cyan)' }}>{rule.pattern}</td>
                  <td style={{ padding: '0.75rem 1rem' }}>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      {rule.level_filter ? <span className={`badge badge-${rule.level_filter.toLowerCase()}`}>{rule.level_filter}</span> : <span style={{ color: 'var(--text-dim)' }}>lvl:*</span>}
                      {rule.service_filter ? <span style={{ color: 'var(--text-secondary)' }}>{rule.service_filter}</span> : <span style={{ color: 'var(--text-dim)' }}>svc:*</span>}
                    </div>
                  </td>
                  <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)' }}>{rule.cooldown_minutes}m</td>
                  <td style={{ padding: '0.75rem 1rem', textAlign: 'center' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.75rem' }}>
                      <button onClick={(e) => handleToggleActive(rule, e)} style={{ background: 'transparent', border: 'none', cursor: 'pointer', padding: 0 }}>
                        {rule.is_active ? <CheckCircle size={18} color="var(--green)" /> : <XCircle size={18} color="var(--text-dim)" />}
                      </button>
                      <button 
                        onClick={(e) => handleDelete(rule.id, e)} 
                        style={{ 
                          background: deletingId === rule.id ? 'var(--red)' : 'transparent', 
                          color: deletingId === rule.id ? '#000' : 'var(--text-dim)', 
                          border: 'none', cursor: 'pointer', padding: '2px 6px', borderRadius: '4px', fontSize: '0.7rem',
                          transition: 'all 0.2s'
                        }}
                      >
                        {deletingId === rule.id ? "Confirm?" : <Trash2 size={16} />}
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}