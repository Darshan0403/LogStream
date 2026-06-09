// src/pages/Rules.jsx
import { useState, useEffect, useRef, useCallback } from 'react';
import { ShieldAlert, Trash2, CheckCircle, XCircle, Plus, ChevronDown } from 'lucide-react';
import { apiFetch } from '../config/api';

// --- Custom Dropdown ---
function CustomSelect({ value, onChange, options, placeholder }) {
  const [isOpen, setIsOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const handleClickOutside = (e) => { if (ref.current && !ref.current.contains(e.target)) setIsOpen(false); };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const selectedLabel = options.find(o => o.value === value)?.label || placeholder;

  return (
    <div ref={ref} style={{ position: 'relative', flex: 1, minWidth: '160px' }}>
      <div 
        onClick={() => setIsOpen(!isOpen)}
        style={{
          background: 'var(--surface)', border: `1px solid ${isOpen ? 'var(--green)' : 'var(--border-bright)'}`,
          borderRadius: 'var(--radius)', padding: '0.5rem 0.75rem', fontSize: '0.875rem', cursor: 'pointer',
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          color: value ? 'var(--text-primary)' : 'var(--text-secondary)', transition: 'border-color 0.2s ease', height: '100%'
        }}
      >
        {selectedLabel}
        <ChevronDown size={16} style={{ transition: 'transform 0.2s ease', transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)' }} />
      </div>

      <div style={{
        position: 'absolute', top: '100%', left: 0, right: 0, marginTop: '0.25rem',
        background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', zIndex: 50, overflow: 'hidden',
        opacity: isOpen ? 1 : 0, transform: isOpen ? 'translateY(0)' : 'translateY(-10px)',
        pointerEvents: isOpen ? 'auto' : 'none', transition: 'all 0.2s ease', boxShadow: '0 4px 12px rgba(0,0,0,0.5)'
      }}>
        {options.map((opt) => (
          <div
            key={opt.value}
            onClick={() => { onChange(opt.value); setIsOpen(false); }}
            style={{ padding: '0.5rem 0.75rem', fontSize: '0.875rem', cursor: 'pointer', color: value === opt.value ? 'var(--green)' : 'var(--text-primary)', background: value === opt.value ? 'rgba(34, 197, 94, 0.1)' : 'transparent', transition: 'background 0.1s' }}
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

export default function Rules() {
  const [rules, setRules] = useState([]);
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState(null);
  const [toast, setToast] = useState(null);

  // Form State
  const [name, setName] = useState('');
  const [pattern, setPattern] = useState('');
  const [level, setLevel] = useState('');
  const [service, setService] = useState('');
  const [cooldown, setCooldown] = useState(5);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const showToast = useCallback((message, type = "success") => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

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
    } finally { setLoading(false); }
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
    const newRule = { name, pattern, level_filter: level || null, service_filter: service || null, cooldown_minutes: parseInt(cooldown, 10), is_active: true };

    try {
      const res = await apiFetch('/api/rules', { method: 'POST', body: JSON.stringify(newRule) });
      if (res.ok) {
        showToast("Rule created successfully", "success");
        setName(''); setPattern(''); setLevel(''); setService(''); setCooldown(5);
        fetchRules();
      } else {
        showToast("Failed to create rule", "error");
      }
    } catch (err) {
      showToast("Network error", "error");
    } finally { setIsSubmitting(false); }
  };

  const handleToggleActive = async (rule) => {
    setRules(rules.map(r => r.id === rule.id ? { ...r, is_active: !r.is_active } : r));
    try {
      const res = await apiFetch(`/api/rules/${rule.id}`, { method: 'PUT', body: JSON.stringify({ ...rule, is_active: !rule.is_active }) });
      if (res.ok) showToast(`Rule ${!rule.is_active ? 'enabled' : 'disabled'}`, "success");
      else throw new Error();
    } catch (err) {
      setRules(rules.map(r => r.id === rule.id ? { ...r, is_active: rule.is_active } : r));
      showToast("Failed to update rule", "error");
    }
  };

  const handleDelete = async (id) => {
    if (deletingId !== id) {
      setDeletingId(id);
      setTimeout(() => setDeletingId(null), 3000);
      return;
    }
    try {
      const res = await apiFetch(`/api/rules/${id}`, { method: 'DELETE' });
      if (res.ok) {
        setRules(rules.filter(r => r.id !== id));
        showToast("Rule deleted", "success");
      } else showToast("Failed to delete rule", "error");
    } catch (err) { showToast("Network error", "error"); }
    setDeletingId(null);
  };

  const serviceOptions = [{ value: '', label: 'Any Service' }, ...services.map(s => ({ value: s, label: s }))];
  const levelOptions = [ { value: '', label: 'Any Level' }, { value: 'ERROR', label: 'ERROR' }, { value: 'FATAL', label: 'FATAL' }, { value: 'WARN', label: 'WARN' }, { value: 'INFO', label: 'INFO' } ];

  return (
    <div style={{ padding: '2rem', maxWidth: '1400px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '2rem' }}>
      {toast && (
        <div style={{ position: 'fixed', bottom: '2rem', right: '2rem', zIndex: 100, background: 'var(--surface)', border: `1px solid ${toast.type === 'success' ? 'var(--green)' : 'var(--red)'}`, color: 'var(--text-primary)', padding: '1rem 1.5rem', borderRadius: 'var(--radius)', fontFamily: 'var(--font-sans)', fontWeight: 500, boxShadow: '0 8px 16px rgba(0,0,0,0.5)', animation: 'fadeInUp 0.3s ease forwards' }}>
          {toast.message}
        </div>
      )}

      <h1 style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-primary)', margin: 0, display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
        <ShieldAlert size={24} color="var(--cyan)" /> Alert Rules Configuration
      </h1>

      <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
        {/* Full Width Form */}
        <form onSubmit={handleCreate} style={{ padding: '1.5rem', borderBottom: '1px solid var(--border)', background: 'var(--bg)', display: 'grid', gridTemplateColumns: '1fr 2fr', gap: '1.5rem', alignItems: 'start' }}>
          
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
            <input required type="text" placeholder="Rule Name (e.g. DB Crash)" value={name} onChange={e => setName(e.target.value)} />
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <CustomSelect value={level} onChange={setLevel} options={levelOptions} placeholder="Any Level" />
              <CustomSelect value={service} onChange={setService} options={serviceOptions} placeholder="Any Service" />
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
             <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
                <input required type="text" placeholder="Regex Pattern (e.g. timeout|refused)" value={pattern} onChange={e => setPattern(e.target.value)} style={{ fontFamily: 'var(--font-mono)' }} />
                <span style={{ fontSize: '0.75rem', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)' }}>Pattern matched against log messages</span>
             </div>
             <div style={{ display: 'flex', gap: '1rem', justifyContent: 'flex-end' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>Cooldown (min):</span>
                  <input type="number" min="0" placeholder="0" value={cooldown} onChange={e => setCooldown(e.target.value)} style={{ width: '80px' }} />
                </div>
                <button type="submit" disabled={isSubmitting} style={{ background: 'var(--green)', color: '#000', border: 'none', padding: '0 2rem', borderRadius: 'var(--radius)', cursor: isSubmitting ? 'not-allowed' : 'pointer', fontWeight: 600 }}>
                  {isSubmitting ? '...' : <><Plus size={16} style={{ verticalAlign: 'middle', marginRight: '4px' }} /> Create Rule</>}
                </button>
             </div>
          </div>
        </form>

        {/* Full Width Table */}
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '0.85rem' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                <th style={{ padding: '1rem 1.5rem' }}>Rule Name</th>
                <th style={{ padding: '1rem 1.5rem' }}>Regex Pattern</th>
                <th style={{ padding: '1rem 1.5rem' }}>Level</th>
                <th style={{ padding: '1rem 1.5rem' }}>Service</th>
                <th style={{ padding: '1rem 1.5rem', width: '100px' }}>Cooldown</th>
                <th style={{ padding: '1rem 1.5rem', textAlign: 'center', width: '120px' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={6} style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-dim)' }}>Loading...</td></tr>
              ) : rules.length === 0 ? (
                <tr><td colSpan={6} style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-dim)' }}>No alert rules configured.<span className="blink">_</span></td></tr>
              ) : (
                rules.map(rule => (
                  <tr key={rule.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', transition: 'background 0.2s' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--surface-hover)'} onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-primary)', fontWeight: 600 }}>{rule.name}</td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--cyan)' }}>{rule.pattern}</td>
                    <td style={{ padding: '1rem 1.5rem' }}>{rule.level_filter ? <span className={`badge badge-${rule.level_filter.toLowerCase()}`}>{rule.level_filter}</span> : <span style={{ color: 'var(--text-dim)' }}>*</span>}</td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-secondary)' }}>{rule.service_filter || '*'}</td>
                    <td style={{ padding: '1rem 1.5rem', color: 'var(--text-secondary)' }}>{rule.cooldown_minutes}m</td>
                    <td style={{ padding: '1rem 1.5rem', textAlign: 'center' }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '1rem' }}>
                        <button onClick={() => handleToggleActive(rule)} style={{ background: 'transparent', border: 'none', cursor: 'pointer', padding: 0 }} title="Toggle Active">
                          {rule.is_active ? <CheckCircle size={20} color="var(--green)" /> : <XCircle size={20} color="var(--text-dim)" />}
                        </button>
                        <button onClick={() => handleDelete(rule.id)} style={{ background: deletingId === rule.id ? 'var(--red)' : 'transparent', color: deletingId === rule.id ? '#000' : 'var(--text-dim)', border: 'none', cursor: 'pointer', padding: '4px 8px', borderRadius: '4px', fontSize: '0.75rem', transition: 'all 0.2s' }}>
                          {deletingId === rule.id ? "Confirm?" : <Trash2 size={18} />}
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
    </div>
  );
}