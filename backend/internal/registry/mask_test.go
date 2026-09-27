package registry

import "testing"

func TestMaskName(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want string
	}{
		{name: "Иванов Иван Иванович", kind: "person", want: "Иванов И. И."},
		{name: "Петров И. И.", kind: "person", want: "Петров И. И."},
		{name: "Ли Мин", kind: "person", want: "Ли М."},
		{name: "ООО «Дом»", kind: "organization", want: "ООО «Дом»"},
		{name: "Муниципальное образование", kind: "municipality", want: "Муниципальное образование"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskName(tt.name, tt.kind); got != tt.want {
				t.Fatalf("MaskName(%q, %q) = %q, want %q", tt.name, tt.kind, got, tt.want)
			}
		})
	}
}
