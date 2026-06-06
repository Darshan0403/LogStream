// src/components/DataRain.jsx
import { useEffect, useRef } from 'react';

const CHAR_POOLS = {
  green: ['200', 'OK', 'GET', 'POST', 'null', 'true', '0', '1', 'ACK'],
  amber: ['429', 'timeout', 'retry', 'WARN', 'slow', '408', 'wait'],
  red:   ['500', 'ERROR', 'FATAL', 'panic', 'refused', '503', 'FAIL'],
  cyan:  ['INFO', 'DEBUG', 'trace', 'ping', '←', '→', 'log', '::'],
};

const COLOR_MAP = {
  green: '#22c55e',
  amber: '#f59e0b',
  red: '#ef4444',
  cyan: '#06b6d4'
};

export default function DataRain() {
  const canvasRef = useRef(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas.getContext('2d');
    let animationFrameId;
    let mouse = { x: -1000, y: -1000 };

    const handleMouseMove = (e) => {
      mouse.x = e.clientX;
      mouse.y = e.clientY;
    };
    window.addEventListener('mousemove', handleMouseMove);

    let columns = [];
    const FONT_SIZE = 14;

    // Helper to distribute colors: 70% Green, 10% Amber, 10% Red, 10% Cyan
    const getWeightedColor = () => {
      const rand = Math.random();
      if (rand < 0.70) return 'green';
      if (rand < 0.80) return 'amber';
      if (rand < 0.90) return 'red';
      return 'cyan';
    };

    const initCanvas = () => {
      canvas.width = window.innerWidth;
      canvas.height = window.innerHeight;
      
      const colCount = Math.floor(canvas.width / FONT_SIZE);
      columns = [];

      for (let i = 0; i < colCount; i++) {
        const colorKey = getWeightedColor(); // Using the new weighted logic
        
        columns.push({
          x: i * FONT_SIZE,
          y: Math.random() * canvas.height,
          speed: 0.5 + Math.random() * 1.5,
          colorKey: colorKey,
          pool: CHAR_POOLS[colorKey],
          char: CHAR_POOLS[colorKey][Math.floor(Math.random() * CHAR_POOLS[colorKey].length)]
        });
      }
    };

    window.addEventListener('resize', initCanvas);
    initCanvas();

    const draw = () => {
      // Semi-transparent black to create the trail effect
      ctx.fillStyle = 'rgba(0, 0, 0, 0.08)';
      ctx.fillRect(0, 0, canvas.width, canvas.height);

      ctx.font = `${FONT_SIZE}px "JetBrains Mono"`;

      columns.forEach((col) => {
        // Calculate distance from mouse
        const dx = mouse.x - col.x;
        const dy = mouse.y - col.y;
        const distance = Math.sqrt(dx * dx + dy * dy);
        
        let opacity = 0.08;
        let isBold = false;

        // Proximity effect
        if (distance < 100) {
          opacity = 0.6 - (distance / 100) * 0.3;
          isBold = true;
        }

        ctx.fillStyle = `${COLOR_MAP[col.colorKey]}${Math.floor(opacity * 255).toString(16).padStart(2, '0')}`;
        ctx.font = `${isBold ? 'bold ' : ''}${FONT_SIZE}px "JetBrains Mono"`;
        ctx.fillText(col.char, col.x, col.y);

        col.y += col.speed;

        // Randomly change character (stays within its assigned color pool)
        if (Math.random() > 0.98) {
          col.char = col.pool[Math.floor(Math.random() * col.pool.length)];
        }

        // Reset to top
        if (col.y > canvas.height) {
          col.y = 0;
          col.speed = 0.5 + Math.random() * 1.5;
        }
      });

      animationFrameId = requestAnimationFrame(draw);
    };

    draw();

    return () => {
      window.removeEventListener('resize', initCanvas);
      window.removeEventListener('mousemove', handleMouseMove);
      cancelAnimationFrame(animationFrameId);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        width: '100vw',
        height: '100vh',
        zIndex: -1,
        pointerEvents: 'none',
      }}
    />
  );
}