package main

import (
	"reflect"
	"testing"
)

func TestParsePrintPages(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		totalPages int
		want       []int
	}{
		{"empty_spec", "", 5, []int{0, 1, 2, 3, 4}},
		{"all", "all", 3, []int{0, 1, 2}},
		{"all_zero_pages", "all", 0, []int{}},
		{"single_page", "1", 5, []int{0}},
		{"multiple_pages", "1,3,5", 5, []int{0, 2, 4}},
		{"range", "1-3", 5, []int{0, 1, 2}},
		{"mixed_range_and_singles", "1-3,5,7-9", 10, []int{0, 1, 2, 4, 6, 7, 8}},
		{"out_of_range_high", "1-100", 5, []int{0, 1, 2, 3, 4}},
		{"out_of_range_low", "0,1,-1", 5, []int{0}},
		{"reverse_range", "3-1", 5, nil},
		{"duplicates", "1,1,2,2,3", 5, []int{0, 1, 2}},
		{"unsorted_input", "5,1,3", 5, []int{0, 2, 4}},
		{"with_spaces", " 1 , 3-5 ", 5, []int{0, 2, 3, 4}},
		{"empty_parts", "1,,3", 5, []int{0, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePrintPages(tt.spec, tt.totalPages)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePrintPages(%q,%d) = %v, want %v", tt.spec, tt.totalPages, got, tt.want)
			}
		})
	}
}
