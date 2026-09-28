import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// В разработке запросы /api проксируются на Go-бэкенд (HTTP_ADDR=:8080 из .env.example).
export default defineConfig({
  plugins: [react()],
  server: {
    // Туннель ngrok для открытия мини-приложения из MAX; точка в начале разрешает все поддомены.
    allowedHosts: ['.ngrok-free.dev', '.ngrok-free.app'],
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
