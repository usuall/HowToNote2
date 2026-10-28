<!-- 알림(토스트, UI 설계 5.5절): 화면 아래 가운데에 잠깐 뜨는 어두운 띠. 8초 뒤 사라지고, 마우스를 올리거나 초점이 들어오면 멈춘다.
     action을 주면 오른쪽에 단추 하나를 둔다(예: 폴더 열기, 가져온 매뉴얼 열기). 여는 쪽이 {#if}로 붙였다 뗀다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'
  import { tip } from './tip'

  let {
    message,
    detail = '',
    action,
    onclose,
  }: { message: string; detail?: string; action?: { label: string; tip: string; run: () => void; icon?: IconName }; onclose: () => void } = $props()

  const LIFE = 8000 // 경로를 읽고 '폴더 열기'를 누를 만큼
  let timer: ReturnType<typeof setTimeout> | undefined
  const start = () => {
    clearTimeout(timer)
    timer = setTimeout(onclose, LIFE)
  }
  const stop = () => clearTimeout(timer)

  onMount(() => {
    start()
    return stop
  })
</script>

<div class="toast" role="status" onpointerenter={stop} onpointerleave={start} onfocusin={stop} onfocusout={start}>
  <span class="ok" aria-hidden="true"><Icon name="check" size={13} /></span>
  <div class="text">
    <span>{message}</span>
    {#if detail}<small {@attach tip(detail)}>{detail}</small>{/if}
  </div>
  {#if action}
    <button class="btn sm" onclick={action.run} {@attach tip(action.tip)}><Icon name={action.icon ?? 'folder-open'} />{action.label}</button>
  {/if}
  <button class="btn icon sm close" aria-label="알림 닫기" onclick={onclose} {@attach tip()}><Icon name="x" /></button>
</div>

<style>
  .toast {
    position: fixed;
    bottom: 20px;
    left: 50%;
    z-index: 950;
    display: flex;
    align-items: center;
    gap: 12px;
    width: min(460px, calc(100vw - 32px));
    padding: 10px 10px 10px 14px;
    font-size: 13px;
    color: var(--surface);
    background: var(--tip-bg);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    transform: translateX(-50%);
  }
  .ok {
    display: grid;
    flex: none;
    place-items: center;
    width: 22px;
    height: 22px;
    background: var(--success);
    border-radius: 50%;
  }
  .text {
    display: grid;
    flex: 1;
    min-width: 0;
  }
  small {
    overflow: hidden;
    font-size: 12px;
    color: var(--text-3);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .btn {
    color: var(--surface);
    background: transparent;
    border-color: var(--text-2);
  }
  .btn:hover:not(:disabled) {
    background: var(--text-2);
  }
  .close {
    border-color: transparent;
  }
</style>
