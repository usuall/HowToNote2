// 내보내기 문서 모델(DocModel, 스펙 7.1절). 저장소의 가이드를 한 번 읽어 네 렌더러(HTML·PDF·DOCX·HWPX)가 같이 쓰는 모양으로 만든다.
// 그림은 프론트가 미리 만든 합성 PNG(cache\rendered)만 쓴다. Go는 그리기를 다시 구현하지 않는다(스펙 3.2절 1).
package export

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png" // 합성 PNG 크기 읽기
	"os"
	"strings"
	"time"

	"howtonote/internal/store"
)

type Doc struct {
	Title string
	Date  string // 표지·머리글의 {{날짜}}
	Steps []Step
}

type Step struct {
	ID     int64
	Number string // "1", "1.1"
	Sub    bool   // 하위 스텝
	Title  string
	Blocks []Block
	Before []Callout // 그림 앞 안내 상자
	After  []Callout // 그림 뒤 안내 상자
	Image  *Image
}

// Image는 합성 PNG 파일이다. Path는 앱 데이터 폴더 안이라 결과물에 쓰지 않고 내용만 넣는다(스펙 7.4절).
type Image struct {
	Path          string
	Width, Height int // 물리 픽셀
}

type Callout struct {
	Kind   string // info, tip, warning, danger
	Title  string // 비면 Kind 이름(알림·도움말·주의·위험)
	Blocks []Block
}

type BlockKind string

const (
	KindPara    BlockKind = "p"
	KindHeading BlockKind = "h"
	KindList    BlockKind = "list"
	KindTable   BlockKind = "table"
)

// Block은 설명 한 덩어리다. Kind에 따라 쓰는 칸이 다르다.
type Block struct {
	Kind    BlockKind
	Level   int       // KindHeading: 3, 4
	Runs    []Run     // KindPara, KindHeading
	Ordered bool      // KindList
	Start   int       // KindList: 번호 목록 시작 번호
	Items   [][]Block // KindList: 항목마다 블록(문단, 안쪽 목록)
	Rows    [][]Cell  // KindTable
}

type Cell struct {
	Header bool
	Blocks []Block
}

// Run은 같은 서식의 글자 묶음이다. Break면 줄바꿈 하나이고 Text는 비어 있다.
type Run struct {
	Text                    string
	Bold, Italic, Underline bool
	Href                    string
	Break                   bool
}

// calloutNames는 편집기(Callouts.svelte)와 같은 안내 상자 이름이다.
var calloutNames = map[string]string{"info": "알림", "tip": "도움말", "warning": "주의", "danger": "위험"}

// Heading은 안내 상자의 제목 줄이다. 제목을 비워 두면 종류 이름을 쓴다.
func (c Callout) Heading() string {
	if c.Title != "" {
		return c.Title
	}
	return calloutNames[c.Kind]
}

// Heading은 스텝 제목 줄이다: 최상위 "1. 제목", 하위 "1.1 제목". 제목이 비었으면(글·그림 스텝) 번호만 쓴다.
func (s Step) Heading() string {
	num := s.Number
	if !s.Sub {
		num += "."
	}
	if s.Title == "" {
		return num
	}
	return num + " " + s.Title
}

// renderVersion은 합성 렌더러(frontend/src/editor/render.ts)의 결과가 바뀔 때 올린다. 그러면 예전 캐시를 쓰지 않는다.
const renderVersion = "1"

// RenderKey는 합성 PNG 캐시 이름이다(스펙 7.1절: 이미지 sha256 + annotations_json + view_json).
func RenderKey(st store.Step) string {
	h := sha256.New()
	for _, s := range []string{renderVersion, st.ImageSHA, st.AnnotationsJSON, st.ViewJSON} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ErrNotRendered는 합성 PNG가 아직 없는 스텝이 있다는 뜻이다. 프론트가 EnsureRendered를 먼저 해야 한다.
var ErrNotRendered = errors.New("export: 합성 그림이 준비되지 않았다")

// Numbers는 화면 번호 규칙(frontend/src/editor/model.ts numbering)과 똑같이 포트한다: 최상위 "1", 하위 "1.1".
// 숨긴 스텝은 번호가 없고(""), 숨긴 최상위 스텝의 하위 스텝도 숨긴 것으로 본다 — 숨긴 최상위가 첫 스텝이어도 같다
// (컨트롤러 재정 M3-C2). head는 바로 앞 최상위 스텝이다. steps는 store.ListSteps 순서(지운 스텝 없음)다.
func Numbers(steps []store.Step) []string {
	out := make([]string, len(steps))
	var head *store.Step
	top, sub := 0, 0
	for i := range steps {
		s := &steps[i]
		child := s.ParentID != 0 && head != nil
		if !child {
			head = s
		}
		switch {
		case s.Hidden || (child && head.Hidden):
			out[i] = ""
		case child:
			sub++
			out[i] = fmt.Sprintf("%d.%d", top, sub)
		default:
			top++
			sub = 0
			out[i] = fmt.Sprintf("%d", top)
		}
	}
	return out
}

// Build는 저장소에서 가이드를 읽어 문서 모델을 만든다. 번호가 없는 스텝(숨김, 또는 숨긴 최상위의 하위)은
// 내보내지 않는다(스펙 7.4절, M3-C2).
func Build(st *store.Store, guideID int64, now time.Time) (Doc, error) {
	g, err := st.GetGuide(guideID)
	if err != nil {
		return Doc{}, err
	}
	steps, err := st.ListSteps(guideID)
	if err != nil {
		return Doc{}, err
	}
	d := Doc{Title: g.Title, Date: now.Format("2006. 1. 2.")}
	for i, num := range Numbers(steps) {
		if num == "" {
			continue
		}
		s := steps[i]
		step := Step{ID: s.ID, Number: num, Sub: strings.Contains(num, "."), Title: s.Title, Blocks: Blocks(s.Description)}
		step.Before, step.After = Callouts(s.CalloutsJSON)
		if s.ImageSHA != "" {
			img, err := rendered(st, s)
			if err != nil {
				return Doc{}, err
			}
			step.Image = img
		}
		d.Steps = append(d.Steps, step)
	}
	return d, nil
}

func rendered(st *store.Store, s store.Step) (*Image, error) {
	path, ok := st.RenderedFile(RenderKey(s))
	if !ok {
		return nil, ErrNotRendered
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: 스텝 %d", ErrNotRendered, s.ID)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil {
		// 깨진 캐시 PNG다(검수 Minor 3): 지워서 Cached(RenderedFile)가 바로 없는 것으로 보게 하고, 프론트가 다음
		// 내보내기에서 다시 그리게 한다. Windows는 열린 파일을 못 지우므로 먼저 닫는다.
		os.Remove(path)
		return nil, fmt.Errorf("%w: 스텝 %d", ErrNotRendered, s.ID)
	}
	return &Image{Path: path, Width: cfg.Width, Height: cfg.Height}, nil
}

// Callouts는 callouts_json을 앞·뒤 안내 상자로 나눈다. 깨진 값은 없는 것으로 본다(편집기와 같은 너그러움).
func Callouts(js string) (before, after []Callout) {
	var raw []struct {
		Type     string          `json:"type"`
		Title    string          `json:"title"`
		Body     json.RawMessage `json:"body"`
		Position string          `json:"position"`
	}
	if json.Unmarshal([]byte(js), &raw) != nil {
		return nil, nil
	}
	for _, r := range raw {
		kind := r.Type
		if _, ok := calloutNames[kind]; !ok {
			kind = "info"
		}
		c := Callout{Kind: kind, Title: r.Title, Blocks: Blocks(string(r.Body))}
		if r.Position == "before" {
			before = append(before, c)
		} else {
			after = append(after, c)
		}
	}
	return before, after
}
