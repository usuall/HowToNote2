// .htn 매뉴얼 파일 바인딩(스펙 5.3절, M4a): 내보내기 창의 'HowtoNote' 카드(ProjectPlan, ExportProject)와
// 첫 화면 '가져오기'(ImportProject, Task 4). 편집용은 원본 그림과 그리기를 그대로, 배포용은 모자이크·자르기를 그림에
// 확정해 넣는다(H1). 오류는 정해 둔 문장으로만 돌려준다(재정 R31).
package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"math"
	"os"
	"path/filepath"

	"howtonote/internal/export"
	"howtonote/internal/project"
	"howtonote/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"modernc.org/sqlite"
)

//go:embed wails.json
var wailsJSON []byte

// appVersion은 wails.json의 info.productVersion이다(manifest.json의 app_version, H8). 설치 파일 버전과 한곳에서 맞춘다.
var appVersion = func() string {
	var c struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	_ = json.Unmarshal(wailsJSON, &c)
	return c.Info.ProductVersion
}()

var (
	errKind        = errors.New("'고쳐 쓸 사람에게'와 '나눠 줄 사람에게' 가운데 하나를 고르세요.")
	errImportBad   = errors.New("HowtoNote 매뉴얼 파일이 아니거나 망가진 파일입니다. 보낸 사람에게 다시 받아 보세요.")
	errImportNewer = errors.New("더 새 HowtoNote에서 만든 파일입니다. HowtoNote를 새 버전으로 설치한 뒤 가져오세요.")
	errImportRead  = errors.New("파일을 읽지 못했습니다. 파일을 바탕화면에 복사한 뒤 다시 가져와 보세요.")
)

// publishParts는 배포용 그림의 재료다: 원본 좌표 그대로 모자이크만 남긴 그리기 JSON과, 그 그림의 캐시 키.
// 프론트는 ProjectPlan으로 받은 재료를 편집기와 같은 렌더러로 자르기까지 반영해 그린다(H7). Go는 모자이크를 그리지 않는다.
func publishParts(s store.Step) (blurJSON, key string) {
	var all []json.RawMessage
	_ = json.Unmarshal([]byte(s.AnnotationsJSON), &all) // 저장소가 JSON 배열임을 검사해 두었다
	blur := []json.RawMessage{}
	for _, a := range all {
		var t struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(a, &t) == nil && t.Type == "blur" {
			blur = append(blur, a)
		}
	}
	b, _ := json.Marshal(blur)
	return string(b), export.RenderKey(store.Step{ImageSHA: s.ImageSHA, AnnotationsJSON: string(b), ViewJSON: s.ViewJSON})
}

// ProjectPlan은 배포용 .htn에 넣을 그림(모자이크·자르기 확정)의 합성 목록이다. 넣을 스텝은 export.Numbers로 고른다:
// 숨긴 스텝과 숨긴 최상위의 하위 스텝은 뺀다(H3). 같은 키는 한 번만 넣는다.
func (a *App) ProjectPlan(guideID int64) ([]RenderJob, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	steps, err := a.st.ListSteps(guideID)
	if err != nil {
		return nil, errLoad
	}
	out := []RenderJob{}
	seen := map[string]bool{}
	for i, num := range export.Numbers(steps) {
		s := steps[i]
		if num == "" || s.ImageSHA == "" {
			continue
		}
		blur, key := publishParts(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		_, cached := a.st.RenderedFile(key)
		out = append(out, RenderJob{StepID: s.ID, ImageSHA: s.ImageSHA, AnnotationsJSON: blur, ViewJSON: s.ViewJSON, Key: key, Cached: cached})
	}
	return out, nil
}

// cropOf는 view_json의 자르기 상자를 그림(w×h) 안으로 줄인다. frontend/src/editor/annotations.ts clampCrop과 같은 규칙이라
// 프론트가 그린 확정 그림의 원점과 크기가 이 값과 같다. 없거나 깨졌으면 그림 전체다.
func cropOf(viewJSON string, w, h int) image.Rectangle {
	var v struct {
		Crop *struct{ X, Y, W, H float64 } `json:"crop"`
	}
	if json.Unmarshal([]byte(viewJSON), &v) != nil || v.Crop == nil || !(v.Crop.W > 0 && v.Crop.H > 0) {
		return image.Rect(0, 0, w, h)
	}
	x := max(0, min(int(math.Round(v.Crop.X)), w-1))
	y := max(0, min(int(math.Round(v.Crop.Y)), h-1))
	cw := max(1, min(int(math.Round(v.Crop.W)), w-x))
	ch := max(1, min(int(math.Round(v.Crop.H)), h-y))
	return image.Rect(x, y, x+cw, y+ch)
}

// shiftAnnotations는 자르기를 확정한 그림에 맞게 모자이크를 빼고 나머지 그리기를 (-dx, -dy)만큼 옮긴다(H1).
// 그림(w×h) 밖으로 모두 나간 것은 뺀다. 모르는 칸은 그대로 둔다(앞으로 생길 칸을 잃지 않게).
func shiftAnnotations(js string, dx, dy, w, h float64) string {
	var all []map[string]any
	_ = json.Unmarshal([]byte(js), &all)
	out := []map[string]any{}
	for _, a := range all {
		if a["type"] == "blur" {
			continue
		}
		x, _ := a["x"].(float64)
		y, _ := a["y"].(float64)
		aw, _ := a["w"].(float64)
		ah, _ := a["h"].(float64)
		x, y = x-dx, y-dy
		if x+aw <= 0 || y+ah <= 0 || x >= w || y >= h {
			continue
		}
		a["x"], a["y"] = x, y
		if pts, ok := a["points"].([]any); ok {
			for i, p := range pts {
				if v, ok := p.(float64); ok {
					pts[i] = v - [2]float64{dx, dy}[i%2]
				}
			}
		}
		out = append(out, a)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// captureOf는 capture_json에서 dpi_scale만 남긴다(새 그리기 기본 크기에 쓴다). 창 제목·프로세스 이름·화면 요소 이름은
// 녹화한 PC의 정보라 넣지 않는다(H4).
func captureOf(js string) json.RawMessage {
	var c struct {
		DPIScale float64 `json:"dpi_scale"`
	}
	if json.Unmarshal([]byte(js), &c) != nil || c.DPIScale <= 0 {
		return json.RawMessage(`{}`)
	}
	b, _ := json.Marshal(c)
	return b
}

// imageSize는 원본 그림의 크기다(자르기 상자를 그림 안으로 줄일 때 쓴다).
func (a *App) imageSize(sha string) (int, int, error) {
	p, ok := a.st.ImageFile(sha)
	if !ok {
		return 0, 0, errLoad
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, 0, errLoad
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, errLoad
	}
	return cfg.Width, cfg.Height, nil
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// projectGuide는 저장소의 매뉴얼을 .htn 내용으로 바꾸고, 넣을 그림의 sha256 → 파일 경로를 함께 돌려준다.
// 편집용: 지우지 않은 스텝 전부(숨긴 스텝 포함)와 원본 그림. 배포용: 번호가 있는 스텝만(H3)과 ProjectPlan으로 올린 확정 그림.
func (a *App) projectGuide(guideID int64, kind string) (project.Guide, map[string]string, error) {
	g, err := a.st.GetGuide(guideID)
	if err != nil {
		return project.Guide{}, nil, errLoad
	}
	steps, err := a.st.ListSteps(guideID)
	if err != nil {
		return project.Guide{}, nil, errLoad
	}
	publish := kind == project.KindPublish
	nums := export.Numbers(steps)
	files := map[string]string{}
	index := map[int64]int{} // 스텝 ID → 배열 번호(H6)
	out := project.Guide{Title: g.Title, Steps: []project.Step{}}
	for i, s := range steps {
		if publish && nums[i] == "" {
			continue
		}
		ps := project.Step{Parent: -1, Kind: s.Kind, Title: s.Title, Hidden: s.Hidden, Capture: captureOf(s.CaptureJSON),
			Annotations: json.RawMessage(s.AnnotationsJSON), Callouts: json.RawMessage(s.CalloutsJSON), View: json.RawMessage(s.ViewJSON)}
		if s.Description != "" {
			ps.Description = json.RawMessage(s.Description)
		}
		// 상위가 파일에 있고 최상위일 때만 하위로 둔다(하위 스텝은 한 단계, project.Read와 같은 규칙)
		if p, ok := index[s.ParentID]; ok && s.ParentID != 0 && out.Steps[p].Parent == -1 {
			ps.Parent = p
		}
		switch {
		case s.ImageSHA == "":
		case !publish:
			p, ok := a.st.ImageFile(s.ImageSHA)
			// ImageFile은 sha 모양만 본다. 원본이 실제로 없으면 project.Write 안에서야 알게 되고 그러면 ExportProject가
			// 폴더 쓰기 오류로 잘못 알리니, 여기서 먼저 있는지 보고 맞는 문장(errLoad)으로 돌려준다
			if _, err := os.Stat(p); !ok || err != nil {
				return project.Guide{}, nil, errLoad
			}
			ps.Image, files[s.ImageSHA] = s.ImageSHA, p
		default:
			_, key := publishParts(s)
			p, ok := a.st.RenderedFile(key)
			if !ok {
				return project.Guide{}, nil, errNotRendered
			}
			sha, err := fileSHA(p)
			if err != nil {
				return project.Guide{}, nil, errNotRendered
			}
			w, h, err := a.imageSize(s.ImageSHA)
			if err != nil {
				return project.Guide{}, nil, err
			}
			c := cropOf(s.ViewJSON, w, h)
			ps.Annotations = json.RawMessage(shiftAnnotations(s.AnnotationsJSON, float64(c.Min.X), float64(c.Min.Y), float64(c.Dx()), float64(c.Dy())))
			ps.View = json.RawMessage(`{}`)
			ps.Image, files[sha] = sha, p
		}
		index[s.ID] = len(out.Steps)
		out.Steps = append(out.Steps, ps)
	}
	return out, files, nil
}

// ExportProject는 매뉴얼을 .htn으로 path에 쓴다. 배포용 그림은 프론트가 먼저 준비한다(ensureRendered에 ProjectPlan).
func (a *App) ExportProject(guideID int64, kind, path string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if kind != project.KindEdit && kind != project.KindPublish {
		return errKind
	}
	// ChooseExportFile이 막 돌려준 경로만 받는다(ExportGuide와 같은 1회용 표)
	a.mu.Lock()
	chosen := a.chosenExport
	a.chosenExport = ""
	a.mu.Unlock()
	if path == "" || path != chosen || !filepath.IsAbs(path) {
		return errExportWrite
	}
	g, files, err := a.projectGuide(guideID, kind)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return errExportWrite
	}
	m := project.Manifest{Format: project.Format, Version: project.Version, AppVersion: appVersion, Kind: kind}
	err = project.Write(f, m, g, func(sha string) ([]byte, error) { return os.ReadFile(files[sha]) })
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path) // 반쪽 .htn을 남기지 않는다
		return errExportWrite
	}
	a.mu.Lock()
	a.lastExport = path
	a.mu.Unlock()
	return nil
}

// ImportProject는 열기 창에서 .htn을 골라 folderID 폴더(첫 화면에서 보고 있는 폴더, H5)에 새 매뉴얼로 가져온다.
// 새 매뉴얼 ID를 돌려주고, 취소하면 0이다.
func (a *App) ImportProject(folderID int64) (int64, error) {
	if err := a.waitReady(); err != nil {
		return 0, err
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return 0, errNoWindow
	}
	path, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:   "가져올 HowtoNote 파일",
		Filters: []runtime.FileFilter{{DisplayName: "HowtoNote 파일 (*.htn)", Pattern: "*.htn"}},
	})
	switch {
	case err != nil:
		return 0, errImportRead
	case path == "":
		return 0, nil
	}
	return a.importFile(path, folderID)
}

// checkPNG는 가져온 그림이 PNG이고 원본 그림 한계(maxSide, maxPixels) 안인지 본다.
// ponytail: 머리(IHDR)만 본다(H9). 뒤가 깨진 PNG는 편집기가 "그림을 불러오지 못했습니다"로 알린다. 신고가 오면 image.Decode로 끝까지 푼다
func checkPNG(b []byte) (int, int, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != "png" || cfg.Width < 1 || cfg.Height < 1 ||
		cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return 0, 0, errImportBad
	}
	return cfg.Width, cfg.Height, nil
}

// importFile은 .htn을 읽고 검사한 뒤 그림과 새 매뉴얼을 넣는다. 그림 넣기(PutImage)와 스텝 넣기 사이에 그림 정리(prune)가
// 끼면 새 그림이 고아로 지워지므로, 넣는 동안 정리와 같은 a.mu를 잡는다(AddImageStep과 같은 이유).
func (a *App) importFile(path string, folderID int64) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, errImportRead
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, errImportRead
	}
	p, err := project.Read(f, fi.Size())
	switch {
	case errors.Is(err, project.ErrNewer):
		return 0, errImportNewer
	case err != nil:
		return 0, errImportBad
	}
	title, err := cleanName(p.Guide.Title)
	if err != nil {
		title = "가져온 매뉴얼"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := map[string]int64{}
	steps := make([]store.ImportStep, len(p.Guide.Steps))
	for i, s := range p.Guide.Steps {
		if s.Image != "" && ids[s.Image] == 0 {
			b, err := p.Image(s.Image)
			if err != nil {
				return 0, errImportBad
			}
			w, h, err := checkPNG(b)
			if err != nil {
				return 0, err
			}
			id, _, err := a.st.PutImage(b, w, h)
			if err != nil {
				return 0, saved(err)
			}
			ids[s.Image] = id
		}
		steps[i] = store.ImportStep{Parent: s.Parent, Kind: s.Kind, Title: s.Title, Description: string(s.Description),
			ImageID: ids[s.Image], Hidden: s.Hidden, CaptureJSON: string(s.Capture), AnnotationsJSON: string(s.Annotations),
			CalloutsJSON: string(s.Callouts), ViewJSON: string(s.View)}
	}
	gid, err := a.st.ImportGuide(folderID, title, steps)
	if err != nil {
		// project.Read가 같은 규칙을 먼저 검사해 ImportGuide의 검증 오류(fmt.Errorf)는 지금 거쳐 가지 않는다.
		// 저장소 오류(sqlite.Error)만 saved()의 디스크 공간 문장으로 보내고, 둘이 갈라지면 나머지는 파일이 망가졌다는 문장으로 보낸다
		var se *sqlite.Error
		if errors.As(err, &se) {
			return 0, saved(err)
		}
		return 0, errImportBad
	}
	return gid, nil
}
