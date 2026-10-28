// samplemanual은 샘플 매뉴얼 'HowtoNote 사용법'(docs/sample-manual)을 만들 데이터를 임시 데이터 폴더에 넣는다.
// 앱이 쓰는 internal/store를 그대로 써서, 앱이 녹화·편집한 것과 같은 행이 생긴다. 그 뒤 내보내기는 앱 화면에서 한다.
//
//	go run ./tools/samplemanual -appdata out/sample-appdata -screens ../docs/sample-manual/screens demo
//	go run ./tools/samplemanual -appdata out/sample-appdata -screens ../docs/sample-manual/screens manual
//
// demo는 앱 화면을 찍을 때 보일 예시 매뉴얼(인사시스템 연차 신청), manual은 찍은 앱 화면으로 만드는 'HowtoNote 사용법'이다.
// 앱이 켜져 있을 때 돌리지 않는다(같은 data.db를 두 프로그램이 고치면 편집기 화면과 DB가 어긋난다).
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"howtonote/internal/store"
)

type node = map[string]any

// mark는 그림에 그릴 빨간 표시다. 단추처럼 작은 것은 클릭 표시(가운데 원), 막대·카드 묶음처럼 넓은 것은 사각형으로 두른다.
type mark struct {
	key  string // marks.json의 이름, 또는 데모처럼 좌표를 바로 줄 때는 빈 문자열
	box  [4]int // x, y, w, h(그림 픽셀). key가 있으면 marks.json 값으로 채운다
	area bool
}

type callout struct {
	Type     string `json:"type"` // info, tip, warning, danger
	Title    string `json:"title"`
	Body     node   `json:"body"`
	Position string `json:"position"` // before, after
}

type step struct {
	title    string
	img      string // screens 폴더의 PNG 이름. 비면 글 스텝
	marks    []mark
	desc     node // Tiptap 문서. nil이면 설명 없음
	callouts []callout
	sub      bool // 바로 앞 최상위 스텝의 하위 스텝(1.1)
}

// screens는 marks.json이다: 앱 화면 PNG마다 UIA로 잰 단추 자리(그림 픽셀)와 화면 배율.
type screens struct {
	Scale   float64                      `json:"scale"`
	Screens map[string]map[string][4]int `json:"screens"`
}

func main() {
	appdata := flag.String("appdata", "", "앱을 띄울 때 APPDATA로 준 임시 폴더. 데이터는 그 안의 HowtoNote에 쓴다")
	dir := flag.String("screens", "", "그림 PNG와 marks.json이 있는 폴더")
	flag.Parse()
	if *appdata == "" || *dir == "" || flag.NArg() != 1 {
		log.Fatal("사용법: samplemanual -appdata <임시 폴더> -screens <그림 폴더> demo|manual")
	}
	// 사용자의 진짜 데이터(%APPDATA%\HowtoNote)에 샘플을 쓰면 안 된다. 같은 폴더면 멈춘다.
	abs, err := filepath.Abs(*appdata)
	if err != nil {
		log.Fatal(err)
	}
	if real := os.Getenv("APPDATA"); real != "" && strings.EqualFold(filepath.Clean(real), abs) {
		log.Fatal("-appdata가 실제 APPDATA입니다. 임시 폴더를 주세요.")
	}
	// 앱 startup과 같은 규칙으로 데이터 폴더를 정한다: settings.json의 data_dir, 없으면 <APPDATA>\HowtoNote.
	cfg := filepath.Join(abs, "HowtoNote")
	set, err := store.LoadSettings(cfg)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cmp.Or(set.DataDir, cfg))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	var title string
	var steps []step
	scale := 1.0 // Edge headless 그림은 배율 1로 찍었다
	switch flag.Arg(0) {
	case "demo":
		title, steps = "인사시스템 연차 신청 방법", demo()
	case "manual":
		var sc screens
		b, err := os.ReadFile(filepath.Join(*dir, "marks.json"))
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(b, &sc); err != nil {
			log.Fatal(err)
		}
		title, steps, scale = "HowtoNote 사용법", manual(), sc.Scale
		for i := range steps {
			for j, m := range steps[i].marks {
				box, ok := sc.Screens[steps[i].img][m.key]
				if !ok {
					log.Fatalf("marks.json에 %s의 %q가 없습니다", steps[i].img, m.key)
				}
				steps[i].marks[j].box = box
			}
		}
	default:
		log.Fatal("demo 또는 manual만 됩니다")
	}
	if err := build(st, *dir, title, steps, scale); err != nil {
		log.Fatal(err)
	}
	fmt.Println("만들었습니다:", title, len(steps), "스텝")
}

// build는 같은 제목의 매뉴얼을 지우고(다시 돌려도 하나만 남게) 새로 만든다. 스텝을 끝에 붙인 뒤
// 설명·안내 상자·하위 스텝은 편집기가 저장하는 길(UpdateSteps)로 쓴다.
func build(st *store.Store, dir, title string, steps []step, scale float64) error {
	gs, err := st.ListGuides()
	if err != nil {
		return err
	}
	for _, g := range gs {
		if g.Title == title {
			if err := st.DeleteGuide(g.ID); err != nil {
				return err
			}
		}
	}
	gid, err := st.CreateGuide(title)
	if err != nil {
		return err
	}
	capture := fmt.Sprintf(`{"dpi_scale":%g}`, scale) // 편집기의 새 객체 크기 단위와 맞춘다
	for i, s := range steps {
		n := store.NewStep{Kind: "text", Title: s.title, CaptureJSON: capture, AnnotationsJSON: annotations(i, s.marks, scale)}
		if s.img != "" {
			b, err := os.ReadFile(filepath.Join(dir, s.img))
			if err != nil {
				return err
			}
			c, _, err := image.DecodeConfig(bytes.NewReader(b))
			if err != nil {
				return fmt.Errorf("%s: %w", s.img, err)
			}
			if n.ImageID, _, err = st.PutImage(b, c.Width, c.Height); err != nil {
				return err
			}
			n.Kind = "image" // 녹화가 아니라 그림 파일로 넣은 스텝과 같은 종류
		}
		if _, err := st.AppendStep(gid, n); err != nil {
			return err
		}
	}
	rows, err := st.ListSteps(gid)
	if err != nil {
		return err
	}
	var parent int64
	for i := range rows {
		s := steps[i]
		if s.desc != nil {
			rows[i].Description = mustJSON(s.desc)
		}
		if s.callouts != nil {
			rows[i].CalloutsJSON = mustJSON(s.callouts)
		}
		if s.sub {
			rows[i].ParentID = parent
		} else {
			parent = rows[i].ID
		}
	}
	return st.UpdateSteps(gid, rows)
}

// annotations는 편집기 create()와 같은 모양의 객체를 만든다(frontend/src/editor/annotations.ts):
// 클릭 표시는 가운데에 48u 상자, 선 3u, 투명도 0.35. 사각형은 선 4u. u는 화면 배율이다.
func annotations(i int, ms []mark, u float64) string {
	const red = "#E5383B"
	r := func(v float64) int { return int(math.Round(v)) }
	out := []node{}
	for j, m := range ms {
		x, y, w, h := float64(m.box[0]), float64(m.box[1]), float64(m.box[2]), float64(m.box[3])
		a := node{"id": fmt.Sprintf("sample-%d-%d", i+1, j+1)}
		if m.area {
			pad := 6 * u // 단추 테두리에 선이 겹치지 않게 조금 바깥을 두른다
			a["type"], a["x"], a["y"], a["w"], a["h"] = "rect", r(x-pad), r(y-pad), r(w+2*pad), r(h+2*pad)
			a["style"] = node{"stroke": red, "fill": "", "width": r(4 * u), "opacity": 1, "font_size": 0}
		} else {
			d := 48 * u
			a["type"], a["x"], a["y"], a["w"], a["h"] = "marker", r(x+w/2-d/2), r(y+h/2-d/2), r(d), r(d)
			a["style"] = node{"stroke": red, "fill": red, "width": r(3 * u), "opacity": 0.35, "font_size": 0}
		}
		out = append(out, a)
	}
	return mustJSON(out)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// 아래는 Tiptap 문서를 짧게 쓰는 도우미다. 편집기 허용 서식(rich.ts)만 쓴다: 문단, 굵게, 글머리 목록, 표.

func doc(blocks ...node) node { return node{"type": "doc", "content": blocks} }

// p는 문단이다. "**굵게**"로 감싼 부분은 굵게 표시한다.
func p(s string) node {
	var runs []node
	for i, part := range strings.Split(s, "**") {
		if part == "" {
			continue
		}
		t := node{"type": "text", "text": part}
		if i%2 == 1 {
			t["marks"] = []node{{"type": "bold"}}
		}
		runs = append(runs, t)
	}
	return node{"type": "paragraph", "content": runs}
}

func ul(items ...string) node { return list("bulletList", items) }
func ol(items ...string) node { return list("orderedList", items) }

func list(kind string, items []string) node {
	var li []node
	for _, s := range items {
		li = append(li, node{"type": "listItem", "content": []node{p(s)}})
	}
	return node{"type": kind, "content": li}
}

// table은 첫 줄을 머리 칸으로 하는 단순 표다.
func table(rows ...[]string) node {
	var tr []node
	for i, r := range rows {
		cell := "tableCell"
		if i == 0 {
			cell = "tableHeader"
		}
		var cs []node
		for _, s := range r {
			cs = append(cs, node{"type": cell, "attrs": node{"colspan": 1, "rowspan": 1}, "content": []node{p(s)}})
		}
		tr = append(tr, node{"type": "tableRow", "content": cs})
	}
	return node{"type": "table", "content": tr}
}

func box(x, y, w, h int) mark { return mark{box: [4]int{x, y, w, h}} }
func at(key string) mark      { return mark{key: key} }
func around(key string) mark  { return mark{key: key, area: true} }

// demo는 앱 화면 사진에 보일 예시 매뉴얼이다. 좌표는 hr.html의 고정 배치(1280×800)에서 읽었다.
func demo() []step {
	return []step{
		{title: "왼쪽 메뉴에서 '연차 신청'을 누릅니다.", img: "hr-1.png", marks: []mark{box(20, 168, 80, 44)},
			desc:     doc(p("인사시스템에 들어가면 왼쪽에 메뉴가 있습니다. **연차 신청**을 누르면 신청 양식이 열립니다.")),
			callouts: []callout{{Type: "tip", Title: "남은 연차 먼저 보기", Body: doc(p("홈 화면의 '남은 연차'에서 쓸 수 있는 날수를 먼저 확인하세요.")), Position: "after"}}},
		{title: "휴가 종류와 기간을 고르고 사유를 적습니다.", img: "hr-2.png", marks: []mark{box(420, 210, 200, 40)},
			desc: doc(p("시작일과 종료일을 고르면 오른쪽에 쓰는 날수가 나옵니다."))},
		{title: "'신청하기'를 누릅니다.", img: "hr-2.png", marks: []mark{box(420, 420, 140, 48)},
			desc: doc(p("누르면 팀장님께 결재 요청이 갑니다."))},
		{title: "신청 내역에서 상태가 '결재 대기'인지 확인합니다.", img: "hr-3.png", marks: []mark{box(1071, 266, 100, 32)}},
	}
}
