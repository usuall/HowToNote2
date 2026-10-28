<!-- 안내 상자(callouts_json, 스펙 5.2절): 종류(알림·도움말·주의·위험), 제목, 본문(설명과 같은 허용 서식), 놓을 자리(스텝 앞·뒤). -->
<script lang="ts">
  import type { JSONContent } from '@tiptap/core'
  import RichText from './RichText.svelte'
  import Icon from '../ui/Icon.svelte'
  import { tip } from '../ui/tip'

  type Kind = 'info' | 'tip' | 'warning' | 'danger'
  type Callout = { type: Kind; title: string; body: JSONContent | null; position: 'before' | 'after' }

  let { json, onchange }: { json: string; onchange: (json: string, key: string) => void } = $props()

  const KINDS: [Kind, string][] = [
    ['info', '알림'],
    ['tip', '도움말'],
    ['warning', '주의'],
    ['danger', '위험'],
  ]

  // parse는 저장된 안내 상자를 읽는다. 깨진 값은 빈 목록으로 보고, 모르는 종류는 '알림'으로 둔다.
  function parse(s: string): Callout[] {
    try {
      const v = JSON.parse(s || '[]')
      if (!Array.isArray(v)) return []
      return v.map((c) => ({
        type: KINDS.some(([k]) => k === c?.type) ? c.type : 'info',
        title: typeof c?.title === 'string' ? c.title : '',
        body: c?.body && typeof c.body === 'object' ? c.body : null,
        position: c?.position === 'before' ? 'before' : 'after',
      }))
    } catch {
      return []
    }
  }

  const list = $derived(parse(json))

  function save(next: Callout[], key = '') {
    onchange(JSON.stringify(next), key)
  }

  function set(i: number, patch: Partial<Callout>, key = '') {
    save(list.map((c, k) => (k === i ? { ...c, ...patch } : c)), key)
  }
</script>

<section class="callouts">
  {#each list as c, i (i)}
    <div class="box {c.type}">
      <div class="head">
        <select aria-label="안내 상자 종류" value={c.type} onchange={(e) => set(i, { type: e.currentTarget.value as Kind })}>
          {#each KINDS as [k, name] (k)}<option value={k}>{name}</option>{/each}
        </select>
        <select
          aria-label="안내 상자 자리"
          value={c.position}
          onchange={(e) => set(i, { position: e.currentTarget.value as Callout['position'] })}
        >
          <option value="before">스텝 앞에</option>
          <option value="after">스텝 뒤에</option>
        </select>
        <button class="btn icon sm remove" aria-label="안내 상자 지우기" onclick={() => save(list.filter((_, k) => k !== i))} {@attach tip()}>
          <Icon name="trash" />
        </button>
      </div>
      <input
        class="title"
        aria-label="안내 상자 제목"
        placeholder="제목(비워도 됩니다)"
        value={c.title}
        oninput={(e) => set(i, { title: e.currentTarget.value }, `title:${i}`)}
      />
      <RichText
        label="안내 상자 본문"
        value={c.body ? JSON.stringify(c.body) : ''}
        onchange={(v) => set(i, { body: v ? JSON.parse(v) : null }, `body:${i}`)}
      />
    </div>
  {/each}
  <div class="add">
    {#each KINDS as [k, name] (k)}
      <button
        class="btn sm"
        onclick={() => save([...list, { type: k, title: '', body: null, position: 'after' }])}
        {@attach tip(`${name} 상자를 스텝 뒤에 넣습니다.`)}
      >
        <span class="swatch {k}" aria-hidden="true"></span>+ {name}
      </button>
    {/each}
  </div>
</section>

<style>
  .callouts {
    display: grid;
    gap: 10px;
  }
  /* 안내 상자 색은 내보낸 문서와 맞춘 내용 색이라 변수 대신 그대로 둔다 */
  .box {
    display: grid;
    gap: 6px;
    padding: 10px;
    background: #e8f0fe;
    border-left: 6px solid #1a73e8;
    border-radius: var(--radius-sm);
  }
  .box.tip {
    background: #e6f4ea;
    border-left-color: #188038;
  }
  .box.warning {
    background: #fef7e0;
    border-left-color: #f9ab00;
  }
  .box.danger {
    background: #fce8e6;
    border-left-color: #d93025;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .head select {
    height: 30px;
    font-size: 13px;
  }
  .remove {
    margin-left: auto;
  }
  .title {
    font-weight: 700;
  }
  .add {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .swatch {
    width: 8px;
    height: 8px;
    background: #1a73e8;
    border-radius: 2px;
  }
  .swatch.tip {
    background: #188038;
  }
  .swatch.warning {
    background: #f9ab00;
  }
  .swatch.danger {
    background: #d93025;
  }
</style>
