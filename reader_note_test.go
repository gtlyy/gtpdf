package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestNoteStore_New(t *testing.T) {
	s := NewNoteStore()
	if s == nil {
		t.Fatal("NewNoteStore() returned nil")
	}
	notes := s.GetAll()
	if len(notes) != 0 {
		t.Errorf("expected empty store, got %d notes", len(notes))
	}
}

func TestNoteStore_AddAndGetAll(t *testing.T) {
	s := NewNoteStore()
	n1 := PDFNote{ID: "1", Page: 0, Text: "hello"}
	n2 := PDFNote{ID: "2", Page: 1, Text: "world"}

	s.Add(n1)
	s.Add(n2)

	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(all))
	}
	if all[0].ID != "1" || all[1].ID != "2" {
		t.Errorf("unexpected order: %+v", all)
	}
}

func TestNoteStore_GetByPage(t *testing.T) {
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "page0a"})
	s.Add(PDFNote{ID: "2", Page: 1, Text: "page1"})
	s.Add(PDFNote{ID: "3", Page: 0, Text: "page0b"})

	page0 := s.GetByPage(0)
	if len(page0) != 2 {
		t.Fatalf("expected 2 notes on page 0, got %d", len(page0))
	}
	if page0[0].Text != "page0a" || page0[1].Text != "page0b" {
		t.Errorf("page 0 notes = %+v", page0)
	}

	page1 := s.GetByPage(1)
	if len(page1) != 1 || page1[0].ID != "2" {
		t.Errorf("page 1 notes = %+v", page1)
	}

	page2 := s.GetByPage(2)
	if len(page2) != 0 {
		t.Errorf("expected empty for page 2, got %d", len(page2))
	}
}

func TestNoteStore_Remove(t *testing.T) {
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "a"})
	s.Add(PDFNote{ID: "2", Page: 0, Text: "b"})
	s.Add(PDFNote{ID: "3", Page: 0, Text: "c"})

	s.Remove("2")
	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 notes after remove, got %d", len(all))
	}
	if all[0].ID != "1" || all[1].ID != "3" {
		t.Errorf("remaining = %+v", all)
	}

	s.Remove("nonexistent")
	if len(s.GetAll()) != 2 {
		t.Error("removing nonexistent ID should not change count")
	}
}

func TestNoteStore_Update(t *testing.T) {
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "old"})

	s.Update("1", "new")
	all := s.GetAll()
	if len(all) != 1 || all[0].Text != "new" {
		t.Errorf("after update = %+v", all)
	}

	s.Update("nonexistent", "x")
	if s.GetAll()[0].Text != "new" {
		t.Error("updating nonexistent ID should not change anything")
	}
}

func TestNoteStore_GetAllIsolation(t *testing.T) {
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "test"})

	all := s.GetAll()
	all[0].Text = "modified"

	original := s.GetAll()
	if original[0].Text != "test" {
		t.Error("GetAll() should return a copy, not a reference")
	}
}

func TestNoteStore_SetFilePath(t *testing.T) {
	s := NewNoteStore()
	s.SetFilePath("/tmp/test.pdf")
	// 无法直接检查 filePath（unexported），但验证不崩溃即可
	s.Add(PDFNote{ID: "1", Page: 0, Text: "x"})
	if len(s.GetAll()) != 1 {
		t.Error("after SetFilePath, store should still work")
	}
}

func TestNoteStore_SaveAndLoad(t *testing.T) {
	tmpFile := t.TempDir() + "/test.pdf"
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "hello", Color: "#FFD600"})
	s.Add(PDFNote{ID: "2", Page: 1, Text: "world", Color: "#2196F3"})

	s.SetFilePath(tmpFile)
	if err := s.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load into a new store
	s2 := NewNoteStore()
	if err := s2.Load(tmpFile); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	all := s2.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 notes after load, got %d", len(all))
	}
	if all[0].Text != "hello" || all[1].Text != "world" {
		t.Errorf("loaded notes = %+v", all)
	}
}

func TestNoteStore_LoadNonExistent(t *testing.T) {
	s := NewNoteStore()
	err := s.Load("/nonexistent/file.pdf")
	if err == nil {
		t.Error("expected error loading non-existent file")
	}
}

func TestPDFNoteJSONRoundTrip(t *testing.T) {
	original := PDFNote{
		ID:        "test-id",
		Page:      5,
		PdfX:      100.5,
		PdfY:      200.75,
		Text:      "Hello 世界",
		Color:     "#FFD600",
		Type:      "text",
		CreatedAt: 1700000000,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PDFNote
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("round-trip = %+v, want %+v", decoded, original)
	}
}

func TestNoteStore_SaveNoFilePath(t *testing.T) {
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "test"})
	err := s.Save()
	if err != nil {
		t.Fatalf("Save() without filePath should not error: %v", err)
	}
}

func TestNoteStore_LoadMissingSidecar(t *testing.T) {
	tmpFile := t.TempDir() + "/test.pdf"
	s := NewNoteStore()
	err := s.Load(tmpFile)
	if err == nil {
		t.Error("expected error when sidecar file doesn't exist")
	}
}

func TestNoteStore_SidecarPath(t *testing.T) {
	// 验证 sidecar 文件路径格式
	tmpFile := t.TempDir() + "/doc.pdf"
	s := NewNoteStore()
	s.Add(PDFNote{ID: "1", Page: 0, Text: "test"})
	s.SetFilePath(tmpFile)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	// 确认 sidecar 文件存在
	sidecar := tmpFile + ".gtpdf.json"
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Errorf("sidecar file %s was not created", sidecar)
	}
}
