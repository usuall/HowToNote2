<!-- 설명 편집기(Tiptap, 스펙 6.3절 허용 서식). 값은 Tiptap JSON 문자열이고 비면 빈 문자열이다.
     실행 취소처럼 밖에서 값이 바뀌면 내용을 맞춘다. 서식 단추는 한글·워드에서 다 아는 모양이라 아이콘과 툴팁으로 두고
     (UI 설계 4.5절), 소제목만 글자로 둔다. 비어 있고 초점이 없으면 칸 위에 회색 안내를 겹쳐 보인다(Placeholder 확장 없이). -->
<script lang="ts">
  import { Editor } from '@tiptap/core'
  import { onMount } from 'svelte'
  import Icon from '../ui/Icon.svelte'
  import { tip } from '../ui/tip'
  import { parseDoc, richExtensions } from './rich'

  let {
    value,
    label,
    placeholder = '',
    onchange,
  }: { value: string; label: string; placeholder?: string; onchange: (json: string) => void } = $props()

  let el: HTMLDivElement
  let editor = $state<Editor>()
  let tick = $state(0) // 선택·내용이 바뀔 때마다 단추의 켜짐 표시와 안내 겹치기를 다시 그린다
  let focused = $state(false)
  let last = '' // 이 편집기가 마지막으로 내보내거나 받은 값
  let linking = $state(false)
  let href = $state('')
  let linkError = $state('')

  const empty = $derived.by(() => (void tick, editor ? editor.isEmpty : !value))

  onMount(() => {
    last = value
    const ed = new Editor({
      element: el,
      extensions: richExtensions,
      content: parseDoc(value),
      onUpdate: ({ editor }) => {
        last = editor.isEmpty ? '' : JSON.stringify(editor.getJSON())
        onchange(last)
      },
      onTransaction: () => tick++,
      onFocus: () => (focused = true),
      onBlur: () => (focused = false),
    })
    editor = ed
    return () => ed.destroy()
  })

  $effect(() => {
    if (editor && value !== last) {
      last = value
      editor.commands.setContent(parseDoc(value), { emitUpdate: false })
    }
  })

  function on(name: string, attrs?: Record<string, unknown>): boolean {
    void tick
    return editor?.isActive(name, attrs) ?? false
  }

  function inTable(): boolean {
    return on('table')
  }

  function startLink() {
    href = editor?.getAttributes('link').href ?? ''
    linkError = ''
    linking = true
  }

  // applyLink은 'naver.com'처럼 주소 이름으로 시작하면 https://를 붙인다. 경로(\\nas\…, /C:/…, ../…)에는 붙이지 않아
  // 주소 검사(rich.ts: http(s)://·mailto:만)에 걸리고, 그러면 안내한다.
  function applyLink() {
    let v = href.trim()
    if (!v) {
      editor?.chain().focus().extendMarkRange('link').unsetLink().run()
      linking = false
      return
    }
    if (/^[^\s/\\:.]+\./.test(v)) v = `https://${v}`
    if (editor?.chain().focus().extendMarkRange('link').setLink({ href: v }).run()) {
      linking = false
    } else {
      linkError = '이 주소는 링크로 쓸 수 없습니다. https://로 시작하는 주소를 넣어 보세요.'
    }
  }
</script>

<div class="rich">
  <!-- 단추를 눌러도 편집기의 선택이 풀리지 않게 mousedown 기본 동작을 막는다 -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="bar" role="toolbar" aria-label={`${label} 서식`} tabindex="-1" onmousedown={(e) => e.preventDefault()}>
    <button class="fmt" aria-label="굵게" aria-pressed={on('bold')} onclick={() => editor?.chain().focus().toggleBold().run()} {@attach tip('굵게 (Ctrl+B)')}><b>가</b></button>
    <button class="fmt" aria-label="기울임" aria-pressed={on('italic')} onclick={() => editor?.chain().focus().toggleItalic().run()} {@attach tip('기울임 (Ctrl+I)')}><i>가</i></button>
    <button class="fmt" aria-label="밑줄" aria-pressed={on('underline')} onclick={() => editor?.chain().focus().toggleUnderline().run()} {@attach tip('밑줄 (Ctrl+U)')}><u>가</u></button>
    <span class="sep" aria-hidden="true"></span>
    <button class="fmt" aria-pressed={on('heading', { level: 3 })} onclick={() => editor?.chain().focus().toggleHeading({ level: 3 }).run()} {@attach tip('이 문단을 소제목으로 만듭니다.')}>소제목</button>
    <button class="fmt" aria-pressed={on('heading', { level: 4 })} onclick={() => editor?.chain().focus().toggleHeading({ level: 4 }).run()} {@attach tip('이 문단을 작은 소제목으로 만듭니다.')}>작은 소제목</button>
    <span class="sep" aria-hidden="true"></span>
    <button class="fmt" aria-label="글머리 목록" aria-pressed={on('bulletList')} onclick={() => editor?.chain().focus().toggleBulletList().run()} {@attach tip()}><Icon name="list" /></button>
    <button class="fmt" aria-label="번호 목록" aria-pressed={on('orderedList')} onclick={() => editor?.chain().focus().toggleOrderedList().run()} {@attach tip()}><Icon name="list-ordered" /></button>
    <button class="fmt" aria-label="링크 걸기" aria-pressed={on('link')} onclick={startLink} {@attach tip()}><Icon name="link" /></button>
    {#if inTable()}
      <span class="sep" aria-hidden="true"></span>
      <button class="fmt" aria-label="줄 추가" onclick={() => editor?.chain().focus().addRowAfter().run()} {@attach tip('줄 추가\n지금 줄 아래에 새 줄을 넣습니다.')}><Icon name="row-add" /></button>
      <button class="fmt" aria-label="칸 추가" onclick={() => editor?.chain().focus().addColumnAfter().run()} {@attach tip('칸 추가\n지금 칸 오른쪽에 새 칸을 넣습니다.')}><Icon name="col-add" /></button>
      <button class="fmt" aria-label="줄 지우기" onclick={() => editor?.chain().focus().deleteRow().run()} {@attach tip('줄 지우기\n지금 줄을 지웁니다.')}><Icon name="row-remove" /></button>
      <button class="fmt" aria-label="칸 지우기" onclick={() => editor?.chain().focus().deleteColumn().run()} {@attach tip('칸 지우기\n지금 칸을 지웁니다.')}><Icon name="col-remove" /></button>
      <button class="fmt" aria-label="표 지우기" onclick={() => editor?.chain().focus().deleteTable().run()} {@attach tip('표 지우기\n표 전체를 지웁니다.')}><Icon name="table-remove" /></button>
    {:else}
      <button class="fmt" aria-label="표 넣기" onclick={() => editor?.chain().focus().insertTable({ rows: 3, cols: 2, withHeaderRow: true }).run()} {@attach tip('표 넣기\n3줄 2칸 표를 넣습니다.')}><Icon name="table" /></button>
    {/if}
  </div>
  {#if linking}
    <div class="link">
      <!-- 한글 조합 중의 Enter는 글자를 확정하는 키라 링크를 걸지 않는다 -->
      <input
        aria-label="링크 주소"
        placeholder="https://"
        bind:value={href}
        onkeydown={(e) => e.key === 'Enter' && !e.isComposing && e.keyCode !== 229 && applyLink()}
      />
      <button class="btn primary sm" onclick={applyLink} {@attach tip(href.trim() ? '고른 글에 이 주소를 겁니다.' : '고른 글의 링크를 뺍니다.')}>
        {href.trim() ? '링크 걸기' : '링크 빼기'}
      </button>
      <button class="btn ghost sm" onclick={() => (linking = false)} {@attach tip('링크를 고치지 않고 그만둡니다.')}>취소</button>
      {#if linkError}<span class="error">{linkError}</span>{/if}
    </div>
  {/if}
  <div class="doc-wrap">
    <div class="doc" aria-label={label} bind:this={el}></div>
    {#if placeholder && empty && !focused}<p class="placeholder" aria-hidden="true">{placeholder}</p>{/if}
  </div>
  <p class="hint">줄을 바꾸려면 Shift+Enter, 새 문단은 Enter를 누르세요.</p>
</div>

<style>
  .rich {
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
  }
  .bar,
  .link {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px;
    padding: 3px;
    border-bottom: 1px solid var(--border);
  }
  .link {
    gap: 6px;
    padding: 6px;
  }
  .fmt {
    min-width: 30px;
    height: 28px;
    padding: 0 6px;
    font-size: 13px;
    color: var(--text-2);
    background: transparent;
    border-color: transparent;
    border-radius: var(--radius-sm);
  }
  /* 켜진 서식: 전역 단추의 마우스 올림 바탕보다 앞서야 흰 글자가 사라지지 않는다 */
  .fmt[aria-pressed='true'],
  .fmt[aria-pressed='true']:hover:not(:disabled) {
    color: var(--surface);
    background: var(--text);
  }
  .sep {
    width: 1px;
    height: 18px;
    margin: 0 4px;
    background: var(--border);
  }
  .link input {
    flex: 1;
    min-width: 200px;
    height: 30px;
  }
  .doc-wrap {
    position: relative;
  }
  .doc {
    min-height: 80px;
    padding: 6px 12px;
  }
  .doc :global(.ProseMirror) {
    min-height: 72px;
    outline: none;
  }
  /* 회색 안내가 첫 줄과 같은 높이에 오게 첫 문단 위 여백을 없앤다 */
  .doc :global(.ProseMirror > :first-child) {
    margin-top: 0;
  }
  .doc :global(table) {
    border-collapse: collapse;
  }
  .doc :global(td),
  .doc :global(th) {
    min-width: 60px;
    padding: 4px 8px;
    vertical-align: top;
    border: 1px solid var(--border-strong);
  }
  .doc :global(th) {
    background: var(--sunken);
  }
  .placeholder {
    position: absolute;
    top: 6px;
    right: 12px;
    left: 12px;
    margin: 0;
    color: var(--text-3);
    pointer-events: none;
  }
  .hint {
    margin: 0;
    padding: 0 12px 6px;
    font-size: 12px;
    color: var(--text-3);
  }
</style>
