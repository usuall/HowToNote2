<!-- 편집기 왼쪽 스텝 목록(UI 설계 5.4절, 스펙 6.1절): 번호·썸네일 카드, 끌어서 순서 바꾸기, 카드마다 ⋯ 메뉴와 오른쪽 단추 메뉴
     (숨기기, 하위 스텝으로, 한 단계 위로, 삭제), '스텝 넣기' 메뉴, 2개 이상 고르면 아래 막대, '삭제한 스텝' 되살리기.
     바꾸기는 모두 model.ts가 만든 행을 onchange로 올려 보낸다. -->
<script lang="ts">
  import { fileBase64 } from '../api'
  import Icon from '../ui/Icon.svelte'
  import Menu, { type MenuItem } from '../ui/Menu.svelte'
  import { tip } from '../ui/tip'
  import { moveBlock, numbering, ordered, setDeleted, setHidden, setIndent, type Step } from './model'

  let {
    steps,
    selected = $bindable(),
    busy,
    onchange,
    onaddtext,
    onaddimage,
    onerror,
  }: {
    steps: Step[]
    selected: number[]
    busy: boolean
    onchange: (rows: Step[]) => void
    onaddtext: () => void
    onaddimage: (base64: string) => void
    onerror: (message: string) => void
  } = $props()

  const list = $derived(ordered(steps))
  const trash = $derived(steps.filter((s) => s.deleted))
  const nums = $derived(numbering(steps))
  const picked = $derived(new Set(selected))
  const allHidden = $derived(selected.length > 0 && list.every((s) => !picked.has(s.id) || s.hidden))

  let dragId = 0
  let dropBefore = $state(0) // 이 카드 앞에 놓는다. -1이면 맨 끝, 0이면 끌고 있지 않음
  let menu = $state<{ id: number; at: HTMLElement | { x: number; y: number } } | null>(null) // 카드 메뉴
  let addMenu = $state<HTMLElement | null>(null) // '스텝 넣기' 메뉴를 연 단추
  let fileInput: HTMLInputElement

  const addItems: MenuItem[] = [
    { label: '글 스텝', icon: 'text', onselect: () => onaddtext() },
    { label: '그림 파일에서…', icon: 'image', onselect: () => fileInput.click() },
  ]

  // pick은 카드 클릭이다: 그냥 누르면 하나, Ctrl은 더하고 빼기, Shift는 마지막으로 고른 카드부터 범위.
  function pick(e: MouseEvent, id: number) {
    if (e.ctrlKey) {
      selected = picked.has(id) ? selected.filter((x) => x !== id) : [...selected, id]
    } else if (e.shiftKey && selected.length) {
      const a = list.findIndex((s) => s.id === selected.at(-1))
      const b = list.findIndex((s) => s.id === id)
      const range = list.slice(Math.min(a, b), Math.max(a, b) + 1).map((s) => s.id)
      selected = [...range.filter((x) => x !== id), id] // 누른 카드가 오른쪽에 보이는 스텝이 된다
    } else {
      selected = [id]
    }
  }

  // openMenu는 카드 메뉴를 연다. 고르지 않은 카드에서 열면 그 카드를 골라, 메뉴가 바꾸는 스텝이 오른쪽에 보이게 한다.
  // 같은 카드의 ⋯를 다시 누르면 닫는다.
  function openMenu(id: number, at: HTMLElement | { x: number; y: number }) {
    if (menu?.id === id && at instanceof HTMLElement) {
      menu = null
      return
    }
    if (!picked.has(id)) selected = [id]
    menu = { id, at }
  }

  // items는 카드 하나의 메뉴다. 할 수 없는 들여쓰기(맨 첫 스텝을 하위로 등)는 model.ts가 바꿀 행이 없다고 하면 막는다.
  function items(id: number): MenuItem[] {
    const s = steps.find((x) => x.id === id)
    if (!s) return []
    return [
      { label: s.hidden ? '보이기' : '숨기기', icon: s.hidden ? 'eye' : 'eye-off', onselect: () => onchange(setHidden(steps, [id], !s.hidden)) },
      { label: '하위 스텝으로', icon: 'indent', disabled: !setIndent(steps, [id], true).length, onselect: () => onchange(setIndent(steps, [id], true)) },
      { label: '한 단계 위로', icon: 'outdent', disabled: !setIndent(steps, [id], false).length, onselect: () => onchange(setIndent(steps, [id], false)) },
      'separator',
      { label: '삭제', icon: 'trash', danger: true, onselect: () => remove([id]) },
    ]
  }

  function dragover(e: DragEvent, i: number) {
    if (!dragId) return
    e.preventDefault()
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    dropBefore = e.clientY > r.top + r.height / 2 ? (list[i + 1]?.id ?? -1) : list[i].id
  }

  function drop(e: DragEvent) {
    e.preventDefault()
    if (dragId && dropBefore) onchange(moveBlock(steps, dragId, dropBefore === -1 ? 0 : dropBefore))
    dragId = 0
    dropBefore = 0
  }

  // remove는 스텝을 지우고, 지운 자리 다음 스텝을 고른다.
  function remove(ids: number[]) {
    const gone = new Set(ids)
    const last = list.findLastIndex((s) => gone.has(s.id))
    const next = list.slice(last + 1).find((s) => !gone.has(s.id)) ?? list.slice(0, last).findLast((s) => !gone.has(s.id))
    onchange(setDeleted(steps, ids, true))
    selected = next ? [next.id] : []
  }

  function key(e: KeyboardEvent) {
    if (e.key === 'Delete' && selected.length) {
      e.preventDefault()
      remove(selected)
    }
  }

  async function chooseImage(e: Event & { currentTarget: HTMLInputElement }) {
    const f = e.currentTarget.files?.[0]
    e.currentTarget.value = '' // 같은 파일을 다시 골라도 change가 오게
    if (!f) return
    try {
      onaddimage(await fileBase64(f))
    } catch (err) {
      onerror(err instanceof Error ? err.message : String(err))
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<aside class="list" aria-label="스텝 목록" onkeydown={key}>
  <div class="head">
    <h2>스텝 <span class="muted">{list.length}개</span></h2>
    <button
      class="btn sm"
      disabled={busy}
      aria-haspopup="menu"
      aria-expanded={!!addMenu}
      onclick={(e) => (addMenu = addMenu ? null : e.currentTarget)}
      {@attach tip('글만 있는 스텝이나 그림 파일을 넣습니다.\nCtrl+V로 그림을 붙여 넣어도 됩니다.')}
    >
      <Icon name="plus" />스텝 넣기
    </button>
    <input bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif" onchange={chooseImage} hidden />
  </div>

  <ol ondragover={(e) => dragId && e.preventDefault()} ondrop={drop}>
    {#each list as s, i (s.id)}
      {@const num = nums.get(s.id)}
      <li
        class:sub={s.parentId}
        class:hidden={!num}
        class:on={picked.has(s.id)}
        class:drop={dropBefore === s.id}
        draggable="true"
        ondragstart={(e) => {
          dragId = s.id
          e.dataTransfer?.setData('text/plain', String(s.id))
        }}
        ondragover={(e) => dragover(e, i)}
        ondragend={() => {
          dragId = 0
          dropBefore = 0
        }}
      >
        <button
          class="card"
          onclick={(e) => pick(e, s.id)}
          oncontextmenu={(e) => {
            e.preventDefault()
            openMenu(s.id, { x: e.clientX, y: e.clientY })
          }}
          {@attach tip(num ? '끌어서 순서를 바꿉니다.' : '숨긴 스텝은 내보낸 문서에 나오지 않습니다.')}
        >
          <span class="num">{num || '숨김'}</span>
          {#if s.imageSha}
            <img src={`/img/${s.imageSha}.png`} alt="" loading="lazy" draggable="false" />
          {:else}
            <span class="noimg">글</span>
          {/if}
          <span class="title">{s.title || '제목 없음'}</span>
        </button>
        <button
          class="btn icon sm more"
          aria-label={`스텝 ${num || '숨김'} 메뉴`}
          aria-haspopup="menu"
          aria-expanded={menu?.id === s.id}
          onclick={(e) => openMenu(s.id, e.currentTarget)}
          {@attach tip('숨기기 · 하위 스텝으로 · 삭제')}
        >
          <Icon name="ellipsis" />
        </button>
      </li>
    {/each}
    {#if dropBefore === -1}<li class="drop end" aria-hidden="true"></li>{/if}
  </ol>

  {#if selected.length > 1}
    <div class="selection" role="toolbar" aria-label="고른 스텝">
      <span>{selected.length}개 고름</span>
      <button class="btn sm" onclick={() => onchange(setHidden(steps, selected, !allHidden))} {@attach tip(allHidden ? '고른 스텝을 다시 보입니다.' : '고른 스텝을 내보낸 문서에서 뺍니다.')}>
        {allHidden ? '보이기' : '숨기기'}
      </button>
      <button class="btn sm" onclick={() => onchange(setIndent(steps, selected, true))} {@attach tip('고른 스텝을 바로 위 스텝 안의 작은 단계로 만듭니다.')}>하위 스텝으로</button>
      <button class="btn sm" onclick={() => onchange(setIndent(steps, selected, false))} {@attach tip('고른 하위 스텝을 보통 스텝으로 되돌립니다.')}>한 단계 위로</button>
      <button class="btn sm del" onclick={() => remove(selected)} {@attach tip('고른 스텝을 지웁니다 (Delete)\n매뉴얼을 닫기 전에는 되살릴 수 있습니다.')}>삭제</button>
    </div>
  {:else}
    <p class="hint">Ctrl이나 Shift를 누른 채 누르면 여러 개를 고릅니다.</p>
  {/if}

  {#if trash.length}
    <details class="trash">
      <summary>삭제한 스텝 {trash.length}개 · 되살리기</summary>
      <p>매뉴얼을 닫으면 완전히 지워집니다.</p>
      <ul>
        {#each trash as s (s.id)}
          <li>
            <span>{s.title || '제목 없음'}</span>
            <button
              class="btn ghost sm"
              onclick={() => {
                onchange(setDeleted(steps, [s.id], false))
                selected = [s.id]
              }}
              {@attach tip('이 스텝을 목록에 되살립니다.')}
            >
              <Icon name="rotate-ccw" />되살리기
            </button>
          </li>
        {/each}
      </ul>
    </details>
  {/if}
</aside>

{#if menu}
  <Menu label="스텝 메뉴" items={items(menu.id)} at={menu.at} onclose={() => (menu = null)} />
{/if}
{#if addMenu}
  <Menu label="스텝 넣기" items={addItems} at={addMenu} onclose={() => (addMenu = null)} />
{/if}

<style>
  .list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--sidebar);
    border-right: 1px solid var(--border);
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 10px 8px 14px;
  }
  .head h2 {
    margin: 0;
    font-size: 14px;
  }
  ol {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 4px;
    margin: 0;
    padding: 0 8px 8px;
    overflow-y: auto;
    list-style: none;
  }
  li {
    position: relative;
    border-top: 2px solid transparent;
  }
  li.drop {
    border-top-color: var(--accent-border);
  }
  li.end {
    height: 0;
  }
  li.sub {
    margin-left: 18px;
  }
  li.sub::before {
    position: absolute;
    top: -6px;
    left: -10px;
    width: 8px;
    height: 50%;
    content: '';
    border-bottom: 1.5px solid var(--border-strong);
    border-left: 1.5px solid var(--border-strong);
    border-radius: 0 0 0 4px;
  }
  .card {
    display: grid;
    grid-template-columns: 30px 64px 1fr;
    gap: 8px;
    align-items: center;
    width: 100%;
    height: auto;
    padding: 6px 30px 6px 6px;
    text-align: left;
    white-space: normal;
    background: transparent;
    border-color: transparent;
  }
  li.on .card,
  li.on .card:hover:not(:disabled) {
    background: var(--surface);
    border-color: var(--accent-border);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  li.hidden .card {
    opacity: 0.5;
  }
  .num {
    display: grid;
    place-items: center;
    justify-self: start;
    min-width: 22px;
    height: 22px;
    padding: 0 5px;
    font-size: 12px;
    font-weight: 700;
    color: var(--surface);
    background: var(--text);
    border-radius: 999px;
  }
  li.on .num {
    color: var(--on-accent);
    background: var(--accent);
  }
  li.sub .num {
    color: var(--text-2);
    background: transparent;
    border: 1.5px solid var(--border-strong);
  }
  li.hidden .num {
    color: var(--text-3);
    background: transparent;
    border: 1.5px dashed var(--border-strong);
  }
  img,
  .noimg {
    width: 64px;
    height: 40px;
    object-fit: cover;
    background: var(--hover);
    border: 1px solid var(--border);
    border-radius: 4px;
  }
  .noimg {
    display: grid;
    place-items: center;
    font-size: 12px;
    color: var(--text-3);
  }
  .title {
    display: -webkit-box;
    overflow: hidden;
    font-size: 13px;
    line-height: 1.35;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .more {
    position: absolute;
    top: 6px;
    right: 4px;
    opacity: 0;
  }
  li:hover .more,
  li.on .more,
  li:focus-within .more,
  .more[aria-expanded='true'] {
    opacity: 1;
  }
  .selection {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
    font-size: 13px;
    border-top: 1px solid var(--border);
  }
  .selection .del {
    color: var(--danger);
  }
  .hint {
    margin: 0;
    padding: 8px 12px;
    font-size: 12px;
    color: var(--text-3);
    border-top: 1px solid var(--border);
  }
  .trash {
    padding: 6px 12px 10px;
    font-size: 12px;
    color: var(--text-2);
    border-top: 1px solid var(--border);
  }
  .trash summary {
    cursor: pointer;
  }
  .trash p {
    margin: 4px 0;
    color: var(--text-3);
  }
  .trash ul {
    display: grid;
    gap: 4px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .trash li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    border: 0;
  }
</style>
