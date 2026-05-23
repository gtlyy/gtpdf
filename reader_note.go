package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type PDFNote struct {
	ID        string  `json:"id"`
	Page      int     `json:"page"`
	PdfX      float64 `json:"pdf_x"`
	PdfY      float64 `json:"pdf_y"`
	Text      string  `json:"text"`
	Color     string  `json:"color"`
	Type      string  `json:"type"`
	CreatedAt int64   `json:"created_at"`
}

type NoteStore struct {
	mu       sync.RWMutex
	notes    []PDFNote
	filePath string
}

func NewNoteStore() *NoteStore {
	return &NoteStore{}
}

func (s *NoteStore) Load(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.filePath = filePath
	s.notes = nil

	sidecarPath := filePath + ".gtpdf.json"
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.notes)
}

func (s *NoteStore) Save() error {
	s.mu.RLock()
	notesCopy := make([]PDFNote, len(s.notes))
	copy(notesCopy, s.notes)
	s.mu.RUnlock()

	if s.filePath == "" {
		return nil
	}

	data, err := json.MarshalIndent(notesCopy, "", "  ")
	if err != nil {
		return err
	}

	sidecarPath := s.filePath + ".gtpdf.json"
	return os.WriteFile(sidecarPath, data, 0644)
}

func (s *NoteStore) Add(note PDFNote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, note)
}

func (s *NoteStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.notes {
		if n.ID == id {
			s.notes = append(s.notes[:i], s.notes[i+1:]...)
			return
		}
	}
}

func (s *NoteStore) Update(id string, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.notes {
		if s.notes[i].ID == id {
			s.notes[i].Text = text
			return
		}
	}
}

func (s *NoteStore) GetByPage(page int) []PDFNote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []PDFNote
	for _, n := range s.notes {
		if n.Page == page {
			result = append(result, n)
		}
	}
	return result
}

func (s *NoteStore) GetAll() []PDFNote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PDFNote, len(s.notes))
	copy(result, s.notes)
	return result
}

func (s *NoteStore) SetFilePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filePath = path
}

type NoteMarker struct {
	widget.BaseWidget
	note    PDFNote
	viewer  *PDFViewerPlus
	pos     fyne.Position
	marker  *canvas.Circle
	icon    *canvas.Text
	bg      *canvas.Rectangle
	objects []fyne.CanvasObject
}

func NewNoteMarker(note PDFNote, viewer *PDFViewerPlus) *NoteMarker {
	m := &NoteMarker{
		note:   note,
		viewer: viewer,
	}
	m.ExtendBaseWidget(m)
	return m
}

func (m *NoteMarker) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.NRGBA{R: 255, G: 235, B: 59, A: 220})
	bg.SetMinSize(fyne.NewSize(24, 24))

	icon := canvas.NewText("📝", nil)
	icon.TextStyle = fyne.TextStyle{Bold: true}
	icon.Alignment = fyne.TextAlignCenter

	m.bg = bg
	m.icon = icon
	m.objects = []fyne.CanvasObject{bg, icon}

	return &noteMarkerRenderer{marker: m}
}

type noteMarkerRenderer struct {
	marker  *NoteMarker
	objects []fyne.CanvasObject
}

func (r *noteMarkerRenderer) Layout(size fyne.Size) {
	r.marker.bg.Resize(size)
	r.marker.icon.Resize(size)
	r.marker.icon.Move(fyne.NewPos(0, 0))
}

func (r *noteMarkerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(22, 22)
}

func (r *noteMarkerRenderer) Refresh() {
	r.marker.bg.Refresh()
	r.marker.icon.Refresh()
}

func (r *noteMarkerRenderer) Destroy() {}

func (r *noteMarkerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.marker.bg, r.marker.icon}
}

type NoteLayer struct {
	widget.BaseWidget
	viewer     *PDFViewerPlus
	noteMode   bool
	pageWidth  float64
	pageHeight float64
	imgOffX    float32
	imgOffY    float32
	imgW       float32
	imgH       float32
}

type NoteHoverLayer struct {
	widget.BaseWidget
	viewer        *PDFViewerPlus
	pageWidth     float64
	pageHeight    float64
	imgOffX       float32
	imgOffY       float32
	imgW          float32
	imgH          float32
	hoveredNoteID string
	hoverMousePos fyne.Position
}

func NewNoteLayer(viewer *PDFViewerPlus) *NoteLayer {
	layer := &NoteLayer{
		viewer: viewer,
	}
	layer.ExtendBaseWidget(layer)
	return layer
}

func (nl *NoteLayer) SetNoteMode(enabled bool) {
	nl.noteMode = enabled
}

func (nl *NoteLayer) calcImageRect() {
	nl.imgOffX, nl.imgOffY, nl.imgW, nl.imgH = 0, 0, 0, 0
	if nl.viewer == nil || nl.viewer.pdfDoc == nil {
		return
	}
	nl.imgOffX, nl.imgOffY, nl.imgW, nl.imgH = calcImageRectCommon(nl.Size(), nl.pageWidth, nl.pageHeight, nl.viewer.zoom)
}

func (nl *NoteLayer) pdfToScreen(pdfX, pdfY float64) (float32, float32) {
	zoom := float64(nl.viewer.zoom)
	sx := nl.imgOffX + float32(pdfX*zoom)
	sy := nl.imgOffY + float32(pdfY*zoom)
	return sx, sy
}

func (nl *NoteLayer) screenToPdf(sx, sy float32) (float64, float64) {
	if nl.pageWidth <= 0 || nl.pageHeight <= 0 || nl.imgW <= 0 || nl.imgH <= 0 {
		return float64(sx) / float64(nl.viewer.zoom), float64(sy) / float64(nl.viewer.zoom)
	}
	ix := float64(sx - nl.imgOffX)
	iy := float64(sy - nl.imgOffY)
	pdfX := ix / float64(nl.imgW) * nl.pageWidth
	pdfY := iy / float64(nl.imgH) * nl.pageHeight
	return pdfX, pdfY
}

func (nl *NoteLayer) CreateRenderer() fyne.WidgetRenderer {
	return &noteLayerRenderer{layer: nl}
}

func (nl *NoteLayer) MouseDown(ev *desktop.MouseEvent) {
	if !nl.noteMode || nl.viewer == nil || nl.viewer.pdfDoc == nil {
		return
	}

	nl.calcImageRect()
	if nl.imgW <= 0 || nl.imgH <= 0 {
		return
	}

	pdfX, pdfY := nl.screenToPdf(ev.Position.X, ev.Position.Y)

	if pdfX < 0 || pdfX > nl.pageWidth || pdfY < 0 || pdfY > nl.pageHeight {
		return
	}

	notes := nl.viewer.noteStore.GetByPage(nl.viewer.currentPage - 1)
	for _, n := range notes {
		nScreenX, nScreenY := nl.pdfToScreen(n.PdfX, n.PdfY)
		hitRadius := float32(15)
		if ev.Position.X > nScreenX-hitRadius && ev.Position.X < nScreenX+hitRadius &&
			ev.Position.Y > nScreenY-hitRadius && ev.Position.Y < nScreenY+hitRadius {
			nl.showViewNoteDialog(n)
			return
		}
	}

	nl.showAddNoteDialog(pdfX, pdfY)
}

func (nl *NoteLayer) MouseUp(ev *desktop.MouseEvent) {}

func NewNoteHoverLayer(viewer *PDFViewerPlus) *NoteHoverLayer {
	layer := &NoteHoverLayer{viewer: viewer}
	layer.ExtendBaseWidget(layer)
	return layer
}

func (hl *NoteHoverLayer) calcImageRect() {
	hl.imgOffX, hl.imgOffY, hl.imgW, hl.imgH = 0, 0, 0, 0
	if hl.viewer == nil || hl.viewer.pdfDoc == nil {
		return
	}
	hl.imgOffX, hl.imgOffY, hl.imgW, hl.imgH = calcImageRectCommon(hl.Size(), hl.pageWidth, hl.pageHeight, hl.viewer.zoom)
}

func (hl *NoteHoverLayer) pdfToScreen(pdfX, pdfY float64) (float32, float32) {
	zoom := float64(hl.viewer.zoom)
	sx := hl.imgOffX + float32(pdfX*zoom)
	sy := hl.imgOffY + float32(pdfY*zoom)
	return sx, sy
}

func (hl *NoteHoverLayer) CreateRenderer() fyne.WidgetRenderer {
	return &noteHoverLayerRenderer{layer: hl}
}

func (hl *NoteHoverLayer) MouseIn(ev *desktop.MouseEvent) {}

func (hl *NoteHoverLayer) ClearHover() {
	hl.hoveredNoteID = ""
	hl.Refresh()
	hl.viewer.parentWin.Canvas().Refresh(hl)
}

func (hl *NoteHoverLayer) MouseOut() {
	hl.ClearHover()
}

func (hl *NoteHoverLayer) MouseMoved(ev *desktop.MouseEvent) {
	if ev.Button != 0 && hl.viewer != nil && hl.viewer.textLayer.selectionEnabled {
		hl.viewer.textLayer.MouseMoved(ev)
	}
	if ev.Button != 0 {
		return
	}

	if !hl.viewer.noteMode {
		hl.ClearHover()
		return
	}

	if hl.viewer == nil || hl.viewer.pdfDoc == nil {
		return
	}

	hl.calcImageRect()
	if hl.imgW <= 0 || hl.imgH <= 0 {
		return
	}

	notes := hl.viewer.noteStore.GetByPage(hl.viewer.currentPage - 1)

	foundID := ""
	for _, n := range notes {
		nScreenX, nScreenY := hl.pdfToScreen(n.PdfX, n.PdfY)
		hitRadius := float32(15)
		if ev.Position.X > nScreenX-hitRadius && ev.Position.X < nScreenX+hitRadius &&
			ev.Position.Y > nScreenY-hitRadius && ev.Position.Y < nScreenY+hitRadius {
			foundID = n.ID
			break
		}
	}

	if foundID != hl.hoveredNoteID {
		hl.hoveredNoteID = foundID
		hl.hoverMousePos = ev.Position
		hl.Refresh()
		hl.viewer.parentWin.Canvas().Refresh(hl)
	}
}

type noteHoverLayerRenderer struct {
	layer   *NoteHoverLayer
	objects []fyne.CanvasObject
}

func (r *noteHoverLayerRenderer) Layout(size fyne.Size) {}

func (r *noteHoverLayerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}

func (r *noteHoverLayerRenderer) Refresh() {
	r.objects = nil

	if r.layer.viewer == nil || r.layer.viewer.pdfDoc == nil {
		return
	}

	r.layer.calcImageRect()
	if r.layer.imgW <= 0 || r.layer.imgH <= 0 {
		return
	}

	if r.layer.hoveredNoteID == "" {
		return
	}

	notes := r.layer.viewer.noteStore.GetByPage(r.layer.viewer.currentPage - 1)

	for _, n := range notes {
		if n.ID == r.layer.hoveredNoteID {
			preview := n.Text
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}

			tipX := r.layer.hoverMousePos.X + 16
			tipY := r.layer.hoverMousePos.Y + 10

			tooltipText := canvas.NewText(preview, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
			tooltipText.TextSize = 12
			textSize := tooltipText.MinSize()

			bgW := float32(280)
			if textSize.Width < 40 {
				bgW = textSize.Width + 24
			}

			bgH := textSize.Height + 12
			if bgH < 28 {
				bgH = 28
			}

			if tipX+bgW > r.layer.Size().Width {
				tipX = r.layer.Size().Width - bgW - 4
			}
			if tipY+bgH > r.layer.Size().Height {
				tipY = r.layer.hoverMousePos.Y - bgH - 6
			}
			if tipX < 2 {
				tipX = 2
			}
			if tipY < 2 {
				tipY = 2
			}

			bg := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 225, A: 245})
			bg.StrokeColor = color.NRGBA{R: 180, G: 180, B: 120, A: 255}
			bg.StrokeWidth = 1
			bg.CornerRadius = 4
			bg.Move(fyne.NewPos(tipX, tipY))
			bg.Resize(fyne.NewSize(bgW, bgH))
			r.objects = append(r.objects, bg)

			tooltipText.Move(fyne.NewPos(tipX+8, tipY+6))
			tooltipText.Resize(fyne.NewSize(bgW-16, bgH-12))
			r.objects = append(r.objects, tooltipText)

			break
		}
	}
}

func (r *noteHoverLayerRenderer) Destroy() {}

func (r *noteHoverLayerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (nl *NoteLayer) showAddNoteDialog(pdfX, pdfY float64) {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("输入笔记内容...")
	entry.Wrapping = fyne.TextWrapWord

	entryScroll := container.NewVScroll(entry)
	entryScroll.SetMinSize(fyne.NewSize(480, 120))

	colors := []string{"#FFD600", "#FF5722", "#4CAF50", "#2196F3", "#9C27B0", "#FF9800"}
	colorNames := []string{"黄色", "红色", "绿色", "蓝色", "紫色", "橙色"}
	selectedColor := colors[0]

	var colorBtns []*widget.Button
	colorRow := container.NewHBox()
	for i, c := range colors {
		c := c
		btn := widget.NewButton(colorNames[i], nil)
		if i == 0 {
			btn.Importance = widget.HighImportance
		}
		colorBtns = append(colorBtns, btn)
		colorRow.Add(btn)
		btn.OnTapped = func() {
			selectedColor = c
			for j, b := range colorBtns {
				if colors[j] == c {
					b.Importance = widget.HighImportance
				} else {
					b.Importance = widget.MediumImportance
				}
				b.Refresh()
			}
		}
	}

	addBtn := widget.NewButton("添加", nil)
	cancelBtn := widget.NewButton("取消", nil)

	content := container.NewVBox(
		// widget.NewLabelWithStyle("添加笔记", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("颜色:"),
		colorRow,
		widget.NewLabel("内容:"),
		entryScroll,
		widget.NewSeparator(),
		container.NewHBox(addBtn, cancelBtn),
	)

	dlg := dialog.NewCustom("添加笔记", "", content, nl.viewer.parentWin)
	dlg.Resize(fyne.NewSize(560, 400))

	addBtn.OnTapped = func() {
		if entry.Text == "" {
			return
		}

		note := PDFNote{
			ID:        fmt.Sprintf("note_%d", time.Now().UnixNano()),
			Page:      nl.viewer.currentPage - 1,
			PdfX:      pdfX,
			PdfY:      pdfY,
			Text:      entry.Text,
			Color:     selectedColor,
			Type:      "note",
			CreatedAt: time.Now().Unix(),
		}

		nl.viewer.noteStore.Add(note)
		if err := nl.viewer.noteStore.Save(); err != nil {
			dialog.ShowError(err, nl.viewer.parentWin)
			return
		}

		nl.viewer.forceRenderPlus()
		nl.viewer.renderCurrentPagePlus()
		dlg.Hide()
	}
	cancelBtn.OnTapped = func() {
		dlg.Hide()
	}

	dlg.Show()
}

func (nl *NoteLayer) showViewNoteDialog(note PDFNote) {
	entry := widget.NewMultiLineEntry()
	entry.SetText(note.Text)
	entry.Wrapping = fyne.TextWrapWord

	entryScroll := container.NewVScroll(entry)
	entryScroll.SetMinSize(fyne.NewSize(480, 120))

	deleteBtn := widget.NewButton("删除笔记", nil)
	saveBtn := widget.NewButton("保存", nil)
	closeBtn := widget.NewButton("关闭", nil)

	content := container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("笔记 (第 %d 页)", note.Page+1), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(fmt.Sprintf("位置: (%.0f, %.0f)", note.PdfX, note.PdfY)),
		widget.NewLabel(fmt.Sprintf("时间: %s", time.Unix(note.CreatedAt, 0).Format("2006-01-02 15:04:05"))),
		widget.NewSeparator(),
		widget.NewLabel("内容:"),
		entryScroll,
		widget.NewSeparator(),
		container.NewHBox(saveBtn, closeBtn, deleteBtn),
	)

	dlg := dialog.NewCustom("查看笔记", "", content, nl.viewer.parentWin)
	dlg.Resize(fyne.NewSize(560, 400))

	deleteBtn.OnTapped = func() {
		dialog.ShowConfirm("确认", "确定删除此笔记？", func(ok bool) {
			if ok {
				nl.viewer.noteStore.Remove(note.ID)
				if err := nl.viewer.noteStore.Save(); err != nil {
					dialog.ShowError(err, nl.viewer.parentWin)
					return
				}
				nl.viewer.forceRenderPlus()
				nl.viewer.renderCurrentPagePlus()
				dlg.Hide()
			}
		}, nl.viewer.parentWin)
	}

	saveBtn.OnTapped = func() {
		nl.viewer.noteStore.Update(note.ID, entry.Text)
		if err := nl.viewer.noteStore.Save(); err != nil {
			dialog.ShowError(err, nl.viewer.parentWin)
		}
		dlg.Hide()
	}
	closeBtn.OnTapped = func() {
		dlg.Hide()
	}

	dlg.Show()
}

type noteLayerRenderer struct {
	layer   *NoteLayer
	objects []fyne.CanvasObject
}

func (r *noteLayerRenderer) Layout(size fyne.Size) {}

func (r *noteLayerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}

func (r *noteLayerRenderer) Refresh() {
	r.objects = nil

	if r.layer.viewer == nil || r.layer.viewer.pdfDoc == nil {
		return
	}

	r.layer.calcImageRect()
	if r.layer.imgW <= 0 || r.layer.imgH <= 0 {
		return
	}

	notes := r.layer.viewer.noteStore.GetByPage(r.layer.viewer.currentPage - 1)

	for _, n := range notes {
		sx, sy := r.layer.pdfToScreen(n.PdfX, n.PdfY)

		noteColor := parseNoteColor(n.Color)

		circle := canvas.NewCircle(noteColor)
		circle.Move(fyne.NewPos(sx-8, sy-8))
		circle.Resize(fyne.NewSize(18, 18))
		circle.StrokeColor = color.NRGBA{R: 0, G: 0, B: 0, A: 200}
		circle.StrokeWidth = 1.5
		r.objects = append(r.objects, circle)

		label := canvas.NewText("📝", color.Black)
		label.TextSize = 12
		label.Move(fyne.NewPos(sx-8, sy-8))
		label.Resize(fyne.NewSize(18, 18))
		r.objects = append(r.objects, label)
	}
}

func (r *noteLayerRenderer) Destroy() {}

func (r *noteLayerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func parseNoteColor(colorStr string) color.Color {
	switch colorStr {
	case "#FFD600":
		return color.NRGBA{R: 255, G: 214, B: 0, A: 200}
	case "#FF5722":
		return color.NRGBA{R: 255, G: 87, B: 34, A: 200}
	case "#4CAF50":
		return color.NRGBA{R: 76, G: 175, B: 80, A: 200}
	case "#2196F3":
		return color.NRGBA{R: 33, G: 150, B: 243, A: 200}
	case "#9C27B0":
		return color.NRGBA{R: 156, G: 39, B: 176, A: 200}
	case "#FF9800":
		return color.NRGBA{R: 255, G: 152, B: 0, A: 200}
	default:
		return color.NRGBA{R: 255, G: 214, B: 0, A: 200}
	}
}

func loadNotesForFile(filePath string) *NoteStore {
	store := NewNoteStore()
	store.SetFilePath(filePath)
	if err := store.Load(filePath); err != nil {
		// no notes file yet, that's fine
	}
	return store
}

func (v *PDFViewerPlus) ShowNoteList() {
	if v.pdfDoc == nil {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	allNotes := v.noteStore.GetAll()

	if len(allNotes) == 0 {
		dialog.ShowInformation("笔记列表", "暂无笔记\n\n提示：点击工具栏「笔记」按钮开启笔记模式，然后在页面上点击即可添加笔记。", v.parentWin)
		return
	}

	listData := allNotes

	var listAllDlg *dialog.CustomDialog

	var noteList *widget.List

	noteList = widget.NewList(
		func() int {
			return len(listData)
		},
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, nil,
				widget.NewButton("删除", nil),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(listData) {
				return
			}
			n := listData[id]
			preview := n.Text
			if len(preview) > 40 {
				preview = preview[:40] + "..."
			}
			border := obj.(*fyne.Container)
			label := border.Objects[0].(*widget.Label)
			label.SetText(fmt.Sprintf("第%d页: %s", n.Page+1, preview))
			delBtn := border.Objects[1].(*widget.Button)
			delID := n.ID
			delBtn.OnTapped = func() {
				dialog.ShowConfirm("确认", "确定删除此条笔记？", func(ok bool) {
					if ok {
						v.noteStore.Remove(delID)
						v.noteStore.Save()
						newData := v.noteStore.GetAll()
						listData = newData
						if len(listData) == 0 {
							listAllDlg.Hide()
							v.forceRenderPlus()
							v.renderCurrentPagePlus()
							return
						}
						noteList.Refresh()
						v.forceRenderPlus()
						v.renderCurrentPagePlus()
					}
				}, v.parentWin)
			}
		},
	)

	noteList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(listData) {
			return
		}
		n := listData[id]
		v.GoToPagePlus(n.Page + 1)
	}

	scrollList := container.NewVScroll(noteList)
	scrollList.SetMinSize(fyne.NewSize(480, 300))

		deleteAllBtn := widget.NewButton("删除全部笔记", func() {
		dialog.ShowConfirm("确认", "确定删除全部笔记？此操作不可撤销。", func(ok bool) {
			if ok {
				v.noteStore = NewNoteStore()
				v.noteStore.SetFilePath(v.filePath)
				v.noteStore.Save()
				v.forceRenderPlus()
				v.renderCurrentPagePlus()
				listAllDlg.Hide()
			}
		}, v.parentWin)
	})

	content := container.NewBorder(nil, deleteAllBtn, nil, nil, scrollList)

	listAllDlg = dialog.NewCustom("笔记列表", "关闭", content, v.parentWin)
	listAllDlg.Resize(fyne.NewSize(560, 450))
	listAllDlg.Show()
}

func (v *PDFViewerPlus) ExportNotes() {
	if v.pdfDoc == nil || v.filePath == "" {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	allNotes := v.noteStore.GetAll()
	if len(allNotes) == 0 {
		dialog.ShowInformation("提示", "暂无笔记可导出", v.parentWin)
		return
	}

	saveDlg := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
		if err != nil || uri == nil {
			return
		}
		defer uri.Close()

		var lines []string
		for _, n := range allNotes {
			lines = append(lines, fmt.Sprintf("第%d页 (%.0f,%.0f): %s", n.Page+1, n.PdfX, n.PdfY, n.Text))
		}

		for _, line := range lines {
			fmt.Fprintln(uri, line)
		}
	}, v.parentWin)
	saveDlg.Resize(fyne.NewSize(800, 600))
	saveDlg.Show()
}
