// src/App.jsx
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import DataRain from './components/DataRain';
import Logs from './pages/Logs';
import LiveTail from './pages/LiveTail';
import Dashboard from './pages/Dashboard';
import Alerts from './pages/Alerts';
import Rules from './pages/Rules';

// --- Smooth Page Transition Wrapper ---
const PageTransition = ({ children }) => {
  return (
    <div style={{ 
      animation: 'fadeInUp 0.3s cubic-bezier(0.16, 1, 0.3, 1) forwards',
      opacity: 0,
      transform: 'translateY(100px)',
      height: '100%',
      display: 'flex',
      flexDirection: 'column'
    }}>
      {children}
    </div>
  );
};

const Placeholder = ({ title }) => (
  <div style={{ padding: '2rem', fontFamily: 'var(--font-mono)' }}>
    <h2 style={{ color: 'var(--green)' }}>{title}</h2>
    <p style={{ color: 'var(--text-secondary)', marginTop: '1rem' }}>
      Component under construction...
    </p>
  </div>
);

export default function App() {
  const location = useLocation(); // Hook into the current route

  return (
    // FIX: Changed minHeight to fixed height and locked overflow
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <style>{`
        @keyframes fadeInUp {
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>

      <DataRain />
      <Sidebar />

      <main style={{ 
        flex: 1, 
        overflowY: 'auto', // FIX: Main handles its own scrolling now
        position: 'relative',
        zIndex: 10 
      }}>
        <PageTransition key={location.pathname}>
          <Routes location={location}>
            <Route path="/" element={<Navigate to="/logs" replace />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/tail" element={<LiveTail />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/alerts" element={<Alerts />} />
            <Route path="/rules" element={<Rules />} />
          </Routes>
        </PageTransition>
      </main>
    </div>
  );
}