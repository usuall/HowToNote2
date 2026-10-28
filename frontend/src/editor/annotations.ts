// 그리기 객체(annotations_json)와 보기 설정(view_json)의 형식, 새 객체 기본값, 읽기 검사. 형식은 스펙 5.2절 그대로이고 좌표는 원본 이미지의 물리 픽셀이다.
export type AnnotationType = 'marker' | 'arrow' | 'rect' | 'ellipse' | 'text' | 'callout' | 'number' | 'blur' | 'highlight'

export type Style = { stroke: string; fill: string; width: number; opacity: number; font_size: number }

// x, y, w, h는 객체를 감싸는 상자다. 화살표만 points(시작 x, y, 끝 x, y)로 그리고 상자는 points의 경계다.
export type Annotation = {
  id: string
  type: AnnotationType
  x: number
  y: number
  w: number
  h: number
  points?: number[]
  text?: string
  style: Style
}

export type Box = { x: number; y: number; w: number; h: number }
export type View = { crop?: Box }
export type Point = { x: number; y: number }

export const TYPES: AnnotationType[] = ['marker', 'arrow', 'rect', 'ellipse', 'text', 'callout', 'number', 'blur', 'highlight']

const RED = '#E5383B'
const YELLOW = '#FFE14D'
const DRAG_MIN = 4 // 이보다 짧게 끌면 클릭으로 보고 기본 크기로 만든다(이미지 px)

// parseAnnotations는 저장된 JSON을 읽는다. 형식이 틀린 항목은 버려, 손상된 값 하나로 편집기 전체가 멈추지 않게 한다.
export function parseAnnotations(json: string): Annotation[] {
  let raw: unknown
  try {
    raw = JSON.parse(json || '[]')
  } catch {
    return []
  }
  if (!Array.isArray(raw)) return []
  return raw.filter(
    (a): a is Annotation =>
      !!a &&
      typeof a.id === 'string' &&
      TYPES.includes(a.type) &&
      [a.x, a.y, a.w, a.h].every(Number.isFinite) &&
      !!a.style &&
      typeof a.style === 'object',
  )
}

export function parseView(json: string): View {
  try {
    const v = JSON.parse(json || '{}')
    const c = v?.crop
    if (c && [c.x, c.y, c.w, c.h].every(Number.isFinite) && c.w > 0 && c.h > 0) return { crop: c }
  } catch {
    // 깨진 값은 자르기 없음으로 본다
  }
  return {}
}

// dpiScale은 capture_json의 dpi_scale이다(새 객체 기본 크기에 쓴다). 없거나 깨졌으면 0이고, 그때는 그림 크기로 정한다.
export function dpiScale(captureJson: string): number {
  try {
    const v = JSON.parse(captureJson || '{}')?.dpi_scale
    return typeof v === 'number' && v > 0 ? v : 0
  } catch {
    return 0
  }
}

// clampCrop은 자르기 상자를 이미지 안으로 줄인다. 없거나 비면 이미지 전체다.
export function clampCrop(crop: Box | undefined, width: number, height: number): Box {
  if (!crop) return { x: 0, y: 0, w: width, h: height }
  const x = Math.max(0, Math.min(Math.round(crop.x), width - 1))
  const y = Math.max(0, Math.min(Math.round(crop.y), height - 1))
  const w = Math.max(1, Math.min(Math.round(crop.w), width - x))
  const h = Math.max(1, Math.min(Math.round(crop.h), height - y))
  return { x, y, w, h }
}

// boxOf는 두 점이 만드는 상자다(어느 방향으로 끌어도 w, h가 양수).
export function boxOf(a: Point, b: Point): Box {
  return {
    x: Math.round(Math.min(a.x, b.x)),
    y: Math.round(Math.min(a.y, b.y)),
    w: Math.round(Math.abs(b.x - a.x)),
    h: Math.round(Math.abs(b.y - a.y)),
  }
}

function style(stroke: string, fill: string, width: number, opacity = 1, font_size = 0): Style {
  return { stroke, fill, width: Math.max(1, Math.round(width)), opacity, font_size: Math.round(font_size) }
}

// nextNumber는 번호 도구가 붙일 다음 번호다(지금 있는 번호 중 가장 큰 값 + 1).
export function nextNumber(list: Annotation[]): string {
  const nums = list.filter((a) => a.type === 'number').map((a) => parseInt(a.text ?? '', 10) || 0)
  return String(Math.max(0, ...nums) + 1)
}

// create는 도구로 그린 새 객체다. from은 누른 곳, to는 뗀 곳(이미지 px), unit은 DPI 배율(capture_json의 dpi_scale)이다.
// 끄는 도구를 클릭만 하면 그 자리에 기본 크기로 만든다. 비숙련 사용자가 끌지 않고 눌러도 무언가 생기게 하려는 것이다.
export function create(type: AnnotationType, from: Point, to: Point, unit: number, list: Annotation[]): Annotation {
  const u = unit > 0 ? unit : 1
  const id = crypto.randomUUID()
  const dragged = Math.hypot(to.x - from.x, to.y - from.y) >= DRAG_MIN
  const around = (w: number, h: number): Box => ({
    x: Math.round(from.x - w / 2),
    y: Math.round(from.y - h / 2),
    w: Math.round(w),
    h: Math.round(h),
  })
  const box = dragged ? boxOf(from, to) : around(160 * u, 90 * u)
  switch (type) {
    case 'marker':
      return { id, type, ...around(48 * u, 48 * u), style: style(RED, RED, 3 * u, 0.35) }
    case 'number':
      return { id, type, ...around(36 * u, 36 * u), text: nextNumber(list), style: style(RED, RED, 1, 1, 20 * u) }
    case 'text':
      return {
        id, type, x: Math.round(from.x), y: Math.round(from.y), w: Math.round(240 * u), h: Math.round(34 * u),
        text: '글자', style: style(RED, '', 1, 1, 24 * u),
      }
    case 'callout': {
      // 꼬리 끝(상자 왼쪽 20%, 아래로 높이의 35%)이 누른 곳에 오게 상자를 놓는다(render.ts calloutTip과 같은 비율)
      const w = 220 * u
      const h = 64 * u
      return {
        id, type, x: Math.round(from.x - w * 0.2), y: Math.round(from.y - h * 1.35), w: Math.round(w), h: Math.round(h),
        text: '설명을 입력하세요', style: style(RED, '#FFFFFF', 3 * u, 1, 20 * u),
      }
    }
    case 'arrow': {
      // 끌면 누른 곳에서 뗀 곳으로, 클릭만 하면 왼쪽 위에서 누른 곳을 가리킨다
      const [a, b] = dragged ? [from, to] : [{ x: from.x - 80 * u, y: from.y - 80 * u }, from]
      const points = [a.x, a.y, b.x, b.y].map(Math.round)
      return { id, type, ...boxOf(a, b), points, style: style(RED, RED, 4 * u) }
    }
    case 'rect':
    case 'ellipse':
      return { id, type, ...box, style: style(RED, '', 4 * u) }
    case 'highlight':
      return { id, type, ...box, style: style('', YELLOW, 1, 0.4) }
    case 'blur':
      return { id, type, ...box, style: style('', '', 12 * u) } // width = 모자이크 칸 크기
  }
}

// recolor는 스타일 막대의 '색'이다. 채움이 주된 색인 객체(클릭 표시, 번호, 형광펜)는 채움도 바꾼다.
export function recolor(a: Annotation, color: string): Annotation {
  const fillToo = a.type === 'marker' || a.type === 'number' || a.type === 'highlight'
  return {
    ...a,
    style: { ...a.style, stroke: a.type === 'highlight' ? '' : color, fill: fillToo ? color : a.style.fill },
  }
}
