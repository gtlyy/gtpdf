package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"gtpdf/pdfium_plus"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type pageEntryWidgetPlus struct {
	widget.BaseWidget
	entry *widget.Entry
}

func newPageEntryWidgetPlus(entry *widget.Entry) *pageEntryWidgetPlus {
	w := &pageEntryWidgetPlus{entry: entry}
	w.ExtendBaseWidget(w)
	return w
}

func (w *pageEntryWidgetPlus) CreateRenderer() fyne.WidgetRenderer {
	return &pageEntryRendererPlus{widget: w, entry: w.entry}
}

type pageEntryRendererPlus struct {
	widget *pageEntryWidgetPlus
	entry  *widget.Entry
}

func (r *pageEntryRendererPlus) Layout(size fyne.Size) {
	r.entry.Resize(size)
	r.entry.Move(fyne.NewPos(0, 0))
}

func (r *pageEntryRendererPlus) MinSize() fyne.Size {
	return fyne.NewSize(50, 36)
}

func (r *pageEntryRendererPlus) Refresh() {
	r.entry.Refresh()
}

func (r *pageEntryRendererPlus) Destroy() {}

func (r *pageEntryRendererPlus) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.entry}
}

type zoomEntryWidgetPlus struct {
	widget.BaseWidget
	entry *widget.SelectEntry
}

func newZoomEntryWidgetPlus(entry *widget.SelectEntry) *zoomEntryWidgetPlus {
	w := &zoomEntryWidgetPlus{entry: entry}
	w.ExtendBaseWidget(w)
	return w
}

func (w *zoomEntryWidgetPlus) CreateRenderer() fyne.WidgetRenderer {
	return &zoomEntryRendererPlus{widget: w, entry: w.entry}
}

type zoomEntryRendererPlus struct {
	widget *zoomEntryWidgetPlus
	entry  *widget.SelectEntry
}

func (r *zoomEntryRendererPlus) Layout(size fyne.Size) {
	r.entry.Resize(size)
	r.entry.Move(fyne.NewPos(0, 0))
}

func (r *zoomEntryRendererPlus) MinSize() fyne.Size {
	return fyne.NewSize(88, 36)
}

func (r *zoomEntryRendererPlus) Refresh() {
	r.entry.Refresh()
}

func (r *zoomEntryRendererPlus) Destroy() {}

func (r *zoomEntryRendererPlus) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.entry}
}

type PDFViewerPlus struct {
	parentWin        fyne.Window
	pdfDoc           *PDFiumDoc
	contentScroll    *container.Scroll
	thumbList        *widget.List
	bookmarkTree     *widget.Tree
	bookmarkData     map[string]pdfium_plus.BookmarkItem
	bookmarkTreeIDs  map[string][]widget.TreeNodeID
	sideTabs         *container.AppTabs
	toolbarContainer *fyne.Container
	contentArea      *fyne.Container
	mainArea         *fyne.Container
	openBtn          *widget.Button
	fitWidthBtn      *widget.Button
	fitPageBtn       *widget.Button
	pageEntry        *widget.Entry
	zoomEntry        *widget.SelectEntry
	zoomUpdating     bool
	pageTotalLabel   *widget.Label
	currentPage      int
	prevPage         int
	totalPages       int
	zoom             float32
	nightMode        bool
	thumbVisible     bool
	fitWidthMode     bool
	filePath         string
	originalFilePath string
	textLayer        *TextSelectionLayer
	searchResults    []pdfium_plus.SearchResult
	searchQuery      string
	searchResult     *pdfium_plus.SearchResult
	searchResultPosition int
	currentResult    int
	searchPanel      *fyne.Container
	searchEntry      *widget.Entry
	searchLabel      *widget.Label
	searchHighlight  *SearchResultHighlightPlus
	resizeTimer *time.Timer
	selectBtn        *widget.Button
	noteStore      *NoteStore
	noteLayer      *NoteLayer
	noteHoverLayer *NoteHoverLayer
	annotLayer      *AnnotLayer
	annotToolLayer  *AnnotToolLayer
	annotSettings   map[AnnotTool]AnnotSettings
	noteMode       bool
	annotMode      bool
	noteBtn        *widget.Button
	annotToggleBtn *widget.Button
	annotBottomBar *fyne.Container
	lastRenderPage int
	lastRenderZoom float32
	renderVer      int32
	noteHintShown  bool
	canvasImg      *canvas.Image
	thumbCache     map[int]image.Image
	thumbMu        sync.Mutex
	lastPageWidth  float64
	lastPageHeight float64
	dirty          bool
	scrollOverlay  *scrollOverlay
	contextOverlay *contextOverlay
	panEnabled     bool
	scrollDebt     float32
	scrollToBottom bool
	resizeWatcher  *resizeWatcher
	contentSplit   *container.Split
}

type scrollOverlay struct {
	widget.BaseWidget
	viewer *PDFViewerPlus
}

func (s *scrollOverlay) Scrolled(ev *fyne.ScrollEvent) {
	v := s.viewer
	if v.pdfDoc == nil || v.contentScroll == nil {
		return
	}
	scroll := v.contentScroll
	size := scroll.Size()
	contentSize := scroll.Content.Size()

	atTop := scroll.Offset.Y <= 1
	atBottom := scroll.Offset.Y+size.Height >= contentSize.Height-1

	const overscrollThreshold float32 = 80

	// Page fits: need to accumulate "overscroll" before flipping
	if contentSize.Height <= size.Height {
		if ev.Scrolled.DY < 0 && v.currentPage < v.totalPages {
			v.scrollDebt -= ev.Scrolled.DY
			if v.scrollDebt > overscrollThreshold {
				v.scrollDebt = 0
				v.scrollToBottom = false
				scroll.Offset = fyne.NewPos(0, 0)
				v.GoToPagePlus(v.currentPage + 1)
			}
		} else if ev.Scrolled.DY > 0 && v.currentPage > 1 {
			v.scrollDebt += ev.Scrolled.DY
			if v.scrollDebt > overscrollThreshold {
				v.scrollDebt = 0
				v.scrollToBottom = true
				v.GoToPagePlus(v.currentPage - 1)
			}
		} else {
			v.scrollDebt = 0
		}
		return
	}

	// Page taller than viewport
	if atTop {
		if ev.Scrolled.DY > 0 && v.currentPage > 1 {
			v.scrollDebt += ev.Scrolled.DY
			if v.scrollDebt > overscrollThreshold {
				v.scrollDebt = 0
				v.scrollToBottom = true
				v.GoToPagePlus(v.currentPage - 1)
			}
			return
		}
		v.scrollDebt = 0
	}
	if atBottom {
		if ev.Scrolled.DY < 0 && v.currentPage < v.totalPages {
			v.scrollDebt -= ev.Scrolled.DY
			if v.scrollDebt > overscrollThreshold {
				v.scrollDebt = 0
				v.scrollToBottom = false
				scroll.Offset = fyne.NewPos(0, 0)
				v.GoToPagePlus(v.currentPage + 1)
			}
			return
		}
		v.scrollDebt = 0
	}
	v.scrollDebt = 0

	newY := scroll.Offset.Y - ev.Scrolled.DY
	if newY+size.Height >= contentSize.Height {
		newY = contentSize.Height - size.Height
	}
	if newY < 0 {
		newY = 0
	}
	scroll.Offset = fyne.NewPos(scroll.Offset.X, newY)
	scroll.Refresh()
}

func (s *scrollOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &scrollOverlayRenderer{}
}

type scrollOverlayRenderer struct{}

func (r *scrollOverlayRenderer) Layout(size fyne.Size)               {}
func (r *scrollOverlayRenderer) MinSize() fyne.Size                   { return fyne.NewSize(1, 1) }
func (r *scrollOverlayRenderer) Refresh()                             {}
func (r *scrollOverlayRenderer) Objects() []fyne.CanvasObject         { return nil }
func (r *scrollOverlayRenderer) Destroy()                             {}

type contextOverlay struct {
	widget.BaseWidget
	viewer      *PDFViewerPlus
	panDragging bool
	panStart    fyne.Position
	panTarget   fyne.Position
	panTicker   *time.Ticker
	panStop     chan struct{}
}

func (c *contextOverlay) startPanTicker() {
	c.stopPanTicker()
	c.panTicker = time.NewTicker(time.Millisecond * 16)
	c.panStop = make(chan struct{})
	go func() {
		for {
			select {
			case <-c.panTicker.C:
				fyne.Do(func() {
					if c.panDragging {
						c.viewer.contentScroll.Offset = c.panTarget
						c.viewer.contentScroll.Refresh()
					}
				})
			case <-c.panStop:
				return
			}
		}
	}()
}

func (c *contextOverlay) stopPanTicker() {
	if c.panTicker != nil {
		c.panTicker.Stop()
		c.panTicker = nil
	}
	if c.panStop != nil {
		close(c.panStop)
		c.panStop = nil
	}
}

func (c *contextOverlay) MouseDown(ev *desktop.MouseEvent) {
	if (c.viewer.panEnabled && ev.Button == desktop.MouseButtonPrimary) || ev.Button == desktop.MouseButtonTertiary {
		c.panDragging = true
		c.panStart = ev.AbsolutePosition
		c.panTarget = c.viewer.contentScroll.Offset
		c.startPanTicker()
		return
	}
	if ev.Button == desktop.MouseButtonSecondary {
		return
	}
	c.viewer.textLayer.MouseDown(ev)
	c.viewer.annotToolLayer.MouseDown(ev)
	c.viewer.noteLayer.MouseDown(ev)
}

func (c *contextOverlay) MouseUp(ev *desktop.MouseEvent) {
	if c.panDragging {
		c.panDragging = false
		c.stopPanTicker()
		// 确保最后一次偏移被应用
		c.viewer.contentScroll.Offset = c.panTarget
		c.viewer.contentScroll.Refresh()
		return
	}
	if ev.Button == desktop.MouseButtonSecondary {
		return
	}
	c.viewer.textLayer.MouseUp(ev)
	c.viewer.annotToolLayer.MouseUp(ev)
	c.viewer.noteLayer.MouseUp(ev)
}

func (c *contextOverlay) MouseMoved(ev *desktop.MouseEvent) {
	if c.panDragging {
		dx := c.panStart.X - ev.AbsolutePosition.X
		dy := c.panStart.Y - ev.AbsolutePosition.Y
		c.panStart = ev.AbsolutePosition

		scroll := c.viewer.contentScroll
		sz := scroll.Size()
		cs := scroll.Content.Size()
		newX := c.panTarget.X + dx
		newY := c.panTarget.Y + dy
		if newX < 0 { newX = 0 }
		if newY < 0 { newY = 0 }
		if newX+sz.Width > cs.Width { newX = cs.Width - sz.Width }
		if newY+sz.Height > cs.Height { newY = cs.Height - sz.Height }
		c.panTarget = fyne.NewPos(newX, newY)
		return
	}
	c.viewer.textLayer.MouseMoved(ev)
	c.viewer.annotToolLayer.MouseMoved(ev)
	c.viewer.noteHoverLayer.MouseMoved(ev)
}

func (c *contextOverlay) MouseIn(ev *desktop.MouseEvent) {
	c.viewer.textLayer.MouseIn(ev)
	c.viewer.annotToolLayer.MouseIn(ev)
	c.viewer.noteHoverLayer.MouseIn(ev)
}

func (c *contextOverlay) MouseOut() {
	c.viewer.textLayer.MouseOut()
	c.viewer.annotToolLayer.MouseOut()
	c.viewer.noteHoverLayer.MouseOut()
}

func (c *contextOverlay) TappedSecondary(ev *fyne.PointEvent) {
	v := c.viewer
	if v.pdfDoc == nil {
		return
	}
	items := []*fyne.MenuItem{
		fyne.NewMenuItem("复制本页", func() { v.CopyPageTextPlus() }),
		fyne.NewMenuItem("全部成图", func() { v.ShowExportDialogPlus() }),
		fyne.NewMenuItem("本页成图", func() { v.ExportCurrentPagePNGPlus() }),
		fyne.NewMenuItem("本页扣图", func() { v.ExtractPageImagesPlus() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("笔记列表", func() { v.ShowNoteList() }),
		fyne.NewMenuItem("夜间模式", func() { v.ToggleNightModePlus() }),
		fyne.NewMenuItem("删除本页", func() { v.DeleteCurrentPagePlus() }),
	}
	menu := fyne.NewMenu("", items...)
	popUp := widget.NewPopUpMenu(menu, v.parentWin.Canvas())
	popUp.ShowAtPosition(ev.AbsolutePosition)
}

func (c *contextOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &contextOverlayRenderer{}
}

type contextOverlayRenderer struct{}

func (r *contextOverlayRenderer) Layout(size fyne.Size)               {}
func (r *contextOverlayRenderer) MinSize() fyne.Size                   { return fyne.NewSize(1, 1) }
func (r *contextOverlayRenderer) Refresh()                             {}
func (r *contextOverlayRenderer) Objects() []fyne.CanvasObject         { return nil }
func (r *contextOverlayRenderer) Destroy()                             {}

func (v *PDFViewerPlus) updateTitle() {
	t := "GtPDF"
	if v.filePath != "" {
		t += " - " + filepath.Base(v.filePath)
		if v.dirty {
			t += " *"
		}
	}
	v.parentWin.SetTitle(t)
}

func (v *PDFViewerPlus) markDirty() {
	if v.pdfDoc == nil || v.dirty {
		return
	}
	v.dirty = true
	v.updateTitle()
}

func (v *PDFViewerPlus) markClean() {
	if !v.dirty {
		return
	}
	v.dirty = false
	v.updateTitle()
}

func createReaderTabPlus(win fyne.Window, filePath ...string) *container.TabItem {
	v := &PDFViewerPlus{
		parentWin:  win,
		zoom:       1.0,
		thumbCache: make(map[int]image.Image),
	}

	v.initUIPlus()

	if len(filePath) > 0 && filePath[0] != "" {
		v.LoadFilePlus(filePath[0])
	}

	win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if strings.EqualFold(filepath.Ext(u.Name()), ".pdf") {
				if v.pdfDoc != nil {
					v.CloseFilePlus()
				}
				v.LoadFilePlus(u.Path())
				return
			}
		}
	})

	win.SetCloseIntercept(func() {
		if v.dirty {
			dialog.ShowConfirm("未保存的修改", "标注有未保存的修改，确定关闭？", func(ok bool) {
				if ok {
					win.Close()
				}
			}, win)
		} else {
			win.Close()
		}
	})

	win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyJ,
		Modifier: desktop.ControlModifier,
	}, func(s fyne.Shortcut) {
		if v.pdfDoc != nil {
			v.NextPagePlus()
		}
	})

	win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyK,
		Modifier: desktop.ControlModifier,
	}, func(s fyne.Shortcut) {
		if v.pdfDoc != nil {
			v.PrevPagePlus()
		}
	})

	return container.NewTabItem("阅读PLUS", v.mainArea)
}

func (v *PDFViewerPlus) initUIPlus() {
	v.pageEntry = widget.NewEntry()
	v.pageEntry.SetPlaceHolder("页码")
	v.pageEntry.OnSubmitted = func(s string) {
		if page, err := strconv.Atoi(s); err == nil && page > 0 && page <= v.totalPages {
			v.GoToPagePlus(page)
		} else {
			v.pageEntry.SetText(fmt.Sprintf("%d", v.currentPage))
		}
	}

	v.pageTotalLabel = widget.NewLabel("/ 0")
	v.zoomEntry = widget.NewSelectEntry([]string{"25%", "50%", "75%", "100%", "150%", "200%", "300%", "400%", "500%"})
	v.zoomEntry.SetText("100%")
	zoomOpts := []string{"25%", "50%", "75%", "100%", "150%", "200%", "300%", "400%", "500%"}
	v.zoomEntry.OnChanged = func(s string) {
		if v.zoomUpdating {
			return
		}
		for _, opt := range zoomOpts {
			if s == opt {
				pctStr := s[:len(s)-1]
				if pct, err := strconv.Atoi(pctStr); err == nil {
					v.zoom = float32(pct) / 100.0
					v.fitWidthMode = false
					v.fitWidthBtn.Importance = widget.MediumImportance
					v.fitWidthBtn.Refresh()
					v.fitPageBtn.Importance = widget.MediumImportance
					v.fitPageBtn.Refresh()
					v.renderCurrentPagePlus()
				}
				return
			}
		}
	}
	v.zoomEntry.OnSubmitted = func(s string) {
		if v.zoomUpdating {
			return
		}
		pctStr := s
		if len(s) > 0 && s[len(s)-1] == '%' {
			pctStr = s[:len(s)-1]
		}
		if pct, err := strconv.Atoi(pctStr); err == nil && pct > 0 {
			v.zoom = float32(pct) / 100.0
			v.fitWidthMode = false
			v.fitWidthBtn.Importance = widget.MediumImportance
			v.fitWidthBtn.Refresh()
			v.fitPageBtn.Importance = widget.MediumImportance
			v.fitPageBtn.Refresh()
			v.renderCurrentPagePlus()
		} else {
			v.zoomUpdating = true
			v.zoomEntry.SetText(fmt.Sprintf("%.0f%%", v.zoom*100))
			v.zoomUpdating = false
		}
	}

	v.thumbList = widget.NewList(
		func() int {
			if v.pdfDoc == nil {
				return 0
			}
			return v.pdfDoc.numPages
		},
		func() fyne.CanvasObject {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(80, 100))
			btn := widget.NewButton("1", nil)
			bg := canvas.NewRectangle(color.Transparent)
			return container.NewStack(bg, container.NewVBox(img, btn))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if v.pdfDoc == nil || int(id) >= v.pdfDoc.numPages {
				return
			}

			wrapper := obj.(*fyne.Container)
			bg := wrapper.Objects[0].(*canvas.Rectangle)
			content := wrapper.Objects[1].(*fyne.Container)
			img := content.Objects[0].(*canvas.Image)
			btn := content.Objects[1].(*widget.Button)

			pageIdx := int(id)
			pageNum := pageIdx + 1

			v.thumbMu.Lock()
			thumbImg, ok := v.thumbCache[pageIdx]
			v.thumbMu.Unlock()
			if ok && thumbImg != nil {
				img.Image = thumbImg
				img.Refresh()
			} else {
				img.Image = nil
				img.Refresh()
				go func(pidx int) {
					thumb, cleanup, err := v.pdfDoc.doc.RenderPage(pidx, 72, false)
					if err == nil && thumb != nil {
						v.thumbMu.Lock()
						v.thumbCache[pidx] = thumb
						if len(v.thumbCache) > 50 {
							farthest := pidx
							farthestDist := -1
							for idx := range v.thumbCache {
								dist := idx - v.currentPage + 1
								if dist < 0 {
									dist = -dist
								}
								if dist > farthestDist {
									farthestDist = dist
									farthest = idx
								}
							}
							delete(v.thumbCache, farthest)
						}
						v.thumbMu.Unlock()
					}
					if cleanup != nil {
						cleanup()
					}
					fyne.Do(func() {
						v.thumbList.RefreshItem(widget.ListItemID(pidx))
					})
				}(pageIdx)
			}

			btn.SetText(fmt.Sprintf("%d", pageNum))
			btn.OnTapped = func() {
				v.GoToPagePlus(pageNum)
			}

			if pageNum == v.currentPage {
				bg.FillColor = theme.PrimaryColor()
			} else {
				bg.FillColor = color.Transparent
			}
			bg.Refresh()
		},
	)

	v.bookmarkTree = widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID {
			return v.bookmarkTreeIDs[id]
		},
		func(id widget.TreeNodeID) bool {
			return len(v.bookmarkTreeIDs[id]) > 0
		},
		func(_ bool) fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TreeNodeID, _ bool, node fyne.CanvasObject) {
			if bm, ok := v.bookmarkData[id]; ok {
				node.(*widget.Label).SetText(fmt.Sprintf("%s  (%d)", bm.Title, bm.Page+1))
			}
		},
	)
	v.bookmarkTree.OnSelected = func(id widget.TreeNodeID) {
		if bm, ok := v.bookmarkData[id]; ok {
			v.GoToPagePlus(bm.Page + 1)
		}
	}

	v.sideTabs = container.NewAppTabs(
		container.NewTabItem("缩略图", v.thumbList),
		container.NewTabItem("书签", v.bookmarkTree),
	)
	v.sideTabs.OnSelected = func(ti *container.TabItem) {
		if ti.Text == "书签" && v.pdfDoc != nil {
			v.loadBookmarks()
		}
	}
	v.contentScroll = container.NewScroll(container.NewVBox())
	v.contentScroll.Direction = container.ScrollBoth
	// 用 resizeWatcher 包装 contentScroll，窗口 resize 时触发 auto-fit
	watcher := &resizeWatcher{inner: v.contentScroll, viewer: v}
	watcher.ExtendBaseWidget(watcher)
	watcher.onResize = func(s fyne.Size) {
		if v.resizeTimer != nil {
			v.resizeTimer.Stop()
		}
		v.resizeTimer = time.NewTimer(300 * time.Millisecond)
		go func() {
			<-v.resizeTimer.C
			fyne.Do(func() {
				if v.pdfDoc != nil {
					v.autoFitPlus()
				}
			})
		}()
	}
	v.resizeWatcher = watcher

	v.textLayer = NewTextSelectionLayer(v)
	v.textLayer.copyCallback = func(text string) {
		if text == "" {
			return
		}
		clip := fyne.CurrentApp().Clipboard()
		if clip != nil {
			clip.SetContent(text)
		}
	}

	v.noteLayer = NewNoteLayer(v)
	v.noteLayer.Hide()
	v.noteHoverLayer = NewNoteHoverLayer(v)
	v.annotLayer = NewAnnotLayer(v)
	v.annotToolLayer = NewAnnotToolLayer(v, v.annotLayer)
	v.annotLayer.toolLayer = v.annotToolLayer
	v.annotSettings = loadAnnotSettings()
	v.noteStore = NewNoteStore()

	v.scrollOverlay = &scrollOverlay{viewer: v}
	v.scrollOverlay.ExtendBaseWidget(v.scrollOverlay)
	v.contextOverlay = &contextOverlay{viewer: v}
	v.contextOverlay.ExtendBaseWidget(v.contextOverlay)
	v.thumbVisible = false
	v.contentArea = container.NewStack(watcher, v.scrollOverlay)

	v.createMergedToolbarPlus()
}

func showOpenMenu(v *PDFViewerPlus) {
	recent := loadRecent()
	if len(recent) == 0 {
		openFileDialog(v)
		return
	}
	var items []*fyne.MenuItem
	for _, p := range recent {
		if p == "" {
			continue
		}
		path := p
		items = append(items, fyne.NewMenuItem(path, func() {
			v.LoadFilePlus(path)
		}))
	}
	if len(items) > 0 {
		items = append(items, fyne.NewMenuItem("清空历史记录", func() {
			clearRecent()
		}))
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items, fyne.NewMenuItem("浏览...", func() {
		openFileDialog(v)
	}))
	menu := fyne.NewMenu("", items...)
	popUp := widget.NewPopUpMenu(menu, v.parentWin.Canvas())
	popUp.ShowAtPosition(v.openBtn.Position().Add(fyne.NewPos(0, v.openBtn.Size().Height)))
}

func openFileDialog(v *PDFViewerPlus) {
	fd := dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
		if err == nil && uri != nil {
			v.LoadFilePlus(uri.URI().Path())
		}
	}, v.parentWin)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
	fd.Resize(fyne.NewSize(800, 600))
	fd.Show()
}

func (v *PDFViewerPlus) createMergedToolbarPlus() {
	v.openBtn = widget.NewButtonWithIcon("打开", theme.FolderOpenIcon(), func() {
		if v.pdfDoc != nil {
			v.CloseFilePlus()
			return
		}
		showOpenMenu(v)
	})

	saveBtn := widget.NewButtonWithIcon("另存", theme.DocumentSaveIcon(), func() { v.SaveFilePlus() })

	prevBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { v.PrevPagePlus() })

	nextBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { v.NextPagePlus() })

	zoomOutBtn := widget.NewButtonWithIcon("", theme.ZoomOutIcon(), func() { v.ZoomOutPlus() })

	zoomInBtn := widget.NewButtonWithIcon("", theme.ZoomInIcon(), func() { v.ZoomInPlus() })

	fitWidthBtn := widget.NewButton("宽度", func() { v.FitWidthPlus() })
	fitWidthBtn.Importance = widget.HighImportance
	v.fitWidthBtn = fitWidthBtn

	fitPageBtn := widget.NewButton("页面", func() { v.FitPagePlus() })
	v.fitPageBtn = fitPageBtn
	v.fitWidthMode = true

	thumbBtn := widget.NewButtonWithIcon("缩略", theme.ListIcon(), func() { v.ToggleThumbnailsPlus(); v.autoFitPlus() })

	panBtn := widget.NewButtonWithIcon("平移", theme.GridIcon(), nil)
	panBtn.OnTapped = func() {
		v.panEnabled = !v.panEnabled
		if v.panEnabled {
			panBtn.Importance = widget.HighImportance
			if v.annotMode {
				v.annotMode = false
				v.annotToolLayer.SetAnnotMode(false)
				v.annotBottomBar.Hide()
				v.annotToggleBtn.Importance = widget.MediumImportance
				v.annotToggleBtn.Refresh()
			}
			if v.noteMode {
				v.noteMode = false
				v.noteLayer.SetNoteMode(false)
				v.noteLayer.Hide()
				v.noteBtn.Importance = widget.MediumImportance
				v.noteBtn.Refresh()
			}
			if v.textLayer.selectionEnabled {
				v.textLayer.selectionEnabled = false
				v.textLayer.ClearSelection()
				v.selectBtn.Importance = widget.MediumImportance
				v.selectBtn.Refresh()
			}
		} else {
			panBtn.Importance = widget.MediumImportance
		}
		panBtn.Refresh()
	}

	v.selectBtn = widget.NewButtonWithIcon("划词", theme.FileIcon(), func() {
		if v.pdfDoc == nil {
			return
		}
		if v.textLayer.selectionEnabled {
			v.textLayer.selectionEnabled = false
			v.textLayer.ocrMode = false
			v.textLayer.ocrImg = nil
			v.textLayer.ClearSelection()
			v.selectBtn.Importance = widget.MediumImportance
			v.selectBtn.Refresh()
			return
		}
		text, err := v.pdfDoc.CopyPageTextPlus(v.currentPage - 1)
		if err != nil || len(text) == 0 {
			if isTesseractAvailable() {
				dialog.ShowConfirm("提示", "当前页是扫描件，将使用 OCR 识别，继续？", func(ok bool) {
					if ok {
						if v.noteMode {
							v.noteMode = false
							v.noteLayer.SetNoteMode(false)
							v.noteLayer.Hide()
							v.noteBtn.Importance = widget.MediumImportance
							v.noteBtn.Refresh()
						}
						if v.annotMode {
							v.annotMode = false
							v.annotToolLayer.SetAnnotMode(false)
							v.annotBottomBar.Hide()
							v.annotToggleBtn.Importance = widget.MediumImportance
							v.annotToggleBtn.Refresh()
						}
						v.enableOCRSelection()
					}
				}, v.parentWin)
				return
			}
			dialog.ShowInformation("提示", "扫描件暂时无法复制文字，请安装 Tesseract OCR 后重试", v.parentWin)
			return
		}
		v.textLayer.ocrMode = false
		v.textLayer.ocrImg = nil
		v.textLayer.selectionEnabled = true
		v.textLayer.hitBoxes = nil
		v.textLayer.hitBoxesPage = -1
		if v.noteMode {
			v.noteMode = false
			v.noteLayer.SetNoteMode(false)
			v.noteLayer.Hide()
			v.noteBtn.Importance = widget.MediumImportance
			v.noteBtn.Refresh()
		}
		if v.annotMode {
			v.annotMode = false
			v.annotToolLayer.SetAnnotMode(false)
			v.annotBottomBar.Hide()
			v.annotToggleBtn.Importance = widget.MediumImportance
			v.annotToggleBtn.Refresh()
		}
		v.selectBtn.Importance = widget.HighImportance
		v.selectBtn.Refresh()
	})
	v.selectBtn.Importance = widget.MediumImportance

	searchBtn := widget.NewButtonWithIcon("搜索", theme.SearchIcon(), func() { v.ShowSearchDialogPlus() })

	printBtn := widget.NewButtonWithIcon("打印", theme.DocumentPrintIcon(), func() { v.ShowPrintDialog() })

	v.noteBtn = widget.NewButtonWithIcon("笔记", theme.DocumentCreateIcon(), func() {
		if v.pdfDoc == nil {
			dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
			return
		}
		v.noteMode = !v.noteMode
		v.noteLayer.SetNoteMode(v.noteMode)
		if v.noteMode {
			v.noteLayer.Show()
			v.noteLayer.Refresh()
			v.textLayer.selectionEnabled = false
			v.textLayer.ClearSelection()
			v.selectBtn.Importance = widget.MediumImportance
			v.selectBtn.Refresh()
			if v.annotMode {
				v.annotMode = false
				v.annotToolLayer.SetAnnotMode(false)
				v.annotBottomBar.Hide()
				v.annotToggleBtn.Importance = widget.MediumImportance
				v.annotToggleBtn.Refresh()
			}
			if !v.noteHintShown {
				v.noteHintShown = true
				dialog.ShowInformation("提示", "笔记不写入 PDF 文件，如需嵌入请使用标注功能。", v.parentWin)
			}
			v.noteBtn.Importance = widget.HighImportance
			v.noteBtn.Refresh()
		} else {
			v.noteLayer.Hide()
			v.noteBtn.Importance = widget.MediumImportance
			v.noteBtn.Refresh()
		}
	})
	v.noteBtn.Importance = widget.MediumImportance

	v.annotToggleBtn = widget.NewButtonWithIcon("标注", theme.DocumentIcon(), func() {
		if v.pdfDoc == nil {
			dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
			return
		}
		if !v.annotMode && v.pdfDoc.HasRotatedPages() && v.pdfDoc.IsScannedDocument() {
			v.askFlattenScanned()
			return
		}
		v.annotMode = !v.annotMode
		v.annotToolLayer.SetAnnotMode(v.annotMode)
		if v.annotMode {
			v.noteMode = false
			v.noteLayer.SetNoteMode(false)
			v.noteLayer.Hide()
			v.noteBtn.Importance = widget.MediumImportance
			v.noteBtn.Refresh()
			v.textLayer.selectionEnabled = false
			v.textLayer.ClearSelection()
			v.selectBtn.Importance = widget.MediumImportance
			v.selectBtn.Refresh()
			v.noteHoverLayer.Hide()
			v.annotBottomBar.Show()
			v.annotToolLayer.SetDefaultTool(AnnotToolHighlight)
			v.annotToggleBtn.Importance = widget.HighImportance
		} else {
			v.annotToolLayer.SetAnnotMode(false)
			v.noteHoverLayer.Show()
			v.annotBottomBar.Hide()
			v.annotToggleBtn.Importance = widget.MediumImportance
		}
		v.annotToggleBtn.Refresh()
		v.annotLayer.Refresh()
		v.parentWin.Canvas().Refresh(v.annotLayer)
	})
	v.annotToggleBtn.Importance = widget.MediumImportance

	pageNavBox := container.NewHBox(newPageEntryWidgetPlus(v.pageEntry), v.pageTotalLabel)

	toolbarContainer := container.New(
		layout.NewHBoxLayout(),
		v.openBtn,              // 打开 / 关闭
		saveBtn,                // 另存
		printBtn,               // 打印
		widget.NewSeparator(),
		pageNavBox,             // 页码输入 / 总页数
		prevBtn,                // 上一页
		nextBtn,                // 下一页
		widget.NewSeparator(),
		zoomOutBtn,             // 缩小
		zoomInBtn,              // 放大
		newZoomEntryWidgetPlus(v.zoomEntry),  // 缩放输入
		fitWidthBtn,            // 适应宽度
		fitPageBtn,             // 适应页面
		widget.NewSeparator(),
		panBtn,                 // 抓手
		v.selectBtn,            // 划词
		v.annotToggleBtn,       // 标注
		v.noteBtn,              // 笔记
		searchBtn,              // 搜索
		thumbBtn,               // 缩略图
	)

	v.contentScroll.Content = container.NewVBox(widget.NewLabel("  请点击「打开」按钮加载 PDF 文件"))

	toolbarScroll := container.NewHScroll(toolbarContainer)
	v.annotBottomBar = container.NewVBox(v.annotToolLayer.Toolbar())
	v.annotBottomBar.Hide()
	v.mainArea = container.NewBorder(toolbarScroll, v.annotBottomBar, nil, nil, v.contentArea)
}

func (v *PDFViewerPlus) LoadFilePlus(path string) {
	if v.pdfDoc != nil {
		v.pdfDoc.Close()
		v.pdfDoc = nil
	}
	v.cleanupTempFilePlus()
	v.filePath = path
	v.originalFilePath = path

	loadingDlg := dialog.NewCustom("加载中...", "请稍候", widget.NewLabel("正在打开PDF文件..."), v.parentWin)
	loadingDlg.Show()
	t0 := time.Now()

	go func() {
		doc, err := LoadPDFiumDocument(path)
		if err != nil {
			fyne.Do(func() {
				loadingDlg.Hide()
				dialog.ShowError(err, v.parentWin)
			})
			return
		}
		logD("[load] doc loaded in %v", time.Since(t0))

		// 取Fyne值(缩放相关) - 这趟fyne.Do必须在显示之前
		var zoom float32
		var canvasScale float32
		ch := make(chan struct{})
		fyne.Do(func() {
			canvasScale = v.parentWin.Canvas().Scale()
			page0 := doc.GetPagePlus(0)
			if page0 != nil && page0.Width > 0 {
				contentWidth := float32(v.parentWin.Canvas().Size().Width)
				if v.thumbVisible {
					contentWidth -= 220
				}
				zoom = contentWidth / float32(page0.Width)
			} else {
				zoom = 1.0
			}
			close(ch)
		})
		<-ch
		logD("[load] zoom=%.2f scale=%.1f in %v", zoom, canvasScale, time.Since(t0))

		// 后台渲染第1页
		t1 := time.Now()
		img, _ := doc.RenderPagePlus(0, zoom, canvasScale, false)
		logD("[load] page 1 rendered in %v", time.Since(t1))

		addRecentFile(path)

		// 单趟fyne.Do: 只做UI显示,不做任何其他操作
		fyne.Do(func() {
			v.pdfDoc = doc
			v.totalPages = doc.PageCountPlus()
			v.currentPage = 1
			v.zoom = zoom
			v.fitWidthMode = true

			nW := float64(doc.GetPagePlus(0).Width) * float64(zoom)
			nH := float64(doc.GetPagePlus(0).Height) * float64(zoom)
			v.lastPageWidth = nW
			v.lastPageHeight = nH
			v.lastRenderPage = 1
			v.lastRenderZoom = zoom

			v.noteStore = loadNotesForFile(path)

			v.markClean()
			v.updateTitle()
			v.pageEntry.SetText("1")
			v.pageTotalLabel.SetText(fmt.Sprintf("/ %d", v.totalPages))

			v.openBtn.SetText("关闭")
			v.openBtn.SetIcon(theme.CancelIcon())
			v.openBtn.Refresh()

			v.zoomUpdating = true
			v.zoomEntry.SetText(fmt.Sprintf("%.0f%%", zoom*100))
			v.zoomUpdating = false
			v.fitWidthBtn.Importance = widget.HighImportance
			v.fitWidthBtn.Refresh()
			v.fitPageBtn.Importance = widget.MediumImportance
			v.fitPageBtn.Refresh()

			// 直接用渲染好的图片创建Stack
			v.canvasImg = canvas.NewImageFromImage(img)
			v.canvasImg.FillMode = canvas.ImageFillOriginal
			contentContainer := container.NewStack(v.canvasImg, v.annotLayer, v.textLayer, v.annotToolLayer, v.noteLayer, v.noteHoverLayer, v.contextOverlay)
			v.contentScroll.Content = contentContainer
			v.canvasImg.SetMinSize(fyne.NewSize(float32(nW), float32(nH)))
			v.contentScroll.Refresh()

			v.noteLayer.pageWidth = doc.GetPagePlus(0).Width
			v.noteLayer.pageHeight = doc.GetPagePlus(0).Height
			v.noteHoverLayer.pageWidth = doc.GetPagePlus(0).Width
			v.noteHoverLayer.pageHeight = doc.GetPagePlus(0).Height
			v.noteHoverLayer.ClearHover()
			v.annotLayer.pageWidth = doc.GetPagePlus(0).Width
			v.annotLayer.pageHeight = doc.GetPagePlus(0).Height
			v.annotToolLayer.Refresh()
			v.annotLayer.Refresh()
			loadingDlg.Hide()
			logD("[load] page 1 displayed, total %v", time.Since(t0))
		})

	}()
}

func (v *PDFViewerPlus) askFlattenScanned() {
	baseName := strings.TrimSuffix(filepath.Base(v.filePath), filepath.Ext(v.filePath))
	flattenPath := filepath.Join(filepath.Dir(v.filePath), baseName+"_flattened.pdf")

	dialog.ShowConfirm("检测到旋转页面",
		"此PDF包含旋转页面(/Rotate)且为扫描件，\n"+
			"直接标注后其他阅读器可能显示位置不对。\n\n"+
			"是否转换为展平图像PDF再处理？\n"+
			"（将另存为同目录下的 "+filepath.Base(flattenPath)+"）",
		func(ok bool) {
			if !ok {
				return
			}
			loadingDlg := dialog.NewCustom("转换中...", "请稍候",
				widget.NewLabel("正在将旋转页面转换为图像PDF，请稍候..."), v.parentWin)
			loadingDlg.Show()
			go func() {
				page0 := v.pdfDoc.GetPagePlus(0)
				dpi := 200.0
				if page0 != nil && page0.Width > 0 && page0.Height > 0 {
					detected, err := detectScanDPI(v.filePath, page0.Width, page0.Height)
					if err == nil {
						dpi = detected
					}
				}

				data, err := v.flattenRotatedPDF(dpi)
				if err != nil {
					fyne.Do(func() {
						loadingDlg.Hide()
						dialog.ShowError(fmt.Errorf("展平失败: %w", err), v.parentWin)
					})
					return
				}
				if err := os.WriteFile(flattenPath, data, 0644); err != nil {
					fyne.Do(func() {
						loadingDlg.Hide()
						dialog.ShowError(fmt.Errorf("写入文件失败: %w", err), v.parentWin)
					})
					return
				}
				fyne.Do(func() {
					loadingDlg.Hide()
					v.LoadFilePlus(flattenPath)
				})
			}()
		}, v.parentWin)
}

func (v *PDFViewerPlus) cleanupTempFilePlus() {
	if v.filePath == "" || v.originalFilePath == "" {
		return
	}
	if filepath.Clean(v.filePath) != filepath.Clean(v.originalFilePath) {
		_ = os.Remove(v.filePath)
	}
}

func buildPageSelectionWithoutPagePlus(totalPages int, removePage int) []string {
	if totalPages <= 1 || removePage < 1 || removePage > totalPages {
		return nil
	}
	parts := make([]string, 0, 2)
	if removePage > 1 {
		if removePage == 2 {
			parts = append(parts, "1")
		} else {
			parts = append(parts, fmt.Sprintf("1-%d", removePage-1))
		}
	}
	if removePage < totalPages {
		if removePage == totalPages-1 {
			parts = append(parts, fmt.Sprintf("%d", totalPages))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", removePage+1, totalPages))
		}
	}
	return parts
}

func (v *PDFViewerPlus) updateThumbnailsPlus() {
	if v.thumbList == nil || v.pdfDoc == nil {
		return
	}
	v.thumbList.Refresh()
}

func (v *PDFViewerPlus) highlightCurrentPagePlus() {
	if v.thumbList == nil {
		return
	}
	if v.prevPage > 0 {
		v.thumbList.RefreshItem(widget.ListItemID(v.prevPage - 1))
	}
	if v.currentPage > 0 {
		v.thumbList.RefreshItem(widget.ListItemID(v.currentPage - 1))
	}
}

func (v *PDFViewerPlus) scrollThumbnailsToPagePlus(page int) {
	if v.thumbList == nil || v.pdfDoc == nil {
		return
	}
	if page < 1 || page > v.pdfDoc.numPages {
		return
	}
	v.thumbList.ScrollTo(widget.ListItemID(page - 1))
}


func (v *PDFViewerPlus) GoToPagePlus(page int) {
	if page < 1 || page > v.totalPages {
		return
	}
	logD("[navigate]   %d -> %d  fitWidth=%v zoom=%.2f", v.currentPage, page, v.fitWidthMode, v.zoom)
	v.prevPage = v.currentPage
	v.currentPage = page
	v.scrollDebt = 0
	v.pageTotalLabel.SetText(fmt.Sprintf("/ %d", v.totalPages))
	v.pageEntry.SetText(fmt.Sprintf("%d", v.currentPage))
	v.highlightCurrentPagePlus()

	if v.fitWidthMode {
		v.FitWidthPlus()
	} else {
		v.renderCurrentPagePlus()
	}

	v.scrollThumbnailsToPagePlus(page)

	if v.searchQuery != "" && v.pdfDoc != nil {
		v.doHighlightCurrentPage()
	}
}

func (v *PDFViewerPlus) removeSearchHighlightFromContent() {
	if v.searchHighlight == nil || v.contentScroll == nil || v.contentScroll.Content == nil {
		return
	}
	content := v.contentScroll.Content.(*fyne.Container)
	var newObjs []fyne.CanvasObject
	for _, obj := range content.Objects {
		if obj != v.searchHighlight {
			newObjs = append(newObjs, obj)
		}
	}
	content.Objects = newObjs
}

func (v *PDFViewerPlus) doHighlightCurrentPage() (yPos float32) {
	if v.pdfDoc == nil || v.searchQuery == "" {
		return 0
	}

	if v.searchResultPosition < 0 || v.searchResultPosition >= len(v.searchResults) {
		v.removeSearchHighlightFromContent()
		return 0
	}

	currentRes := v.searchResults[v.searchResultPosition]
	if currentRes.Page != v.currentPage-1 {
		v.removeSearchHighlightFromContent()
		return 0
	}

	pageText, err := v.pdfDoc.GetPageText(v.currentPage - 1)
	if err != nil || pageText == nil {
		return 0
	}

	zoom := float64(v.zoom)
	boxes := BuildHitBoxesFromPageText(pageText, zoom)
	if len(boxes) == 0 {
		return 0
	}

	queryRunes := []rune(v.searchQuery)
	queryCharCount := len(queryRunes)

	skipCount := currentRes.MatchIndex

	foundIdx := -1
	for i := 0; i <= len(boxes)-queryCharCount; i++ {
		var sb strings.Builder
		for j := 0; j < queryCharCount; j++ {
			sb.WriteString(boxes[i+j].Text)
		}
		if sb.String() == currentRes.Text {
			if skipCount == 0 {
				foundIdx = i
				break
			}
			skipCount--
		}
	}

	if foundIdx < 0 || foundIdx >= len(boxes) {
		return 0
	}

	// Apply image offset (same as calcImageRect in AnnotLayer)
	imgOffX := float32(0)
	imgOffY := float32(0)
	imgW := float32(0)
	imgH := float32(0)
	if v.annotLayer != nil {
		imgW = float32(v.annotLayer.pageWidth * zoom)
		imgH = float32(v.annotLayer.pageHeight * zoom)
		widgetSize := v.annotLayer.Size()
		if widgetSize.Width > 0 && widgetSize.Height > 0 {
			imgOffX = (widgetSize.Width - imgW) / 2
			imgOffY = (widgetSize.Height - imgH) / 2
		}
	}

	var matches []MatchPositionPlus
	for j := 0; j < queryCharCount; j++ {
		hb := boxes[foundIdx+j]
		matches = append(matches, MatchPositionPlus{
			X:      hb.ScreenX + imgOffX,
			Y:      hb.ScreenY + imgOffY,
			Width:  hb.ScreenW,
			Height: hb.ScreenH,
		})
	}

	if v.searchHighlight == nil {
		v.searchHighlight = NewSearchResultHighlightPlus()
	}
	v.searchHighlight.SetMatches(matches)

	if v.contentScroll.Content != nil {
		content := v.contentScroll.Content.(*fyne.Container)
		found := false
		for _, obj := range content.Objects {
			if obj == v.searchHighlight {
				found = true
				break
			}
		}
		if !found {
			content.Objects = append(content.Objects, v.searchHighlight)
		}

		targetY := matches[0].Y
		scrollSize := v.contentScroll.Size()
		offsetY := targetY - scrollSize.Height/2
		if offsetY < 0 {
			offsetY = 0
		}
		maxY := imgH - float32(scrollSize.Height)
		if offsetY > maxY {
			offsetY = maxY
		}
		if offsetY < 0 {
			offsetY = 0
		}
		v.contentScroll.Offset = fyne.NewPos(0, offsetY)
		v.contentScroll.Refresh()
	}

	return matches[0].Y
}

func (v *PDFViewerPlus) clearSearchHighlight() {
	v.searchQuery = ""
	v.searchResultPosition = -1

	if v.searchHighlight == nil {
		return
	}

	if v.contentScroll.Content != nil {
		content := v.contentScroll.Content.(*fyne.Container)
		var newObjs []fyne.CanvasObject
		for _, obj := range content.Objects {
			if obj != v.searchHighlight {
				newObjs = append(newObjs, obj)
			}
		}
		content.Objects = newObjs
	}

	v.searchHighlight.SetMatches(nil)
	v.contentScroll.Refresh()
}

func (v *PDFViewerPlus) SaveFilePlus() {
	if v.pdfDoc == nil {
		return
	}
	saveDlg := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
		if err != nil || uri == nil {
			return
		}
		targetPath := uri.URI().Path()
		uri.Close()

		if filepath.Ext(strings.ToLower(targetPath)) != ".pdf" {
			targetPath += ".pdf"
		}

		if v.filePath == "" {
			dialog.ShowError(fmt.Errorf("未找到当前文件路径"), v.parentWin)
			return
		}

		savingDlg := dialog.NewCustom("保存中...", "请稍候", widget.NewLabel("正在保存PDF文件，请稍候..."), v.parentWin)
		savingDlg.Show()

		go func() {
			buf, saveErr := v.pdfDoc.doc.SaveAsCopyToBuffer()
			if saveErr != nil {
				fyne.Do(func() { savingDlg.Hide(); dialog.ShowError(saveErr, v.parentWin) })
				return
			}

			buf, _ = fixFreeTextAPBytes(buf)

			if err := os.WriteFile(targetPath, buf, 0644); err != nil {
				fyne.Do(func() { savingDlg.Hide(); dialog.ShowError(err, v.parentWin) })
				return
			}

			fyne.Do(func() {
				savingDlg.Hide()
				v.LoadFilePlus(targetPath)
				dialog.ShowInformation("成功", fmt.Sprintf("已保存到: %s", targetPath), v.parentWin)
			})
		}()
	}, v.parentWin)
	saveDlg.Resize(fyne.NewSize(800, 600))
	saveDlg.Show()
}

func (v *PDFViewerPlus) SaveNow() {
	if v.pdfDoc == nil || v.filePath == "" {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	dialog.ShowConfirm("覆盖保存", "确定覆盖当前文件并保存标注？\n"+filepath.Base(v.filePath), func(ok bool) {
		if !ok {
			return
		}
		tmpPath := v.filePath + ".gtpdf.tmp"

		savingDlg := dialog.NewCustom("保存中...", "请稍候", widget.NewLabel("正在保存PDF文件，请稍候..."), v.parentWin)
		savingDlg.Show()

		go func() {
			buf, err := v.pdfDoc.doc.SaveAsCopyToBuffer()
			if err != nil {
				fyne.Do(func() { savingDlg.Hide(); dialog.ShowError(fmt.Errorf("保存失败: %w", err), v.parentWin) })
				return
			}

			buf, _ = fixFreeTextAPBytes(buf)

			if err := os.WriteFile(tmpPath, buf, 0644); err != nil {
				fyne.Do(func() { savingDlg.Hide(); dialog.ShowError(err, v.parentWin) })
				return
			}

			if err := os.Rename(tmpPath, v.filePath); err != nil {
				fyne.Do(func() { savingDlg.Hide(); dialog.ShowError(err, v.parentWin) })
				return
			}
			fyne.Do(func() { savingDlg.Hide(); v.markClean(); dialog.ShowInformation("成功", "已保存", v.parentWin) })
		}()
	}, v.parentWin)
}

func (v *PDFViewerPlus) DeleteCurrentPagePlus() {
	if v.pdfDoc == nil || v.filePath == "" {
		return
	}
	if v.totalPages <= 1 {
		dialog.ShowInformation("提示", "只剩 1 页，无法继续删除", v.parentWin)
		return
	}

	msg := "确认删除当前页？"
	dialog.ShowConfirm("删除", msg, func(ok bool) {
		if !ok {
			return
		}

		loadingDlg := dialog.NewCustom("处理中...", "请稍候", widget.NewLabel("正在删除当前页..."), v.parentWin)
		loadingDlg.Show()

		go func() {
			pageSel := buildPageSelectionWithoutPagePlus(v.totalPages, v.currentPage)
			if len(pageSel) == 0 {
				fyne.Do(func() {
					loadingDlg.Hide()
					dialog.ShowError(fmt.Errorf("生成页面范围失败"), v.parentWin)
				})
				return
			}

			tempOutput := filepath.Join(os.TempDir(), fmt.Sprintf("gtpdf_plus_%d_%d.pdf", time.Now().UnixNano(), v.currentPage))
			conf := model.NewDefaultConfiguration()
			if err := api.TrimFile(v.filePath, tempOutput, pageSel, conf); err != nil {
				fyne.Do(func() {
					loadingDlg.Hide()
					dialog.ShowError(err, v.parentWin)
				})
				return
			}

			doc, err := LoadPDFiumDocument(tempOutput)
			if err != nil {
				_ = os.Remove(tempOutput)
				fyne.Do(func() {
					loadingDlg.Hide()
					dialog.ShowError(err, v.parentWin)
				})
				return
			}

			fyne.Do(func() {
				prevPath := v.filePath
				if v.pdfDoc != nil {
					v.pdfDoc.Close()
				}
				v.pdfDoc = doc
				v.filePath = tempOutput
				v.totalPages = doc.PageCountPlus()
				if v.currentPage > v.totalPages {
					v.currentPage = v.totalPages
				}
				if v.currentPage < 1 {
					v.currentPage = 1
				}

				v.pageTotalLabel.SetText(fmt.Sprintf("/ %d", v.totalPages))
				v.pageEntry.SetText(fmt.Sprintf("%d", v.currentPage))
				v.forceRenderPlus()
				v.renderCurrentPagePlus()
				v.updateThumbnailsPlus()
				v.highlightCurrentPagePlus()
				loadingDlg.Hide()

				if v.originalFilePath != "" && filepath.Clean(prevPath) != filepath.Clean(v.originalFilePath) {
					_ = os.Remove(prevPath)
				}
			})
		}()
	}, v.parentWin)
}

func (v *PDFViewerPlus) NextPagePlus() {
	if v.currentPage < v.totalPages {
		v.GoToPagePlus(v.currentPage + 1)
	}
}

func (v *PDFViewerPlus) PrevPagePlus() {
	if v.currentPage > 1 {
		v.GoToPagePlus(v.currentPage - 1)
	}
}

func (v *PDFViewerPlus) forceRenderPlus() {
	v.lastRenderPage = -1
	v.lastRenderZoom = -1
	atomic.AddInt32(&v.renderVer, 1)
}

func (v *PDFViewerPlus) renderCurrentPagePlus() {
	if v.pdfDoc == nil || v.contentScroll == nil {
		return
	}

	if v.currentPage == v.lastRenderPage && v.zoom == v.lastRenderZoom {
		return
	}

	ver := atomic.AddInt32(&v.renderVer, 1)
	t0 := time.Now()

	page := v.pdfDoc.GetPagePlus(v.currentPage - 1)
	if page == nil {
		return
	}
	canvasScale := v.parentWin.Canvas().Scale()

	pageIdx := v.currentPage - 1
	curZoom := v.zoom
	curNightMode := v.nightMode

	// 优先检查缓存
	dpi := 72.0 * float64(curZoom) * float64(canvasScale)
	roundedDPI := int(math.Round(dpi))
	cacheKey := zoomCacheKeyPlus{page: pageIdx, dpi: roundedDPI, nightMode: curNightMode}
	v.pdfDoc.zoomMu.Lock()
	cached, cachedOK := v.pdfDoc.zoomCache[cacheKey]
	v.pdfDoc.zoomMu.Unlock()

	nW := page.Width * float64(curZoom)
	nH := page.Height * float64(curZoom)

	if cachedOK && cached != nil {
		v.lastRenderPage = v.currentPage
		v.lastRenderZoom = v.zoom
		v.lastPageWidth = nW
		v.lastPageHeight = nH
		if v.canvasImg == nil {
			v.canvasImg = canvas.NewImageFromImage(nil)
			v.canvasImg.FillMode = canvas.ImageFillOriginal
			contentContainer := container.NewStack(v.canvasImg, v.annotLayer, v.textLayer, v.annotToolLayer, v.noteLayer, v.noteHoverLayer, v.contextOverlay)
			v.contentScroll.Content = contentContainer
		}
		v.canvasImg.Image = cached
		v.canvasImg.SetMinSize(fyne.NewSize(float32(nW), float32(nH)))
		v.canvasImg.Refresh()
		v.textLayer.ClearSelection()
		v.noteLayer.pageWidth = page.Width
		v.noteLayer.pageHeight = page.Height
		v.noteHoverLayer.pageWidth = page.Width
		v.noteHoverLayer.pageHeight = page.Height
		v.noteHoverLayer.ClearHover()
		v.annotLayer.pageWidth = page.Width
		v.annotLayer.pageHeight = page.Height
		v.annotToolLayer.Refresh()
		v.annotLayer.Refresh()
		if v.noteMode {
			v.noteLayer.Refresh()
		}
		if v.scrollToBottom {
			v.scrollToBottom = false
			maxY := nH - float64(v.contentScroll.Size().Height)
			if maxY < 0 {
				maxY = 0
			}
			v.contentScroll.Offset = fyne.NewPos(0, float32(maxY))
		} else {
			v.contentScroll.Offset = fyne.NewPos(0, 0)
		}
		v.contentScroll.Refresh()
		logD("[view] ver=%d page=%d zoom=%.2f cache HIT in %v", ver, v.currentPage, v.zoom, time.Since(t0))
		return
	}

	logD("[view] ver=%d page=%d zoom=%.2f fitWidth=%v start", ver, v.currentPage, v.zoom, v.fitWidthMode)
	v.lastRenderPage = v.currentPage
	v.lastRenderZoom = v.zoom

	if v.canvasImg == nil {
		v.canvasImg = canvas.NewImageFromImage(nil)
		v.canvasImg.FillMode = canvas.ImageFillOriginal
		contentContainer := container.NewStack(v.canvasImg, v.annotLayer, v.textLayer, v.annotToolLayer, v.noteLayer, v.noteHoverLayer, v.contextOverlay)
		v.contentScroll.Content = contentContainer
	}

	v.textLayer.ClearSelection()

	v.noteLayer.pageWidth = page.Width
	v.noteLayer.pageHeight = page.Height
	if v.noteMode {
		v.noteLayer.Refresh()
	}

	v.noteHoverLayer.pageWidth = page.Width
	v.noteHoverLayer.pageHeight = page.Height
	v.noteHoverLayer.ClearHover()

	v.annotLayer.pageWidth = page.Width
	v.annotLayer.pageHeight = page.Height

	v.annotToolLayer.Refresh()

	v.lastPageWidth = nW
	v.lastPageHeight = nH

	logD("[view] ver=%d plan done in %v", ver, time.Since(t0))

	// 缓存未命中:后台渲染,layout+换图在render完成后原子执行,避免抖动
	go func() {
		t1 := time.Now()
		img, err := v.pdfDoc.RenderPagePlus(pageIdx, curZoom, canvasScale, curNightMode)
		if err != nil || img == nil {
			img = image.NewRGBA(image.Rect(0, 0, int(page.Width), int(page.Height)))
		}
		logD("[view] ver=%d render done in %v", ver, time.Since(t1))

		fyne.Do(func() {
			if atomic.LoadInt32(&v.renderVer) != ver || v.canvasImg == nil {
				logD("[view] ver=%d stale (current ver=%d), discard", ver, atomic.LoadInt32(&v.renderVer))
				return
			}
			v.canvasImg.SetMinSize(fyne.NewSize(float32(nW), float32(nH)))
			v.canvasImg.Image = img
			v.canvasImg.Refresh()
			v.contentScroll.Refresh()
			if v.scrollToBottom {
				v.scrollToBottom = false
				maxY := nH - float64(v.contentScroll.Size().Height)
				if maxY < 0 {
					maxY = 0
				}
				v.contentScroll.Offset = fyne.NewPos(0, float32(maxY))
			} else {
				v.contentScroll.Offset = fyne.NewPos(0, 0)
			}
			logD("[view] ver=%d displayed in %v", ver, time.Since(t0))
		})
		fyne.Do(func() {
			if atomic.LoadInt32(&v.renderVer) != ver {
				return
			}
			v.annotLayer.Refresh()
			if v.noteMode {
				v.noteLayer.Refresh()
			}
		})
	}()
}

func (v *PDFViewerPlus) setZoomPlus(zoom float32) {
	if zoom < 0.1 {
		zoom = 0.1
	}
	if zoom > 5.0 {
		zoom = 5.0
	}
	v.zoom = zoom
	v.fitWidthMode = false
	v.fitWidthBtn.Importance = widget.MediumImportance
	v.fitWidthBtn.Refresh()
	v.fitPageBtn.Importance = widget.MediumImportance
	v.fitPageBtn.Refresh()
	v.zoomUpdating = true
	v.zoomEntry.SetText(fmt.Sprintf("%.0f%%", zoom*100))
	v.zoomUpdating = false
	v.renderCurrentPagePlus()
}

func (v *PDFViewerPlus) ZoomInPlus() {
	v.setZoomPlus(v.zoom + 0.1)
}

func (v *PDFViewerPlus) ZoomOutPlus() {
	if v.zoom <= 0.25 {
		return
	}
	v.setZoomPlus(v.zoom - 0.1)
}

func (v *PDFViewerPlus) contentWidth() float32 {
	winSize := v.parentWin.Canvas().Size()
	if v.contentSplit != nil {
		return float32(winSize.Width) * (1 - float32(v.contentSplit.Offset))
	}
	if v.thumbVisible {
		return float32(winSize.Width) - 220
	}
	return float32(winSize.Width)
}

func (v *PDFViewerPlus) FitWidthPlus() {
	if v.pdfDoc == nil {
		return
	}

	v.fitWidthMode = true
	v.fitWidthBtn.Importance = widget.HighImportance
	v.fitWidthBtn.Refresh()
	v.fitPageBtn.Importance = widget.MediumImportance
	v.fitPageBtn.Refresh()

	page := v.pdfDoc.GetPagePlus(v.currentPage - 1)
	if page == nil {
		return
	}

	contentWidth := v.contentWidth()

	if contentWidth <= 0 || page.Width <= 0 {
		return
	}

	v.zoom = float32(contentWidth) / float32(page.Width)

	v.zoomUpdating = true
	v.zoomEntry.SetText(fmt.Sprintf("%.0f%%", v.zoom*100))
	v.zoomUpdating = false
	v.renderCurrentPagePlus()
}

func (v *PDFViewerPlus) FitPagePlus() {
	if v.pdfDoc == nil {
		return
	}

	v.fitWidthMode = false
	v.fitWidthBtn.Importance = widget.MediumImportance
	v.fitWidthBtn.Refresh()
	v.fitPageBtn.Importance = widget.HighImportance
	v.fitPageBtn.Refresh()

	page := v.pdfDoc.GetPagePlus(v.currentPage - 1)
	if page == nil {
		return
	}

	winSize := v.parentWin.Canvas().Size()
	contentWidth := v.contentWidth()
	contentHeight := float32(winSize.Height) - 60

	widthRatio := contentWidth / float32(page.Width)
	heightRatio := contentHeight / float32(page.Height)

	v.zoom = widthRatio
	if heightRatio < widthRatio {
		v.zoom = heightRatio
	}

	if v.zoom < 0.1 {
		v.zoom = 0.1
	}

	v.zoomUpdating = true
	v.zoomEntry.SetText(fmt.Sprintf("%.0f%%", v.zoom*100))
	v.zoomUpdating = false
	v.renderCurrentPagePlus()
}

func (v *PDFViewerPlus) autoFitPlus() {
	if v.pdfDoc == nil {
		return
	}
	if v.fitWidthMode {
		v.FitWidthPlus()
	} else {
		v.renderCurrentPagePlus()
	}
}

func (v *PDFViewerPlus) ToggleFullscreenPlus() {
	if v.parentWin.FullScreen() {
		v.parentWin.SetFullScreen(false)
	} else {
		v.parentWin.SetFullScreen(true)
	}
}

// resizeWatcher wraps contentScroll to detect canvas resize without polling.
type resizeWatcher struct {
	widget.BaseWidget
	inner    fyne.CanvasObject
	onResize func(fyne.Size)
	viewer   *PDFViewerPlus
}

func (r *resizeWatcher) Resize(s fyne.Size) {
	r.BaseWidget.Resize(s)
	if r.onResize != nil && r.inner != nil {
		r.inner.Resize(s)
		r.onResize(s)
	}
}

func (r *resizeWatcher) MinSize() fyne.Size {
	if r.inner != nil {
		return r.inner.MinSize()
	}
	return fyne.NewSize(0, 0)
}

func (r *resizeWatcher) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.inner)
}

func (v *PDFViewerPlus) ToggleNightModePlus() {
	v.nightMode = !v.nightMode
	v.forceRenderPlus()
	v.renderCurrentPagePlus()
}

func (v *PDFViewerPlus) flattenBookmarks(items []pdfium_plus.BookmarkItem, parentID string) []widget.TreeNodeID {
	var ids []widget.TreeNodeID
	for i, bm := range items {
		id := fmt.Sprintf("%s_%d", parentID, i)
		v.bookmarkData[id] = bm
		if len(bm.Children) > 0 {
			v.bookmarkTreeIDs[id] = v.flattenBookmarks(bm.Children, id)
		}
		ids = append(ids, id)
	}
	return ids
}

func (v *PDFViewerPlus) loadBookmarks() {
	if v.pdfDoc == nil {
		return
	}
	bms, err := v.pdfDoc.GetBookmarksPlus()
	if err != nil || len(bms) == 0 {
		return
	}
	v.bookmarkData = make(map[string]pdfium_plus.BookmarkItem)
	v.bookmarkTreeIDs = make(map[string][]widget.TreeNodeID)
	v.bookmarkTreeIDs[""] = v.flattenBookmarks(bms, "")
	v.bookmarkTree.Refresh()
}

func (v *PDFViewerPlus) ToggleThumbnailsPlus() {
	if v.contentArea == nil {
		return
	}
	v.thumbVisible = !v.thumbVisible
	if v.thumbVisible {
		if v.sideTabs.CurrentTabIndex() == 1 && v.pdfDoc != nil {
			v.loadBookmarks()
		}
		rightPane := container.NewStack(v.resizeWatcher, v.scrollOverlay)
		split := container.NewHSplit(v.sideTabs, rightPane)
		split.Offset = 0.15
		v.contentSplit = split
		v.contentArea.Objects = []fyne.CanvasObject{split}
	} else {
		v.contentArea.Objects = []fyne.CanvasObject{v.resizeWatcher, v.scrollOverlay}
		v.contentSplit = nil
	}
	v.contentArea.Refresh()
	if v.searchHighlight != nil && v.pdfDoc != nil {
		if content, ok := v.contentScroll.Content.(*fyne.Container); ok {
			objs := make([]fyne.CanvasObject, 0, len(content.Objects))
			for _, o := range content.Objects {
				if o != v.searchHighlight {
					objs = append(objs, o)
				}
			}
			content.Objects = objs
		}
		v.searchHighlight.SetMatches(nil)
	}
}

func (v *PDFViewerPlus) CopyPageTextPlus() {
	if v.pdfDoc == nil || v.currentPage < 1 {
		return
	}

	text, err := v.pdfDoc.CopyPageTextPlus(v.currentPage - 1)
	if err != nil {
		dialog.ShowError(err, v.parentWin)
		return
	}

	if len(text) == 0 {
		if isTesseractAvailable() {
			v.OCRPageAndCopy()
			return
		}
		dialog.ShowInformation("提示", "本页没有可提取的文本", v.parentWin)
		return
	}

	v.parentWin.Clipboard().SetContent(text)
	dialog.ShowInformation("已复制", fmt.Sprintf("已复制第 %d 页文本到剪贴板 (%d 字符)", v.currentPage, len(text)), v.parentWin)
}

func (v *PDFViewerPlus) enableOCRSelection() {
	if v.pdfDoc == nil {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	img, cleanup, err := v.pdfDoc.RenderPageRaw(v.currentPage-1, ocrDPI, false)
	if err != nil {
		dialog.ShowError(fmt.Errorf("OCR 页面渲染失败: %v", err), v.parentWin)
		return
	}
	defer cleanup()

	v.textLayer.SetOCRMode(img, ocrDPI)

	v.textLayer.selectionEnabled = !v.textLayer.selectionEnabled
	v.textLayer.hitBoxes = nil
	v.textLayer.hitBoxesPage = -1

	if v.textLayer.selectionEnabled {
		v.selectBtn.Importance = widget.HighImportance
		v.selectBtn.Refresh()
	} else {
		v.selectBtn.Importance = widget.MediumImportance
		v.selectBtn.Refresh()
		v.textLayer.ClearSelection()
	}
}

func (v *PDFViewerPlus) OCRPageAndCopy() {
	if v.pdfDoc == nil {
		return
	}

	progress := widget.NewProgressBarInfinite()
	progressLabel := widget.NewLabel("正在 OCR 识别中...")
	progressBox := container.NewVBox(progressLabel, progress)
	progressDlg := dialog.NewCustomWithoutButtons("OCR 识别", progressBox, v.parentWin)
	progressDlg.Show()

	go func() {
		img, cleanup, err := v.pdfDoc.RenderPageRaw(v.currentPage-1, ocrDPI, false)
		if err != nil {
			fyne.Do(func() {
				progressDlg.Hide()
				dialog.ShowError(fmt.Errorf("OCR 页面渲染失败: %v", err), v.parentWin)
			})
			return
		}
		defer cleanup()

		text, err := ocrImage(img, 3)
		fyne.Do(func() {
			progressDlg.Hide()
			if err != nil || text == "" {
				dialog.ShowError(fmt.Errorf("OCR 识别失败: %v", err), v.parentWin)
				return
			}
			v.parentWin.Clipboard().SetContent(text)
			dialog.ShowInformation("OCR 已复制", fmt.Sprintf("已识别第 %d 页文本并复制到剪贴板 (%d 字符)", v.currentPage, len(text)), v.parentWin)
		})
	}()
}

func (v *PDFViewerPlus) ShowSearchDialogPlus() {
	if v.pdfDoc == nil {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	v.searchEntry = widget.NewEntry()
	v.searchEntry.SetPlaceHolder("输入搜索关键词...")

	v.searchLabel = widget.NewLabel("")
	v.searchLabel.Hide()

	currentSearchIdx := -1

	resultList := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {},
	)
	resultList.Hide()

	resultScroll := container.NewScroll(resultList)
	resultScroll.SetMinSize(fyne.NewSize(320, 300))

	searchBtn := widget.NewButton("搜索", nil)
	searchBtn.Importance = widget.HighImportance

	inputRow := container.NewBorder(nil, nil, nil, searchBtn, v.searchEntry)

	header := container.NewVBox(inputRow, v.searchLabel)

	prevBtn := widget.NewButtonWithIcon("上一个", theme.NavigateBackIcon(), nil)

	nextBtn := widget.NewButtonWithIcon("下一个", theme.NavigateNextIcon(), nil)

	navRow := container.NewHBox(prevBtn, widget.NewSeparator(), nextBtn)

	content := container.NewVBox(header, resultScroll, navRow)

	searchWin := fyne.CurrentApp().NewWindow("搜索")
	searchWin.SetContent(content)
	searchWin.SetPadded(true)
	searchWin.Resize(fyne.NewSize(350, 450))

	v.parentWin.SetOnClosed(func() {
		searchWin.Close()
	})

	searchWin.SetOnClosed(func() {
		v.clearSearchHighlight()
	})

	searchWin.Show()

	doSearch := func(query string) {
		if query == "" {
			return
		}

		v.clearSearchHighlight()
		v.searchQuery = query

		v.searchResultPosition = -1

		resultList.Length = func() int { return 0 }
		resultList.Refresh()

		go func() {
			results, err := v.pdfDoc.SearchPlus(query)
			fyne.Do(func() {
				if err != nil || len(results) == 0 {
					v.searchLabel.SetText("未找到: " + query)
					v.searchLabel.Show()
					return
				}

				totalMatches := len(results)
				pageSet := make(map[int]bool)
				for _, r := range results {
					pageSet[r.Page] = true
				}
				totalPages := len(pageSet)

				v.searchResults = results

				v.searchLabel.SetText(fmt.Sprintf("共 %d 页 %d 处匹配", totalPages, totalMatches))
				v.searchLabel.Show()

				resultList.Length = func() int { return len(results) }
				resultList.CreateItem = func() fyne.CanvasObject {
					return widget.NewLabel("")
				}

				resultList.UpdateItem = func(id widget.ListItemID, obj fyne.CanvasObject) {
					lbl := obj.(*widget.Label)
					currentIdx := 0
					for _, r := range v.searchResults {
						if currentIdx == id {
							pageNum := r.Page + 1
							ctx := strings.ReplaceAll(strings.ReplaceAll(r.Context, "\n", " "), "\r", " ")
							lbl.SetText(fmt.Sprintf("P%d [%d] %s", pageNum, r.MatchIndex+1, ctx))
							return
						}
						currentIdx++
					}
				}

				resultList.OnSelected = func(id widget.ListItemID) {
					currentIdx := 0
					for _, r := range v.searchResults {
						if currentIdx == id {
							v.searchResultPosition = currentIdx
							v.GoToPagePlus(r.Page + 1)
							return
						}
						currentIdx++
					}
				}

				resultList.Refresh()
				resultList.Show()

				navRow.Show()
				currentSearchIdx = 0

				if len(v.searchResults) > 0 {
					resultList.Select(0)
					v.searchResultPosition = 0
					v.GoToPagePlus(v.searchResults[0].Page + 1)
				}
			})
		}()
	}

	searchBtn.OnTapped = func() {
		doSearch(v.searchEntry.Text)
	}

	v.searchEntry.OnSubmitted = func(s string) {
		doSearch(s)
	}

	prevBtn.OnTapped = func() {
		if len(v.searchResults) == 0 || currentSearchIdx < 0 {
			return
		}
		currentSearchIdx--
		if currentSearchIdx < 0 {
			currentSearchIdx = len(v.searchResults) - 1
		}
		resultList.Select(widget.ListItemID(currentSearchIdx))
	}

	nextBtn.OnTapped = func() {
		if len(v.searchResults) == 0 || currentSearchIdx < 0 {
			return
		}
		currentSearchIdx++
		if currentSearchIdx >= len(v.searchResults) {
			currentSearchIdx = 0
		}
		resultList.Select(widget.ListItemID(currentSearchIdx))
	}
}

func (v *PDFViewerPlus) ShowExportDialogPlus() {
	if v.pdfDoc == nil {
		return
	}

	baseName := filepath.Base(v.filePath)
	defaultPrefix := baseName[:len(baseName)-len(filepath.Ext(baseName))]

	dpiOptions := []string{
		"72 DPI - 屏幕显示",
		"96 DPI - Windows标准",
		"150 DPI - 普通查看",
		"200 DPI - 较好质量",
		"300 DPI - 高质量打印",
		"400 DPI - 更高精度",
		"600 DPI - 专业级",
	}
	dpiPresets := []int{72, 96, 150, 200, 300, 400, 600}

	dpiSelect := widget.NewSelect(dpiOptions, nil)
	dpiSelect.SetSelected("300 DPI - 高质量打印")

	prefixEntry := widget.NewEntry()
	prefixEntry.SetText(defaultPrefix)
	prefixEntry.SetPlaceHolder("文件名前缀")

	startEntry := widget.NewEntry()
	startEntry.SetText("1")
	startEntry.SetPlaceHolder("起始页")

	endEntry := widget.NewEntry()
	endEntry.SetText(fmt.Sprintf("%d", v.totalPages))
	endEntry.SetPlaceHolder("结束页")

	formatSelect := widget.NewSelect([]string{"PNG", "JPEG"}, nil)
	formatSelect.SetSelected("PNG")

	mergeCheck := widget.NewCheck("合并为一张图片", nil)
	appendSelect := widget.NewSelect([]string{"竖向", "横向"}, nil)
	appendSelect.SetSelected("竖向")
	delCheck := widget.NewCheck("删除中间文件", nil)
	annotCheck := widget.NewCheck("包含标注", nil)
	annotCheck.SetChecked(true)

	content := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("文件名前缀:"), nil, prefixEntry),
		container.NewGridWithColumns(4,
			widget.NewLabel("DPI:"), dpiSelect,
			widget.NewLabel("格式:"), formatSelect,
			widget.NewLabel("起始页:"), startEntry,
			widget.NewLabel("结束页:"), endEntry,
		),
		container.NewHBox(mergeCheck, widget.NewLabel("方向:"), appendSelect),
		delCheck,
		annotCheck,
	)

	dialog.ShowForm("导出为图片", "确定", "取消", []*widget.FormItem{
		widget.NewFormItem("", content),
	}, func(ok bool) {
		if !ok {
			return
		}

		dpiIndex := dpiSelect.SelectedIndex()
		if dpiIndex < 0 || dpiIndex >= len(dpiPresets) {
			dpiIndex = 4
		}
		dpi := float64(dpiPresets[dpiIndex])

		prefix := prefixEntry.Text
		if prefix == "" {
			prefix = defaultPrefix
		}

		startPage, _ := strconv.Atoi(startEntry.Text)
		if startPage < 1 {
			startPage = 1
		}
		endPage, _ := strconv.Atoi(endEntry.Text)
		if endPage < startPage || endPage > v.totalPages {
			endPage = v.totalPages
		}

		formatVal := formatSelect.Selected
		ext := ".png"
		if formatVal == "JPEG" {
			ext = ".jpg"
		}

		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			saveDir := uri.Path()

			progress := widget.NewProgressBarInfinite()
			label := widget.NewLabel("正在导出图片...")
			progressDlg := dialog.NewCustomWithoutButtons("导出", container.NewVBox(label, progress), v.parentWin)
			progressDlg.Show()

			go func() {
				var exportedPaths []string

				for i := startPage; i <= endPage; i++ {
					fyne.Do(func() {
						label.SetText(fmt.Sprintf("正在导出第 %d/%d 页...", i, endPage))
					})

					img, cleanup, err := renderPageForExport(v.pdfDoc.doc, i-1, dpi, annotCheck.Checked, v.dirty, v.filePath)
					if err != nil || img == nil {
						if cleanup != nil {
							cleanup()
						}
						continue
					}

					outPath := filepath.Join(saveDir, fmt.Sprintf("%s_%d%s", prefix, i, ext))
					exportImagePlus(img, outPath, formatVal)

					if cleanup != nil {
						cleanup()
					}
					exportedPaths = append(exportedPaths, outPath)
				}

				fyne.Do(func() {
					progressDlg.Hide()
					showExportResult(v, saveDir, prefix, ext, exportedPaths, mergeCheck, appendSelect, delCheck)
				})
			}()
		}, v.parentWin)
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	}, v.parentWin)
}

func (v *PDFViewerPlus) ClosePlus() {
	if v.pdfDoc != nil {
		v.pdfDoc.Close()
		v.pdfDoc = nil
	}
	v.cleanupTempFilePlus()
}

func (v *PDFViewerPlus) CloseFilePlus() {
	// 停止后台渲染goroutine
	atomic.AddInt32(&v.renderVer, 1)
	if v.resizeTimer != nil {
		v.resizeTimer.Stop()
		v.resizeTimer = nil
	}
	v.ClosePlus()

	v.filePath = ""
	v.originalFilePath = ""
	v.totalPages = 0
	v.currentPage = 0
	v.forceRenderPlus()
	v.zoom = 1.0
	v.nightMode = false
	v.searchQuery = ""
	v.noteMode = false
	v.annotMode = false
	v.noteStore = NewNoteStore()
	v.dirty = false

	v.canvasImg = nil
	v.thumbCache = make(map[int]image.Image)
	v.lastPageWidth = 0
	v.lastPageHeight = 0
	v.contentScroll.Content = container.NewVBox(widget.NewLabel("  请点击「打开」按钮加载 PDF 文件"))
	v.contentScroll.Refresh()
	v.pageEntry.SetText("")
	v.pageTotalLabel.SetText("/ 0")
	v.updateTitle()
	if v.thumbList != nil {
		v.thumbList.Refresh()
	}
	v.bookmarkData = nil
	v.bookmarkTreeIDs = nil
	if v.bookmarkTree != nil {
		v.bookmarkTree.Refresh()
	}

	v.textLayer.ClearSelection()
	v.textLayer.selectionEnabled = false

	v.noteLayer.Hide()
	v.noteHoverLayer.Hide()
	v.annotBottomBar.Hide()
	v.annotToolLayer.SetAnnotMode(false)

	v.selectBtn.Importance = widget.MediumImportance
	v.selectBtn.Refresh()
	v.noteBtn.Importance = widget.MediumImportance
	v.noteBtn.Refresh()
	v.annotToggleBtn.Importance = widget.MediumImportance
	v.annotToggleBtn.Refresh()
	v.fitWidthBtn.Importance = widget.HighImportance
	v.fitWidthBtn.Refresh()
	v.fitPageBtn.Importance = widget.MediumImportance
	v.fitPageBtn.Refresh()
	v.fitWidthMode = true

	v.openBtn.SetText("打开")
	v.openBtn.SetIcon(theme.FolderOpenIcon())
	v.openBtn.Refresh()
}

func showExportResult(v *PDFViewerPlus, saveDir, prefix, ext string, exportedPaths []string,
	mergeCheck *widget.Check, appendSelect *widget.Select, delCheck *widget.Check) {

	if len(exportedPaths) == 0 {
		dialog.ShowInformation("导出", "没有页面被导出", v.parentWin)
		return
	}

	if mergeCheck.Checked && len(exportedPaths) > 1 {
		vertical := appendSelect.Selected == "竖向"
		mergedPath := filepath.Join(saveDir, prefix+"_all-in-one.png")

		var images []image.Image
		for _, p := range exportedPaths {
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			img, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				continue
			}
			images = append(images, img)
		}

		if len(images) > 1 {
			var totalW, totalH int
			if vertical {
				for _, img := range images {
					b := img.Bounds()
					totalH += b.Dy()
					if b.Dx() > totalW {
						totalW = b.Dx()
					}
				}
			} else {
				for _, img := range images {
					b := img.Bounds()
					totalW += b.Dx()
					if b.Dy() > totalH {
						totalH = b.Dy()
					}
				}
			}

			merged := image.NewRGBA(image.Rect(0, 0, totalW, totalH))
			var offsetY, offsetX int
			for _, img := range images {
				b := img.Bounds()
				draw.Draw(merged, image.Rect(offsetX, offsetY, offsetX+b.Dx(), offsetY+b.Dy()), img, b.Min, draw.Src)
				if vertical {
					offsetY += b.Dy()
				} else {
					offsetX += b.Dx()
				}
			}

			mf, err := os.Create(mergedPath)
			if err == nil {
				png.Encode(mf, merged)
				mf.Close()
			}

			if delCheck.Checked {
				for _, p := range exportedPaths {
					os.Remove(p)
				}
			}
			dialog.ShowInformation("导出完成", fmt.Sprintf("已导出 %d 页并合并", len(images)), v.parentWin)
		} else {
			dialog.ShowInformation("导出完成", fmt.Sprintf("已导出 %d 页", len(exportedPaths)), v.parentWin)
		}
	} else {
		dialog.ShowInformation("导出完成", fmt.Sprintf("已导出 %d 页到: %s", len(exportedPaths), saveDir), v.parentWin)
	}
}

func exportImagePlus(img image.Image, path string, format string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	if format == "PNG" {
		png.Encode(f, img)
	} else {
		jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
	}
}

func (v *PDFViewerPlus) ExportCurrentPagePNGPlus() {
	if v.pdfDoc == nil {
		return
	}

	annotCheck := widget.NewCheck("包含标注", nil)
	annotCheck.SetChecked(true)
	content := container.NewVBox(
		widget.NewLabel("将当前页导出为 PNG"),
		widget.NewSeparator(),
		annotCheck,
	)
	dialog.ShowCustomConfirm("转PNG", "确定", "取消", content, func(ok bool) {
		if !ok {
			return
		}
		includeAnnots := annotCheck.Checked

		img, cleanup, err := renderPageForExport(v.pdfDoc.doc, v.currentPage-1, 300.0, includeAnnots, v.dirty, v.filePath)
		if err != nil || img == nil {
			dialog.ShowError(fmt.Errorf("渲染失败"), v.parentWin)
			return
		}
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		baseName := filepath.Base(v.filePath)
		defaultName := baseName[:len(baseName)-len(filepath.Ext(baseName))]
		defaultName = fmt.Sprintf("%s_page_%d.png", defaultName, v.currentPage)

		saveDialog := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			if err != nil || uri == nil {
				return
			}
			defer uri.Close()

			if err := png.Encode(uri, img); err != nil {
				dialog.ShowError(err, v.parentWin)
				return
			}

			savedPath := uri.URI().Path()
			if filepath.Ext(savedPath) != ".png" {
				newPath := savedPath + ".png"
				os.Rename(savedPath, newPath)
				savedPath = newPath
			}

			dialog.ShowInformation("成功", fmt.Sprintf("已保存到: %s", savedPath), v.parentWin)
		}, v.parentWin)

		saveDialog.SetFileName(defaultName)
		saveDialog.Resize(fyne.NewSize(800, 600))
		saveDialog.Show()
	}, v.parentWin)
}

func (v *PDFViewerPlus) ShowPrintDialog() {
	if v.pdfDoc == nil || v.filePath == "" {
		dialog.ShowInformation("提示", "请先打开PDF文件", v.parentWin)
		return
	}

	var allPrinters []string
	selectedPrinter := 0

	printerList := widget.NewList(
		func() int { return len(allPrinters) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < len(allPrinters) {
				obj.(*widget.Label).SetText(allPrinters[id])
			}
		},
	)
	printerList.OnSelected = func(id widget.ListItemID) {
		selectedPrinter = id
	}

	go func() {
		printers, err := listPrinters()
		if err != nil || len(printers) == 0 {
			fyne.Do(func() {
				printerList.Refresh()
			})
			return
		}
		fyne.Do(func() {
			allPrinters = printers
			printerList.Refresh()
			if len(printers) > 0 {
				printerList.Select(0)
			}
		})
	}()

	pageAll := widget.NewRadioGroup([]string{"全部页面", "当前页", "页码范围"}, func(s string) {})
	pageAll.SetSelected("全部页面")
	pageEntry := widget.NewEntry()
	pageEntry.SetPlaceHolder("如: 1-3,5,7")
	pageEntry.Disable()
	pageAll.OnChanged = func(s string) {
		if s == "页码范围" {
			pageEntry.Enable()
		} else {
			pageEntry.Disable()
		}
	}
	pageInfo := widget.NewLabel(fmt.Sprintf("共 %d 页，当前第 %d 页", v.totalPages, v.currentPage))

	copiesEntry := widget.NewEntry()
	copiesEntry.SetText("1")
	copiesEntry.Validator = nil
	decCopies := widget.NewButton("-", func() {
		n, _ := strconv.Atoi(copiesEntry.Text)
		if n > 1 {
			copiesEntry.SetText(fmt.Sprintf("%d", n-1))
		}
	})
	incCopies := widget.NewButton("+", func() {
		n, _ := strconv.Atoi(copiesEntry.Text)
		copiesEntry.SetText(fmt.Sprintf("%d", n+1))
	})
	copiesBar := container.NewHBox(widget.NewLabel("份数:"), decCopies, copiesEntry, incCopies)

	chkCollate := widget.NewCheck("逐份", nil)
	chkCollate.SetChecked(true)
	chkReverse := widget.NewCheck("逆序", nil)

	duplexSel := widget.NewSelect([]string{"单面打印", "双面(长边)", "双面(短边)"}, nil)
	duplexSel.SetSelected("双面(长边)")

	colorSel := widget.NewSelect([]string{"彩色", "灰度"}, nil)
	colorSel.SetSelected("彩色")

	numberUpSel := widget.NewSelect([]string{"1", "2", "4", "6", "9", "16"}, nil)
	numberUpSel.SetSelected("1")

	mediaSel := widget.NewSelect([]string{"自动", "A4", "A3", "A5", "Letter", "Legal", "B5"}, nil)
	mediaSel.SetSelected("自动")

	listWrap := &minSizeWrap{child: printerList, minH: 110}
	printerBox := container.NewBorder(
		widget.NewLabelWithStyle("打印机:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		listWrap,
	)

	leftCol := container.NewVBox(
		widget.NewLabelWithStyle("打印范围", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pageAll,
		pageEntry,
		pageInfo,
	)
	rightCol := container.NewVBox(
		widget.NewLabelWithStyle("副本", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		copiesBar,
		container.NewHBox(chkCollate, chkReverse),
		container.NewHBox(widget.NewLabel("双面打印:"), duplexSel),
		container.NewHBox(widget.NewLabel("颜色模式:"), colorSel),
		container.NewHBox(widget.NewLabel("每面页数:"), numberUpSel),
		container.NewHBox(widget.NewLabel("纸张大小:"), mediaSel),
	)
	midGrid := container.NewGridWithColumns(2, leftCol, rightCol)

	doPrint := func() {
		pages := ""
		switch pageAll.Selected {
		case "全部页面":
			pages = "all"
		case "当前页":
			pages = fmt.Sprintf("%d", v.currentPage)
		case "页码范围":
			pages = pageEntry.Text
			if strings.TrimSpace(pages) == "" {
				dialog.ShowInformation("提示", "请输入页码范围", v.parentWin)
				return
			}
		}
		copies, _ := strconv.Atoi(copiesEntry.Text)
		if copies < 1 {
			copies = 1
		}

		duplex := ""
		if duplexSel.Selected == "双面(长边)" {
			duplex = "long-edge"
		} else if duplexSel.Selected == "双面(短边)" {
			duplex = "short-edge"
		}

		numberUp, _ := strconv.Atoi(numberUpSel.Selected)
		if numberUp < 1 {
			numberUp = 1
		}

		media := ""
		if mediaSel.Selected != "自动" {
			media = mediaSel.Selected
		}

		opts := PrintOptions{
			Printer:   "",
			Copies:    copies,
			Pages:     "all",
			Reverse:   chkReverse.Checked,
			Collate:   chkCollate.Checked,
			Duplex:    duplex,
			Grayscale: colorSel.Selected == "灰度",
			NumberUp:  numberUp,
			MediaSize: media,
		}
		if selectedPrinter < len(allPrinters) {
			opts.Printer = allPrinters[selectedPrinter]
		}

		go func() {
			tmpDir := os.TempDir()
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("gtpdf_print_%d.pdf", time.Now().UnixNano()))
			defer os.Remove(tmpFile)

			printPages := parsePrintPages(pages, v.totalPages)
			if len(printPages) == 0 {
				fyne.Do(func() {
					dialog.ShowInformation("提示", "没有需要打印的页面", v.parentWin)
				})
				return
			}

			pdfData, err := v.flattenForPrint(printPages)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("生成打印文档失败: %w", err), v.parentWin)
				})
				return
			}
			if err := os.WriteFile(tmpFile, pdfData, 0644); err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("写入临时文件失败: %w", err), v.parentWin)
				})
				return
			}

			err = printPDF(tmpFile, opts)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("打印失败: %w", err), v.parentWin)
				})
				return
			}
			fyne.Do(func() {
				dialog.ShowInformation("打印", "打印任务已提交", v.parentWin)
			})
		}()
	}

	dialogContent := container.NewVBox(
		printerBox,
		widget.NewSeparator(),
		midGrid,
	)

	dialog.ShowCustomConfirm("打印", "打印", "取消", dialogContent, func(ok bool) {
		if ok {
			doPrint()
		}
	}, v.parentWin)
}

func (v *PDFViewerPlus) ExtractPageImagesPlus() {
	if v.pdfDoc == nil || v.filePath == "" {
		return
	}

	pageStr := fmt.Sprintf("%d", v.currentPage)
	conf := model.NewDefaultConfiguration()

	f, err := os.Open(v.filePath)
	if err != nil {
		dialog.ShowError(err, v.parentWin)
		return
	}
	defer f.Close()

	images, err := api.ExtractImagesRaw(f, []string{pageStr}, conf)
	if err != nil {
		dialog.ShowError(err, v.parentWin)
		return
	}

	if len(images) == 0 || len(images[0]) == 0 {
		dialog.ShowInformation("提示", "本页没有图片", v.parentWin)
		return
	}

	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		outDir := uri.Path()

		err = api.ExtractImagesFile(v.filePath, outDir, []string{pageStr}, conf)
		if err != nil {
			dialog.ShowError(err, v.parentWin)
			return
		}

		dialog.ShowInformation("成功", fmt.Sprintf("图片已导出到: %s", outDir), v.parentWin)
	}, v.parentWin)
	fd.Resize(fyne.NewSize(800, 600))
	fd.Show()
}

// renderPageForExport renders a page from a temporary document copy, so the
// original document's annotation state is not affected by FPDF_RENDER_FLAG_ANNOT.
func renderPageForExport(doc *pdfium_plus.PDFiumDocument, pageIdx int, dpi float64, annots bool, dirty bool, filePath string) (image.Image, func(), error) {
	if !annots {
		return doc.RenderPage(pageIdx, dpi, false)
	}

	// 检测 FreeText 标注
	hasFreeText := false
	if list, err := doc.GetAnnotations(pageIdx); err == nil {
		for _, a := range list {
			if a.Type == "FreeText" {
				hasFreeText = true
				break
			}
		}
	}

	if hasFreeText && dirty {
		// 有 FreeText 且有修改：fixFreeTextAPBytes → 写临时文件 → 打开 → 渲染
		buf, err := doc.SaveAsCopyToBuffer()
		if err != nil {
			return nil, nil, err
		}
		buf, _ = fixFreeTextAPBytes(buf)

		tmpFile, err := os.CreateTemp("", "gtpdf_export_*.pdf")
		if err != nil {
			return nil, nil, err
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.Write(buf); err != nil {
			tmpFile.Close(); os.Remove(tmpPath)
			return nil, nil, err
		}
		tmpFile.Close()
		defer os.Remove(tmpPath)

		tmpDoc, err := pdfium_plus.OpenDocument(tmpPath)
		if err != nil {
			return nil, nil, err
		}
		resp, err := tmpDoc.RenderPageWithAllAnnots(pageIdx, dpi)
		if err != nil {
			tmpDoc.Close()
			return nil, nil, err
		}
		cl := resp.CleanupFunc
		return resp.Result.Image, func() {
			if cl != nil { cl() }
			tmpDoc.Close()
		}, nil
	}

	if hasFreeText && filePath != "" {
		// 有 FreeText 但已保存：直接从已保存文件打开（fixFreeTextAPBytes已在上次保存时执行过）
		tmpDoc, err := pdfium_plus.OpenDocument(filePath)
		if err != nil {
			return nil, nil, err
		}
		resp, err := tmpDoc.RenderPageWithAllAnnots(pageIdx, dpi)
		if err != nil {
			tmpDoc.Close()
			return nil, nil, err
		}
		cl := resp.CleanupFunc
		return resp.Result.Image, func() {
			if cl != nil { cl() }
			tmpDoc.Close()
		}, nil
	}

	// 无 FreeText 或 无文件路径：简易 buffer → OpenDocumentFromBuffer
	buf, err := doc.SaveAsCopyToBuffer()
	if err != nil {
		return nil, nil, err
	}
	tmpDoc, err := pdfium_plus.OpenDocumentFromBuffer(buf)
	if err != nil {
		return nil, nil, err
	}
	resp, err := tmpDoc.RenderPageWithAllAnnots(pageIdx, dpi)
	if err != nil {
		tmpDoc.Close()
		return nil, nil, err
	}
	cl := resp.CleanupFunc
	return resp.Result.Image, func() {
		if cl != nil { cl() }
		tmpDoc.Close()
	}, nil
}

type minSizeWrap struct {
	widget.BaseWidget
	child fyne.CanvasObject
	minW  float32
	minH  float32
}

func (m *minSizeWrap) MinSize() fyne.Size {
	c := m.child.MinSize()
	return fyne.NewSize(
		max(c.Width, m.minW),
		max(c.Height, m.minH),
	)
}

func (m *minSizeWrap) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.child)
}
