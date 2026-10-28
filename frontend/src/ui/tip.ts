// 툴팁(UI 설계 4.6절). {@attach tip('…')}로 붙인다. 문서에 툴팁 요소를 하나만 두고 글과 자리만 바꾼다.
// 고정 위치 요소라 스크롤 칸이나 overflow: hidden 안의 단추에서도 잘리지 않는다.
import type { Attachment } from 'svelte/attachments'

export type Rect = { left: number; top: number; width: number; height: number }
type Size = { width: number; height: number }

const GAP = 8 // 대상과 툴팁 사이
const EDGE = 8 // 창 끝과 툴팁 사이
const DELAY = 300 // 마우스를 올린 뒤 기다리는 시간(ms). 지나가기만 하는 마우스에는 뜨지 않게

// place는 툴팁이나 메뉴의 왼쪽 위 좌표다. 대상 아래가 기본이고, 아래에 자리가 없고 위에 있으면 위로 뒤집는다.
// 가로는 'center'면 대상 가운데, 'start'면 대상 왼쪽 끝에 맞춘 뒤 창 좌우 끝에서 EDGE 안으로 들인다.
export function place(target: Rect, box: Size, view: Size, align: 'center' | 'start' = 'center'): { left: number; top: number } {
  let top = target.top + target.height + GAP
  const above = target.top - GAP - box.height
  if (top + box.height > view.height - EDGE && above >= EDGE) top = above
  const want = align === 'start' ? target.left : target.left + target.width / 2 - box.width / 2
  const left = Math.min(Math.max(want, EDGE), Math.max(EDGE, view.width - EDGE - box.width))
  return { left, top }
}

let el: HTMLDivElement | undefined
let timer: ReturnType<typeof setTimeout> | undefined
let owner: Element | null = null
let described: Element | null = null // aria-describedby를 받은 요소(막힌 단추는 target이 아니라 안의 button)

function node(): HTMLDivElement {
  if (!el) {
    el = document.createElement('div')
    el.id = 'tip'
    el.className = 'tooltip'
    el.setAttribute('role', 'tooltip')
    el.hidden = true
    addEventListener('scroll', hideTip, true) // 스크롤하면 대상이 움직여 툴팁이 엉뚱한 곳에 남는다
  }
  return el
}

function show(target: Element, text: string, describe: boolean) {
  clearTimeout(timer)
  if (!text || !target.isConnected) return
  const t = node()
  // 모달 <dialog>는 최상위 층이라 body의 툴팁이 그 뒤에 가린다. 열린 창 안의 대상이면 툴팁을 그 창 안에 둔다.
  const host = target.closest('dialog[open]') ?? document.body
  if (t.parentElement !== host) host.append(t)
  t.textContent = text
  t.style.left = '0px' // 지난 자리(창 오른쪽 끝)에 눌려 폭이 줄어든 채로 재지 않게
  t.style.top = '0px'
  t.hidden = false
  const p = place(target.getBoundingClientRect(), { width: t.offsetWidth, height: t.offsetHeight }, { width: innerWidth, height: innerHeight })
  t.style.left = `${p.left}px`
  t.style.top = `${p.top}px`
  owner = target
  // 초점을 받는 요소에 달아야 스크린 리더가 읽는다: 막힌 단추를 감싼 target이면 초점은 안의 button이 받는다
  if (describe) {
    described = target.querySelector('button') ?? target
    described.setAttribute('aria-describedby', t.id)
  }
}

export function hideTip() {
  clearTimeout(timer)
  if (el) el.hidden = true
  described?.removeAttribute('aria-describedby')
  described = null
  owner = null
}

// tip은 툴팁 첨부다. text를 주지 않으면 대상의 aria-label을 쓴다. 아이콘만 있는 단추는 aria-label 하나로 이름과
// 툴팁을 같이 정하고, 이름과 같은 글이 aria-describedby로 한 번 더 읽히지 않게 한다.
// 초점은 focusin으로 받아, 막힌 단추를 감싼 요소에 붙여도 안의 단추에 초점이 오면 뜬다.
export function tip(text?: string): Attachment<HTMLElement> {
  return (target) => {
    const describe = text !== undefined
    const read = () => text ?? target.getAttribute('aria-label') ?? ''
    const enter = () => {
      clearTimeout(timer)
      timer = setTimeout(() => show(target, read(), describe), DELAY)
    }
    const focus = (e: FocusEvent) => {
      if ((e.target as Element).matches(':focus-visible')) show(target, read(), describe)
    }
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') hideTip()
    }
    target.addEventListener('pointerenter', enter)
    target.addEventListener('pointerleave', hideTip)
    target.addEventListener('pointerdown', hideTip)
    target.addEventListener('focusin', focus)
    target.addEventListener('focusout', hideTip)
    target.addEventListener('keydown', key)
    return () => {
      target.removeEventListener('pointerenter', enter)
      target.removeEventListener('pointerleave', hideTip)
      target.removeEventListener('pointerdown', hideTip)
      target.removeEventListener('focusin', focus)
      target.removeEventListener('focusout', hideTip)
      target.removeEventListener('keydown', key)
      if (owner === target) hideTip()
    }
  }
}
