// 설명·안내 상자 편집기의 허용 서식(스펙 6.3절): 문단, 소제목(H3, H4), 굵게, 기울임, 밑줄, 글머리·번호 목록, 링크, 단순 표, 줄바꿈.
// 스키마에 없는 서식은 붙여넣을 때 ProseMirror가 버리거나 문단으로 바꾼다. 그래서 DOCX·HWPX 변환(M3 Go)은 아래 이름만 알면 된다.
import { getSchema, type JSONContent } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'

// 셀 병합 없음: 붙여넣은 표의 colspan·rowspan은 1로 읽고(빈 칸은 prosemirror-tables가 채운다), 셀 안은 문단만 둔다.
const noMerge = {
  colspan: { default: 1, parseHTML: () => 1 },
  rowspan: { default: 1, parseHTML: () => 1 },
  colwidth: { default: null, parseHTML: () => null },
}
const Cell = TableCell.extend({ content: 'paragraph+', addAttributes: () => noMerge })
const Header = TableHeader.extend({ content: 'paragraph+', addAttributes: () => noMerge })

export const richExtensions = [
  StarterKit.configure({
    blockquote: false,
    code: false,
    codeBlock: false,
    horizontalRule: false,
    strike: false,
    heading: { levels: [3, 4] },
    // 주소는 http(s)://와 mailto:만 받는다. Tiptap 기본 검사는 tel:·ftp: 등과 글자로 시작하지 않는 주소(\\nas\…, /C:/…, ../…)를
    // 모두 통과시켜 로컬·네트워크 경로가 결과물에 들어간다(스펙 7.4절). 붙여넣기·자동 링크·링크 걸기가 모두 이 검사를 지난다.
    // 그래서 'naver.com'처럼 앞에 http(s)://가 없는 글자는 자동 링크가 되지 않는다. '링크' 단추는 https://를 붙여 준다.
    link: { openOnClick: false, autolink: true, defaultProtocol: 'https', isAllowedUri: (u) => /^(https?:\/\/|mailto:)/i.test(u) },
  }),
  Table.configure({ resizable: false }),
  TableRow,
  Header,
  Cell,
]

// M3 Go 변환기와 맞출 이름 목록. 바꾸면 M3 변환기도 같이 바꾼다.
export const ALLOWED_NODES = [
  'doc', 'paragraph', 'text', 'heading', 'hardBreak', 'bulletList', 'orderedList', 'listItem',
  'table', 'tableRow', 'tableHeader', 'tableCell',
]
export const ALLOWED_MARKS = ['bold', 'italic', 'underline', 'link']

export const richSchema = getSchema(richExtensions)

// parseDoc은 저장된 설명(빈 문자열 또는 Tiptap JSON)을 읽는다. 깨졌으면 빈 문서다.
export function parseDoc(json: string): JSONContent | null {
  if (!json) return null
  try {
    const d = JSON.parse(json)
    return d && d.type === 'doc' ? d : null
  } catch {
    return null
  }
}
