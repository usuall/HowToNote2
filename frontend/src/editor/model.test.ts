// 스텝 목록 로직 테스트: 순서 바꾸기(중간값, 하위 스텝과 함께), 들여쓰기, 삭제·복구, 번호, 실행 취소 기록.
import { describe, expect, test } from 'vitest'
import { History, moveBlock, numbering, ordered, placeAfter, setDeleted, setHidden, setIndent, spread, type Step } from './model'

// step은 테스트용 행이다. sort는 id와 같게 둔다.
function step(id: number, parentId = 0, extra: Partial<Step> = {}): Step {
  return {
    id, guideId: 1, parentId, sort: id, kind: 'click', title: `스텝${id}`, description: '', imageSha: '',
    hidden: false, deleted: false, captureJson: '{}', annotationsJson: '[]', calloutsJson: '[]', viewJson: '{}', ...extra,
  }
}

// apply는 바뀐 행을 목록에 덮어쓴다(편집기가 하는 일).
function apply(steps: Step[], rows: Step[]): Step[] {
  return steps.map((s) => rows.find((r) => r.id === s.id) ?? s)
}

const ids = (steps: Step[]) => ordered(steps).map((s) => s.id)
const parents = (steps: Step[]) => ordered(steps).map((s) => s.parentId)

describe('spread', () => {
  test('사이 값과 끝 값', () => {
    expect(spread(1, 2, 1)).toEqual([1.5])
    expect(spread(1, 2, 3)).toEqual([1.25, 1.5, 1.75])
    expect(spread(3.5, undefined, 2)).toEqual([4, 5])
    expect(spread(undefined, 1, 1)).toEqual([0.5])
    expect(spread(undefined, undefined, 1)).toEqual([1])
  })
})

describe('moveBlock', () => {
  test('옮긴 스텝만 중간값 sort를 받는다', () => {
    const steps = [step(1), step(2), step(3)]
    const rows = moveBlock(steps, 3, 1)
    expect(rows).toEqual([{ ...step(3), sort: 0.5 }])
    expect(ids(apply(steps, rows))).toEqual([3, 1, 2])
  })

  test('맨 끝으로', () => {
    const steps = [step(1), step(2), step(3)]
    expect(ids(apply(steps, moveBlock(steps, 1, 0)))).toEqual([2, 3, 1])
  })

  test('최상위 스텝은 하위 스텝과 함께 움직인다', () => {
    const steps = [step(1), step(2, 1), step(3), step(4)]
    const after = apply(steps, moveBlock(steps, 1, 4))
    expect(ids(after)).toEqual([3, 1, 2, 4])
    expect(parents(after)).toEqual([0, 0, 1, 0])
  })

  test('하위 스텝을 옮기면 새 자리 앞의 최상위 스텝 아래로 간다', () => {
    const steps = [step(1), step(2, 1), step(3)]
    const after = apply(steps, moveBlock(steps, 2, 0))
    expect(ids(after)).toEqual([1, 3, 2])
    expect(parents(after)).toEqual([0, 0, 3])
  })

  test('맨 앞으로 옮긴 하위 스텝은 최상위가 된다', () => {
    const steps = [step(1), step(2, 1)]
    const after = apply(steps, moveBlock(steps, 2, 1))
    expect(ids(after)).toEqual([2, 1])
    expect(parents(after)).toEqual([0, 0])
  })

  test('자기 묶음 안으로는 옮기지 않는다', () => {
    const steps = [step(1), step(2, 1), step(3)]
    expect(moveBlock(steps, 1, 2)).toEqual([])
    expect(moveBlock(steps, 1, 1)).toEqual([])
  })

  test('사이 값을 만들 수 없으면 1, 2, 3…으로 다시 매긴다', () => {
    const steps = [step(1, 0, { sort: 1 }), step(2, 0, { sort: 1 + Number.EPSILON }), step(3, 0, { sort: 5 })]
    const after = apply(steps, moveBlock(steps, 3, 2))
    expect(ids(after)).toEqual([1, 3, 2])
    expect(ordered(after).map((s) => s.sort)).toEqual([1, 2, 3])
  })
})

describe('들여쓰기·숨김·삭제', () => {
  test('setIndent는 바로 앞 최상위 스텝 아래로 넣고 첫 스텝은 들이지 않는다', () => {
    const steps = [step(1), step(2), step(3)]
    const after = apply(steps, setIndent(steps, [1, 2, 3], true))
    expect(parents(after)).toEqual([0, 1, 1])
    expect(parents(apply(after, setIndent(after, [3], false)))).toEqual([0, 1, 0])
  })

  test('지운 최상위 스텝의 하위 스텝은 앞 최상위 스텝으로 옮긴다', () => {
    const steps = [step(1), step(2), step(3, 2)]
    const rows = setDeleted(steps, [2], true)
    const after = apply(steps, rows)
    expect(ids(after)).toEqual([1, 3])
    expect(parents(after)).toEqual([0, 1])
    expect(after.find((s) => s.id === 2)?.deleted).toBe(true)
    expect(ids(apply(after, setDeleted(after, [2], false)))).toEqual([1, 2, 3])
  })

  test('이미 하위인 스텝을 다시 들여쓰면 바뀌는 행이 없다', () => {
    const steps = [step(1), step(2, 1), step(3)]
    expect(setIndent(steps, [2], true)).toEqual([])
  })

  test('하위 스텝이 있는 최상위 스텝을 들여쓰면 그 하위 스텝도 앞 최상위 스텝 아래로 간다', () => {
    const steps = [step(1), step(2), step(3, 2)]
    const rows = setIndent(steps, [2], true)
    expect(rows.map((r) => [r.id, r.parentId])).toEqual([[2, 1], [3, 1]])
    expect(parents(apply(steps, rows))).toEqual([0, 1, 1])
  })

  test('setHidden은 바뀐 행만 돌려준다', () => {
    const steps = [step(1), step(2, 0, { hidden: true })]
    expect(setHidden(steps, [1, 2], true)).toEqual([{ ...step(1), hidden: true }])
  })
})

test('placeAfter는 고른 스텝 바로 뒤, 없으면 맨 끝이고 하위 스텝 사이면 같은 상위 아래다', () => {
  const steps = [step(1), step(2, 1), step(3), step(4, 0, { deleted: true })]
  expect(placeAfter(steps, 1)).toEqual({ sort: 1.5, parentId: 1 }) // 1과 그 하위 2 사이
  expect(placeAfter(steps, 2)).toEqual({ sort: 2.5, parentId: 1 }) // 하위 2 뒤
  expect(placeAfter(steps, 3)).toEqual({ sort: 4, parentId: 0 })
  expect(placeAfter(steps, 0)).toEqual({ sort: 4, parentId: 0 })
  expect(placeAfter([], 0)).toEqual({ sort: 1, parentId: 0 })
})

test('numbering은 최상위 1, 하위 1.1이고 숨긴 스텝은 번호가 없다', () => {
  const steps = [step(1), step(2, 1), step(3, 1, { hidden: true }), step(4, 1), step(5), step(6, 0, { deleted: true })]
  expect([...numbering(steps)]).toEqual([
    [1, '1'], [2, '1.1'], [3, ''], [4, '1.2'], [5, '2'],
  ])
})

test('숨긴 최상위 스텝의 하위 스텝은 번호가 없다(같이 숨긴 것으로 본다)', () => {
  const steps = [step(1), step(2, 1), step(3, 0, { hidden: true }), step(4, 3), step(5)]
  expect([...numbering(steps)]).toEqual([
    [1, '1'], [2, '1.1'], [3, ''], [4, ''], [5, '2'],
  ])
})

test('맨 앞 최상위 스텝을 숨겨도 그 하위 스텝이 1번이 되지 않는다', () => {
  const steps = [step(1, 0, { hidden: true }), step(2, 1), step(3)]
  expect([...numbering(steps)]).toEqual([
    [1, ''], [2, ''], [3, '1'],
  ])
})

describe('History', () => {
  test('되돌리기와 다시 하기', () => {
    const h = new History()
    expect([h.canUndo, h.canRedo]).toEqual([false, false])
    h.push([step(1)], [{ ...step(1), title: 'a' }])
    expect([h.canUndo, h.canRedo]).toEqual([true, false])
    expect(h.undo()).toEqual([step(1)])
    expect([h.canUndo, h.canRedo]).toEqual([false, true])
    expect(h.undo()).toBeNull()
    expect(h.redo()).toEqual([{ ...step(1), title: 'a' }])
    expect(h.redo()).toBeNull()
  })

  test('같은 칸을 이어서 고치면 한 번으로 묶고, 새 바꾸기는 다시 하기를 비운다', () => {
    const h = new History()
    h.push([step(1)], [{ ...step(1), title: 'a' }], 'title:1', 0)
    h.push([{ ...step(1), title: 'a' }], [{ ...step(1), title: 'ab' }], 'title:1', 1000)
    h.push([{ ...step(1), title: 'ab' }], [{ ...step(1), title: 'abc' }], 'title:1', 5000) // 1.5초가 지나 따로
    expect(h.undo()?.[0].title).toBe('ab')
    expect(h.undo()?.[0].title).toBe('스텝1')
    h.push([step(2)], [{ ...step(2), hidden: true }])
    expect(h.redo()).toBeNull()
  })
})
