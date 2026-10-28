<!-- 첫 화면(UI 설계 5.1절): 위 막대(로고, 3단계 막대), 왼쪽 폴더 막대, 새 매뉴얼 카드(녹화 범위, 녹화 시작), 매뉴얼 카드 목록.
     폴더·매뉴얼 관리는 ⋯ 메뉴와 확인 창으로 한다. 바인딩 호출은 run()으로 감싸 끝날 때까지 단추를 막고 목록을 다시 읽는다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import {
    BeginRegionSelect, CreateFolder, DeleteFolder, DeleteGuide, ImportProject, ListFolders, ListGuides, MoveGuide, RenameFolder,
    RenameGuide, StartRecording,
  } from '../wailsjs/go/main/App'
  import type { store } from '../wailsjs/go/models'
  import { errorText } from './api'
  import FolderBar from './FolderBar.svelte'
  import GuideCard from './GuideCard.svelte'
  import ConfirmDialog from './ui/ConfirmDialog.svelte'
  import Icon from './ui/Icon.svelte'
  import type { IconName } from './ui/icons'
  import Toast from './ui/Toast.svelte'
  import StepBar from './ui/StepBar.svelte'
  import { tip } from './ui/tip'

  let {
    notice,
    onstarting,
    onstarted,
    onregion,
    onopen,
  }: {
    notice: string
    onstarting: () => void
    onstarted: () => void
    onregion: (width: number, height: number) => void
    onopen: (id: number) => void
  } = $props()

  type Mode = 'screen' | 'window' | 'region'
  // [값, 이름, 아이콘, 툴팁]. 값은 Go StartRecording이 받는 녹화 범위다(region은 영역 고르기 화면을 거친다)
  const MODES: [Mode, string, IconName, string][] = [
    ['screen', '전체 화면', 'monitor', '모니터 화면 전체를 찍습니다.'],
    ['window', '창 하나', 'app-window', '클릭하는 창만 찍습니다.'],
    ['region', '영역 지정', 'region', '마우스로 끌어서 고른 부분만 찍습니다.'],
  ]
  const ARROWS: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }

  let mode = $state<Mode>('screen')
  let guides = $state<store.Guide[]>([])
  let folders = $state<store.Folder[]>([])
  let folder = $state(0) // 보고 있는 폴더. 0이면 모든 매뉴얼
  let renaming = $state(0) // 카드 제목 자리에서 이름을 고치는 매뉴얼
  let asking = $state<{ kind: 'guide' | 'folder'; id: number; title: string } | null>(null) // 지울지 묻는 대상
  let loaded = $state(false) // 처음 읽기 전에는 빈 화면 안내를 띄우지 않는다
  let busy = $state(false)
  let error = $state('')
  let imported = $state(0) // 방금 가져온 매뉴얼. 알림의 '열기'가 연다

  const shown = $derived(folder ? guides.filter((g) => g.folderId === folder) : guides)
  const current = $derived(folders.find((f) => f.id === folder))
  const counts = $derived(new Map(folders.map((f) => [f.id, guides.filter((g) => g.folderId === f.id).length])))

  async function load() {
    const [gs, fs] = await Promise.all([ListGuides(), ListFolders()])
    guides = gs ?? []
    folders = fs ?? []
    if (!folders.some((f) => f.id === folder)) folder = 0
    loaded = true
  }

  onMount(() => {
    load().catch((e) => (error = errorText(e)))
  })

  // run은 관리 동작 하나의 바인딩 호출을 감싼다: 끝날 때까지 단추를 막고, 성공하면 목록을 다시 읽는다. 성공 여부를 돌려준다.
  async function run(call: () => Promise<unknown>): Promise<boolean> {
    busy = true
    error = ''
    try {
      await call()
      await load()
      return true
    } catch (e) {
      error = errorText(e)
      return false
    } finally {
      busy = false
    }
  }

  async function start() {
    busy = true
    error = ''
    try {
      if (mode === 'region') {
        const f = await BeginRegionSelect()
        onregion(f.width, f.height)
      } else {
        onstarting() // step/warning 이벤트가 StartRecording이 끝나기 전에 올 수 있어, 여기서 먼저 상태를 비운다
        await StartRecording(mode)
        onstarted()
      }
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  // modeKey는 녹화 범위 세그먼트의 화살표 키다. 라디오 묶음처럼 옮기면 바로 고르고, 고른 조각만 Tab을 받는다.
  function modeKey(e: KeyboardEvent & { currentTarget: HTMLElement }) {
    const d = ARROWS[e.key]
    if (!d) return
    e.preventDefault()
    const i = (MODES.findIndex(([m]) => m === mode) + d + MODES.length) % MODES.length
    mode = MODES[i][0]
    e.currentTarget.parentElement?.querySelectorAll<HTMLElement>('[role=radio]')[i]?.focus()
  }

  async function renameGuide(id: number, title: string) {
    if (await run(() => RenameGuide(id, title))) renaming = 0
  }

  // remove는 확인 창의 '지우기'다. 실패해도 창을 닫아, 그 뒤에 뜬 오류 띠가 보이게 한다.
  async function remove() {
    const a = asking
    if (!a) return
    await run(() => (a.kind === 'guide' ? DeleteGuide(a.id) : DeleteFolder(a.id)))
    asking = null
  }

  // importFile은 '가져오기'다. 보고 있는 폴더에 넣고(M4a H5), 끝나면 알림에서 바로 열 수 있게 한다. 취소하면 0이라 알림이 없다.
  async function importFile() {
    let id = 0
    if ((await run(async () => (id = await ImportProject(folder)))) && id) imported = id
  }
</script>

<div class="home">
  <header class="top">
    <span class="logo"><b aria-hidden="true">H</b>HowtoNote</span>
    <StepBar current={1} />
  </header>

  <div class="body">
    <FolderBar
      {folders}
      {counts}
      total={guides.length}
      {folder}
      {busy}
      onpick={(id) => (folder = id)}
      oncreate={(name) => run(() => CreateFolder(name))}
      onrename={(id, name) => run(() => RenameFolder(id, name))}
      ondelete={(f) => (asking = { kind: 'folder', id: f.id, title: f.title })}
    />

    <main class="main">
      <section class="new" aria-labelledby="new-title">
        <h1 id="new-title">새 매뉴얼 만들기</h1>
        <p>'녹화 시작'을 누르고 평소처럼 클릭하세요. 클릭할 때마다 화면이 한 장씩 찍힙니다.</p>
        <div class="row">
          <span class="label" id="mode-label">녹화 범위</span>
          <div class="seg" role="radiogroup" aria-labelledby="mode-label">
            {#each MODES as [m, name, icon, help] (m)}
              <button
                role="radio"
                aria-checked={mode === m}
                tabindex={mode === m ? 0 : -1}
                disabled={busy}
                onclick={() => (mode = m)}
                onkeydown={modeKey}
                {@attach tip(help)}
              >
                <Icon name={icon} />{name}
              </button>
            {/each}
          </div>
          <button
            class="btn primary start"
            disabled={busy}
            onclick={start}
            {@attach tip('녹화를 시작합니다.\n일시정지 Ctrl+Shift+F9 · 끝내기 Ctrl+Shift+F11')}
          >
            <span class="dot" aria-hidden="true"></span>녹화 시작
          </button>
        </div>
      </section>

      {#if error}
        <p class="banner error" role="alert"><Icon name="alert-triangle" />{error}</p>
      {:else if notice}
        <p class="banner notice" role="status">{notice}</p>
      {/if}

      <div class="list-head">
        <h2>{current?.title ?? '모든 매뉴얼'}</h2>
        <span class="muted">{shown.length}개 · 최근에 고친 순</span>
        <button
          class="btn sm import"
          disabled={busy}
          onclick={importFile}
          {@attach tip('다른 PC에서 받은 HowtoNote 파일(.htn)을 지금 보고 있는 폴더에 매뉴얼로 가져옵니다.')}
        >
          <Icon name="import" />가져오기
        </button>
      </div>

      {#if loaded && guides.length === 0}
        <div class="empty">
          <h3>아직 만든 매뉴얼이 없습니다</h3>
          <p>위에서 '녹화 시작'을 누르면 첫 매뉴얼이 생깁니다. 이렇게 세 단계로 끝납니다.</p>
          <ol class="how">
            <li><b>녹화하기</b><span>평소처럼 클릭하며 일을 따라 합니다. 클릭할 때마다 한 장씩 찍힙니다.</span></li>
            <li><b>다듬기</b><span>찍힌 화면에 표시를 하고, 순서를 바꾸고, 설명을 씁니다.</span></li>
            <li><b>내보내기</b><span>PDF, 한글, 워드, 웹 문서로 저장해 나눠 줍니다.</span></li>
          </ol>
        </div>
      {:else if loaded && shown.length === 0}
        <p class="empty-folder">이 폴더는 비어 있습니다. 매뉴얼 카드의 ⋯ 메뉴에서 이 폴더로 옮길 수 있습니다.</p>
      {:else}
        <ul class="grid">
          {#each shown as g (g.id)}
            <li>
              <GuideCard
                guide={g}
                {folders}
                {busy}
                renaming={renaming === g.id}
                onopen={() => onopen(g.id)}
                onstartrename={() => (renaming = g.id)}
                oncancelrename={() => (renaming = 0)}
                onrename={(t) => renameGuide(g.id, t)}
                onmove={(id) => run(() => MoveGuide(g.id, id))}
                ondelete={() => (asking = { kind: 'guide', id: g.id, title: g.title })}
              />
            </li>
          {/each}
        </ul>
      {/if}
    </main>
  </div>
</div>

{#if asking}
  <ConfirmDialog
    title={asking.kind === 'guide' ? '매뉴얼 지우기' : '폴더 지우기'}
    message={asking.kind === 'guide'
      ? `'${asking.title}' 매뉴얼을 지울까요? 지운 매뉴얼은 되살릴 수 없습니다.`
      : `'${asking.title}' 폴더를 지울까요? 안의 매뉴얼은 지워지지 않고 '모든 매뉴얼'에 남습니다.`}
    oncancel={() => (asking = null)}
  >
    <button class="btn ghost" disabled={busy} onclick={() => (asking = null)} {@attach tip('지우지 않고 돌아갑니다 (Esc)')}>취소</button>
    <button class="btn danger" disabled={busy} onclick={remove} {@attach tip('지금 지웁니다.')}>지우기</button>
  </ConfirmDialog>
{/if}

{#if imported}
  {@const id = imported}
  <Toast
    message="매뉴얼을 가져왔습니다."
    action={{ label: '열기', icon: 'pencil', tip: '가져온 매뉴얼을 편집기에서 엽니다.', run: () => onopen(id) }}
    onclose={() => (imported = 0)}
  />
{/if}

<style>
  .home {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100vh;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 24px;
    height: 52px;
    padding: 0 18px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .logo {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 15px;
    font-weight: 700;
  }
  .logo b {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    font-size: 13px;
    color: var(--on-accent);
    background: var(--accent);
    border-radius: var(--radius-sm);
  }
  .body {
    display: flex;
    min-height: 0;
  }
  .main {
    display: grid;
    flex: 1;
    align-content: start;
    gap: 12px;
    min-width: 0;
    padding: 20px 24px 32px;
    overflow-y: auto;
  }
  .new {
    padding: 18px 20px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-sm);
  }
  .new h1 {
    margin: 0 0 2px;
    font-size: 18px;
  }
  .new p {
    margin: 0 0 14px;
    color: var(--text-2);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .label {
    font-size: 13px;
    color: var(--text-2);
  }
  .start {
    margin-left: auto;
  }
  .dot {
    width: 10px;
    height: 10px;
    background: var(--rec);
    border-radius: 50%;
  }
  .list-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 8px;
  }
  .list-head h2 {
    margin: 0;
    font-size: 15px;
  }
  .import {
    margin-left: auto;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 12px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .empty {
    padding: 26px;
    text-align: center;
    background: var(--sunken);
    border: 1.5px dashed var(--border-strong);
    border-radius: var(--radius);
  }
  .empty h3 {
    margin: 0 0 4px;
    font-size: 15px;
  }
  .empty p {
    margin: 0;
    color: var(--text-2);
  }
  .how {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin: 18px 0 0;
    padding: 0;
    text-align: left;
    list-style: none;
    counter-reset: how;
  }
  .how li {
    display: grid;
    gap: 2px;
    padding: 12px 14px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    counter-increment: how;
  }
  .how li::before {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    margin-bottom: 4px;
    font-size: 12px;
    font-weight: 700;
    color: var(--on-accent);
    content: counter(how);
    background: var(--accent);
    border-radius: 50%;
  }
  .how span {
    font-size: 13px;
    color: var(--text-2);
  }
  .empty-folder {
    margin: 0;
    padding: 18px;
    color: var(--text-2);
    background: var(--sunken);
    border-radius: var(--radius);
  }
</style>
