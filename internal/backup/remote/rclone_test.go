package remote

import "testing"

func TestRemotePath(t *testing.T) {
	cases := []struct {
		target string
		id     string
		want   string
	}{
		{"b2:bucket", "abc123", "b2:bucket/abc123"},
		{"b2:bucket/", "abc123", "b2:bucket/abc123"},
		{"r2:", "abc123", "r2:abc123"},
		{"r2:backups/prod", "xyz", "r2:backups/prod/xyz"},
	}
	for _, c := range cases {
		r := NewRclone(c.target)
		got := r.remotePath(c.id)
		if got != c.want {
			t.Errorf("remotePath(%q, %q) = %q, want %q", c.target, c.id, got, c.want)
		}
	}
}

func TestExtractLsjsonName(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{`{"Path":"abc","Name":"abc","Size":-1,"MimeType":"inode/directory","ModTime":"2026-04-24T10:00:00Z","IsDir":true},`, "abc"},
		{`{"Path":"manifest.json","Name":"manifest.json","Size":120,"IsDir":false},`, ""},
		{`[`, ""},
		{`]`, ""},
		{``, ""},
		{`not json`, ""},
	}
	for _, c := range cases {
		got := extractLsjsonName(c.line)
		if got != c.want {
			t.Errorf("extractLsjsonName(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
