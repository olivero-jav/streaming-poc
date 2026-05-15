package api

import "testing"

func TestIsAllowedOrigin(t *testing.T) {
	t.Parallel()

	allowed := map[string]struct{}{
		"http://localhost:4200":  {},
		"http://127.0.0.1:4200":  {},
		"https://app.example.com": {},
	}

	cases := []struct {
		name       string
		origin     string
		allowNgrok bool
		want       bool
	}{
		{"empty origin", "", false, false},
		{"empty origin even with ngrok allowed", "", true, false},
		{"explicit allow-list hit", "http://localhost:4200", false, true},
		{"explicit allow-list hit (https)", "https://app.example.com", false, true},
		{"explicit allow-list miss without ngrok", "https://evil.com", false, false},
		{"ngrok-free https accepted when allowed", "https://abc123.ngrok-free.app", true, true},
		{"ngrok.io https accepted when allowed", "https://abc123.ngrok.io", true, true},
		{"ngrok rejected when allowNgrok=false", "https://abc123.ngrok-free.app", false, false},
		{"ngrok http (not https) rejected", "http://abc123.ngrok-free.app", true, false},
		{"random https domain rejected even with ngrok", "https://evil.com", true, false},
		{"ngrok subdomain spoof rejected", "https://evil.com.ngrok-free.app.fake.com", true, false},
		{"unparseable origin rejected", "://not a url", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAllowedOrigin(tc.origin, allowed, tc.allowNgrok)
			if got != tc.want {
				t.Errorf("isAllowedOrigin(%q, allow=%v): got %v, want %v", tc.origin, tc.allowNgrok, got, tc.want)
			}
		})
	}
}
