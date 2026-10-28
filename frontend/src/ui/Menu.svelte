<!-- 메뉴(UI 설계 4.7절). ⋯ 메뉴, '스텝 넣기', 마우스 오른쪽 단추 메뉴에 같이 쓴다. 여는 쪽이 {#if}로 붙였다 떼고, 닫을 일이
     생기면 onclose를 부른다. 키보드: ↑↓·Home·End로 옮기고(막힌 항목은 건너뜀), Enter·Space로 실행, Esc·Tab으로 닫는다.
     키보드로 닫으면 열기 전에 초점이 있던 곳(보통 여는 단추)으로 돌아간다. -->
<script lang="ts" module>
  import type { IconName } from './icons'

  export type MenuItem =
    | { label: string; icon?: IconName; danger?: boolean; disabled?: boolean; checked?: boolean; onselect: () => void }
    | { heading: string }
    | 'separator'
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import { place } from './tip'

  let {
    items,
    at,
    label,
    onclose,
  }: { items: MenuItem[]; at: HTMLElement | { x: number; y: number }; label: string; onclose: () => void } = $props()

  let box: HTMLDivElement
  let pos = $state({ left: 0, top: 0 })
  const before = document.activeElement as HTMLElement | null

  // 막힌 항목은 키보드로 갈 수 없다. Svelte가 disabled를 속성으로 쓰므로 속성으로 고른다.
  const buttons = () => [...box.querySelectorAll<HTMLButtonElement>('button:not([disabled])')]

  function close() {
    onclose()
    before?.focus()
  }

  function pick(item: Extract<MenuItem, { onselect: () => void }>) {
    close()
    item.onselect()
  }

  function key(e: KeyboardEvent) {
    const list = buttons()
    const i = list.indexOf(document.activeElement as HTMLButtonElement)
    const go = (n: number) => {
      e.preventDefault()
      list[(n + list.length) % list.length]?.focus()
    }
    if (e.key === 'ArrowDown') go(i + 1)
    else if (e.key === 'ArrowUp') go(i < 0 ? -1 : i - 1)
    else if (e.key === 'Home') go(0)
    else if (e.key === 'End') go(-1)
    else if (e.key === 'Escape' || e.key === 'Tab') {
      e.preventDefault()
      e.stopPropagation() // 편집기·캔버스의 Esc 처리로 넘어가지 않게
      close()
    }
  }

  onMount(() => {
    const r = at instanceof HTMLElement ? at.getBoundingClientRect() : { left: at.x, top: at.y, width: 0, height: 0 }
    pos = place(r, { width: box.offsetWidth, height: box.offsetHeight }, { width: innerWidth, height: innerHeight }, 'start')
    buttons()[0]?.focus()
    // 여는 단추를 다시 누르면 그 단추가 메뉴를 닫으므로 바깥 누르기에서 뺀다. 바깥을 누르면 초점은 누른 곳으로 간다.
    const outside = (e: PointerEvent) => {
      const t = e.target as Node
      if (!box.contains(t) && !(at instanceof HTMLElement && at.contains(t))) onclose()
    }
    addEventListener('pointerdown', outside, true)
    addEventListener('resize', onclose)
    return () => {
      removeEventListener('pointerdown', outside, true)
      removeEventListener('resize', onclose)
    }
  })
</script>

<div
  class="menu"
  role="menu"
  aria-label={label}
  tabindex="-1"
  bind:this={box}
  style:left="{pos.left}px"
  style:top="{pos.top}px"
  onkeydown={key}
>
  {#each items as item, i (i)}
    {#if item === 'separator'}
      <hr />
    {:else if 'heading' in item}
      <p class="heading" role="presentation">{item.heading}</p>
    {:else}
      <button
        role={item.checked === undefined ? 'menuitem' : 'menuitemradio'}
        aria-checked={item.checked}
        class:danger={item.danger}
        disabled={item.disabled}
        tabindex="-1"
        onclick={() => pick(item)}
      >
        <span class="mark">
          {#if item.checked}<Icon name="check" />{:else if item.icon}<Icon name={item.icon} />{/if}
        </span>
        {item.label}
      </button>
    {/if}
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: 900;
    min-width: 180px;
    max-height: calc(100vh - 16px);
    overflow-y: auto;
    padding: 4px;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
  }
  .menu:focus-visible {
    outline: none; /* 초점은 항목이 받는다 */
  }
  button {
    width: 100%;
    height: 32px;
    justify-content: flex-start;
    gap: 8px;
    padding: 0 10px;
    font-size: 13px;
    background: transparent;
    border: 0;
    border-radius: var(--radius-sm);
  }
  /* 메뉴 항목은 초점 테두리 대신 바탕색으로 지금 항목을 보인다 */
  button:focus-visible {
    outline: none;
    background: var(--hover);
  }
  .danger {
    color: var(--danger);
  }
  .mark {
    display: inline-flex;
    width: 16px;
  }
  .heading {
    margin: 6px 10px 2px;
    font-size: 12px;
    color: var(--text-3);
  }
  hr {
    margin: 4px 0;
    border: 0;
    border-top: 1px solid var(--border);
  }
</style>
