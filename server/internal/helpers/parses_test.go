package helpers

import (
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty string returns nil",
			in:   "",
			want: nil,
		},
		{
			name: "single value",
			in:   "http://localhost:1420",
			want: []string{"http://localhost:1420"},
		},
		{
			name: "trims whitespace and skips empty values",
			in:   " http://localhost:1420 , , http://localhost:3000 ,, tauri://localhost ",
			want: []string{"http://localhost:1420", "http://localhost:3000", "tauri://localhost"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitCSV(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitCSV() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
