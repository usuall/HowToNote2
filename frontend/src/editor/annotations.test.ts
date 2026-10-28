// 그리기 객체 형식 테스트: 읽기 검사, 자르기 상자, 도구별 새 객체, 번호, 색 바꾸기.
import { describe, expect, test } from 'vitest'
import { clampCrop, create, dpiScale, nextNumber, parseAnnotations, parseView, recolor, type Annotation } from './annotations'

const style = { stroke: '#E5383B', fill: '#E5383B', width: 3, opacity: 0.35, font_size: 0 }

describe('parseAnnotations', () => {
  test('M1 녹화기가 만든 클릭 표시를 그대로 읽는다', () => {
    const json = `[{"id":"m1","type":"marker","x":44,"y":64,"w":72,"h":72,"style":{"stroke":"#E5383B","fill":"#E5383B","width":5,"opacity":0.35,"font_size":0}}]`
    expect(parseAnnotations(json)).toEqual([{ id: 'm1', type: 'marker', x: 44, y: 64, w: 72, h: 72, style: { ...style, width: 5 } }])
  })

  test('깨진 JSON과 형식이 틀린 항목은 버린다', () => {
    expect(parseAnnotations('{')).toEqual([])
    expect(parseAnnotations('{"a":1}')).toEqual([])
    expect(parseAnnotations('')).toEqual([])
    const ok = { id: 'a', type: 'rect', x: 1, y: 2, w: 3, h: 4, style }
    const json = JSON.stringify([ok, { ...ok, type: 'star' }, { ...ok, x: 'x' }, { ...ok, style: undefined }, null])
    expect(parseAnnotations(json)).toEqual([ok])
  })
})

describe('보기 설정', () => {
  test('parseView는 올바른 자르기만 받는다', () => {
    expect(parseView('{"crop":{"x":1,"y":2,"w":30,"h":40}}')).toEqual({ crop: { x: 1, y: 2, w: 30, h: 40 } })
    expect(parseView('{"crop":{"x":1,"y":2,"w":0,"h":40}}')).toEqual({})
    expect(parseView('{}')).toEqual({})
    expect(parseView('nope')).toEqual({})
  })

  test('dpiScale은 capture_json의 배율이고 없으면 0이다', () => {
    expect(dpiScale('{"point":{"x":1,"y":2},"dpi_scale":1.5}')).toBe(1.5)
    expect(dpiScale('{}')).toBe(0)
    expect(dpiScale('')).toBe(0)
    expect(dpiScale('{')).toBe(0)
  })

  test('clampCrop은 이미지 안으로 줄이고, 없으면 전체다', () => {
    expect(clampCrop(undefined, 200, 100)).toEqual({ x: 0, y: 0, w: 200, h: 100 })
    expect(clampCrop({ x: -10, y: 90.4, w: 500, h: 50 }, 200, 100)).toEqual({ x: 0, y: 90, w: 200, h: 10 })
  })
})

describe('create', () => {
  const at = { x: 500, y: 300 }

  test('클릭 표시는 누른 곳이 가운데이고 크기는 DPI 배율을 따른다(M1 녹화기와 같은 모양)', () => {
    const m = create('marker', at, at, 1.5, [])
    expect(m).toMatchObject({ type: 'marker', x: 464, y: 264, w: 72, h: 72 })
    expect(m.style).toEqual({ stroke: '#E5383B', fill: '#E5383B', width: 5, opacity: 0.35, font_size: 0 })
    expect(m.id).toMatch(/^[0-9a-f-]{36}$/)
  })

  test('끄는 도구는 끈 상자, 클릭만 하면 기본 크기 상자다', () => {
    expect(create('rect', { x: 50, y: 60 }, { x: 10, y: 20 }, 1, [])).toMatchObject({ x: 10, y: 20, w: 40, h: 40 })
    expect(create('ellipse', at, at, 1, [])).toMatchObject({ x: 420, y: 255, w: 160, h: 90 })
    expect(create('blur', at, { x: 501, y: 301 }, 2, [])).toMatchObject({ x: 340, y: 210, w: 320, h: 180, style: { width: 24 } })
    expect(create('highlight', at, { x: 600, y: 320 }, 1, []).style).toMatchObject({ fill: '#FFE14D', opacity: 0.4 })
  })

  test('화살표는 끈 방향, 클릭만 하면 왼쪽 위에서 누른 곳을 가리킨다', () => {
    expect(create('arrow', { x: 100, y: 10 }, { x: 20, y: 50 }, 1, [])).toMatchObject({
      points: [100, 10, 20, 50], x: 20, y: 10, w: 80, h: 40,
    })
    expect(create('arrow', at, at, 1, []).points).toEqual([420, 220, 500, 300])
  })

  test('말풍선 꼬리 끝이 누른 곳에 온다', () => {
    const c = create('callout', at, at, 1, [])
    expect(c.x + c.w * 0.2).toBeCloseTo(500, 0)
    expect(c.y + c.h * 1.35).toBeCloseTo(300, 0)
  })

  test('번호는 지금 있는 가장 큰 번호 다음이다', () => {
    const n = (text: string): Annotation => ({ id: text, type: 'number', x: 0, y: 0, w: 1, h: 1, text, style })
    expect(nextNumber([])).toBe('1')
    expect(nextNumber([n('1'), n('3'), n('x')])).toBe('4')
    expect(create('number', at, at, 1, [n('2')]).text).toBe('3')
  })
})

test('recolor는 채움이 주된 객체만 채움도 바꾼다', () => {
  const rect = create('rect', { x: 0, y: 0 }, { x: 10, y: 10 }, 1, [])
  expect(recolor(rect, '#1A73E8').style).toMatchObject({ stroke: '#1A73E8', fill: '' })
  const marker = create('marker', { x: 0, y: 0 }, { x: 0, y: 0 }, 1, [])
  expect(recolor(marker, '#1A73E8').style).toMatchObject({ stroke: '#1A73E8', fill: '#1A73E8' })
  const hl = create('highlight', { x: 0, y: 0 }, { x: 10, y: 10 }, 1, [])
  expect(recolor(hl, '#00FF00').style).toMatchObject({ stroke: '', fill: '#00FF00' })
})
