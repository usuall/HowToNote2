<!-- 편집기(스펙 6.1절): 위 막대(처음 화면, 매뉴얼 제목, 저장 상태, 3단계 막대, 실행 취소, 이어서 녹화, 내보내기, UI 설계 5.4절), 왼쪽 스텝 목록, 오른쪽 고른 스텝.
     모든 바꾸기는 change()로 모여 실행 취소 기록(가이드 단위, 메모리)과 마지막 변경 1초 뒤 자동 저장(스펙 6.3절)을 거친다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { AddImageStep, AddTextStep, CloseGuide, ContinueRecording, OpenGuide, RenameGuide, RevealFile, SaveSteps } from '../../wailsjs/go/main/App'
  import { errorText, fileBase64 } from '../api'
  import { forgetHistory, historyFor, numbering, placeAfter, type Step } from './model'
  import StepList from './StepList.svelte'
  import StepPane from './StepPane.svelte'
  import ExportDialog from '../export/ExportDialog.svelte'
  import Toast from '../ui/Toast.svelte'
  import Icon from '../ui/Icon.svelte'
  import StepBar from '../ui/StepBar.svelte'
  import { tip } from '../ui/tip'

  let {
    id,
    onhome,
    onrecording,
    onstarting,
  }: { id: number; onhome: () => void; onrecording: () => void; onstarting: () => void } = $props()

  const history = $derived(historyFor(id))
  let title = $state('')
  let steps = $state<Step[]>([])
  let selected = $state<number[]>([])
  let status = $state<'saved' | 'pending' | 'saving' | 'failed'>('saved')
  let busy = $state(false)
  let error = $state('')
  let exporting = $state(false) // 내보내기 창이 열려 있는지. ponytail: 열린 채로 닫으면(D13) 쓰다 만 문서가 남을 수 있다
  let saved = $state<{ format: string; path: string } | null>(null) // 방금 저장한 파일(알림)
  const SAVED_AS: Record<string, string> = { pdf: 'PDF', hwpx: '한글 문서', docx: '워드 문서', html: '웹 문서', htn: 'HowtoNote 파일' }

  const current = $derived(steps.find((s) => s.id === selected.at(-1) && !s.deleted))
  const nums = $derived(numbering(steps)) // 오른쪽 칸의 "스텝 3" 표시. 목록과 같은 번호다
  // History는 반응형이 아니다. 기록이 바뀌는 곳(change·undo·redo·add)은 모두 steps를 새로 넣으므로 steps를 따라 다시 읽는다.
  const canUndo = $derived.by(() => (void steps, history.canUndo))
  const canRedo = $derived.by(() => (void steps, history.canRedo))

  const dirty = new Set<number>()
  let timer: ReturnType<typeof setTimeout> | undefined
  let saving: Promise<void> = Promise.resolve()
  let saveError = '' // save가 띄운 오류. 저장이 다시 되면 이 오류만 지운다(그사이 다른 오류가 떴으면 남긴다)
  let renaming: Promise<void> = Promise.resolve() // 가이드 제목 저장(닫기 전에 기다린다)

  onMount(async () => {
    try {
      const d = await OpenGuide(id)
      title = d.guide.title
      steps = [...d.steps, ...d.deleted]
      selected = d.steps.length ? [d.steps[0].id] : []
    } catch (e) {
      error = errorText(e)
    }
  })

  // change는 편집기의 모든 바꾸기가 지나는 곳이다. 바뀐 행의 이전 값을 실행 취소 기록에 담고 화면과 저장에 반영한다.
  function change(rows: Step[], key = '') {
    if (!rows.length) return
    const before = rows.map((r) => $state.snapshot(steps.find((s) => s.id === r.id)!))
    history.push(before, rows.map((r) => $state.snapshot(r)), key)
    apply(rows)
  }

  // apply는 행을 목록에 덮어쓰고(없으면 더하고) 1초 뒤 저장을 예약한다. 실행 취소·다시 실행도 이 길로 온다.
  function apply(rows: Step[]) {
    const byId = new Map(rows.map((r) => [r.id, r]))
    steps = [...steps.map((s) => byId.get(s.id) ?? s), ...rows.filter((r) => !steps.some((s) => s.id === r.id))]
    for (const r of rows) dirty.add(r.id)
    status = 'pending'
    schedule(1000)
  }

  function schedule(ms: number) {
    clearTimeout(timer)
    timer = setTimeout(() => flush().catch(() => {}), ms)
  }

  // flush는 기다리는 저장을 지금 한다. 저장은 한 번에 하나씩 차례로 하고, 실패하면 다음 저장 때 다시 보낸다.
  function flush(): Promise<void> {
    clearTimeout(timer)
    const run = saving.then(save)
    saving = run.catch(() => {})
    return run
  }

  async function save() {
    if (!dirty.size) return
    const ids = [...dirty]
    dirty.clear()
    status = 'saving'
    try {
      await SaveSteps(id, ids.map((i) => $state.snapshot(steps.find((s) => s.id === i)!)))
      status = dirty.size ? 'pending' : 'saved'
      if (error === saveError) error = ''
      saveError = ''
    } catch (e) {
      for (const i of ids) dirty.add(i)
      status = 'failed'
      error = saveError = errorText(e)
      schedule(5000) // 더 고치지 않아도 다시 저장해 본다(디스크 공간을 비우면 저절로 저장된다)
      throw e
    }
  }

  function undo() {
    const rows = history.undo()
    if (rows) show(rows)
  }

  function redo() {
    const rows = history.redo()
    if (rows) show(rows)
  }

  // show는 되돌린 행을 반영하고, 지운 스텝이 아니면 그 스텝을 골라 무엇이 바뀌었는지 보이게 한다.
  // 모두 지운 행이면(되살리기를 되돌림) 고른 목록에서 이제 보이지 않는 스텝을 뺀다.
  function show(rows: Step[]) {
    apply(rows)
    const live = rows.find((r) => !r.deleted)
    selected = live ? [live.id] : selected.filter((i) => steps.some((s) => s.id === i && !s.deleted))
  }

  // add는 글·그림 스텝을 고른 스텝 바로 뒤에 넣는다. 넣기의 실행 취소는 그 스텝을 지운 것으로 표시한다.
  async function add(call: (sort: number, parentId: number) => Promise<Step>) {
    busy = true
    error = ''
    try {
      const at = placeAfter(steps, current?.id ?? 0)
      const row = await call(at.sort, at.parentId)
      history.push([{ ...row, deleted: true }], [row])
      steps = [...steps, row]
      selected = [row.id]
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  const addText = () => add((sort, parent) => AddTextStep(id, sort, parent))
  const addImage = (data: string) => add((sort, parent) => AddImageStep(id, sort, parent, data))

  // settle은 저장이 걸릴 수 있는 모든 것을 정리한다: 초점이 빠질 때 저장하는 칸(그림 위 글 고치기, 가이드 제목)을 먼저
  // blur해 그 값을 저장 대기에 넣고, 제목 저장과 남은 편집 저장을 기다린다. close()와 내보내기 전(beforeExport)이 같이 쓴다.
  async function settle() {
    ;(document.activeElement as HTMLElement | null)?.blur()
    await renaming
    await flush()
  }

  // close는 가이드를 닫는다: 남은 편집을 저장하고, 지운 스텝을 영구히 지우고, 실행 취소 기록을 버린다(스펙 6.3절).
  // App.svelte가 창을 닫기 전에도 부른다. busy 동안은 실행 취소를 막아, 저장 뒤 영구 삭제 전에 행이 바뀌지 않게 한다.
  export async function close() {
    busy = true
    try {
      await settle()
      await CloseGuide(id)
      forgetHistory(id)
    } catch (e) {
      error = errorText(e)
      throw e
    } finally {
      busy = false
    }
  }

  async function run(call: () => Promise<void>) {
    busy = true
    try {
      await call()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  const home = () => run(async () => {
    await close()
    onhome()
  })

  // onstarting()은 녹화 툴바로 바뀌기 전에 불러, 지난 녹화의 스텝 수·썸네일·안내·일시정지 상태가 남아 있지 않게 한다(Ruling M2-C4).
  const continueRecording = () => run(async () => {
    error = ''
    await flush()
    onstarting()
    await ContinueRecording(id)
    onrecording()
  })

  // rename은 제목 칸의 change(초점이 빠질 때)다. close가 기다릴 수 있게 약속을 남긴다.
  // 칸은 미리 잡아 둔다: await 뒤에는 e.currentTarget이 비어 있다.
  function rename(e: Event & { currentTarget: HTMLInputElement }) {
    const input = e.currentTarget
    const v = input.value
    renaming = RenameGuide(id, v).then(
      () => {
        title = v.trim()
      },
      (err) => {
        error = errorText(err)
        input.value = title
      },
    )
  }

  // 글을 쓰는 칸 밖에서만 편집기의 실행 취소를 쓴다. 글 칸 안의 Ctrl+Z는 그 칸 자체의 실행 취소다.
  function keydown(e: KeyboardEvent) {
    if (exporting || busy || !e.ctrlKey || (e.target as HTMLElement).closest('input, textarea, select, [contenteditable="true"]')) return
    const k = e.key.toLowerCase()
    if (k === 'z' && !e.shiftKey) {
      e.preventDefault()
      undo()
    } else if (k === 'y' || (k === 'z' && e.shiftKey)) {
      e.preventDefault()
      redo()
    }
  }

  // 글 칸 밖에서 붙여넣은 그림은 새 그림 스텝이 된다. 설명·안내 상자 본문에는 그림을 넣을 수 없어(허용 서식) 그대로 두면
  // 말없이 사라지므로, 글 없이 그림만 붙여넣었으면 거기서도 그림 스텝으로 넣는다. 워드·엑셀에서 복사한 글에는 그림 사본이
  // 같이 오므로 글(text/plain)이 있으면 설명 편집기에 맡긴다. 설명 편집기보다 먼저 받으려고 캡처 단계에서 듣는다.
  async function paste(e: ClipboardEvent) {
    if (exporting) return
    const t = e.target as HTMLElement
    if (t.closest('input, textarea')) return
    if (t.closest('[contenteditable="true"]') && e.clipboardData?.types.includes('text/plain')) return
    const f = [...(e.clipboardData?.files ?? [])].find((x) => x.type.startsWith('image/'))
    if (!f) return
    e.preventDefault()
    e.stopPropagation()
    try {
      await addImage(await fileBase64(f))
    } catch (err) {
      error = errorText(err)
    }
  }

  const STATUS = { saved: '저장됨', pending: '저장 대기 중', saving: '저장 중…', failed: '저장 못 함' }
</script>

<svelte:window onkeydown={keydown} onpastecapture={paste} />

<div class="editor">
  <header class="top">
    <button class="btn ghost" disabled={busy} onclick={home} {@attach tip('저장하고 첫 화면으로 돌아갑니다.')}>
      <Icon name="arrow-left" />처음 화면
    </button>
    <span class="sep" aria-hidden="true"></span>
    <input class="title" aria-label="매뉴얼 제목" value={title} onchange={rename} />
    <span
      class="status"
      class:failed={status === 'failed'}
      role="status"
      {@attach tip(status === 'failed' ? '저장하지 못했습니다. 5초 뒤 저절로 다시 저장해 봅니다.' : '고친 내용은 1초 뒤 저절로 저장됩니다.')}
    >
      {#if status === 'saved'}<Icon name="check" size={14} />{:else if status === 'failed'}<Icon name="alert-triangle" size={14} />{/if}
      {STATUS[status]}
    </span>
    <StepBar current={exporting ? 3 : 2} />
    <span class="sep" aria-hidden="true"></span>
    <!-- 막힌 단추는 마우스 이벤트를 받지 않아, 툴팁은 감싼 요소에 단다 -->
    <span {@attach tip('실행 취소 (Ctrl+Z)')}>
      <button class="btn icon" aria-label="실행 취소" disabled={busy || !canUndo} onclick={undo}><Icon name="undo" /></button>
    </span>
    <span {@attach tip('다시 실행 (Ctrl+Y)')}>
      <button class="btn icon" aria-label="다시 실행" disabled={busy || !canRedo} onclick={redo}><Icon name="redo" /></button>
    </span>
    <button
      class="btn continue"
      aria-label="이어서 녹화"
      disabled={busy}
      onclick={continueRecording}
      {@attach tip('빠진 장면을 더 찍습니다. 새 스텝은 목록 끝에 붙습니다.')}
    >
      <span class="dot" aria-hidden="true"></span><span class="label">이어서 녹화</span>
    </button>
    <button class="btn primary" disabled={busy} onclick={() => (exporting = true)} {@attach tip('PDF, 한글(HWPX), 워드, 웹 문서나 HowtoNote 파일로 저장합니다.')}>
      <Icon name="export" />내보내기
    </button>
  </header>
  {#if error}<p class="banner error" role="alert"><Icon name="alert-triangle" />{error}</p>{/if}
  {#if exporting}
    <ExportDialog
      guideId={id}
      beforeExport={settle}
      onclose={() => (exporting = false)}
      ondone={(format, path) => {
        exporting = false
        saved = { format, path }
      }}
    />
  {/if}
  {#if saved}
    {@const file = saved}
    <Toast
      message={`${SAVED_AS[file.format] ?? '문서'}로 저장했습니다.`}
      detail={file.path}
      action={{ label: '폴더 열기', tip: '저장한 파일을 탐색기에서 고른 채로 엽니다.', run: () => RevealFile(file.path).catch((e) => (error = errorText(e))) }}
      onclose={() => (saved = null)}
    />
  {/if}
  <div class="panes">
    <StepList {steps} bind:selected {busy} onchange={(rows) => change(rows)} onaddtext={addText} onaddimage={addImage} onerror={(m) => (error = m)} />
    {#if current}
      {#key current.id}
        <StepPane step={current} num={nums.get(current.id) ?? ''} onchange={change} />
      {/key}
    {:else}
      <p class="empty">왼쪽 목록에서 스텝을 고르세요. 스텝이 없으면 '스텝 넣기'나 '이어서 녹화'를 누르세요.</p>
    {/if}
  </div>
</div>

<style>
  .editor {
    display: grid;
    grid-template-rows: auto auto 1fr;
    /* 열 폭을 창에 묶는다. 그냥 두면 열이 내용의 최대 폭(긴 설명 한 줄 등)만큼 넓어져 위 막대와 그림 칸이 창 밖으로 밀린다 */
    grid-template-columns: minmax(0, 1fr);
    height: 100vh;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 52px;
    padding: 0 12px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .sep {
    flex: none;
    width: 1px;
    height: 22px;
    background: var(--border);
  }
  .title {
    flex: 1;
    min-width: 80px;
    font-size: 15px;
    font-weight: 700;
    background: transparent;
    border-color: transparent;
  }
  .title:hover,
  .title:focus {
    background: var(--surface);
    border-color: var(--border-strong);
  }
  .status {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 76px;
    font-size: 12px;
    color: var(--text-3);
    white-space: nowrap;
  }
  .status :global(svg) {
    color: var(--success);
  }
  .status.failed,
  .status.failed :global(svg) {
    color: var(--danger);
  }
  .continue .dot {
    width: 9px;
    height: 9px;
    background: var(--rec);
    border-radius: 50%;
  }
  .banner {
    margin: 8px 16px 0;
  }
  .panes {
    display: grid;
    grid-template-columns: 236px 1fr;
    grid-row: 3; /* 오류 띠가 없을 때도 늘 1fr 행을 채운다(F5) */
    min-height: 0;
  }
  .empty {
    padding: 24px;
    color: var(--text-2);
  }
  /* 좁은 창(UI 설계 4.10절): 위 막대의 고정 폭 부품이 약 810px라 1000px보다 좁으면 '이어서 녹화'를 아이콘만 남긴다 */
  @media (max-width: 1000px) {
    .continue .label {
      display: none;
    }
    .continue {
      width: 34px;
      padding: 0;
    }
  }
</style>
