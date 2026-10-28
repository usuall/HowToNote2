// 2단계: OCR 대체, 제목, PNG 인코딩, 이미지·스텝 저장, 툴바 알림. saver 고루틴 하나가 순서대로 해서 스텝 순서가 클릭 순서와 같다.
package recorder

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/png"
	"time"

	"howtonote/internal/store"
	"howtonote/internal/title"
	"howtonote/internal/uia"
)

func (r *Recorder) saver() {
	for s := range r.shots {
		r.save(s)
	}
	// M1-carry-3: 끝날 때 훅이 버린 클릭 수를 같이 알린다. hook.Stop()이 이미 끝난 뒤라(loop의 defer 순서) 최종값이다.
	r.emit(Event{Type: "stopped", GuideID: r.guideID, Steps: r.steps, Dropped: r.hook.DroppedClicks()})
	close(r.done)
}

func (r *Recorder) save(s shot) {
	if s.kind == "double" {
		r.mergeDouble()
		return
	}
	r.lastID = 0        // 이 스텝 저장이 실패하면 다음 더블클릭이 엉뚱한 스텝을 고치지 않게 먼저 비운다
	var ip *image.Point // 이미지 좌표의 표시 지점
	if s.point != nil {
		p := s.point.Sub(s.area.Min)
		ip = &p
	}
	if r.opt.OCR != nil && ip != nil && title.NeedsText(s.in) {
		s.in.OCRText = r.opt.OCR.TextAround(s.img, *ip, time.Second)
	}
	var buf bytes.Buffer
	// 가장 빠른 압축: 4K 화면도 500ms 목표 안에 넣으려는 것이다. 파일이 조금 커지는 것은 감수한다.
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, s.img); err != nil {
		r.warn(msgSave)
		return
	}
	imgID, sha, err := r.st.PutImage(buf.Bytes(), s.img.Rect.Dx(), s.img.Rect.Dy())
	if err != nil {
		r.warn(msgSave)
		return
	}

	r.settleCheckbox(&s) // C1: 체크박스는 여기(stage 2)에서 느긋하게 "누른 뒤" 상태를 다시 읽는다
	t := title.Make(s.in, r.opt.Style)

	if r.guideID == 0 {
		if r.guideID, err = r.st.CreateGuide("새 매뉴얼 " + s.at.Format("2006-01-02 15:04")); err != nil {
			r.warn(msgSave)
			return
		}
	}
	id, err := r.st.AppendStep(r.guideID, store.NewStep{
		Kind:            s.kind,
		Title:           t,
		ImageID:         imgID,
		CaptureJSON:     captureJSON(ip, s.scale, s.monitor, s.win.Title, s.win.Process, element{s.el.Name, s.el.ControlType, s.el.AutomationID}),
		AnnotationsJSON: markerJSON(ip, s.scale, rand.Text()),
	})
	if err != nil {
		r.warn(msgSave)
		return
	}
	r.steps++
	if s.kind == "click" {
		r.lastID, r.lastIn, r.lastSHA = id, s.in, sha
	}
	r.emit(Event{Type: "step", GuideID: r.guideID, StepID: id, Title: t, ImageSHA: sha, Steps: r.steps,
		LatencyMs: time.Since(s.at).Milliseconds(), GrabMs: s.grabMs})
}

// mergeDouble은 마지막 클릭 스텝의 제목을 더블클릭 문구로 바꾼다. 두 번째 클릭은 찍지 않는다:
// 첫 클릭 때 찍은 화면(더블클릭으로 화면이 바뀌기 전)이 이 스텝에 맞는 화면이다.
func (r *Recorder) mergeDouble() {
	if r.lastID == 0 {
		return
	}
	in := r.lastIn
	in.Double = true
	t := title.Make(in, r.opt.Style)
	if err := r.st.SetStepTitle(r.lastID, t); err != nil {
		r.warn(msgSave)
		return
	}
	r.emit(Event{Type: "step", GuideID: r.guideID, StepID: r.lastID, Title: t, ImageSHA: r.lastSHA, Steps: r.steps})
	r.lastID = 0
}

// settleCheckbox는 체크박스 클릭의 제목을 "누른 뒤" 실제 상태로 맞춘다(컨트롤러 규칙 C1, 스펙 4.3절).
// stage 1(onClick)이 아니라 여기 stage 2(saver)에서 기다린다 - PNG 인코딩·PutImage가 이미 끝난 뒤라 다음
// 클릭의 촬영(stage 1)은 이 대기와 전혀 무관하게 이미 끝나 있다(고침: 원래 onClick에 있던 대기를 옮김).
// 오른쪽 클릭은 체크박스를 토글하지 않으므로 기다리지 않고, ControlType을 체크박스가 아닌 값으로 바꿔
// title.Make의 선택/해제 분기를 건너뛰고 일반 "...를 클릭합니다." + 오른쪽 클릭 틀을 쓰게 한다.
// 다시 읽은 요소가 체크박스가 아니거나 읽기 자체가 실패하면(시간 초과 등) 오른쪽 클릭처럼 일반 클릭 제목을 쓴다:
// 첫 읽기는 화면을 찍은 뒤에 하므로 이미 "누른 뒤" 상태일 때가 많아, 뒤집으면 오히려 틀린 제목이 된다.
func (r *Recorder) settleCheckbox(s *shot) {
	if s.el.ControlType != uia.CheckBoxControlType {
		return
	}
	if s.right {
		s.in.ControlType = 0 // uia.CheckBoxControlType이 아닌 값이면 충분하다 - title 쪽 분기만 피하면 된다
		return
	}
	if wait := s.at.Add(toggleSettle).Sub(time.Now()); wait > 0 {
		time.Sleep(wait)
	}
	if s.point != nil {
		if el2, err := r.ua.ElementAt(int32(s.point.X), int32(s.point.Y), uiaTimeout); err == nil && el2.ControlType == uia.CheckBoxControlType {
			s.in.Checked = el2.ToggleState == 1
			return
		}
	}
	s.in.ControlType = 0 // 다시 읽기 실패: 선택/해제를 추측하지 않는다
}
