// 프론트 공용: 녹화 이벤트 형식(Go recorder.Event와 같은 필드), 오류 문구, 날짜 표시.
export type RecorderEvent = {
  type: 'step' | 'paused' | 'resumed' | 'warning' | 'stopped'
  guideId: number
  stepId?: number
  title?: string
  imageSha?: string
  steps: number
  message?: string
  latencyMs?: number
}

// errorText는 Go가 돌려준 오류(이미 사용자용 한국어 문장)를 꺼낸다. 형식이 다르면 일반 안내로 바꾼다.
export function errorText(e: unknown): string {
  if (typeof e === 'string' && e) return e
  if (e instanceof Error && e.message) return e.message
  return '알 수 없는 문제가 생겼습니다. 프로그램을 다시 실행해 보세요.'
}

// formatDate는 RFC 3339 시각을 사용자 PC의 한국어 날짜·시각으로 보여 준다.
export function formatDate(s: string): string {
  const d = new Date(s)
  return isNaN(d.getTime()) ? '' : d.toLocaleString('ko-KR', { dateStyle: 'medium', timeStyle: 'short' })
}

// fileBase64는 붙여넣거나 고른 그림 파일을 base64(머리말 없이)로 읽는다. Go AddImageStep이 받는 형식이다.
export function fileBase64(f: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).split(',')[1] ?? '')
    r.onerror = () => reject(new Error('그림 파일을 읽지 못했습니다. 다른 그림으로 해 보세요.'))
    r.readAsDataURL(f)
  })
}
