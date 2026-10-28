import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  // 내려받지 않는 로컬 앱이라 번들 크기 경고(기본 500kB)는 의미가 없다. Konva·Tiptap을 넣은 크기에 맞춰 올린다.
  build: { chunkSizeWarningLimit: 2000 },
  // vitest에서는 svelte가 서버용 빌드로 풀려 mount를 못 쓴다(lifecycle_function_unavailable). 테스트 때만 브라우저용으로 푼다.
  ...(process.env.VITEST ? { resolve: { conditions: ['browser'] } } : {}),
})
