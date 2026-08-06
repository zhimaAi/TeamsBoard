import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { AntDesignVueResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const port = Number(env.VITE_PORT) || 5173
  const proxyTarget = env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:18900'

  return {
    plugins: [
      vue(),
      Components({
        resolvers: [
          AntDesignVueResolver({
            importStyle: false, // css in js
          }),
        ],
      }),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port,
      strictPort: true,
      proxy: {
        '/api/local': {
          target: proxyTarget,
          changeOrigin: true,
          ws: true,
          configure: (proxy) => {
            // Fail quickly when the backend is not started, returning a clear error instead of letting the request hang forever
            proxy.on('error', (err, _req, res) => {
              const message =
                `本地后端未启动 (${proxyTarget})，请先运行 Go 后端：cd .. && go run ./cmd/client`
              console.error('[vite proxy] /api/local 代理失败:', err.code || err.message)
              if (res && typeof res.writeHead === 'function') {
                res.writeHead(502, { 'Content-Type': 'application/json; charset=utf-8' })
                res.end(JSON.stringify({ error: message }))
              }
            })
          },
        },
      },
    },
  }
})
