package filecommands

import (
	"context"
	"testing"

	"ai-smart-storage/internal/database"
)

func TestParse(t *testing.T) {
	cases := []struct {
		text      string
		found     bool
		err       bool
		action    Action
		target    Target
		all       bool
		positions []int
		query     string
	}{
		{text: "kirim semua file", found: true, action: ActionSend, target: TargetFiles, all: true},
		{text: "download semua foto", found: true, action: ActionDownload, target: TargetImages, all: true},
		{text: "unduh file 1, 2, 3", found: true, action: ActionDownload, target: TargetFiles, positions: []int{1, 2, 3}},
		{text: "kirim gambar 4", found: true, action: ActionSend, target: TargetImages, positions: []int{4}},
		{text: "kirim file laporan", found: true, action: ActionSend, target: TargetFiles, query: "laporan"},
		{text: "tolong kirim file laporan", found: false},
		{text: "kirim file 0,2", found: true, err: true},
		{text: "kirim file 1,1", found: true, err: true},
	}
	for _, tc := range cases {
		got, found, err := Parse(tc.text)
		if found != tc.found || (err != nil) != tc.err {
			t.Errorf("Parse(%q) = (%+v, %v, %v), want found=%v err=%v", tc.text, got, found, err, tc.found, tc.err)
			continue
		}
		if err != nil {
			continue
		}
		if got.Action != tc.action || got.Target != tc.target || got.All != tc.all || got.Query != tc.query {
			t.Errorf("Parse(%q) = %+v, want action=%q target=%q all=%v query=%q", tc.text, got, tc.action, tc.target, tc.all, tc.query)
		}
		if len(got.Positions) != len(tc.positions) {
			t.Errorf("Parse(%q) positions = %v, want %v", tc.text, got.Positions, tc.positions)
		}
		for i := range got.Positions {
			if got.Positions[i] != tc.positions[i] {
				t.Errorf("Parse(%q) positions = %v, want %v", tc.text, got.Positions, tc.positions)
				break
			}
		}
	}
}

type sourceStub struct {
	documents []database.Document
	pages     int
}

func (s *sourceStub) Documents(_ context.Context, _ uint64, limit, offset int) ([]database.Document, error) {
	s.pages++
	if offset >= len(s.documents) {
		return nil, nil
	}
	end := offset + limit
	if end > len(s.documents) {
		end = len(s.documents)
	}
	return s.documents[offset:end], nil
}

func (s *sourceStub) SearchDocuments(context.Context, uint64, string, int) ([]database.Document, error) {
	return nil, nil
}

func TestSelectPagesAndFiltersImages(t *testing.T) {
	documents := make([]database.Document, 0, 101)
	for i := 0; i < 101; i++ {
		mime := "application/pdf"
		if i%2 == 0 {
			mime = "image/jpeg"
		}
		documents = append(documents, database.Document{FileName: "file", MimeType: mime})
	}
	source := &sourceStub{documents: documents}
	selected, err := Select(context.Background(), source, 1, Command{All: true, Target: TargetImages})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 51 || source.pages != 2 {
		t.Fatalf("selected %d images across %d pages, want 51 across 2", len(selected), source.pages)
	}
}
