// 편집기 스텝 목록의 순수 로직: 순서(sort 중간값), 하위 스텝(parent_id), 숨김·삭제, 번호, 실행 취소 기록.
// 편집기의 모든 바꾸기는 "바뀐 스텝 행"을 만들고, 그 행을 화면·자동 저장·실행 취소가 같이 쓴다.
import type { store } from '../../wailsjs/go/models'

export type Step = store.Step

// ordered는 지운 스텝을 뺀 목록을 순서대로 돌려준다(Go ListSteps와 같은 정렬: sort, id).
export function ordered(steps: Step[]): Step[] {
  return steps.filter((s) => !s.deleted).sort((a, b) => a.sort - b.sort || a.id - b.id)
}

// spread는 prev와 next 사이에 k개를 고르게 넣을 sort 값이다. next가 없으면 끝에 정수로 붙인다(Go SortBetween과 같은 규칙).
export function spread(prev: number | undefined, next: number | undefined, k: number): number[] {
  if (next === undefined) {
    const base = Math.floor(prev ?? 0)
    return Array.from({ length: k }, (_, i) => base + i + 1)
  }
  const lo = prev ?? next - 1
  return Array.from({ length: k }, (_, i) => lo + ((next - lo) * (i + 1)) / (k + 1))
}

// fixParents는 하위 스텝의 parent_id를 "바로 앞의 최상위 스텝"으로 맞춘다. 앞에 최상위 스텝이 없으면 최상위가 된다.
// 들여쓰기는 한 단계뿐이라(번호 1.1까지) parent_id는 순서에서 정해지는 값이고, 순서·삭제·들여쓰기 뒤에 늘 다시 맞춘다.
// parentId가 -1이면 "하위로 만들어 달라"는 뜻이다(setIndent).
export function fixParents(list: Step[]): Step[] {
  let top = 0
  return list.map((s) => {
    if (!s.parentId) {
      top = s.id
      return s
    }
    if (!top) {
      top = s.id
      return { ...s, parentId: 0 }
    }
    return s.parentId === top ? s : { ...s, parentId: top }
  })
}

// changed는 next 중 원래 행과 sort·parentId·deleted·hidden이 다른 행만 고른다.
function changed(steps: Step[], next: Step[]): Step[] {
  const byId = new Map(steps.map((s) => [s.id, s]))
  return next.filter((s) => {
    const o = byId.get(s.id)
    return !o || o.sort !== s.sort || o.parentId !== s.parentId || o.deleted !== s.deleted || o.hidden !== s.hidden
  })
}

// moveBlock은 스텝 id를 beforeId 바로 앞으로 옮긴다(beforeId가 0이면 맨 끝). 최상위 스텝은 하위 스텝과 함께 움직인다.
// 옮긴 스텝만 sort를 새로 받고, 실수 정밀도가 바닥나 사이 값을 만들 수 없으면 전체를 1, 2, 3…으로 다시 매긴다.
export function moveBlock(steps: Step[], id: number, beforeId: number): Step[] {
  const list = ordered(steps)
  const i = list.findIndex((s) => s.id === id)
  if (i < 0) return []
  let end = i + 1
  if (!list[i].parentId) while (end < list.length && list[end].parentId) end++
  const block = list.slice(i, end)
  if (block.some((s) => s.id === beforeId)) return []
  const rest = [...list.slice(0, i), ...list.slice(end)]
  const at = beforeId ? rest.findIndex((s) => s.id === beforeId) : rest.length
  if (at < 0) return []
  const prev = rest[at - 1]?.sort
  const next = rest[at]?.sort
  const sorts = spread(prev, next, block.length)
  let result = [...rest.slice(0, at), ...block.map((s, k) => ({ ...s, sort: sorts[k] })), ...rest.slice(at)]
  const bounds = [prev ?? -Infinity, ...sorts, next ?? Infinity]
  if (bounds.some((v, k) => k > 0 && !(v > bounds[k - 1]))) result = result.map((s, k) => ({ ...s, sort: k + 1 }))
  return changed(steps, fixParents(result))
}

// setIndent는 고른 스텝을 하위 스텝으로 만들거나(on) 최상위로 되돌린다.
export function setIndent(steps: Step[], ids: number[], on: boolean): Step[] {
  const pick = new Set(ids)
  const list = ordered(steps).map((s) => (pick.has(s.id) ? { ...s, parentId: on ? s.parentId || -1 : 0 } : s))
  return changed(steps, fixParents(list))
}

// setDeleted는 고른 스텝을 지우거나(on) 되살린다. 지운 스텝은 가이드를 닫을 때까지 DB에 남는다(스펙 6.3절).
// 지운 최상위 스텝의 하위 스텝은 바로 앞 최상위 스텝 아래로 옮겨, 영구 삭제 때 함께 지워지지 않게 한다.
export function setDeleted(steps: Step[], ids: number[], on: boolean): Step[] {
  const pick = new Set(ids)
  const marked = steps.map((s) => (pick.has(s.id) ? { ...s, deleted: on } : s))
  const live = fixParents(ordered(marked))
  return changed(steps, [...live, ...marked.filter((s) => s.deleted)])
}

export function setHidden(steps: Step[], ids: number[], on: boolean): Step[] {
  const pick = new Set(ids)
  return changed(steps, steps.map((s) => (pick.has(s.id) ? { ...s, hidden: on } : s)))
}

// placeAfter는 id 스텝 바로 뒤(0이거나 없으면 맨 끝)에 새 스텝을 넣을 자리다. 하위 스텝 사이에 넣으면 같은 상위 스텝의
// 하위 스텝이 된다. 그래야 뒤따르는 하위 스텝들의 상위가 새 스텝으로 바뀌지 않는다.
export function placeAfter(steps: Step[], id: number): { sort: number; parentId: number } {
  const list = ordered(steps)
  const i = list.findIndex((s) => s.id === id)
  const at = i < 0 ? list.length - 1 : i
  const prev = list[at]
  const next = list[at + 1]
  return { sort: spread(prev?.sort, next?.sort, 1)[0], parentId: next?.parentId || prev?.parentId || 0 }
}

// numbering은 화면과 내보내기에 쓸 번호다: 최상위 "1", 하위 "1.1". 숨긴 스텝은 번호가 없다(내보내지 않는다, 스펙 7.4절).
// 숨긴 최상위 스텝의 하위 스텝도 숨긴 것으로 본다. 그러지 않으면 앞 스텝의 번호를 이어 받거나 "1"로 올라선다.
// 번호가 빈 스텝이 곧 내보내지 않을 스텝이다(M3).
export function numbering(steps: Step[]): Map<number, string> {
  const out = new Map<number, string>()
  let top = 0
  let sub = 0
  let head: Step | undefined // 바로 앞 최상위 스텝
  for (const s of ordered(steps)) {
    const child = s.parentId && head
    if (!child) head = s
    if (s.hidden || (child && head!.hidden)) {
      out.set(s.id, '')
    } else if (child) {
      out.set(s.id, `${top}.${++sub}`)
    } else {
      top++
      sub = 0
      out.set(s.id, `${top}`)
    }
  }
  return out
}

// History는 가이드 하나의 실행 취소 기록이다(스펙 6.3절, 메모리에만). 바꾸기 전후 행을 통째로 담는다.
// key가 같은 바꾸기가 1.5초 안에 이어지면(같은 칸에 글자 치기) 한 번으로 묶는다.
type Entry = { before: Step[]; after: Step[]; key: string; at: number }

export class History {
  private done: Entry[] = []
  private undone: Entry[] = []

  push(before: Step[], after: Step[], key = '', now = Date.now()) {
    const last = this.done.at(-1)
    if (key && last?.key === key && now - last.at < 1500) {
      last.after = after
      last.at = now
    } else {
      this.done.push({ before, after, key, at: now })
      if (this.done.length > 200) this.done.shift() // ponytail: 200번 넘게 쌓이면 오래된 것부터 버린다
    }
    this.undone = []
  }

  get canUndo(): boolean {
    return this.done.length > 0
  }

  get canRedo(): boolean {
    return this.undone.length > 0
  }

  undo(): Step[] | null {
    const e = this.done.pop()
    if (!e) return null
    this.undone.push(e)
    return e.before
  }

  redo(): Step[] | null {
    const e = this.undone.pop()
    if (!e) return null
    this.done.push(e)
    return e.after
  }
}

// 가이드별 기록. '녹화 이어서 하기'로 편집기 화면을 잠시 떠나도 남고, 가이드를 닫을 때 지운다.
const histories = new Map<number, History>()

export function historyFor(guideId: number): History {
  let h = histories.get(guideId)
  if (!h) histories.set(guideId, (h = new History()))
  return h
}

export function forgetHistory(guideId: number) {
  histories.delete(guideId)
}
