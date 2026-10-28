<!-- 그리기 캔버스(스펙 6.2절). 원본 이미지와 그리기 객체를 render.ts의 addScene으로 그려 내보내기 결과와 같게 보인다.
     좌표는 모두 원본 이미지의 물리 픽셀이고, 화면 맞춤은 Stage 배율로만 한다. 바뀐 값은 JSON 문자열로 onchange에 올린다. -->
<script lang="ts">
  import Konva from 'konva'
  import { onMount, untrack } from 'svelte'
  import {
    clampCrop, create, parseAnnotations, parseView, recolor, type Annotation, type AnnotationType, type Box, type Point,
  } from './annotations'
  import { addScene, loadImage, nodeFor, readBack, textHeight, type Source } from './render'
  import Icon from '../ui/Icon.svelte'
  import type { IconName } from '../ui/icons'
  import { tip } from '../ui/tip'

  let {
    sha,
    annotationsJson,
    viewJson,
    unit,
    onchange,
  }: {
    sha: string
    annotationsJson: string
    viewJson: string
    unit: number // DPI 배율(capture_json의 dpi_scale). 0이면 그림 크기로 정한다
    onchange: (patch: { annotationsJson?: string; viewJson?: string }) => void
  } = $props()

  type Tool = 'select' | 'crop' | AnnotationType
  // [도구, 라벨, 단축키, 아이콘, 툴팁의 둘째 줄]. 늘 보이는 '선택'과 5개(스펙 6.2절), '더보기' 4개, 막대 오른쪽 끝의 '자르기'
  type ToolInfo = [Tool, string, string, IconName, string]
  const SELECT: ToolInfo = ['select', '선택', 'V', 'pointer', '그린 것을 골라 옮기거나 크기를 바꿉니다.']
  const MAIN: ToolInfo[] = [
    ['marker', '클릭 표시', 'M', 'marker', '누른 자리를 빨간 원으로 표시합니다.'],
    ['arrow', '화살표', 'A', 'arrow', '끌어서 화살표를 그립니다.'],
    ['rect', '사각형', 'R', 'square', '끌어서 네모를 그립니다.'],
    ['text', '글자', 'T', 'type', '누른 자리에 글을 씁니다.'],
    ['blur', '모자이크', 'B', 'mosaic', '개인정보를 끌어서 가립니다.'],
  ]
  const MORE: ToolInfo[] = [
    ['ellipse', '타원', 'E', 'ellipse', '끌어서 동그라미를 그립니다.'],
    ['callout', '말풍선', 'C', 'callout', '말풍선을 넣고 글을 씁니다.'],
    ['number', '번호', 'N', 'hash', '누른 자리에 차례 번호를 붙입니다.'],
    ['highlight', '형광펜', 'H', 'highlighter', '끌어서 형광펜으로 칠합니다.'],
  ]
  const CROP: ToolInfo = ['crop', '자르기', 'X', 'crop', '남길 부분만 남깁니다.']
  const KEYS = new Map<string, Tool>([SELECT, ...MAIN, ...MORE, CROP].map(([t, , k]) => [k.toLowerCase(), t]))
  const help = ([, name, k, , line]: ToolInfo) => `${name} (${k})\n${line}`
  const COLORS: [string, string][] = [
    ['#E5383B', '빨강'],
    ['#F57C00', '주황'],
    ['#FFE14D', '노랑'],
    ['#2E7D32', '초록'],
    ['#1A73E8', '파랑'],
    ['#1F2329', '검정'],
    ['#FFFFFF', '흰색'],
  ]

  let tool = $state<Tool>('select')
  let more = $state(false)
  let selectedId = $state('')
  let cropBox = $state<Box | null>(null) // 자르기 중인 상자(이미지 px)
  let editing = $state<{ id: string; text: string; x: number; y: number; w: number; h: number; size: number } | null>(null)
  let bar = $state<{ x: number; y: number } | null>(null) // 스타일 막대 위치(캔버스 안 CSS px)
  let src = $state<Source>()
  let width = $state(0)
  let error = $state('')

  // 두 JSON 문자열을 먼저 $derived로 받는다. 이 prop은 StepPane의 step을 거쳐 읽혀 제목·설명을 한 글자 칠 때마다
  // 다시 계산되는데, 문자열이 같으면 $derived가 거기서 멈춰 장면(Konva)을 다시 만들지 않는다.
  const annText = $derived(annotationsJson)
  const viewText = $derived(viewJson)
  const list = $derived(parseAnnotations(annText))
  const view = $derived(parseView(viewText))
  const sel = $derived(list.find((a) => a.id === selectedId))
  const u = $derived(unit > 0 ? unit : Math.max(1, (src?.width ?? 1920) / 1920))

  let box: HTMLDivElement
  let holder: HTMLDivElement
  let stage: Konva.Stage | undefined
  let layer: Konva.Layer
  let scene: Konva.Group
  let tr: Konva.Transformer
  let draft: Konva.Group | Konva.Arrow | null = null
  let from: Point | null = null
  let k = 1 // 화면 배율(CSS px / 이미지 px)
  let shownAt: Point = { x: 0, y: 0 } // 화면 왼쪽 위의 이미지 좌표(자르기 시작점)
  const ALL = ['top-left', 'top-center', 'top-right', 'middle-right', 'middle-left', 'bottom-left', 'bottom-center', 'bottom-right']

  onMount(() => {
    stage = new Konva.Stage({ container: holder, width: 1, height: 1 })
    layer = new Konva.Layer()
    scene = new Konva.Group()
    tr = new Konva.Transformer({
      rotateEnabled: false,
      flipEnabled: false,
      boundBoxFunc: (old, b) => (Math.abs(b.width) < 4 || Math.abs(b.height) < 4 ? old : b),
    })
    layer.add(scene, tr)
    stage.add(layer)
    stage.on('mousedown', down)
    stage.on('mousemove', move)
    window.addEventListener('mouseup', up)
    loadImage(`/img/${sha}.png`)
      .then((img) => (src = img))
      .catch((e) => (error = e.message))
    return () => {
      window.removeEventListener('mouseup', up)
      stage?.destroy()
    }
  })

  // 장면 다시 그리기: 그림·객체·자르기·도구·폭이 바뀔 때. 선택만 바뀔 때는 다시 만들지 않는다(누르는 중인 노드가 사라지면 끌기가 끊긴다).
  $effect(() => {
    if (!stage || !src) return
    const full = { x: 0, y: 0, w: src.width, h: src.height }
    const shown = tool === 'crop' ? full : clampCrop(view.crop, src.width, src.height)
    k = Math.min(1, width / shown.w, (window.innerHeight * 0.7) / shown.h)
    stage.size({ width: Math.round(shown.w * k), height: Math.round(shown.h * k) })
    stage.scale({ x: k, y: k })
    scene.position({ x: -shown.x, y: -shown.y })
    shownAt = { x: shown.x, y: shown.y }
    scene.destroyChildren()
    addScene(scene, src, list)
    for (const node of scene.find('.annotation')) wire(node)
    if (tool === 'crop' && cropBox) addCropOverlay(cropBox, full)
    untrack(attach)
  })

  // 선택이 바뀌면 변형 손잡이와 스타일 막대만 옮긴다.
  $effect(() => {
    void selectedId
    void tool
    attach()
  })

  function wire(node: Konva.Node) {
    const a = list.find((x) => x.id === node.id())
    if (!a) return
    node.draggable(tool === 'select')
    node.on('mousedown', () => {
      if (tool === 'select') selectedId = a.id
    })
    node.on('dragstart transformstart', () => (bar = null))
    node.on('dragend transformend', () => update(readBack(a, node)))
    node.on('dblclick', () => {
      if (tool === 'select' && (a.type === 'text' || a.type === 'callout')) edit(a)
    })
  }

  function attach() {
    if (!stage) return
    if (tool === 'crop') {
      const r = scene.findOne('.crop')
      tr.setAttrs({ keepRatio: false, enabledAnchors: ALL })
      tr.nodes(r ? [r] : [])
      bar = null
      return
    }
    const node = selectedId ? scene.findOne((n: Konva.Node) => n.id() === selectedId) : undefined
    const a = list.find((x) => x.id === selectedId)
    if (!node || !a || tool !== 'select') {
      tr.nodes([])
      bar = null
      return
    }
    const round = a.type === 'marker' || a.type === 'number'
    tr.setAttrs({
      keepRatio: round,
      enabledAnchors:
        a.type === 'text' ? ['middle-left', 'middle-right'] : round ? ['top-left', 'top-right', 'bottom-left', 'bottom-right'] : ALL,
    })
    tr.nodes([node])
    const r = node.getClientRect()
    bar = { x: Math.max(0, r.x), y: Math.max(0, r.y - 44) }
  }

  // 자르기 중에는 고른 영역 밖을 어둡게 하고, 영역 상자에 변형 손잡이를 단다.
  function addCropOverlay(c: Box, full: Box) {
    const dim = { fill: 'rgba(0,0,0,0.5)', listening: false }
    scene.add(
      new Konva.Rect({ ...dim, x: 0, y: 0, width: full.w, height: c.y }),
      new Konva.Rect({ ...dim, x: 0, y: c.y + c.h, width: full.w, height: full.h - c.y - c.h }),
      new Konva.Rect({ ...dim, x: 0, y: c.y, width: c.x, height: c.h }),
      new Konva.Rect({ ...dim, x: c.x + c.w, y: c.y, width: full.w - c.x - c.w, height: c.h }),
    )
    const r = new Konva.Rect({ name: 'crop', x: c.x, y: c.y, width: c.w, height: c.h, stroke: '#FFFFFF', strokeWidth: 2 / k, draggable: true })
    r.on('dragend transformend', () => {
      cropBox = clampCrop(
        { x: r.x(), y: r.y(), w: r.width() * r.scaleX(), h: r.height() * r.scaleY() },
        full.w,
        full.h,
      )
    })
    scene.add(r)
  }

  function pointer(): Point {
    return scene.getRelativePointerPosition() ?? { x: 0, y: 0 }
  }

  function down(e: Konva.KonvaEventObject<MouseEvent>) {
    box.focus({ preventScroll: true }) // 도구 단축키를 받게
    if (!src || editing) return
    if (tool === 'select') {
      if (!e.target.findAncestor('.annotation', true)) selectedId = ''
      return
    }
    if (tool === 'crop') return
    from = pointer()
  }

  function move() {
    if (!from || !src || tool === 'select' || tool === 'crop') return
    draft?.destroy()
    draft = nodeFor(create(tool, from, pointer(), u, list), src)
    scene.add(draft)
  }

  function up() {
    if (!from || tool === 'select' || tool === 'crop') return
    const a = create(tool, from, pointer(), u, list)
    from = null
    draft?.destroy()
    draft = null
    onchange({ annotationsJson: JSON.stringify([...list, a]) })
    tool = 'select'
    selectedId = a.id
    if (a.type === 'text' || a.type === 'callout') edit(a) // 새 글자·말풍선은 바로 글을 고치게 한다
  }

  function update(a: Annotation) {
    const prev = list.find((x) => x.id === a.id)
    if (prev && JSON.stringify(prev) === JSON.stringify(a)) return // 끌기·변형이 제자리로 끝나면 실행 취소 기록을 늘리지 않는다
    onchange({ annotationsJson: JSON.stringify(list.map((x) => (x.id === a.id ? a : x))) })
  }

  function removeSelected() {
    if (!selectedId) return
    onchange({ annotationsJson: JSON.stringify(list.filter((x) => x.id !== selectedId)) })
    selectedId = ''
  }

  // edit은 객체 위에 글 고치는 칸을 띄운다. 위치는 객체 상자를 화면 배율로 바꿔 정한다(노드를 다시 만들어도 같은 자리).
  function edit(a: Annotation) {
    editing = {
      id: a.id,
      text: a.text ?? '',
      x: (a.x - shownAt.x) * k,
      y: (a.y - shownAt.y) * k,
      w: Math.max(a.w * k, 120),
      h: Math.max(a.h * k, a.style.font_size * k * 1.4),
      size: a.style.font_size * k,
    }
  }

  function commitText() {
    if (!editing) return
    const a = list.find((x) => x.id === editing!.id)
    const text = editing.text.trim()
    editing = null
    if (!a) return
    if (!text) {
      onchange({ annotationsJson: JSON.stringify(list.filter((x) => x.id !== a.id)) }) // 글을 다 지우면 객체도 지운다
      return
    }
    const next = { ...a, text }
    update(a.type === 'text' ? { ...next, h: textHeight(next) } : next)
  }

  function setTool(t: Tool) {
    tool = t
    selectedId = ''
    if (MORE.some(([m]) => m === t)) more = true
    if (t === 'crop' && src) cropBox = clampCrop(view.crop, src.width, src.height)
  }

  function applyCrop() {
    if (!cropBox || !src) return
    const whole = cropBox.x === 0 && cropBox.y === 0 && cropBox.w === src.width && cropBox.h === src.height
    onchange({ viewJson: JSON.stringify(whole ? {} : { crop: cropBox }) })
    tool = 'select'
  }

  function key(e: KeyboardEvent) {
    if (editing || e.ctrlKey || e.altKey || e.metaKey) return
    if ((e.target as HTMLElement).closest('select, input, textarea')) return // 스타일 막대에서 친 글자는 도구 단축키가 아니다
    if (e.key === 'Escape') {
      tool = 'select'
      selectedId = ''
    } else if ((e.key === 'Delete' || e.key === 'Backspace') && selectedId) {
      e.preventDefault()
      removeSelected()
    } else {
      // 한글 입력 상태에서는 e.key가 한글 자모('ㅍ')로 오므로 자판 위치(e.code 'KeyV')로도 찾는다
      const t = KEYS.get(e.key.toLowerCase()) ?? KEYS.get(e.code.replace(/^Key/, '').toLowerCase())
      if (t) setTool(t)
    }
  }

  // 스타일 막대의 선택지(이미지 px, DPI 배율을 곱한다). 지금 값에 가장 가까운 것을 보인다.
  const px = (n: number) => Math.max(1, Math.round(n * u))
  const widths = $derived<[number, string][]>(
    sel?.type === 'blur'
      ? [[px(6), '약하게'], [px(12), '보통'], [px(24), '강하게']]
      : [[px(2), '가늘게'], [px(4), '보통'], [px(8), '굵게']],
  )
  const sizes = $derived<[number, string][]>([[px(16), '작게'], [px(24), '보통'], [px(36), '크게']])

  function nearest(opts: [number, string][], v: number): number {
    return opts.reduce((best, [o]) => (Math.abs(o - v) < Math.abs(best - v) ? o : best), opts[0][0])
  }

  function color(a: Annotation): string {
    return a.type === 'marker' || a.type === 'number' || a.type === 'highlight' ? a.style.fill : a.style.stroke
  }

  function setStyle(patch: Partial<Annotation['style']>) {
    if (!sel) return
    const next = { ...sel, style: { ...sel.style, ...patch } }
    update(next.type === 'text' ? { ...next, h: textHeight(next) } : next)
  }
</script>

<!-- 단축키(스펙 6.2절)는 이 영역에 초점이 있을 때만 받는다. 설명을 쓰는 중에 글자가 도구 단축키로 먹히지 않게 하려는 것이다. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div class="canvas" bind:this={box} tabindex="0" role="application" aria-label="그림과 그리기 도구" onkeydown={key}>
  <div class="tools" role="toolbar" aria-label="그리기 도구">
    {#each [SELECT, ...MAIN] as info (info[0])}
      <button class="tool" class:on={tool === info[0]} aria-pressed={tool === info[0]} onclick={() => setTool(info[0])} {@attach tip(help(info))}>
        <Icon name={info[3]} />{info[1]}
      </button>
    {/each}
    <button class="tool" aria-expanded={more} onclick={() => (more = !more)} {@attach tip(more ? '둘째 도구 줄을 접습니다.' : '타원, 말풍선, 번호, 형광펜을 꺼냅니다.')}>
      <Icon name={more ? 'chevron-up' : 'chevron-down'} />{more ? '접기' : '더보기'}
    </button>
    <button class="tool crop" class:on={tool === 'crop'} aria-pressed={tool === 'crop'} onclick={() => setTool('crop')} {@attach tip(help(CROP))}>
      <Icon name={CROP[3]} />{CROP[1]}
    </button>
  </div>
  {#if more}
    <div class="tools" role="toolbar" aria-label="그리기 도구 더보기">
      {#each MORE as info (info[0])}
        <button class="tool" class:on={tool === info[0]} aria-pressed={tool === info[0]} onclick={() => setTool(info[0])} {@attach tip(help(info))}>
          <Icon name={info[3]} />{info[1]}
        </button>
      {/each}
    </div>
  {/if}
  {#if tool === 'crop'}
    <div class="crop-bar">
      <span>남길 영역의 모서리를 끌어 맞추세요.</span>
      <button class="btn primary sm" onclick={applyCrop} {@attach tip('고른 영역만 남깁니다.')}>자르기 적용</button>
      <button class="btn sm" onclick={() => src && (cropBox = { x: 0, y: 0, w: src.width, h: src.height })} {@attach tip('영역을 그림 전체로 되돌립니다.')}>
        자르기 없애기
      </button>
      <button class="btn ghost sm" onclick={() => setTool('select')} {@attach tip('자르지 않고 그만둡니다 (Esc)')}>취소</button>
    </div>
  {/if}

  <div class="wrap">
    <!-- 폭은 안쪽 칸으로 잰다. 바깥 칸의 여백이 clientWidth에 들어가면 무대가 넘친다. 떠 있는 막대와 글 칸도 이 칸 기준이다 -->
    <div class="inner" bind:clientWidth={width}>
      <div bind:this={holder}></div>
      {#if sel && bar && !editing}
        <div class="style" style:left="{bar.x}px" style:top="{bar.y}px">
          {#if sel.type !== 'blur'}
            <div class="swatches" role="radiogroup" aria-label="색">
              {#each COLORS as [c, name] (c)}
                <button
                  class="swatch"
                  role="radio"
                  aria-checked={color(sel).toUpperCase() === c}
                  aria-label={name}
                  style:background={c}
                  onclick={() => update(recolor(sel, c))}
                  {@attach tip()}
                ></button>
              {/each}
            </div>
          {/if}
          {#if sel.type === 'text' || sel.type === 'callout'}
            <label>
              글자 크기
              <select value={nearest(sizes, sel.style.font_size)} onchange={(e) => setStyle({ font_size: Number(e.currentTarget.value) })}>
                {#each sizes as [v, name] (name)}<option value={v}>{name}</option>{/each}
              </select>
            </label>
          {:else if sel.type !== 'highlight' && sel.type !== 'number'}
            <label>
              {sel.type === 'blur' ? '세기' : '두께'}
              <select value={nearest(widths, sel.style.width)} onchange={(e) => setStyle({ width: Number(e.currentTarget.value) })}>
                {#each widths as [v, name] (name)}<option value={v}>{name}</option>{/each}
              </select>
            </label>
          {/if}
          <button class="btn ghost sm" onclick={removeSelected} {@attach tip('고른 것을 지웁니다 (Delete)')}>지우기</button>
        </div>
      {/if}
      {#if editing}
        <textarea
          class="edit"
          aria-label="글 고치기"
          style:left="{editing.x}px"
          style:top="{editing.y}px"
          style:width="{editing.w}px"
          style:height="{editing.h}px"
          style:font-size="{editing.size}px"
          bind:value={editing.text}
          onblur={commitText}
          onkeydown={(e) => {
            if (e.isComposing || e.keyCode === 229) return // 한글 조합 중의 Esc·Enter는 글 고치기를 끝내거나 지우면 안 된다
            if (e.key === 'Escape') {
              e.stopPropagation() // 캔버스 루트의 단축키 처리로 다시 넘어가지 않게
              editing = null
            } else if (e.key === 'Enter' && e.ctrlKey) commitText()
          }}
          {@attach (el) => {
            el.focus()
            el.select()
          }}
        ></textarea>
      {/if}
    </div>
  </div>
  <p class="hint">그린 것을 누르면 위에 작은 막대가 떠서 색과 두께를 바꿀 수 있습니다. Delete를 누르면 지워집니다.</p>
  {#if error}<p class="banner error">{error}</p>{/if}
</div>

<style>
  .canvas {
    display: grid;
    gap: 6px;
  }
  /* 캔버스 영역은 단축키를 받으려고 초점을 받는다. 바깥 테두리 대신 그림 칸(.wrap)에 테두리를 그려 보인다 */
  .canvas:focus-visible {
    outline: none;
  }
  /* 캔버스 영역에 키보드 초점이 오면 그림 칸에 테두리를 그려, 단축키를 받는 곳임을 보인다 */
  .canvas:focus-visible .wrap {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .tools,
  .crop-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px;
    padding: 4px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .crop-bar {
    gap: 8px;
    padding: 6px 10px;
    background: var(--accent-soft);
    border-color: var(--accent-border);
  }
  .tool {
    height: 32px;
    gap: 6px;
    padding: 0 10px;
    font-size: 13px;
    background: transparent;
    border-color: transparent;
    border-radius: var(--radius-sm);
  }
  .tool.on,
  .tool.on:hover:not(:disabled) {
    font-weight: 700;
    color: var(--on-accent);
    background: var(--accent);
  }
  .crop {
    margin-left: auto;
  }
  .wrap {
    min-width: 0;
    padding: 12px;
    background: var(--canvas);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .inner {
    position: relative;
    min-width: 0;
  }
  .style {
    position: absolute;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 8px;
    font-size: 12px;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
  }
  .style select {
    height: 26px;
    padding: 0 4px;
    font-size: 12px;
  }
  .swatches {
    display: flex;
    gap: 4px;
  }
  .swatch {
    width: 18px;
    height: 18px;
    padding: 0;
    border: 2px solid var(--surface);
    border-radius: 50%;
    box-shadow: 0 0 0 1px var(--border-strong);
  }
  .swatch[aria-checked='true'] {
    box-shadow: 0 0 0 2px var(--text);
  }
  .hint {
    margin: 0;
    font-size: 12px;
    color: var(--text-3);
  }
  .edit {
    position: absolute;
    padding: 0;
    font-family: 'Pretendard', 'Malgun Gothic', sans-serif;
    line-height: 1.3;
    resize: none;
    background: rgba(255, 255, 255, 0.95);
    border: 2px solid var(--focus);
    border-radius: 0;
  }
</style>
