package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const virtBufBefore = 3
const virtBufAfter = 6
const virtPoolSize = virtBufBefore + 1 + virtBufAfter

type virtualScrollPage struct {
	widget.BaseWidget
	viewer *PDFViewerPlus
}

func newVirtualScrollPage(viewer *PDFViewerPlus) *virtualScrollPage {
	v := &virtualScrollPage{viewer: viewer}
	v.ExtendBaseWidget(v)
	return v
}

func (v *virtualScrollPage) Resize(size fyne.Size) {
	v.BaseWidget.Resize(size)
	v.Refresh()
}

func (v *virtualScrollPage) MinSize() fyne.Size {
	n := len(v.viewer.pageHeights)
	if n == 0 {
		return fyne.NewSize(100, 0)
	}
	totalH := v.viewer.pageYOffsets[n-1] + v.viewer.pageHeights[n-1] + 4
	w := float32(595)
	if len(v.viewer.pageImages) > 0 && v.viewer.pageImages[0] != nil {
		w = v.viewer.pageImages[0].MinSize().Width
	}
	return fyne.NewSize(w, totalH)
}

func (v *virtualScrollPage) CreateRenderer() fyne.WidgetRenderer {
	pool := make([]*fyne.Container, virtPoolSize)
	assigned := make([]int, virtPoolSize)
	for i := range pool {
		pool[i] = container.NewStack()
		assigned[i] = -1
	}
	seps := make([]*canvas.Rectangle, virtPoolSize)
	for i := range seps {
		seps[i] = canvas.NewRectangle(color.NRGBA{0, 0, 0, 25})
	}
	return &virtualScrollRenderer{
		vpage:    v,
		pool:     pool,
		assigned: assigned,
		seps:     seps,
	}
}

type virtualScrollRenderer struct {
	vpage    *virtualScrollPage
	pool     []*fyne.Container
	assigned []int
	seps     []*canvas.Rectangle
}

func (r *virtualScrollRenderer) Layout(size fyne.Size) {
	viewer := r.vpage.viewer
	if viewer == nil || viewer.contentScroll == nil || len(viewer.pageYOffsets) == 0 {
		return
	}

	offsetY := viewer.contentScroll.Offset.Y
	viewportH := viewer.contentScroll.Size().Height
	if viewportH <= 0 {
		viewportH = size.Height
	}
	if viewportH <= 0 {
		return
	}

	n := len(viewer.pageYOffsets)
	firstVis := sort.Search(n, func(i int) bool {
		return viewer.pageYOffsets[i]+viewer.pageHeights[i] > offsetY
	})
	if firstVis < 0 {
		firstVis = 0
	}
	lastVis := firstVis
	for lastVis < n && viewer.pageYOffsets[lastVis] < offsetY+viewportH {
		lastVis++
	}
	lastVis--

	firstVis -= virtBufBefore
	if firstVis < 0 {
		firstVis = 0
	}
	lastVis += virtBufAfter
	if lastVis >= n {
		lastVis = n - 1
	}
	if lastVis-firstVis+1 > virtPoolSize {
		lastVis = firstVis + virtPoolSize - 1
	}
	for lastVis-firstVis+1 < virtPoolSize && lastVis < n-1 {
		lastVis++
	}

	logD("[virt-layout] offY=%.0f vpH=%.0f first=%d last=%d n=%d", offsetY, viewportH, firstVis+1, lastVis+1, n)

	for i, pg := range r.assigned {
		if pg >= 0 && (pg < firstVis || pg > lastVis) {
			for len(r.pool[i].Objects) > 0 {
				r.pool[i].Remove(r.pool[i].Objects[0])
			}
			r.assigned[i] = -1
			r.seps[i].Hide()
		}
	}

	for pg := firstVis; pg <= lastVis; pg++ {
		slotIdx := -1
		for i, p := range r.assigned {
			if p == pg {
				slotIdx = i
				break
			}
		}
		if slotIdx < 0 {
			for i, p := range r.assigned {
				if p < 0 {
					slotIdx = i
					break
				}
			}
		}
		if slotIdx < 0 {
			continue
		}

		viewer.ensureTextLayer(pg)

		if r.assigned[slotIdx] != pg {
			for len(r.pool[slotIdx].Objects) > 0 {
				r.pool[slotIdx].Remove(r.pool[slotIdx].Objects[0])
			}
			stack := viewer.pageStacks[pg]
			if stack != nil {
				r.pool[slotIdx].Add(stack)
				r.assigned[slotIdx] = pg
			}
		}

		if !viewer.scrollDragging {
			full := viewer.getFullImage(pg)
			if full != nil && pg < len(viewer.pageImages) && viewer.pageImages[pg] != nil && viewer.pageImages[pg].Image != full {
				viewer.pageImages[pg].Image = full
				viewer.pageImages[pg].Refresh()
			} else if full == nil && pg >= 0 && pg < len(viewer.pageImages) && viewer.pageImages[pg] != nil {
				viewer.renderContinuousPage(pg)
			}
		}

		slot := r.pool[slotIdx]
		slot.Move(fyne.NewPos(0, viewer.pageYOffsets[pg]))
		slot.Resize(fyne.NewSize(size.Width, viewer.pageHeights[pg]))
		slot.Show()

		if pg < lastVis {
			sep := r.seps[slotIdx]
			sepY := viewer.pageYOffsets[pg] + viewer.pageHeights[pg] + 1
			sep.Move(fyne.NewPos(0, sepY))
			sep.Resize(fyne.NewSize(size.Width, 4))
			sep.Show()
		}
	}

	{
		var sb strings.Builder
		sb.WriteString("[virt-assign]")
		for i, pg := range r.assigned {
			if pg >= 0 {
				fmt.Fprintf(&sb, " [%d]=%d", i, pg+1)
			}
		}
		logD("%s", sb.String())
	}

	for i, pg := range r.assigned {
		if pg < 0 {
			r.pool[i].Hide()
			r.seps[i].Hide()
		}
	}
}

func (r *virtualScrollRenderer) MinSize() fyne.Size {
	return r.vpage.MinSize()
}

func (r *virtualScrollRenderer) Refresh() {
	r.Layout(r.vpage.Size())
}

func (r *virtualScrollRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.pool)+len(r.seps))
	for i := range r.pool {
		objs = append(objs, r.pool[i])
	}
	for i := range r.seps {
		objs = append(objs, r.seps[i])
	}
	return objs
}

func (r *virtualScrollRenderer) Destroy() {}
