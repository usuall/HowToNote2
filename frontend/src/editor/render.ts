// 합성 이미지 렌더러(스펙 3.2절 1). 원본 이미지 + 그리기 객체 + 자르기를 원본 해상도의 Konva 장면으로 만든다.
// 편집기 캔버스(Canvas.svelte)와 내보내기(M3 EnsureRendered)가 같은 nodeFor·addScene을 써서 화면과 결과물이 같다.
// DOM 컴포넌트에 묶이지 않아 화면 없이(headless) 부를 수 있고, 테스트는 Node에서 konva/canvas-backend로 돌린다.
// 그림 결과가 바뀌면 internal/export/doc.go의 renderVersion을 올린다.
import Konva from 'konva'
import { clampCrop, type Annotation, type View } from './annotations'

// Source는 원본 이미지다. 브라우저에서는 로드가 끝난 HTMLImageElement, 테스트에서는 node-canvas 캔버스다.
export type Source = CanvasImageSource & { width: number; height: number }

const FONT = "'Malgun Gothic', '맑은 고딕', sans-serif"

// calloutTip은 말풍선 꼬리 끝의 상자 안 좌표다. 상자 크기에서 정해서 옮기거나 늘려도 따라온다.
export function calloutTip(w: number, h: number) {
  return { x: w * 0.2, y: h * 1.35 }
}

// nodeFor는 그리기 객체 하나를 Konva 노드로 만든다. 화살표는 Konva.Arrow(점 좌표 그대로), 나머지는 (x, y)에 놓인
// Group이고 자식은 (0, 0)~(w, h) 안에 그린다. 그래서 끌기·크기 바꾸기 결과를 readBack 하나로 되읽는다.
export function nodeFor(a: Annotation, src: Source): Konva.Group | Konva.Arrow {
  const s = a.style
  if (a.type === 'arrow') {
    const p = a.points ?? [a.x, a.y, a.x + a.w, a.y + a.h]
    return new Konva.Arrow({
      id: a.id, name: 'annotation', points: p, stroke: s.stroke, fill: s.stroke, strokeWidth: s.width,
      pointerLength: s.width * 4, pointerWidth: s.width * 4, lineCap: 'round', opacity: s.opacity,
      hitStrokeWidth: Math.max(s.width, 16),
    })
  }
  const g = new Konva.Group({ id: a.id, name: 'annotation', x: a.x, y: a.y, opacity: s.opacity })
  const { w, h } = a
  const line = { stroke: s.stroke || undefined, strokeWidth: s.width, hitStrokeWidth: Math.max(s.width, 16) }
  switch (a.type) {
    case 'rect':
      g.add(new Konva.Rect({ width: w, height: h, fill: s.fill || undefined, ...line }))
      break
    case 'ellipse':
      g.add(new Konva.Ellipse({ x: w / 2, y: h / 2, radiusX: w / 2, radiusY: h / 2, fill: s.fill || undefined, ...line }))
      break
    case 'marker':
      g.add(new Konva.Ellipse({ x: w / 2, y: h / 2, radiusX: w / 2, radiusY: h / 2, fill: s.fill, ...line }))
      break
    case 'highlight':
      // 곱하기 합성: 형광펜처럼 아래 글자가 어두워지지 않고 비친다
      g.add(new Konva.Rect({ width: w, height: h, fill: s.fill, globalCompositeOperation: 'multiply' }))
      break
    case 'number':
      g.add(new Konva.Ellipse({ x: w / 2, y: h / 2, radiusX: w / 2, radiusY: h / 2, fill: s.fill }))
      g.add(new Konva.Text({
        width: w, height: h, text: a.text ?? '', fontSize: Math.round(h * 0.6), fontFamily: FONT, fontStyle: 'bold',
        fill: '#FFFFFF', align: 'center', verticalAlign: 'middle',
      }))
      break
    case 'text':
      if (s.fill) g.add(new Konva.Rect({ width: w, height: h, fill: s.fill }))
      g.add(textNode(a))
      break
    case 'callout': {
      const tip = calloutTip(w, h)
      const r = Math.min(12, h / 4)
      g.add(new Konva.Shape({
        width: w, height: h, fill: s.fill || '#FFFFFF', ...line,
        sceneFunc(ctx, shape) {
          ctx.beginPath()
          ctx.moveTo(r, 0)
          ctx.arcTo(w, 0, w, h, r)
          ctx.arcTo(w, h, 0, h, r)
          ctx.lineTo(w * 0.3, h)
          ctx.lineTo(tip.x, tip.y)
          ctx.lineTo(w * 0.1, h)
          ctx.arcTo(0, h, 0, 0, r)
          ctx.arcTo(0, 0, w, 0, r)
          ctx.closePath()
          ctx.fillStrokeShape(shape)
        },
      }))
      const pad = Math.max(4, s.width * 2)
      g.add(new Konva.Text({
        x: pad, y: pad, width: Math.max(1, w - pad * 2), height: Math.max(1, h - pad * 2), text: a.text ?? '',
        fontSize: s.font_size, fontFamily: FONT, fill: '#1F2329', align: 'center', verticalAlign: 'middle', lineHeight: 1.3,
      }))
      break
    }
    case 'blur': {
      // 원본 이미지에서 그 영역만 잘라 칸마다 평균색으로 칠한다. 원본은 바꾸지 않는다(스펙 5.3절: 확정은 배포용 .htn·내보내기에서만).
      const img = new Konva.Image({
        image: src, width: w, height: h, crop: { x: a.x, y: a.y, width: w, height: h },
        filters: [Konva.Filters.Pixelate], pixelSize: Math.max(2, s.width),
      })
      if (w >= 1 && h >= 1) img.cache({ pixelRatio: 1 })
      g.add(img)
      break
    }
  }
  return g
}

function textNode(a: Annotation): Konva.Text {
  return new Konva.Text({
    width: a.w, text: a.text ?? '', fontSize: a.style.font_size, fontFamily: FONT, fill: a.style.stroke, lineHeight: 1.3,
  })
}

// textHeight는 글자 객체의 실제 높이다. 글자를 고친 뒤 h를 맞출 때 쓴다.
export function textHeight(a: Annotation): number {
  return Math.ceil(textNode(a).height())
}

// readBack은 끌기·크기 바꾸기가 끝난 노드의 위치와 배율을 객체 좌표로 되돌린다. 노드는 바꾸지 않는다(다음 그리기에서 새로 만든다).
export function readBack(a: Annotation, node: Konva.Node): Annotation {
  if (a.type === 'arrow') {
    const t = node.getTransform()
    const p = a.points ?? [a.x, a.y, a.x + a.w, a.y + a.h]
    const s = t.point({ x: p[0], y: p[1] })
    const e = t.point({ x: p[2], y: p[3] })
    const points = [s.x, s.y, e.x, e.y].map(Math.round)
    return {
      ...a, points, x: Math.min(points[0], points[2]), y: Math.min(points[1], points[3]),
      w: Math.abs(points[2] - points[0]), h: Math.abs(points[3] - points[1]),
    }
  }
  return {
    ...a, x: Math.round(node.x()), y: Math.round(node.y()),
    w: Math.max(1, Math.round(a.w * node.scaleX())), h: Math.max(1, Math.round(a.h * node.scaleY())),
  }
}

// addScene은 원본 이미지와 그리기 객체를 이미지 좌표 그대로 parent에 넣는다. 편집기와 내보내기가 같이 쓴다.
export function addScene(parent: Konva.Container, src: Source, annotations: Annotation[]) {
  parent.add(new Konva.Image({ image: src, width: src.width, height: src.height, listening: false }))
  for (const a of annotations) parent.add(nodeFor(a, src))
}

// compose는 화면에 붙지 않은 장면을 만든다. 크기는 자른 영역 그대로(원본 해상도)다. 쓴 뒤 destroy()한다.
export function compose(src: Source, annotations: Annotation[], view: View): Konva.Stage {
  const c = clampCrop(view.crop, src.width, src.height)
  const stage = new Konva.Stage({
    container: typeof document === 'undefined' ? undefined : document.createElement('div'),
    width: c.w,
    height: c.h,
  })
  const layer = new Konva.Layer({ x: -c.x, y: -c.y, listening: false })
  addScene(layer, src, annotations)
  stage.add(layer)
  return stage
}

// renderCanvas는 합성 결과를 원본 해상도 캔버스로 돌려준다(pixelRatio 1: 화면 배율과 상관없이 물리 픽셀 그대로).
export function renderCanvas(src: Source, annotations: Annotation[], view: View): HTMLCanvasElement {
  const stage = compose(src, annotations, view)
  try {
    return stage.toCanvas({ pixelRatio: 1 })
  } finally {
    stage.destroy()
  }
}

// renderPNG는 내보내기용 합성 PNG다. M3의 EnsureRendered가 스텝마다 부른다.
export function renderPNG(src: Source, annotations: Annotation[], view: View): Promise<Blob> {
  const canvas = renderCanvas(src, annotations, view)
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('PNG를 만들지 못했습니다. 다시 시도해 보세요.'))), 'image/png'),
  )
}

// loadImage는 /img/<sha>.png를 읽어 Source로 쓸 수 있게 기다린다.
export function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('그림을 불러오지 못했습니다. 매뉴얼을 닫았다가 다시 열어 보세요.'))
    img.src = url
  })
}
