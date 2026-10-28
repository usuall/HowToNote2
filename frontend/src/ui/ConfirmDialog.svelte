<!-- 확인 창(UI 설계 4.8절): <dialog>를 showModal()로 띄워 초점 가두기와 Esc를 브라우저에 맡긴다. 여는 쪽이 {#if}로 붙였다
     떼고, 단추는 children으로 넣는다. 브라우저가 처음 초점을 첫 단추에 주므로 안전한 단추(취소)를 먼저 둔다.
     cancelable이 false면 Esc로 닫히지 않는다(창 닫기 실패: 닫아도 저장하지 않은 창이 그대로 남아 헷갈린다). -->
<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    title,
    message,
    cancelable = true,
    oncancel,
    children,
  }: { title: string; message: string; cancelable?: boolean; oncancel?: () => void; children: Snippet } = $props()

  const uid = $props.id()
  // {#if}로 떼어내면 포커스가 body로 빠지므로, 열 때 초점을 적어 두고 닫을 때 되돌린다(Menu와 같은 방식)
  const before = document.activeElement as HTMLElement | null
</script>

<dialog
  class="dialog"
  aria-labelledby="{uid}-title"
  aria-describedby="{uid}-message"
  oncancel={(e) => {
    e.preventDefault() // 닫기는 여는 쪽이 {#if}를 내려서 한다
    if (cancelable) oncancel?.()
  }}
  onclose={(e) => {
    // Chromium은 사용자 동작 없이 Esc를 거듭 누르면 preventDefault를 무시하고 닫는다. 닫히면 안 되는 창은 다시 띄운다.
    if (!cancelable && e.currentTarget.isConnected) e.currentTarget.showModal()
  }}
  {@attach (d) => { d.showModal(); return () => before?.focus() }}
>
  <h2 id="{uid}-title">{title}</h2>
  <p id="{uid}-message">{message}</p>
  <div class="buttons">{@render children()}</div>
</dialog>

<style>
  .dialog {
    width: min(440px, calc(100vw - 32px));
    padding: 20px;
    color: var(--text);
    background: var(--surface);
    border: 0;
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
  }
  .dialog::backdrop {
    background: var(--scrim);
  }
  h2 {
    margin: 0 0 6px;
    font-size: 18px;
  }
  p {
    margin: 0 0 18px;
    line-height: 1.55;
    color: var(--text-2);
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
