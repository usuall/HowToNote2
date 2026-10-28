// capture_json·annotations_json 형식 테스트. 필드 이름과 좌표 기준은 스펙 5.2절이다.
package recorder

import (
	"image"
	"testing"
	"time"
)

func TestCaptureJSON(t *testing.T) {
	p := image.Pt(80, 100)
	got := captureJSON(&p, 1.5, image.Rect(0, 0, 2560, 1440), "제목 없음 - 메모장", "notepad.exe",
		element{Name: "저장", ControlType: 50000, AutomationID: "btnSave"})
	want := `{"point":{"x":80,"y":100},"dpi_scale":1.5,"monitor":{"x":0,"y":0,"w":2560,"h":1440},` +
		`"window_title":"제목 없음 - 메모장","process":"notepad.exe","element":{"name":"저장","control_type":50000,"automation_id":"btnSave"}}`
	if got != want {
		t.Errorf("captureJSON =\n%s\nwant\n%s", got, want)
	}
	if got := captureJSON(nil, 1, image.Rect(-1920, 0, 0, 1080), "", "", element{}); got !=
		`{"point":null,"dpi_scale":1,"monitor":{"x":-1920,"y":0,"w":1920,"h":1080},"window_title":"","process":"","element":{"name":"","control_type":0,"automation_id":""}}` {
		t.Errorf("표시 지점 없는 captureJSON = %s", got)
	}
}

func TestMarkerJSON(t *testing.T) {
	p := image.Pt(80, 100)
	want := `[{"id":"m1","type":"marker","x":44,"y":64,"w":72,"h":72,"style":{"stroke":"#E5383B","fill":"#E5383B","width":5,"opacity":0.35,"font_size":0}}]`
	if got := markerJSON(&p, 1.5, "m1"); got != want {
		t.Errorf("markerJSON =\n%s\nwant\n%s", got, want)
	}
	if got := markerJSON(nil, 1, "m1"); got != "[]" {
		t.Errorf("표시 지점이 없으면 [] 이어야 한다: %s", got)
	}
}

func TestIsDouble(t *testing.T) {
	t0 := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	at := image.Pt(100, 100)
	box := image.Pt(4, 4) // 96dpi 기본 SM_CXDOUBLECLK·SM_CYDOUBLECLK
	limit := 500 * time.Millisecond
	cases := []struct {
		name   string
		prevAt time.Time
		after  time.Duration
		p      image.Point
		want   bool
	}{
		{"같은 자리 바로", t0, 120 * time.Millisecond, at, true},
		{"사각형 가장자리", t0, 120 * time.Millisecond, image.Pt(102, 98), true},
		{"사각형 밖", t0, 120 * time.Millisecond, image.Pt(103, 100), false},
		{"시간 끝", t0, limit, at, true},
		{"시간 지남", t0, limit + time.Millisecond, at, false},
		{"앞 클릭 없음", time.Time{}, 0, at, false},
	}
	for _, c := range cases {
		if got := isDouble(c.prevAt, t0.Add(c.after), at, c.p, limit, box); got != c.want {
			t.Errorf("%s: isDouble = %v, want %v", c.name, got, c.want)
		}
	}
}
