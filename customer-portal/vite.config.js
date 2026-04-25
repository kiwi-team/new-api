import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import path from 'path';

export default defineConfig({
  base: '/portal/',
  plugins: [react()],
  resolve: {
    alias: {
      // Bypass package.json exports restriction for Semi Design CSS
      '@douyinfe/semi-ui/dist/css/semi.css': path.resolve(
        __dirname,
        'node_modules/@douyinfe/semi-ui/dist/css/semi.css'
      ),
    },
  },
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      },
    },
  },
});
