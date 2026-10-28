// EnsureRendered 테스트: 캐시에 없는 스텝만 그리고, Go가 준 키로 올리고, 진행을 알린다. 바인딩·렌더러는 가짜로 바꾼다.
import { beforeEach, expect, test, vi } from 'vitest'

const calls: string[] = []
const MARKER = '[{"id":"m","type":"marker","x":1,"y":1,"w":4,"h":4,"style":{"stroke":"#E5383B","fill":"#E5383B","width":1,"opacity":0.35,"font_size":0}}]'
vi.mock('../../wailsjs/go/main/App', () => ({
  ExportPlan: vi.fn(async () => [
    { stepId: 1, imageSha: 'aa', annotationsJson: MARKER, viewJson: '{}', key: 'k1', cached: false },
    { stepId: 2, imageSha: 'bb', annotationsJson: '[]', viewJson: '{}', key: 'k2', cached: true },
    { stepId: 3, imageSha: 'cc', annotationsJson: '[]', viewJson: '{"crop":{"x":1,"y":2,"w":3,"h":4}}', key: 'k3', cached: false },
  ]),
  PutRendered: vi.fn(async (key: string, data: string) => {
    calls.push(`put ${key} ${data}`)
  }),
}))
vi.mock('../editor/render', () => ({
  loadImage: vi.fn(async (url: string) => {
    calls.push(`load ${url}`)
    return { width: 10, height: 10, url }
  }),
  renderPNG: vi.fn(async (img: { url: string }, a: unknown[], v: { crop?: object }) => {
    calls.push(`render ${img.url} ${a.length} ${v.crop ? 'crop' : 'full'}`)
    return new Blob(['png'])
  }),
}))
vi.mock('../api', () => ({ fileBase64: vi.fn(async () => 'BASE64') }))

const { ensureRendered } = await import('./ensure')
const app = await import('../../wailsjs/go/main/App')

beforeEach(() => {
  calls.length = 0
})

test('캐시에 없는 스텝만 원본 그림을 읽어 그리고 Go 키로 올린다', async () => {
  const progress: string[] = []
  await ensureRendered(7, (d, t) => progress.push(`${d}/${t}`))
  expect(app.ExportPlan).toHaveBeenCalledWith(7)
  expect(calls).toEqual([
    'load /img/aa.png', 'render /img/aa.png 1 full', 'put k1 BASE64',
    'load /img/cc.png', 'render /img/cc.png 0 crop', 'put k3 BASE64',
  ])
  expect(progress).toEqual(['0/2', '1/2', '2/2'])
})

test('올리기가 실패하면 멈추고 오류를 그대로 넘긴다', async () => {
  vi.mocked(app.PutRendered).mockRejectedValueOnce('저장하지 못했습니다.')
  await expect(ensureRendered(7)).rejects.toBe('저장하지 못했습니다.')
  expect(calls).toEqual(['load /img/aa.png', 'render /img/aa.png 1 full'])
})

test('plan을 주면 그 목록으로 준비한다(배포용 .htn 그림)', async () => {
  const plan = vi.fn(async () => [{ stepId: 9, imageSha: 'dd', annotationsJson: '[]', viewJson: '{}', key: 'k9', cached: false }])
  await ensureRendered(7, () => {}, plan as never)
  expect(plan).toHaveBeenCalledWith(7)
  expect(calls).toEqual(['load /img/dd.png', 'render /img/dd.png 0 full', 'put k9 BASE64'])
})
