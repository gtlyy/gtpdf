package main

import (
	"image"
	"sync"
	"testing"
)

func TestRingBufferBasic(t *testing.T) {
	v := &PDFViewerPlus{
		pageFullIndex: make(map[int]int),
		ringMu:        sync.Mutex{},
	}
	for i := range v.pageFullPages {
		v.pageFullPages[i] = -1
	}

	img0 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img1 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img2 := image.NewRGBA(image.Rect(0, 0, 10, 10))

	v.fillRingSlotLocked(0, img0)
	v.fillRingSlotLocked(1, img1)
	v.fillRingSlotLocked(2, img2)

	if v.fillCount != 3 {
		t.Fatalf("fillCount = %d, want 3", v.fillCount)
	}
	if v.fillHead != 3 {
		t.Errorf("fillHead = %d, want 3", v.fillHead)
	}

	if got := v.getFullImage(0); got != img0 {
		t.Error("getFullImage(0) mismatch")
	}
	if got := v.getFullImage(1); got != img1 {
		t.Error("getFullImage(1) mismatch")
	}
	if got := v.getFullImage(2); got != img2 {
		t.Error("getFullImage(2) mismatch")
	}
	if got := v.getFullImage(3); got != nil {
		t.Error("getFullImage(3) should be nil")
	}
}

func TestRingOverflowEvictsOldest(t *testing.T) {
	v := &PDFViewerPlus{
		pageFullIndex: make(map[int]int),
		ringMu:        sync.Mutex{},
	}
	for i := range v.pageFullPages {
		v.pageFullPages[i] = -1
	}

	n := 205
	imgs := make([]*image.RGBA, n)
	for i := range imgs {
		imgs[i] = image.NewRGBA(image.Rect(0, 0, 10, 10))
	}
	for i := 0; i < n; i++ {
		v.fillRingSlotLocked(i, imgs[i])
	}

	if v.fillCount != 200 {
		t.Fatalf("fillCount = %d, want 200", v.fillCount)
	}

	for i := 0; i < 5; i++ {
		if got := v.getFullImage(i); got != nil {
			t.Errorf("page %d should be evicted, got image", i)
		}
	}
	for i := 5; i < n; i++ {
		if got := v.getFullImage(i); got != imgs[i] {
			t.Errorf("page %d mismatch after eviction", i)
		}
	}
}

func TestRingThumbnailCreated(t *testing.T) {
	v := &PDFViewerPlus{
		pageFullIndex: make(map[int]int),
		ringMu:        sync.Mutex{},
	}
	for i := range v.pageFullPages {
		v.pageFullPages[i] = -1
	}

	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	v.fillRingSlotLocked(42, img)

	thumb := v.getThumbImage(42)
	if thumb == nil {
		t.Fatal("getThumbImage(42) = nil, want non-nil")
	}
	if thumb.Bounds().Dx() != 15 || thumb.Bounds().Dy() != 30 {
		t.Errorf("thumbnail size = %dx%d, want 15x30",
			thumb.Bounds().Dx(), thumb.Bounds().Dy())
	}
}

func TestRingUpdateExistingSlot(t *testing.T) {
	v := &PDFViewerPlus{
		pageFullIndex: make(map[int]int),
		ringMu:        sync.Mutex{},
	}
	for i := range v.pageFullPages {
		v.pageFullPages[i] = -1
	}

	oldImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
	newImg := image.NewRGBA(image.Rect(0, 0, 10, 10))

	v.fillRingSlotLocked(99, oldImg)
	v.fillRingSlotLocked(99, newImg)

	if v.fillCount != 1 {
		t.Errorf("fillCount = %d, want 1 (should reuse slot)", v.fillCount)
	}
	if got := v.getFullImage(99); got != newImg {
		t.Error("getFullImage(99) should return newImg after update")
	}
}

func TestGetPageYOffset(t *testing.T) {
	v := &PDFViewerPlus{
		pageHeights: []float32{100, 150, 200, 120, 180},
	}
	v.pageYOffsets = make([]float32, len(v.pageHeights))
	var acc float32
	for i, h := range v.pageHeights {
		v.pageYOffsets[i] = acc
		acc += h + 4
	}

	tests := []struct {
		page int
		want float32
	}{
		{0, 0},
		{1, 104},
		{2, 258},
		{3, 462},
		{4, 586},
	}
	for _, tt := range tests {
		got := v.getPageYOffset(tt.page)
		if got != tt.want {
			t.Errorf("getPageYOffset(%d) = %.0f, want %.0f", tt.page, got, tt.want)
		}
	}
}
