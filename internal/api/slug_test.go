package api

import "testing"

func TestSlugFromRemote(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard hatch remote",
			url:  "https://tok123@git.gethatch.eu/deploy/myapp.git",
			want: "myapp",
		},
		{
			name: "remote without token",
			url:  "https://git.gethatch.eu/deploy/coolapp.git",
			want: "coolapp",
		},
		{
			name:    "non-hatch remote",
			url:     "https://github.com/user/repo.git",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SlugFromRemote(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}

	// Additional tests for new format without /deploy/ prefix
	newTests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "new format without /deploy/ prefix",
			url:  "https://tok123@git.gethatch.eu/myapp.git",
			want: "myapp",
		},
		{
			name: "new format without token",
			url:  "https://git.gethatch.eu/coolapp.git",
			want: "coolapp",
		},
	}

	for _, tt := range newTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SlugFromRemote(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSlug(t *testing.T) {
	if got := NormalizeSlug("  myapp  "); got != "myapp" {
		t.Fatalf("expected 'myapp', got %q", got)
	}
}
