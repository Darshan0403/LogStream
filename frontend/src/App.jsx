// src/App.jsx
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import DataRain from './components/DataRain';
import Logs from './pages/Logs';
import LiveTail from './pages/LiveTail';
import Dashboard from './pages/Dashboard';
import Alerts from './pages/Alerts';

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
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <style>{`
        @keyframes fadeInUp {
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>

      <DataRain />
      <Sidebar />

      <main style={{ 
        flex: 1, 
        overflowY: 'scroll', 
        position: 'relative',
        zIndex: 10 
      }}>
        {/* The 'key' forces React to play the animation on every route change */}
        <PageTransition key={location.pathname}>
          <Routes location={location}>
            <Route path="/" element={<Navigate to="/logs" replace />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/tail" element={<LiveTail />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/alerts" element={<Alerts />} />
          </Routes>
        </PageTransition>
      </main>
    </div>
  );
}