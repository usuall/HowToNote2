// 내보내기 전 합성 그림 준비(스펙 7.1절 EnsureRendered). 그림이 있는 보이는 스텝마다 편집기와 같은 렌더러(render.ts)로
// 원본 해상도 PNG를 만들어 Go 캐시(cache\rendered)에 올린다. 캐시 키는 Go가 정하고, 이미 있는 그림은 건너뛴다.
// plan에 ProjectPlan을 주면 배포용 .htn 그림(모자이크·자르기 확정, M4a H7)을 같은 길로 준비한다.
import { ExportPlan, PutRendered } from '../../wailsjs/go/main/App'
import { fileBase64 } from '../api'
import { parseAnnotations, parseView } from '../editor/annotations'
import { loadImage, renderPNG } from '../editor/render'

export async function ensureRendered(
  guideId: number,
  onProgress: (done: number, total: number) => void = () => {},
  plan: typeof ExportPlan = ExportPlan,
): Promise<void> {
  const todo = ((await plan(guideId)) ?? []).filter((j) => !j.cached)
  onProgress(0, todo.length)
  for (const [i, j] of todo.entries()) {
    const img = await loadImage(`/img/${j.imageSha}.png`)
    const png = await renderPNG(img, parseAnnotations(j.annotationsJson), parseView(j.viewJson))
    await PutRendered(j.key, await fileBase64(png))
    onProgress(i + 1, todo.length)
  }
}
