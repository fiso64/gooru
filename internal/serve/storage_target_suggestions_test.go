package serve

import (
	"reflect"
	"testing"
)

func TestStorageTargetSuggestions(t *testing.T) {
	targets := []UploadTarget{{ID: "archive"}, {ID: "photos"}, {ID: "ANY"}, {ID: ""}}
	for _, test := range []struct {
		name   string
		prefix string
		limit  int
		want   []string
	}{
		{name: "all values include special any", prefix: "@in_target:", want: []string{"@in_target:any", "@in_target:archive", "@in_target:photos"}},
		{name: "filters value prefix", prefix: "@in_target:ph", want: []string{"@in_target:photos"}},
		{name: "preserves negation", prefix: "-@in_target:a", want: []string{"-@in_target:any", "-@in_target:archive"}},
		{name: "limits results", prefix: "@in_target:", limit: 2, want: []string{"@in_target:any", "@in_target:archive"}},
		{name: "ignores other syntax", prefix: "@saved:", want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			items := storageTargetSuggestions(test.prefix, targets, test.limit)
			got := make([]string, 0, len(items))
			for _, item := range items {
				got = append(got, item.Name)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("suggestions=%v, want %v", got, test.want)
			}
		})
	}
}
