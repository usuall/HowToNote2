<!-- 첫 화면 매뉴얼 카드(UI 설계 5.1절): 썸네일, 제목, 스텝 수와 날짜. 누르면 열고, ⋯ 메뉴나 마우스 오른쪽 단추로
     이름 바꾸기·폴더로 옮기기·삭제를 한다. 이름 바꾸기는 제목 자리에서 NameEdit로 한다(단추 안에는 입력 칸을 넣을 수 없어 밖에 둔다). -->
<script lang="ts">
  import type { store } from '../wailsjs/go/models'
  import { formatDate } from './api'
  import NameEdit from './NameEdit.svelte'
  import Icon from './ui/Icon.svelte'
  import Menu, { type MenuItem } from './ui/Menu.svelte'
  import { tip } from './ui/tip'

  let {
    guide,
    folders,
    busy,
    renaming,
    onopen,
    onstartrename,
    oncancelrename,
    onrename,
    onmove,
    ondelete,
  }: {
    guide: store.Guide
    folders: store.Folder[]
    busy: boolean
    renaming: boolean
    onopen: () => void
    onstartrename: () => void
    oncancelrename: () => void
    onrename: (title: string) => void
    onmove: (folderId: number) => void
    ondelete: () => void
  } = $props()

  let menu = $state<HTMLElement | { x: number; y: number } | null>(null)

  const items = $derived<MenuItem[]>([
    { label: '이름 바꾸기', icon: 'pencil', onselect: onstartrename },
    { heading: '폴더로 옮기기' },
    { label: '폴더 없음', checked: guide.folderId === 0, onselect: () => onmove(0) },
    ...folders.map((f) => ({ label: f.title, checked: guide.folderId === f.id, onselect: () => onmove(f.id) })),
    'separator',
    { label: '삭제', icon: 'trash', danger: true, onselect: ondelete },
  ])
</script>

<article class="card" class:open-menu={!!menu}>
  <button
    class="open"
    disabled={renaming}
    onclick={onopen}
    oncontextmenu={(e) => {
      e.preventDefault()
      menu = { x: e.clientX, y: e.clientY }
    }}
    {@attach tip('눌러서 엽니다. 마우스 오른쪽 단추로 메뉴를 엽니다.')}
  >
    <span class="thumb">
      {#if guide.thumbSha}
        <img src={`/img/${guide.thumbSha}.png`} alt="" loading="lazy" draggable="false" />
      {:else}
        <span class="noimg"><Icon name="text" size={20} />글</span>
      {/if}
    </span>
    {#if !renaming}
      <span class="text">
        <strong>{guide.title}</strong>
        <small>스텝 {guide.stepCount}개 · {formatDate(guide.updatedAt)}</small>
      </span>
    {/if}
  </button>
  {#if renaming}
    <div class="rename">
      <NameEdit label="매뉴얼 이름" value={guide.title} {busy} onsave={onrename} oncancel={oncancelrename} />
    </div>
  {/if}
  <button
    class="btn icon sm more"
    aria-label={`'${guide.title}' 매뉴얼 메뉴`}
    aria-haspopup="menu"
    aria-expanded={!!menu}
    disabled={busy}
    onclick={(e) => (menu = menu ? null : e.currentTarget)}
    {@attach tip('이름 바꾸기 · 폴더 옮기기 · 삭제')}
  >
    <Icon name="ellipsis" />
  </button>
</article>

{#if menu}
  <Menu label="매뉴얼 메뉴" {items} at={menu} onclose={() => (menu = null)} />
{/if}

<style>
  .card {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-sm);
  }
  .card:hover,
  .card.open-menu {
    border-color: var(--border-strong);
    box-shadow: var(--shadow-md);
  }
  .open {
    display: grid;
    width: 100%;
    height: auto;
    padding: 0;
    text-align: left;
    white-space: normal;
    background: transparent;
    border: 0;
  }
  .open:hover:not(:disabled) {
    background: transparent;
  }
  .open:disabled {
    opacity: 1;
  }
  .thumb {
    display: block;
    aspect-ratio: 16 / 10;
    overflow: hidden;
    background: var(--hover);
    border-bottom: 1px solid var(--border);
    border-radius: var(--radius) var(--radius) 0 0;
  }
  .thumb img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: top left;
  }
  .noimg {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    height: 100%;
    font-size: 13px;
    color: var(--text-3);
  }
  .text {
    display: grid;
    gap: 2px;
    min-width: 0;
    padding: 10px 40px 10px 12px;
  }
  strong {
    overflow: hidden;
    font-size: 14px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  small {
    font-size: 12px;
    color: var(--text-3);
  }
  .rename {
    padding: 8px;
  }
  .more {
    position: absolute;
    right: 6px;
    bottom: 8px;
  }
</style>
