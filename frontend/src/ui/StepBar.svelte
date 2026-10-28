<!-- 3단계 막대(UI 설계 4.9절): 녹화하기 → 다듬기 → 내보내기 중 지금 단계를 보인다. 표시만 하고 눌러도 아무 일이 없다
     (누르면 옮겨 가게 하면 '1 녹화하기'를 누를 때 무엇이 일어나야 하는지 모호하다). 창이 좁으면 번호만 남는다. -->
<script lang="ts">
  import Icon from './Icon.svelte'
  import { tip } from './tip'

  let { current }: { current: 1 | 2 | 3 } = $props()

  const STEPS = [
    ['녹화하기', '평소처럼 클릭하며 일을 따라 합니다. 클릭할 때마다 한 장씩 찍힙니다.'],
    ['다듬기', '찍힌 화면에 표시를 하고 설명을 씁니다.'],
    ['내보내기', 'PDF, 한글(HWPX), 워드, 웹 문서나 HowtoNote 파일로 저장합니다.'],
  ]
</script>

<ol class="steps" aria-label="매뉴얼 만드는 단계">
  {#each STEPS as [name, help], i (name)}
    {#if i > 0}<li class="line" aria-hidden="true"></li>{/if}
    <li
      class:done={i + 1 < current}
      class:on={i + 1 === current}
      aria-current={i + 1 === current ? 'step' : undefined}
      {@attach tip(help)}
    >
      <span class="num">
        {#if i + 1 < current}<Icon name="check" size={12} /><span class="sr-only">(끝남)</span>{:else}{i + 1}{/if}
      </span>
      <span class="name">{name}</span>
    </li>
  {/each}
</ol>

<style>
  .steps {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 13px;
    color: var(--text-3);
  }
  li {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    white-space: nowrap;
    border-radius: 999px;
  }
  .line {
    width: 14px;
    height: 1.5px;
    padding: 0;
    background: var(--border-strong);
    border-radius: 0;
  }
  .num {
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    font-size: 12px;
    font-weight: 700;
    border: 1.5px solid currentColor;
    border-radius: 50%;
  }
  .done .num {
    color: var(--success);
    background: var(--success-soft);
    border-color: var(--success);
  }
  .on {
    font-weight: 700;
    color: var(--text);
    background: var(--accent-soft);
  }
  .on .num {
    color: var(--on-accent);
    background: var(--accent);
    border-color: var(--accent-border);
  }
  @media (max-width: 1000px) {
    .name {
      display: none;
    }
  }
</style>
