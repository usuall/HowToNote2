<!-- 내보내기 창(UI 설계 5.5절, 스펙 7.3절): 형식 카드 5개(문서 4종과 HowtoNote 파일)와 '저장'만 누르면 끝나고, 세부 옵션은 '고급 설정'에 접어 둔다.
     저장: 편집 저장 → 파일 고르기 → 합성 그림 준비(EnsureRendered) → Go가 문서를 만들어 쓴다. 끝나면 ondone으로 알려
     편집기가 창을 닫고 알림(토스트)을 띄운다. 저장하는 동안은 Esc와 닫기를 막는다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { ChooseExportFile, DeletePreset, ExportGuide, ExportProject, ListPresets, ProjectPlan, SavePreset } from '../../wailsjs/go/main/App'
  import { store } from '../../wailsjs/go/models'
  import { errorText } from '../api'
  import Icon from '../ui/Icon.svelte'
  import { tip } from '../ui/tip'
  import { ensureRendered } from './ensure'

  let {
    guideId,
    beforeExport,
    onclose,
    ondone,
  }: {
    guideId: number
    beforeExport: () => Promise<void>
    onclose: () => void
    ondone: (format: string, path: string) => void
  } = $props()

  // {#if}로 떼어내면 포커스가 body로 빠지므로, 열 때 초점을 적어 두고 닫을 때 되돌린다(Menu와 같은 방식)
  const before = document.activeElement as HTMLElement | null

  // Go export.Options와 같은 칸(JSON 이름). 기본값은 Go가 문서 모양 목록에 채워 보낸다.
  type Options = {
    paper: string
    landscape: boolean
    cover: boolean
    toc: boolean
    stepPerPage: boolean
    imageWidth: number
    header: string
    footer: string
    font: string
    embedFont: boolean
  }

  // badge 색은 파일 형식을 알아보게 하는 표식 색이라(내용 색) 변수 대신 그대로 둔다
  const FORMATS = [
    { id: 'pdf', name: 'PDF', badge: 'PDF', color: '#d64545', note: '인쇄하거나 그대로 나눠 줄 때' },
    { id: 'hwpx', name: '한글', badge: 'HWPX', color: '#1f8fbf', note: '한/글에서 고쳐 쓸 때' },
    { id: 'docx', name: '워드', badge: 'DOCX', color: '#2b5bb8', note: 'Word에서 고쳐 쓸 때' },
    { id: 'html', name: '웹 문서', badge: 'HTML', color: '#2f9e44', note: '브라우저로 열어 볼 때' },
    { id: 'htn', name: 'HowtoNote', badge: 'HTN', color: '#7048e8', note: '다른 PC의 HowtoNote에서 이어 쓸 때' },
  ]
  // .htn 종류(M4a H1·H3·H10). 처음에는 편집용이 골라져 있다
  const KINDS = [
    { id: 'edit', name: '고쳐 쓸 사람에게', note: '그림과 그리기를 그대로 넣습니다. 받은 사람이 HowtoNote에서 이어서 고칩니다.' },
    {
      id: 'publish',
      name: '나눠 줄 사람에게',
      note: '모자이크와 자르기를 그림에 확정합니다. 받은 사람은 가린 부분을 되살릴 수 없고, 숨긴 스텝은 넣지 않습니다.',
    },
  ]
  const ARROWS: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }
  const PRESET_HELP = '용지, 표지, 목차, 머리글 같은 설정을 묶어 둔 것입니다. 고급 설정에서 바꾼 것을 새 모양으로 저장할 수 있습니다.'

  let presets = $state<store.Preset[]>([])
  let presetId = $state(0)
  let format = $state('pdf')
  let options = $state<Options | null>(null)
  let newName = $state('')
  let busy = $state(false)
  let status = $state('') // 저장하는 동안의 진행 글
  let progress = $state(0) // 0~1
  let error = $state('')
  let kind = $state('edit')

  const formatName = $derived(FORMATS.find((f) => f.id === format)?.name ?? '')

  onMount(async () => {
    try {
      presets = (await ListPresets()) ?? []
      if (presets.length) pick(presets[0])
    } catch (e) {
      error = errorText(e)
    }
  })

  function pick(p: store.Preset) {
    presetId = p.id
    format = p.format || 'pdf'
    options = JSON.parse(p.options)
  }

  function choosePreset(e: Event & { currentTarget: HTMLSelectElement }) {
    const p = presets.find((x) => x.id === Number(e.currentTarget.value))
    if (p) pick(p)
  }

  // formatKey는 형식 카드 묶음의 화살표 키다(라디오 묶음: 옮기면 바로 고르고, 고른 카드만 Tab을 받는다).
  function formatKey(e: KeyboardEvent & { currentTarget: HTMLElement }) {
    const d = ARROWS[e.key]
    if (!d) return
    e.preventDefault()
    const i = (FORMATS.findIndex((f) => f.id === format) + d + FORMATS.length) % FORMATS.length
    format = FORMATS[i].id
    e.currentTarget.parentElement?.querySelectorAll<HTMLElement>('[role=radio]')[i]?.focus()
  }

  async function run(call: () => Promise<void>) {
    busy = true
    error = ''
    try {
      await call()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
      status = ''
      progress = 0
    }
  }

  const save = () =>
    run(async () => {
      const htn = format === 'htn'
      if (!htn && !options) return
      await beforeExport()
      const path = await ChooseExportFile(guideId, format)
      if (!path) return
      // 문서는 늘, .htn은 배포용일 때만 그림을 준비한다(편집용은 원본 그림을 그대로 넣는다)
      if (!htn || kind === 'publish') {
        const what = htn ? '그림 확정 중' : '그림 준비 중'
        status = what
        await ensureRendered(
          guideId,
          (done, total) => {
            status = `${what} ${done} / ${total}`
            progress = total ? (done / total) * 0.8 : 0.8
          },
          htn ? ProjectPlan : undefined,
        )
      }
      status = htn ? '파일 만드는 중…' : '문서 만드는 중…'
      progress = 0.9
      if (htn) await ExportProject(guideId, kind, path)
      else await ExportGuide(guideId, format, JSON.stringify(options), path)
      ondone(format, path)
    })

  // savePreset은 지금 설정을 이름 붙여 저장한다. 같은 이름이 있으면 그 문서 모양을 고친다.
  const savePreset = () =>
    run(async () => {
      const name = newName.trim()
      const same = presets.find((p) => p.name === name)
      const id = await SavePreset(store.Preset.createFrom({ id: same?.id ?? 0, name, format, options: JSON.stringify(options) }))
      presets = (await ListPresets()) ?? []
      presetId = id
      newName = ''
    })

  const deletePreset = () =>
    run(async () => {
      await DeletePreset(presetId)
      presets = (await ListPresets()) ?? []
      if (presets.length) pick(presets[0])
    })
</script>

<dialog
  class="dialog"
  aria-labelledby="export-title"
  oncancel={(e) => {
    e.preventDefault() // 닫기는 편집기가 {#if}를 내려서 한다
    if (!busy) onclose()
  }}
  onclose={(e) => {
    // Chromium은 Esc를 거듭 누르면 preventDefault를 무시하고 닫는다. 저장하는 동안이면 다시 띄운다.
    if (!e.currentTarget.isConnected) return
    if (busy) e.currentTarget.showModal()
    else onclose()
  }}
  {@attach (d) => { d.showModal(); return () => before?.focus() }}
>
  <div class="head">
    <div>
      <h2 id="export-title">내보내기</h2>
      <p>{busy && status ? `${formatName}로 만드는 중입니다. 창을 닫지 마세요.` : "어떤 문서로 저장할까요? 고르고 '저장'을 누르면 끝납니다."}</p>
    </div>
    <button class="btn icon sm" aria-label="닫기" disabled={busy} onclick={onclose} {@attach tip()}><Icon name="x" /></button>
  </div>

  <div class="body">
    <div class="formats" role="radiogroup" aria-label="파일 형식">
      {#each FORMATS as f (f.id)}
        <button
          class="format"
          role="radio"
          aria-checked={format === f.id}
          tabindex={format === f.id ? 0 : -1}
          disabled={busy}
          onclick={() => (format = f.id)}
          onkeydown={formatKey}
          {@attach tip(`${f.name}: ${f.note}`)}
        >
          {#if format === f.id}<span class="check" aria-hidden="true"><Icon name="check" size={12} /></span>{/if}
          <span class="doc" aria-hidden="true"><b style:background={f.color}>{f.badge}</b></span>
          <strong>{f.name}</strong>
          <small>{f.note}</small>
        </button>
      {/each}
    </div>

    {#if format === 'htn'}
      <fieldset class="kinds" disabled={busy}>
        <legend>누구에게 보내나요?</legend>
        {#each KINDS as k (k.id)}
          <label class="kind" class:on={kind === k.id}>
            <input type="radio" name="htn-kind" value={k.id} bind:group={kind} />
            <span><b>{k.name}</b><small>{k.note}</small></span>
          </label>
        {/each}
      </fieldset>
    {:else}
      <div class="line">
        <label for="preset">문서 모양</label>
        <select id="preset" aria-describedby="preset-help" value={presetId} onchange={choosePreset} disabled={busy}>
          {#each presets as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
        </select>
        <span class="what" {@attach tip(PRESET_HELP)}>무엇인가요?</span>
        <span id="preset-help" class="sr-only">{PRESET_HELP}</span>
      </div>

      {#if options}
        <details class="advanced">
          <summary>
            <Icon name="chevron-right" />고급 설정
            <span class="muted">· 용지, 방향, 표지, 목차, 스텝마다 새 쪽, 그림 크기, 글꼴, 머리글·바닥글</span>
          </summary>
          <div class="grid">
            <label>용지 <select bind:value={options.paper}><option value="A4">A4</option><option value="Letter">Letter</option></select></label>
            <label>방향
              <select bind:value={options.landscape}><option value={false}>세로</option><option value={true}>가로</option></select>
            </label>
            <label><input type="checkbox" bind:checked={options.cover} /> 표지 넣기</label>
            <label><input type="checkbox" bind:checked={options.toc} /> 목차 넣기</label>
            <label><input type="checkbox" bind:checked={options.stepPerPage} /> 스텝마다 새 쪽</label>
            <label>그림 최대 폭(%) <input type="number" min="10" max="100" step="10" bind:value={options.imageWidth} /></label>
            <label>글꼴
              <select bind:value={options.font}>
                <option value="">기본(형식에 맞게)</option>
                <option value="맑은 고딕">맑은 고딕</option>
                <option value="함초롬돋움">함초롬돋움</option>
              </select>
            </label>
            {#if format === 'html'}<label><input type="checkbox" bind:checked={options.embedFont} /> 글꼴을 파일에 넣기</label>{/if}
            <label class="wide">머리글 <input bind:value={options.header} /></label>
            <label class="wide">바닥글 <input bind:value={options.footer} /></label>
            <p class="hint wide">머리글·바닥글에 {'{{매뉴얼제목}}'}, {'{{날짜}}'}, {'{{쪽}}'}, {'{{전체쪽}}'}을 쓰면 그 값으로 바뀝니다.</p>
          </div>
          <div class="line">
            <input placeholder="문서 모양 이름" aria-label="문서 모양 이름" bind:value={newName} disabled={busy} />
            <button class="btn sm" disabled={busy || !newName.trim()} onclick={savePreset} {@attach tip('지금 고른 설정을 이 이름으로 저장합니다. 같은 이름이 있으면 고칩니다.')}>
              문서 모양으로 저장
            </button>
            {#if presets.length && presetId !== presets[0].id}
              <button class="btn ghost sm" disabled={busy} onclick={deletePreset} {@attach tip('고른 문서 모양을 지웁니다.')}>이 문서 모양 지우기</button>
            {/if}
          </div>
        </details>
      {/if}
    {/if}

    {#if error}<p class="banner error" role="alert"><Icon name="alert-triangle" />{error}</p>{/if}
  </div>

  {#if busy && status}
    <div class="foot progress" role="status">
      <p>{status}</p>
      <div class="bar"><i style:width="{progress * 100}%"></i></div>
    </div>
  {:else}
    <div class="foot">
      <button class="btn ghost" disabled={busy} onclick={onclose} {@attach tip('저장하지 않고 창을 닫습니다 (Esc)')}>취소</button>
      <button class="btn primary" disabled={busy || (!options && format !== 'htn')} onclick={save} {@attach tip('저장할 곳을 고르는 창이 뜹니다.')}>
        <Icon name="download" />저장
      </button>
    </div>
  {/if}
</dialog>

<style>
  .dialog {
    width: min(560px, calc(100vw - 32px));
    max-height: calc(100vh - 32px);
    padding: 0;
    overflow: auto;
    color: var(--text);
    background: var(--surface);
    border: 0;
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
  }
  .dialog::backdrop {
    background: var(--scrim);
  }
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: 18px 20px 4px;
  }
  h2 {
    margin: 0;
    font-size: 18px;
  }
  .head p {
    margin: 2px 0 0;
    font-size: 13px;
    color: var(--text-2);
  }
  .body {
    display: grid;
    gap: 12px;
    padding: 12px 20px 16px;
  }
  .formats {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 8px;
  }
  .format {
    position: relative;
    display: grid;
    justify-items: center;
    gap: 6px;
    height: auto;
    padding: 14px 4px 12px;
    white-space: normal;
    border: 1.5px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .format[aria-checked='true'],
  .format[aria-checked='true']:hover:not(:disabled) {
    background: var(--accent-soft);
    border-color: var(--accent-border);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  .check {
    position: absolute;
    top: 6px;
    right: 6px;
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    color: var(--on-accent);
    background: var(--accent);
    border-radius: 50%;
  }
  .doc {
    position: relative;
    width: 34px;
    height: 42px;
    background: var(--surface);
    border: 1.5px solid var(--border-strong);
    border-radius: 4px;
  }
  .doc b {
    position: absolute;
    bottom: 6px;
    left: -6px;
    padding: 1px 4px;
    font-size: 12px;
    font-weight: 700;
    color: #fff;
    border-radius: 3px;
  }
  .format strong {
    font-size: 14px;
  }
  .format small {
    font-size: 12px;
    line-height: 1.3;
    color: var(--text-3);
  }
  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    font-size: 13px;
  }
  .line label {
    color: var(--text-2);
  }
  .line select {
    min-width: 220px;
  }
  .kinds {
    display: grid;
    gap: 8px;
    margin: 0;
    padding: 0;
    border: 0;
  }
  .kinds legend {
    margin-bottom: 6px;
    padding: 0;
    font-size: 13px;
    color: var(--text-2);
  }
  .kind {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 12px;
    border: 1.5px solid var(--border);
    border-radius: var(--radius);
    cursor: pointer;
  }
  .kind.on {
    background: var(--accent-soft);
    border-color: var(--accent-border);
  }
  .kind input {
    margin: 3px 0 0;
    accent-color: var(--accent-border);
  }
  .kind span {
    display: grid;
    gap: 2px;
  }
  .kind b {
    font-size: 14px;
  }
  .kind small {
    font-size: 12px;
    line-height: 1.4;
    color: var(--text-3);
  }
  .what {
    color: var(--text-3);
    text-decoration: underline dotted;
    cursor: help;
  }
  .advanced summary {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 0;
    font-size: 13px;
    color: var(--text-2);
    cursor: pointer;
    border-top: 1px solid var(--border);
    list-style: none;
  }
  .advanced[open] summary :global(svg) {
    transform: rotate(90deg);
  }
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px 16px;
    margin: 6px 0 12px;
    font-size: 13px;
  }
  .grid label {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .wide {
    grid-column: 1 / -1;
  }
  .wide input {
    flex: 1;
  }
  .hint {
    margin: 0;
    font-size: 12px;
    color: var(--text-3);
  }
  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 20px 16px;
    background: var(--sunken);
    border-top: 1px solid var(--border);
  }
  .progress {
    display: grid;
    gap: 8px;
  }
  .progress p {
    margin: 0;
    font-size: 13px;
    color: var(--text-2);
  }
  .bar {
    height: 8px;
    overflow: hidden;
    background: var(--hover);
    border-radius: 999px;
  }
  .bar i {
    display: block;
    height: 100%;
    background: var(--accent);
    border-radius: 999px;
    transition: width 0.2s;
  }
</style>
