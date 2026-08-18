package handler

import "testing"

func TestOIDCFrontendRedirectURI(t *testing.T) {
	tests := []struct {
		name     string
		callback string
		want     string
	}{
		{
			name:     "gateway callback returns to UI root",
			callback: "https://knowledge.example.com/api/v1/auth/oidc/callback",
			want:     "https://knowledge.example.com/",
		},
		{
			name:     "query is never reflected into final redirect",
			callback: "http://127.0.0.1:5173/api/v1/auth/oidc/callback?untrusted=value",
			want:     "http://127.0.0.1:5173/",
		},
		{
			name:     "invalid callback falls back to local root",
			callback: "not-a-url",
			want:     "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oidcFrontendRedirectURI(tt.callback); got != tt.want {
				t.Fatalf("oidcFrontendRedirectURI() = %q, want %q", got, tt.want)
			}
		})
	}
}
