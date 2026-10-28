<!-- 녹화 툴바(UI 설계 5.2절, 스펙 3.3절): 460×170 창. 마지막으로 찍힌 화면과 스텝 번호, 녹화 상태, 한 줄 안내,
     일시정지·다시 시작과 녹화 끝내기, 단축키 한 줄. 다음에 누를 단추(녹화 중에는 끝내기, 일시정지 중에는 다시 시작)를 노랗게 둔다. -->
<script lang="ts">
  import { PauseRecording, ResumeRecording, StopRecording } from '../wailsjs/go/main/App'
  import { errorText } from './api'
  import Icon from './ui/Icon.svelte'
  import { tip } from './ui/tip'

  let {
    steps,
    lastImage,
    paused,
    notice,
  }: { steps: number; lastImage: string; paused: boolean; notice: string } = $props()

  let busy = $state(false)
  let error = $state('')

  const help = $derived(
    paused ? '지금은 클릭해도 찍히지 않습니다.' : steps ? '클릭할 때마다 한 장씩 찍힙니다.' : '평소처럼 화면을 클릭해 보세요.',
  )
  const message = $derived(error || notice) // 오류나 알림이 있으면 안내 대신 보인다

  // run은 단추 하나의 바인딩 호출을 감싼다: 끝날 때까지 단추를 막고, 실패하면 한 문장 안내를 보인다.
  async function run(call: () => Promise<void>) {
    busy = true
    error = ''
    try {
      await call()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<div class="toolbar">
  {#if lastImage}
    <div class="thumb">
      <img src={`/img/${lastImage}.png`} alt="마지막으로 찍힌 화면" />
      <span class="count">{steps}</span>
    </div>
  {:else}
    <div class="thumb empty">아직 찍힌 화면이 없습니다</div>
  {/if}
  <div class="side">
    <p class="status">
      <span class="pill" class:paused><span class="dot" aria-hidden="true"></span>{paused ? '일시정지됨' : '녹화 중'}</span>
      <strong>스텝 {steps}개</strong>
    </p>
    <!-- 툴바 칸이 좁아 긴 알림은 말줄임하고, 전체 글은 툴팁으로 보인다 -->
    <p class="help" class:error={!!error} class:notice={!error && !!notice} {@attach tip(message || undefined)}>{message || help}</p>
    <div class="buttons">
      {#if paused}
        <button class="btn primary" disabled={busy} onclick={() => run(ResumeRecording)} {@attach tip('녹화를 다시 시작합니다 (Ctrl+Shift+F9)')}>
          <Icon name="play" />다시 시작
        </button>
        <button class="btn" disabled={busy} onclick={() => run(StopRecording)} {@attach tip('녹화를 끝내고 다듬기 화면으로 갑니다 (Ctrl+Shift+F11)')}>
          <Icon name="stop" />녹화 끝내기
        </button>
      {:else}
        <button class="btn" disabled={busy} onclick={() => run(PauseRecording)} {@attach tip('잠깐 멈춥니다 (Ctrl+Shift+F9)\n멈춘 동안의 클릭은 찍히지 않습니다.')}>
          <Icon name="pause" />일시정지
        </button>
        <button class="btn primary" disabled={busy} onclick={() => run(StopRecording)} {@attach tip('녹화를 끝내고 다듬기 화면으로 갑니다 (Ctrl+Shift+F11)')}>
          <Icon name="stop" />녹화 끝내기
        </button>
      {/if}
    </div>
    <!-- 단축키는 internal/recorder/session_windows.go의 hotkeys와 같이 바꾼다. F11만 누르면 브라우저가 전체 화면이 되므로
         Ctrl+Shift+를 적는다 -->
    <p class="keys">{paused ? 'Ctrl+Shift+F9: 다시 시작' : 'Ctrl+Shift+F10: 클릭 없이 지금 화면 찍기'}</p>
  </div>
</div>

<style>
  .toolbar {
    display: grid;
    grid-template-columns: 116px 1fr;
    gap: 12px;
    height: 100vh;
    padding: 10px;
    background: var(--surface);
  }
  .thumb {
    position: relative;
    height: 100%;
    max-height: 119px;
    overflow: hidden;
    background: var(--hover);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }
  .thumb img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  /* 썸네일 위 번호: 어떤 화면 위에서도 읽히게 반투명 검정 바탕을 쓴다(내용 색) */
  .count {
    position: absolute;
    right: 5px;
    bottom: 5px;
    padding: 1px 6px;
    font-size: 12px;
    font-weight: 700;
    color: #fff;
    background: rgba(26, 26, 26, 0.75);
    border-radius: 999px;
  }
  .thumb.empty {
    display: grid;
    place-items: center;
    padding: 8px;
    font-size: 12px;
    text-align: center;
    color: var(--text-3);
    background: var(--sunken);
    border: 1.5px dashed var(--border-strong);
  }
  .side {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 9px 2px 7px;
    font-size: 12px;
    font-weight: 700;
    color: var(--danger);
    background: var(--danger-soft);
    border-radius: 999px;
  }
  .pill .dot {
    width: 9px;
    height: 9px;
    background: var(--rec);
    border-radius: 50%;
    animation: blink 1.4s infinite;
  }
  .pill.paused {
    color: var(--notice);
    background: var(--accent-soft);
  }
  .pill.paused .dot {
    background: var(--accent-border);
    animation: none;
  }
  @keyframes blink {
    50% {
      opacity: 0.25;
    }
  }
  .help {
    margin: 0;
    overflow: hidden;
    font-size: 13px;
    color: var(--text-2);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .help.error {
    color: var(--danger);
  }
  .help.notice {
    color: var(--notice);
  }
  .buttons {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
    margin-top: auto;
  }
  .buttons .btn {
    padding: 0 8px;
  }
  .keys {
    margin: 0;
    font-size: 12px;
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
