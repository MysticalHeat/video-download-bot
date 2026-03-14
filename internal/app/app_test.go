package app

import "testing"

func TestExtractFirstURL(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{name: "plain url", text: "https://example.com/video", want: "https://example.com/video", ok: true},
		{name: "url in text", text: "look https://example.com/video now", want: "https://example.com/video", ok: true},
		{name: "invalid", text: "hello world", want: "", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractFirstURL(tc.text)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
