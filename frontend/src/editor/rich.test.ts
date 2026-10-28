// @vitest-environment happy-dom
// 설명 허용 서식 테스트: 스키마에 허용 노드·마크만 있는지, 붙여넣은 HTML이 허용 서식으로 걸러지는지 본다.
// 붙여넣기와 같은 ProseMirror DOMParser 규칙을 쓰는 generateJSON으로 확인한다(DOM이 필요해 happy-dom 환경).
import { generateJSON, type JSONContent } from '@tiptap/core'
import { expect, test } from 'vitest'
import { ALLOWED_MARKS, ALLOWED_NODES, parseDoc, richExtensions, richSchema } from './rich'

// walk는 문서의 모든 노드 이름, 마크 이름, 링크 주소, 셀 병합 값을 모은다.
function walk(d: JSONContent, out = { nodes: new Set<string>(), marks: new Set<string>(), hrefs: [] as string[], spans: [] as number[] }) {
  out.nodes.add(d.type!)
  for (const m of d.marks ?? []) {
    out.marks.add(m.type)
    if (m.type === 'link') out.hrefs.push(m.attrs?.href)
  }
  if (d.type === 'tableCell' || d.type === 'tableHeader') out.spans.push(d.attrs?.colspan, d.attrs?.rowspan)
  for (const c of d.content ?? []) walk(c, out)
  return out
}

test('스키마에는 허용한 노드와 마크만 있다', () => {
  expect(Object.keys(richSchema.nodes).sort()).toEqual([...ALLOWED_NODES].sort())
  expect(Object.keys(richSchema.marks).sort()).toEqual([...ALLOWED_MARKS].sort())
})

test('붙여넣은 HTML은 허용 서식만 남는다', () => {
  const html = `
    <h1>큰 제목</h1><h2>둘째 제목</h2><h3>소제목</h3>
    <p style="color:red"><span style="font-size:30px"><b>굵게</b> <i>기울임</i> <u>밑줄</u> <s>취소선</s> <code>코드</code></span></p>
    <blockquote>인용</blockquote><pre>코드 블록</pre><hr><img src="a.png">
    <p><a href="https://example.com">좋은 링크</a> <a href="javascript:alert(1)">나쁜 링크</a> <a href="file:///C:/Users/a.txt">로컬</a></p>
    <ul><li>글머리</li></ul><ol><li>번호</li></ol>
    <table><tr><th colspan="2">머리</th></tr><tr><td rowspan="2"><ul><li>셀 목록</li></ul></td><td>b</td></tr><tr><td>c</td></tr></table>
    <p>첫 줄<br>둘째 줄</p>`
  const doc = generateJSON(html, richExtensions)
  const found = walk(doc)
  for (const n of found.nodes) expect(ALLOWED_NODES).toContain(n)
  for (const m of found.marks) expect(ALLOWED_MARKS).toContain(m)
  expect(found.nodes).toContain('table')
  expect(found.nodes).toContain('hardBreak')
  expect(found.marks).toEqual(new Set(['bold', 'italic', 'underline', 'link']))
  expect(found.hrefs).toEqual(['https://example.com'])
  expect(found.spans.every((v) => v === 1)).toBe(true)
  const headings = doc.content!.filter((n: JSONContent) => n.type === 'heading').map((n: JSONContent) => n.attrs?.level)
  expect(headings).toEqual([3])
  expect(JSON.stringify(doc)).toContain('큰 제목') // 허용하지 않는 제목도 글자는 문단으로 남는다
  expect(JSON.stringify(doc)).toContain('인용')
})

test('링크는 http·https·mailto 주소만 남고 로컬·네트워크 경로와 그 밖의 주소는 버린다(스펙 7.4절)', () => {
  const bad = ['\\\\nas\\share\\a.docx','/C:/Users/hong/a.docx', '../../Users/hong/a.docx', 'tel:010-1234-5678', 'ftp://example.com/a']
  const good = ['https://example.com/a', 'http://example.com', 'mailto:hong@example.com']
  const html = [...bad, ...good].map((h) => `<p><a href="${h}">링크</a></p>`).join('')
  expect(walk(generateJSON(html, richExtensions)).hrefs).toEqual(good)
})

test('parseDoc은 빈 값과 깨진 값을 빈 문서로 본다', () => {
  expect(parseDoc('')).toBeNull()
  expect(parseDoc('{')).toBeNull()
  expect(parseDoc('{"type":"paragraph"}')).toBeNull()
  expect(parseDoc('{"type":"doc","content":[]}')).toEqual({ type: 'doc', content: [] })
})
