<!-- 편집기 오른쪽: 고른 스텝 하나(UI 설계 5.4절). 위에서부터 스텝 번호와 제목, 그림과 그리기 도구, 설명, 안내 상자.
     바꾼 값은 그 스텝의 새 행으로 만들어 onchange에 올린다. key는 같은 칸을 이어 고칠 때 실행 취소를 한 번으로 묶는 이름이다. -->
<script lang="ts">
  import { dpiScale } from './annotations'
  import Callouts from './Callouts.svelte'
  import Canvas from './Canvas.svelte'
  import type { Step } from './model'
  import RichText from './RichText.svelte'

  let { step, num, onchange }: { step: Step; num: string; onchange: (rows: Step[], key: string) => void } = $props()

  function set(patch: Partial<Step>, key = '') {
    onchange([{ ...step, ...patch }], key && `${key}:${step.id}`)
  }
</script>

<section class="pane">
  <p class="label">{num ? `스텝 ${num}` : '숨긴 스텝'}</p>
  <input
    class="title"
    aria-label="스텝 제목"
    placeholder="이 스텝에서 할 일을 적으세요"
    value={step.title}
    oninput={(e) => set({ title: e.currentTarget.value }, 'title')}
  />
  {#if step.imageSha}
    <Canvas
      sha={step.imageSha}
      annotationsJson={step.annotationsJson}
      viewJson={step.viewJson}
      unit={dpiScale(step.captureJson)}
      onchange={(patch) => set(patch)}
    />
  {/if}
  <h3>설명 <span class="muted">비워 둬도 됩니다</span></h3>
  <RichText
    label="설명"
    placeholder="이 스텝에서 무엇을 하는지 적으세요. 예: 화면 오른쪽 위의 '연차 신청' 단추를 누릅니다."
    value={step.description}
    onchange={(v) => set({ description: v }, 'description')}
  />
  <h3>안내 상자 <span class="muted">주의할 점이나 도움말을 눈에 띄는 상자로 넣습니다</span></h3>
  <Callouts json={step.calloutsJson} onchange={(v, key) => set({ calloutsJson: v }, key && `callout-${key}`)} />
</section>

<style>
  .pane {
    display: grid;
    align-content: start;
    gap: 10px;
    min-width: 0;
    padding: 16px 24px 48px;
    overflow-y: auto;
  }
  .label {
    margin: 0;
    font-size: 12px;
    font-weight: 700;
    color: var(--text-3);
  }
  .title {
    height: 40px;
    font-size: 18px;
    font-weight: 700;
  }
  h3 {
    margin: 14px 0 0;
    font-size: 14px;
  }
</style>
