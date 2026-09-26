package wails

import (
	"encoding/json"
	"testing"
)

func TestEmptyCollectionIsNotNull(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{name: "nil", items: nil, want: `[]`},
		{name: "empty", items: []string{}, want: `[]`},
		{name: "populated", items: []string{"item"}, want: `["item"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(NonNilSlice(tt.items))
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
