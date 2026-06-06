// src/pages/Dashboard.jsx
import Stats from '../components/Stats';

export default function Dashboard() {
  return (
    <div style={{ padding: '2rem', maxWidth: '1600px', margin: '0 auto' }}>
      <h1 style={{ fontFamily: 'var(--font-mono)', marginBottom: '2rem', color: 'var(--text-primary)' }}>
        {'>_'} Analytics Dashboard
      </h1>
      
      <Stats />
    </div>
  );
}