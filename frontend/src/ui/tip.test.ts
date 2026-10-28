// 툴팁·메뉴 자리 계산 테스트: 기본은 대상 아래 가운데, 아래에 자리가 없으면 위로 뒤집고, 창 좌우 끝에서 8px 안으로 들인다.
import { expect, test } from 'vitest'
import { place } from './tip'

const view = { width: 1000, height: 700 }

test('기본은 대상 아래 가운데에 8px 띄워 둔다', () => {
  expect(place({ left: 100, top: 100, width: 40, height: 20 }, { width: 60, height: 30 }, view)).toEqual({ left: 90, top: 128 })
})

test('아래에 자리가 없으면 위로 뒤집는다', () => {
  expect(place({ left: 100, top: 660, width: 40, height: 20 }, { width: 60, height: 30 }, view)).toEqual({ left: 90, top: 622 })
})

test('위에도 자리가 없으면 아래에 둔다', () => {
  expect(place({ left: 100, top: 20, width: 40, height: 20 }, { width: 60, height: 800 }, view).top).toBe(48)
})

test('창 좌우 끝에서는 8px 안으로 맞춘다', () => {
  expect(place({ left: 0, top: 100, width: 20, height: 20 }, { width: 100, height: 30 }, view).left).toBe(8)
  expect(place({ left: 980, top: 100, width: 20, height: 20 }, { width: 100, height: 30 }, view).left).toBe(892)
})

test("'start'는 대상 왼쪽 끝에 맞춘다(메뉴)", () => {
  expect(place({ left: 300, top: 100, width: 40, height: 20 }, { width: 160, height: 90 }, view, 'start')).toEqual({ left: 300, top: 128 })
})
