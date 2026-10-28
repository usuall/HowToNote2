<!-- 앱 화면 전환: 첫 화면 → (영역 고르기) → 녹화 툴바 → 편집기. 녹화 이벤트와 창 닫기 알림은 여기서 한 번만 구독한다. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { EventsOn, Quit } from '../wailsjs/runtime/runtime'
  import { CancelClose } from '../wailsjs/go/main/App'
  import Home from './Home.svelte'
  import Toolbar from './Toolbar.svelte'
  import RegionSelect from './RegionSelect.svelte'
  import Editor from './editor/Editor.svelte'
  import ConfirmDialog from './ui/ConfirmDialog.svelte'
  import { tip } from './ui/tip'
  import { errorText, type RecorderEvent } from './api'

  type View =
    | { name: 'home' }
    | { name: 'region'; width: number; height: number }
    | { name: 'toolbar' }
    | { name: 'guide'; id: number }

  let view = $state<View>({ name: 'home' })
  let homeNotice = $state('')
  let steps = $state(0)
  let lastImage = $state('')
  let paused = $state(false)
  let notice = $state('')
  let editor = $state<{ close: () => Promise<void> }>()
  let closeError = $state('') // 창을 닫으며 한 저장이 실패한 까닭. 있으면 고를 단추를 보인다
  let closing = $state(false)
  let discard = false // '저장하지 않고 닫기'를 골랐다

  // EventsOn이 돌려주는 해제 함수를 onMount 정리 함수에서 불러, 화면이 사라질 때 구독을 푼다.
  onMount(() => {
    const offRecorder = EventsOn('recorder', (e: RecorderEvent) => {
      if (e.type === 'step') {
        steps = e.steps
        lastImage = e.imageSha ?? ''
      } else if (e.type === 'paused') {
        paused = true
      } else if (e.type === 'resumed') {
        paused = false
      } else if (e.type === 'warning') {
        notice = e.message ?? ''
      } else if (e.type === 'stopped') {
        homeNotice = e.guideId ? '' : '녹화된 스텝이 없습니다. 녹화 중에 화면을 클릭해 보세요.'
        view = e.guideId ? { name: 'guide', id: e.guideId } : { name: 'home' }
      }
    })
    const offClosing = EventsOn('app:closing', closeApp)
    return () => {
      offRecorder()
      offClosing()
    }
  })

  // starting은 StartRecording/StartRegionRecording을 부르기 '전'에 상태를 비운다. 그 약속이 끝나기 전에
  // "warning"이나 "step" 이벤트가 먼저 와도(바로 위 EventsOn 콜백) 여기서 지우지 않게 하려는 순서다.
  function starting() {
    steps = 0
    lastImage = ''
    paused = false
    notice = ''
  }

  function recording() {
    view = { name: 'toolbar' }
  }

  // closeApp은 창을 닫기 전(Go beforeClose가 보낸 'app:closing') 열린 가이드를 저장하고 닫은 뒤 앱을 끝낸다.
  // 저장에 실패하면 CancelClose로 다음 닫기를 다시 막게 하고 두 단추를 보인다. 그러지 않으면 다음 X가 저장 없이 닫는다.
  // 프론트가 멈춰 여기까지 못 오면 CancelClose를 부르지 않으니, 한 번 더 닫으면 그대로 닫힌다(D9).
  async function closeApp() {
    closing = true
    closeError = ''
    try {
      if (!discard) await editor?.close()
      Quit()
    } catch (e) {
      await CancelClose().catch(() => {})
      closeError = errorText(e)
    } finally {
      closing = false
    }
  }

  // 두 단추는 Quit만 부른다. Wails Quit도 beforeClose를 지나는데 CancelClose로 되돌렸으니 막히고 'app:closing'이 다시 와
  // closeApp이 한 번 더 돈다: 다시 저장하거나(discard가 아니면), 저장 없이 Quit한다(이번에는 막히지 않는다).
  function retryClose() {
    closing = true
    Quit()
  }

  function quitWithoutSaving() {
    discard = true
    closing = true
    Quit()
  }

  // blockFiles는 창 밖(탐색기 등)에서 끌어 온 파일을 받지 않는다. 그대로 두면 WebView2가 그 파일을 새 창으로 연다.
  // Wails 2.16의 DragAndDrop.DisableWebViewDrop은 WebView를 만들기(Embed) 전에 적용돼 아무 일도 하지 않아서 여기서 막는다.
  // 스텝 카드 끌기는 'text/plain'만 실어 'Files'가 없으니 그대로 된다.
  function blockFiles(e: DragEvent) {
    if (!e.dataTransfer?.types.includes('Files')) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'none'
  }

  function home(message = '') {
    homeNotice = message
    view = { name: 'home' }
  }
</script>

<svelte:window ondragover={blockFiles} ondrop={blockFiles} />

{#if view.name === 'home'}
  <Home
    notice={homeNotice}
    onstarting={starting}
    onstarted={recording}
    onregion={(width, height) => (view = { name: 'region', width, height })}
    onopen={(id) => (view = { name: 'guide', id })}
  />
{:else if view.name === 'region'}
  <RegionSelect width={view.width} height={view.height} onstarting={starting} onstarted={recording} onexit={home} />
{:else if view.name === 'toolbar'}
  <Toolbar {steps} {lastImage} {paused} {notice} />
{:else}
  {#key view.id}
    <Editor bind:this={editor} id={view.id} onhome={() => home()} onrecording={recording} onstarting={starting} />
  {/key}
{/if}

{#if closeError}
  <ConfirmDialog
    title="창을 닫지 못했습니다"
    message={`${closeError} 저장하지 않고 닫으면 저장하지 못한 내용은 사라집니다.`}
    cancelable={false}
  >
    <button class="btn primary" disabled={closing} onclick={retryClose} {@attach tip('저장을 한 번 더 해 보고, 되면 창을 닫습니다.')}>
      다시 저장하고 닫기
    </button>
    <button class="btn danger" disabled={closing} onclick={quitWithoutSaving} {@attach tip('저장하지 못한 내용을 버리고 창을 닫습니다.')}>
      저장하지 않고 닫기
    </button>
  </ConfirmDialog>
{/if}

