import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Dev proxy mirrors the nginx reverse-proxy used in production (services/web/nginx.conf):
// same-origin /v1, /ws and /s3 so the browser never hits CORS and MinIO presigned
// URLs (signed for Host "minio:9000") resolve through us.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/v1': { target: 'http://localhost:8080', changeOrigin: true },
      '/healthz': { target: 'http://localhost:8080', changeOrigin: true },
      '/ws': { target: 'ws://localhost:8090', ws: true },
      '/s3': {
        target: 'http://localhost:9000',
        changeOrigin: false,
        rewrite: (p) => p.replace(/^\/s3/, ''),
        configure: (proxy) => {
          // Presigned signature was computed against Host "minio:9000".
          proxy.on('proxyReq', (proxyReq) => proxyReq.setHeader('host', 'minio:9000'));
        },
      },
    },
  },
});
