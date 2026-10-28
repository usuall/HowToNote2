<!-- 이름 고치기 칸: Enter나 '저장'으로 저장하고 Esc나 '취소'로 그만둔다. 첫 화면의 폴더·매뉴얼 이름에 쓴다.
     좁은 폴더 막대에도 들어가게, 자리가 모자라면 단추가 다음 줄로 내려간다. -->
<script lang="ts">
  import { untrack } from 'svelte'
  import { tip } from './ui/tip'

  let {
    value = '',
    label,
    busy = false,
    onsave,
    oncancel,
  }: { value?: string; label: string; busy?: boolean; onsave: (name: string) => void; oncancel: () => void } = $props()

  let text = $state(untrack(() => value)) // 처음 값만 받아 고친다

  function key(e: KeyboardEvent) {
    if (e.isComposing || e.keyCode === 229) return // 한글 조합 중 Enter·Esc는 무시한다(조합 중인 글자가 그대로 저장되는 것을 막는다)
    if (e.key === 'Enter') {
      if (!busy) onsave(text)
    } else if (e.key === 'Escape') oncancel()
  }
</script>

<span class="name-edit">
  <input aria-label={label} placeholder={label} bind:value={text} onkeydown={key} {@attach (el) => el.focus()} />
  <button class="btn primary sm" disabled={busy} onclick={() => onsave(text)} {@attach tip('이 이름으로 저장합니다 (Enter)')}>저장</button>
  <button class="btn ghost sm" onclick={oncancel} {@attach tip('고치지 않고 그만둡니다 (Esc)')}>취소</button>
</span>

<style>
  .name-edit {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    width: 100%;
  }
  input {
    flex: 1 1 140px;
    min-width: 0;
    height: 32px;
  }
</style>
