<!-- 영역 고르기: 멈춘 화면 위에서 끌어 영역을 고른다. 좌표는 멈춘 화면(물리 픽셀) 기준으로 바꿔 Go에 넘긴다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { CancelRegionSelect, StartRegionRecording } from '../wailsjs/go/main/App'
  import { errorText } from './api'
  import { tip } from './ui/tip'

  let {
    width,
    height,
    onstarting,
    onstarted,
    onexit,
  }: {
    width: number
    height: number
    onstarting: () => void
    onstarted: () => void
    onexit: (message?: string) => void
  } = $props()

  type Box = { x: number; y: number; w: number; h: number }
  const minRegion = 20 // Go app.go의 minRegion과 같다
  const src = `/frozen.png?${Date.now()}` // 지난번 멈춘 화면을 캐시에서 쓰지 않게

  let img = $state<HTMLImageElement>()
  let from: { x: number; y: number } | null = null
  let box = $state<Box | null>(null)
  let busy = $state(false)

  // 멈춘 화면을 창 전체에 늘려 보이므로, CSS 픽셀에 (실제 크기 / 보이는 크기)를 곱하면 물리 픽셀이다.
  function physical(b: Box, el: HTMLImageElement): Box {
    const sx = width / el.clientWidth
    const sy = height / el.clientHeight
    return { x: Math.round(b.x * sx), y: Math.round(b.y * sy), w: Math.round(b.w * sx), h: Math.round(b.h * sy) }
  }

  function bigEnough(b: Box | null): boolean {
    if (!b || !img) return false
    const p = physical(b, img)
    return p.w >= minRegion && p.h >= minRegion
  }

  // 고른 영역의 크기를 멈춘 화면(물리 픽셀) 기준으로 보인다. 녹화되는 그림 크기가 이것이다.
  const size = $derived(box && img && box.w > 0 ? physical(box, img) : null)

  function down(e: PointerEvent) {
    if (busy) return
    from = { x: e.clientX, y: e.clientY }
    box = { x: e.clientX, y: e.clientY, w: 0, h: 0 }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function move(e: PointerEvent) {
    if (!from) return
    box = {
      x: Math.min(from.x, e.clientX),
      y: Math.min(from.y, e.clientY),
      w: Math.abs(e.clientX - from.x),
      h: Math.abs(e.clientY - from.y),
    }
  }

  function up() {
    from = null
  }

  async function start() {
    if (!box || !img) return
    busy = true
    try {
      const p = physical(box, img)
      onstarting() // step/warning 이벤트가 StartRegionRecording이 끝나기 전에 올 수 있어, 여기서 먼저 상태를 비운다
      await StartRegionRecording(p.x, p.y, p.w, p.h)
      onstarted()
    } catch (e) {
      onexit(errorText(e)) // Go가 이미 전체 화면을 풀었으므로 첫 화면에서 안내한다
    } finally {
      busy = false
    }
  }

  async function cancel() {
    busy = true
    let message = ''
    try {
      await CancelRegionSelect()
    } catch (e) {
      message = errorText(e)
    } finally {
      busy = false
    }
    onexit(message)
  }

  onMount(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) cancel()
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="region" onpointerdown={down} onpointermove={move} onpointerup={up}>
  <img bind:this={img} {src} alt="" draggable="false" />
  {#if box}
    <div class="box" style:left="{box.x}px" style:top="{box.y}px" style:width="{box.w}px" style:height="{box.h}px">
      {#if size}<span class="size">{size.w} × {size.h}</span>{/if}
    </div>
  {:else}
    <div class="dim"></div>
  {/if}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="panel" onpointerdown={(e) => e.stopPropagation()}>
    <p>녹화할 부분을 마우스로 끌어서 고르세요.</p>
    <span {@attach tip(bigEnough(box) ? '고른 부분만 녹화합니다.' : '조금 더 크게 고르세요.')}>
      <button class="btn primary" disabled={busy || !bigEnough(box)} onclick={start}>이 영역 녹화 시작</button>
    </span>
    <button class="btn ghost" disabled={busy} onclick={cancel} {@attach tip('녹화하지 않고 첫 화면으로 돌아갑니다 (Esc)')}>
      취소 <kbd>Esc</kbd>
    </button>
  </div>
</div>

<style>
  .region {
    position: fixed;
    inset: 0;
    overflow: hidden;
    cursor: crosshair;
    user-select: none;
    background: #000;
  }
  img {
    display: block;
    width: 100vw;
    height: 100vh;
    pointer-events: none;
  }
  /* 고르기 전에는 화면 전체를 옅게, 고른 뒤에는 고른 곳 밖을 더 어둡게 가린다(멈춘 화면 위라 내용 색) */
  .dim {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    pointer-events: none;
  }
  .box {
    position: absolute;
    border: 2px solid var(--accent-border);
    box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.5);
    pointer-events: none;
  }
  .size {
    position: absolute;
    right: 6px;
    bottom: 6px;
    padding: 2px 8px;
    font-size: 12px;
    font-weight: 700;
    color: var(--on-accent);
    white-space: nowrap;
    background: var(--accent);
    border-radius: 4px;
  }
  .panel {
    position: absolute;
    top: 16px;
    left: 50%;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 8px 8px 16px;
    white-space: nowrap;
    cursor: default;
    background: var(--surface);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    transform: translateX(-50%);
  }
  .panel p {
    margin: 0 8px 0 0;
  }
</style>
