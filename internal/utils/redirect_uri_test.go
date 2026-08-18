package utils

import "testing"

func TestValidateAllowedRedirectURI(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		allowed   string
		wantErr   bool
	}{
		{name: "exact https URI", requested: "https://knowledge.example.com/auth/callback", allowed: "https://knowledge.example.com/auth/callback", wantErr: false},
		{name: "host casing is normalized", requested: "https://KNOWLEDGE.example.com/", allowed: "https://knowledge.example.com", wantErr: false},
		{name: "relative same-origin path", requested: "/login/complete", allowed: "/login/complete", wantErr: false},
		{name: "unlisted path", requested: "https://knowledge.example.com/evil", allowed: "https://knowledge.example.com/auth/callback", wantErr: true},
		{name: "attacker host", requested: "https://attacker.example/callback", allowed: "https://knowledge.example.com/callback", wantErr: true},
		{name: "scheme-relative redirect", requested: "//attacker.example/callback", allowed: "//attacker.example/callback", wantErr: true},
		{name: "userinfo redirect", requested: "https://knowledge.example.com@attacker.example/callback", allowed: "https://knowledge.example.com@attacker.example/callback", wantErr: true},
		{name: "fragment supplied by caller", requested: "https://knowledge.example.com/#token", allowed: "https://knowledge.example.com/#token", wantErr: true},
		{name: "allowlist missing", requested: "https://knowledge.example.com/", allowed: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllowedRedirectURI(tt.requested, tt.allowed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAllowedRedirectURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
