// 합성 렌더러 테스트: Node에서 node-canvas로 실제 픽셀을 그려 본다(원본 해상도, 자르기, 사각형, 모자이크, 형광펜, 되읽기).
import 'konva/canvas-backend'
import Konva from 'konva'
import { createCanvas } from 'canvas'
import { describe, expect, test } from 'vitest'
import type { Annotation } from './annotations'
import { nodeFor, readBack, renderCanvas, type Source } from './render'

// source는 200x100 원본이다: 왼쪽 절반은 1px 흑백 바둑판, 오른쪽 절반은 흰색.
function source(): Source {
  const c = createCanvas(200, 100)
  const ctx = c.getContext('2d')
  ctx.fillStyle = '#FFFFFF'
  ctx.fillRect(0, 0, 200, 100)
  ctx.fillStyle = '#000000'
  for (let y = 0; y < 100; y++) for (let x = 0; x < 100; x++) if ((x + y) % 2) ctx.fillRect(x, y, 1, 1)
  return c as unknown as Source
}

function pixel(c: HTMLCanvasElement, x: number, y: number): number[] {
  return Array.from(c.getContext('2d')!.getImageData(x, y, 1, 1).data)
}

const style = { stroke: '#FF0000', fill: '', width: 4, opacity: 1, font_size: 0 }
const ann = (a: Partial<Annotation>): Annotation => ({ id: 'a', type: 'rect', x: 0, y: 0, w: 10, h: 10, style, ...a })

describe('renderCanvas', () => {
  test('자르기가 없으면 원본 크기 그대로다', () => {
    const src = source()
    const out = renderCanvas(src, [], {})
    expect([out.width, out.height]).toEqual([200, 100])
    expect(pixel(out, 1, 0)).toEqual([0, 0, 0, 255])
    expect(pixel(out, 150, 50)).toEqual([255, 255, 255, 255])
  })

  test('자르기는 그 영역만 원본 해상도로 남긴다', () => {
    const out = renderCanvas(source(), [], { crop: { x: 99, y: 10, w: 50, h: 20 } })
    expect([out.width, out.height]).toEqual([50, 20])
    expect(pixel(out, 0, 0)).toEqual([0, 0, 0, 255]) // 원본 (99, 10): 99+10 홀수 → 검정
    expect(pixel(out, 10, 5)).toEqual([255, 255, 255, 255]) // 원본 (109, 15): 오른쪽 흰 영역
  })

  test('자르기 상자가 이미지를 넘으면 이미지 안으로 줄인다', () => {
    const out = renderCanvas(source(), [], { crop: { x: 180, y: 90, w: 100, h: 100 } })
    expect([out.width, out.height]).toEqual([20, 10])
  })

  test('사각형은 테두리만 그리고 안쪽은 원본이다', () => {
    const out = renderCanvas(source(), [ann({ x: 120, y: 20, w: 40, h: 40 })], {})
    expect(pixel(out, 120, 40)).toEqual([255, 0, 0, 255])
    expect(pixel(out, 140, 40)).toEqual([255, 255, 255, 255])
  })

  test('모자이크는 원본에서 칸마다 평균색을 만들고 원본은 바꾸지 않는다', () => {
    const src = source()
    const out = renderCanvas(src, [ann({ type: 'blur', x: 0, y: 0, w: 40, h: 40, style: { ...style, width: 10 } })], {})
    const [r, g, b, a] = pixel(out, 5, 5)
    expect(Math.abs(r - 128)).toBeLessThanOrEqual(2) // 흑백 바둑판의 평균은 회색
    expect([g, b, a]).toEqual([r, r, 255])
    expect(pixel(out, 1, 1)).toEqual(pixel(out, 8, 8)) // 한 칸 안은 같은 색
    expect(pixel(out, 41, 0)).toEqual([0, 0, 0, 255]) // 영역 밖은 원본 그대로
    expect(pixel(src as unknown as HTMLCanvasElement, 1, 0)).toEqual([0, 0, 0, 255]) // 원본 이미지는 그대로
  })

  test('모자이크는 먼저 그린 객체를 덮고 원본 픽셀로 칠한다', () => {
    const red = ann({ id: 'r', x: 110, y: 10, w: 60, h: 60, style: { ...style, fill: '#FF0000' } })
    const blur = ann({ id: 'b', type: 'blur', x: 120, y: 20, w: 20, h: 20, style: { ...style, width: 10 } })
    const out = renderCanvas(source(), [red, blur], {})
    expect(pixel(out, 125, 25)).toEqual([255, 255, 255, 255]) // 원본(흰색)에서 만든 모자이크
    expect(pixel(out, 150, 50)).toEqual([255, 0, 0, 255])
  })

  test('형광펜은 흰 바탕을 노랗게 비춘다', () => {
    const hl = ann({ type: 'highlight', x: 120, y: 20, w: 40, h: 20, style: { stroke: '', fill: '#FFE14D', width: 1, opacity: 1, font_size: 0 } })
    const [r, g, b] = pixel(renderCanvas(source(), [hl], {}), 130, 30)
    expect([r, g, b]).toEqual([255, 225, 77])
  })

  test('모든 종류가 오류 없이 그려진다', () => {
    const all: Annotation[] = [
      ann({ id: '1', type: 'marker', x: 10, y: 10, w: 48, h: 48, style: { ...style, fill: '#FF0000', opacity: 0.35 } }),
      ann({ id: '2', type: 'arrow', points: [10, 10, 90, 60] }),
      ann({ id: '3', type: 'ellipse', x: 20, y: 20, w: 50, h: 30 }),
      ann({ id: '4', type: 'text', x: 5, y: 5, w: 120, h: 30, text: '글자', style: { ...style, font_size: 20 } }),
      ann({ id: '5', type: 'callout', x: 100, y: 5, w: 90, h: 40, text: '말풍선', style: { ...style, fill: '#FFFFFF', font_size: 14 } }),
      ann({ id: '6', type: 'number', x: 150, y: 60, w: 30, h: 30, text: '1', style: { ...style, fill: '#FF0000' } }),
    ]
    const out = renderCanvas(source(), all, {})
    expect(out.width).toBe(200)
    expect(pixel(out, 165, 75)[0]).toBeGreaterThan(200) // 번호 원은 빨갛다(가운데 흰 글자 옆)
  })
})

describe('readBack', () => {
  test('상자 객체는 이동과 배율을 x, y, w, h로 되돌린다', () => {
    const a = ann({ x: 10, y: 20, w: 30, h: 40 })
    const n = nodeFor(a, source())
    n.position({ x: 15, y: 25 })
    n.scale({ x: 2, y: 0.5 })
    expect(readBack(a, n)).toMatchObject({ x: 15, y: 25, w: 60, h: 20 })
  })

  test('화살표는 점을 옮기고 상자를 다시 계산한다', () => {
    const a = ann({ type: 'arrow', points: [10, 10, 50, 30], x: 10, y: 10, w: 40, h: 20 })
    const n = nodeFor(a, source()) as Konva.Arrow
    n.position({ x: 5, y: -5 })
    expect(readBack(a, n)).toMatchObject({ points: [15, 5, 55, 25], x: 15, y: 5, w: 40, h: 20 })
  })
})
