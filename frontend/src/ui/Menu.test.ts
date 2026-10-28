// @vitest-environment happy-dom
// 메뉴 테스트: 열면 첫 항목에 초점, ↓↑는 막힌 항목을 건너뛰며 돌고, 누르면 닫은 뒤 실행하고, Esc는 닫고 여는 단추로 초점을 돌린다.
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, expect, test } from 'vitest'
import Menu, { type MenuItem } from './Menu.svelte'

afterEach(() => {
  document.body.innerHTML = ''
})

function setup(items: MenuItem[]) {
  const trigger = document.createElement('button')
  document.body.append(trigger)
  trigger.focus()
  let closed = 0
  const menu = mount(Menu, { target: document.body, props: { items, at: trigger, label: '테스트 메뉴', onclose: () => closed++ } })
  flushSync()
  return { trigger, menu, closed: () => closed }
}

const focused = () => document.activeElement?.textContent?.trim()
const press = (key: string) => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))

test('열면 첫 항목, ↓↑는 막힌 항목을 건너뛰며 돌고, 누르면 닫은 뒤 실행한다', () => {
  const ran: string[] = []
  const { menu, closed } = setup([
    { label: '이름 바꾸기', onselect: () => ran.push('이름') },
    { label: '막힘', disabled: true, onselect: () => ran.push('막힘') },
    'separator',
    { label: '삭제', danger: true, onselect: () => ran.push(`삭제(닫힘 ${closed()})`) },
  ])
  expect(focused()).toBe('이름 바꾸기')
  press('ArrowDown')
  expect(focused()).toBe('삭제')
  press('ArrowDown')
  expect(focused()).toBe('이름 바꾸기') // 끝에서 처음으로 돈다
  press('ArrowUp')
  expect(focused()).toBe('삭제')
  ;(document.activeElement as HTMLButtonElement).click()
  expect(ran).toEqual(['삭제(닫힘 1)'])
  unmount(menu)
})

test('Esc는 닫고 열기 전 초점(여는 단추)으로 돌아간다', () => {
  const { trigger, menu, closed } = setup([{ label: '하나', onselect: () => {} }])
  press('Escape')
  expect(closed()).toBe(1)
  expect(document.activeElement).toBe(trigger)
  unmount(menu)
})

test('고름 항목은 menuitemradio와 aria-checked로, 소제목은 단추가 아닌 글로 보인다', () => {
  const { menu } = setup([
    { heading: '폴더로 옮기기' },
    { label: '폴더 없음', checked: false, onselect: () => {} },
    { label: '인사', checked: true, onselect: () => {} },
  ])
  const radios = [...document.querySelectorAll('[role=menuitemradio]')]
  expect(radios.map((r) => r.getAttribute('aria-checked'))).toEqual(['false', 'true'])
  expect(focused()).toBe('폴더 없음')
  unmount(menu)
})
