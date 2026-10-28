// 스텝의 capture_json·annotations_json을 만든다. 필드 이름은 스펙 5.2절 그대로이고, 좌표는 원본 이미지의 물리 픽셀이다.
package recorder

import (
	"encoding/json"
	"image"
	"math"
	"time"
)

const markerRadius = 24 // 96dpi 기준 클릭 표시 반지름(px). 실제 크기는 DPI 배율을 곱한다

type xy struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type element struct {
	Name         string `json:"name"`
	ControlType  int32  `json:"control_type"`
	AutomationID string `json:"automation_id"`
}

type captureInfo struct {
	Point       *xy     `json:"point"`
	DPIScale    float64 `json:"dpi_scale"`
	Monitor     box     `json:"monitor"` // 찍은 지점의 모니터(화면 좌표)
	WindowTitle string  `json:"window_title"`
	Process     string  `json:"process"`
	Element     element `json:"element"`
}

type style struct {
	Stroke   string  `json:"stroke"`
	Fill     string  `json:"fill"`
	Width    int     `json:"width"`
	Opacity  float64 `json:"opacity"`
	FontSize int     `json:"font_size"`
}

type annotation struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
	Style style  `json:"style"`
}

// captureJSON의 p는 이미지 좌표다(nil이면 표시 지점 없음).
func captureJSON(p *image.Point, scale float64, monitor image.Rectangle, windowTitle, process string, el element) string {
	c := captureInfo{
		DPIScale:    scale,
		Monitor:     box{monitor.Min.X, monitor.Min.Y, monitor.Dx(), monitor.Dy()},
		WindowTitle: windowTitle,
		Process:     process,
		Element:     el,
	}
	if p != nil {
		c.Point = &xy{p.X, p.Y}
	}
	b, _ := json.Marshal(c) // 고정 구조체라 실패하지 않는다
	return string(b)
}

// markerJSON은 p(이미지 좌표)에 자동 클릭 표시 하나를 둔다. 크기와 선 두께는 DPI 배율을 따른다.
func markerJSON(p *image.Point, scale float64, id string) string {
	if p == nil {
		return "[]"
	}
	r := int(math.Round(markerRadius * scale))
	b, _ := json.Marshal([]annotation{{
		ID: id, Type: "marker", X: p.X - r, Y: p.Y - r, W: 2 * r, H: 2 * r,
		Style: style{Stroke: "#E5383B", Fill: "#E5383B", Width: int(math.Round(3 * scale)), Opacity: 0.35},
	}})
	return string(b)
}

// isDouble은 이번 클릭(at, p)이 앞 클릭(prevAt, prev)과 함께 더블클릭 한 번인지 본다. Windows와 같은 규칙이다:
// 시간은 시스템 더블클릭 시간(limit) 이내, 위치는 앞 클릭을 가운데로 한 더블클릭 사각형(size) 안. prevAt이 0이면 앞 클릭이 없다.
func isDouble(prevAt, at time.Time, prev, p image.Point, limit time.Duration, size image.Point) bool {
	d := p.Sub(prev)
	return !prevAt.IsZero() && at.Sub(prevAt) <= limit && 2*abs(d.X) <= size.X && 2*abs(d.Y) <= size.Y
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
