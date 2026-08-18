package service

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestIsRefreshTokenClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims jwt.MapClaims
		want   bool
	}{
		{name: "refresh", claims: jwt.MapClaims{"type": "refresh"}, want: true},
		{name: "access", claims: jwt.MapClaims{"type": "access"}, want: false},
		{name: "missing type", claims: jwt.MapClaims{"user_id": "u1"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRefreshTokenClaims(tc.claims); got != tc.want {
				t.Fatalf("isRefreshTokenClaims(%v) = %v, want %v", tc.claims, got, tc.want)
			}
		})
	}
}
