<!-- 첫 화면 왼쪽 폴더 막대(UI 설계 5.1절): 모든 매뉴얼, 폴더 목록, 새 폴더. 접으면 아이콘만 남고 이름은 툴팁으로 보인다.
     접힌 상태는 localStorage에 기억하되, 읽기·쓰기가 안 되는 환경에서는 펼친 채로 동작한다. -->
<script lang="ts">
  import type { store } from '../wailsjs/go/models'
  import NameEdit from './NameEdit.svelte'
  import Icon from './ui/Icon.svelte'
  import Menu, { type MenuItem } from './ui/Menu.svelte'
  import { tip } from './ui/tip'

  let {
    folders,
    counts,
    total,
    folder,
    busy,
    onpick,
    oncreate,
    onrename,
    ondelete,
  }: {
    folders: store.Folder[]
    counts: Map<number, number>
    total: number
    folder: number
    busy: boolean
    onpick: (id: number) => void
    oncreate: (name: string) => Promise<boolean>
    onrename: (id: number, name: string) => Promise<boolean>
    ondelete: (f: store.Folder) => void
  } = $props()

  const KEY = 'howtonote.sidebar'
  let collapsed = $state(read())
  let naming = $state(0) // 이름 칸을 연 폴더. -1이면 새 폴더
  let menu = $state<{ f: store.Folder; at: HTMLElement | { x: number; y: number } } | null>(null)

  function read(): boolean {
    try {
      return localStorage.getItem(KEY) === 'collapsed'
    } catch {
      return false
    }
  }

  function toggle() {
    collapsed = !collapsed
    naming = 0
    try {
      localStorage.setItem(KEY, collapsed ? 'collapsed' : 'open')
    } catch {
      // 기억하지 못해도 이번 실행 동안은 접힌 채로 둔다
    }
  }

  // 이름 칸은 펼친 막대에만 들어간다. 접힌 막대에서 이름을 고치거나 폴더를 만들면 이번만 펼친다(기억하지 않는다).
  function name(id: number) {
    collapsed = false
    naming = id
  }

  async function create(title: string) {
    if (await oncreate(title)) naming = 0
  }

  async function rename(id: number, title: string) {
    if (await onrename(id, title)) naming = 0
  }

  // open은 폴더 메뉴를 연다. 같은 폴더의 ⋯를 다시 누르면 닫는다.
  function open(f: store.Folder, at: HTMLElement | { x: number; y: number }) {
    menu = menu?.f.id === f.id && at instanceof HTMLElement ? null : { f, at }
  }

  function items(f: store.Folder): MenuItem[] {
    return [
      { label: '이름 바꾸기', icon: 'pencil', onselect: () => name(f.id) },
      'separator',
      { label: '지우기', icon: 'trash', danger: true, onselect: () => ondelete(f) },
    ]
  }
</script>

<nav class="bar" class:collapsed aria-label="폴더">
  <button
    class="btn icon sm toggle"
    aria-label={collapsed ? '폴더 막대 펴기' : '폴더 막대 접기'}
    aria-expanded={!collapsed}
    onclick={toggle}
    {@attach tip()}
  >
    <Icon name="panel-left" />
  </button>

  <button
    class="item"
    class:on={folder === 0}
    aria-current={folder === 0 ? 'page' : undefined}
    onclick={() => onpick(0)}
    {@attach tip(collapsed ? `모든 매뉴얼 (${total}개)` : '모든 매뉴얼을 봅니다.')}
  >
    <Icon name="grid" /><span class="name" class:sr-only={collapsed}>모든 매뉴얼</span><span class="count">{total}</span>
  </button>

  <p class="heading">폴더</p>
  {#each folders as f (f.id)}
    {#if naming === f.id}
      <NameEdit label="폴더 이름" value={f.title} {busy} onsave={(t) => rename(f.id, t)} oncancel={() => (naming = 0)} />
    {:else}
      <div class="row">
        <button
          class="item"
          class:on={folder === f.id}
          aria-current={folder === f.id ? 'page' : undefined}
          onclick={() => onpick(f.id)}
          oncontextmenu={(e) => {
            e.preventDefault()
            open(f, { x: e.clientX, y: e.clientY })
          }}
          {@attach tip(collapsed ? `${f.title} (${counts.get(f.id) ?? 0}개)` : `'${f.title}' 폴더의 매뉴얼을 봅니다.`)}
        >
          <Icon name="folder" /><span class="name" class:sr-only={collapsed}>{f.title}</span><span class="count">{counts.get(f.id) ?? 0}</span>
        </button>
        {#if !collapsed}
          <button
            class="btn icon sm more"
            aria-label={`'${f.title}' 폴더 메뉴`}
            aria-haspopup="menu"
            aria-expanded={menu?.f.id === f.id}
            disabled={busy}
            onclick={(e) => open(f, e.currentTarget)}
            {@attach tip('이름 바꾸기 · 지우기')}
          >
            <Icon name="ellipsis" />
          </button>
        {/if}
      </div>
    {/if}
  {/each}

  {#if naming === -1}
    <NameEdit label="새 폴더 이름" {busy} onsave={create} oncancel={() => (naming = 0)} />
  {:else}
    <button class="item add" disabled={busy} onclick={() => name(-1)} {@attach tip('매뉴얼을 묶어 둘 폴더를 만듭니다.')}>
      <Icon name="folder-plus" /><span class="name" class:sr-only={collapsed}>새 폴더</span>
    </button>
  {/if}
</nav>

{#if menu}
  <Menu label="폴더 메뉴" items={items(menu.f)} at={menu.at} onclose={() => (menu = null)} />
{/if}

<style>
  .bar {
    display: flex;
    flex: none;
    flex-direction: column;
    gap: 2px;
    width: 196px;
    padding: 10px;
    overflow-y: auto;
    background: var(--sidebar);
    border-right: 1px solid var(--border);
  }
  .bar.collapsed {
    width: 56px;
    padding: 10px 8px;
  }
  .toggle {
    align-self: flex-end;
    margin-bottom: 4px;
  }
  .collapsed .toggle {
    align-self: center;
  }
  .item {
    width: 100%;
    justify-content: flex-start;
    gap: 8px;
    padding: 0 10px;
    font-size: 13px;
    color: var(--text-2);
    background: transparent;
    border-color: transparent;
  }
  .item.on,
  .item.on:hover:not(:disabled) {
    font-weight: 700;
    color: var(--text);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
  }
  .collapsed .item {
    justify-content: center;
    padding: 0;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-align: left;
    text-overflow: ellipsis;
  }
  .count {
    font-size: 12px;
    font-weight: 400;
    color: var(--text-3);
  }
  .collapsed .count,
  .collapsed .heading {
    display: none;
  }
  .heading {
    margin: 10px 10px 4px;
    font-size: 12px;
    color: var(--text-3);
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
  }
  .more {
    position: absolute;
    right: 3px;
    opacity: 0;
  }
  .row:hover .more,
  .row:focus-within .more,
  .more[aria-expanded='true'] {
    opacity: 1;
  }
  .row:hover .count,
  .row:focus-within .count,
  .row:has(.more[aria-expanded='true']) .count {
    visibility: hidden;
  }
  .add {
    margin-top: 4px;
    color: var(--text-3);
  }
</style>
