package config

import "testing"

func TestApplySSHAnimationPolicy(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantSSH    bool
		wantGIFs   bool
		wantRoles  bool
		wantVideos bool
	}{
		{
			name:       "SSH connection disables animations",
			env:        map[string]string{"SSH_CONNECTION": "10.0.0.2 5555 10.0.0.1 22"},
			wantSSH:    true,
			wantGIFs:   false,
			wantRoles:  false,
			wantVideos: true,
		},
		{
			name:       "SSH tty is sufficient",
			env:        map[string]string{"SSH_TTY": "/dev/pts/4"},
			wantSSH:    true,
			wantGIFs:   false,
			wantRoles:  false,
			wantVideos: true,
		},
		{
			name:       "local session preserves settings",
			env:        map[string]string{},
			wantSSH:    false,
			wantGIFs:   true,
			wantRoles:  true,
			wantVideos: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Display.NoAnimationsOverSSH = true
			cfg.Display.RoleGradientAnimations = true
			lookup := func(key string) string { return tt.env[key] }

			if got := IsSSH(lookup); got != tt.wantSSH {
				t.Fatalf("IsSSH() = %v, want %v", got, tt.wantSSH)
			}
			got := ApplySSHAnimationPolicy(cfg, lookup)
			if got.Media.AnimateGIFs != tt.wantGIFs {
				t.Errorf("Media.AnimateGIFs = %v, want %v", got.Media.AnimateGIFs, tt.wantGIFs)
			}
			if got.Display.RoleGradientAnimations != tt.wantRoles {
				t.Errorf("Display.RoleGradientAnimations = %v, want %v", got.Display.RoleGradientAnimations, tt.wantRoles)
			}
			if got.Media.VideoEnabled != tt.wantVideos {
				t.Errorf("Media.VideoEnabled = %v, want %v", got.Media.VideoEnabled, tt.wantVideos)
			}
		})
	}
}

func TestApplySSHAnimationPolicyRequiresOptIn(t *testing.T) {
	cfg := Default()
	cfg.Display.NoAnimationsOverSSH = false
	cfg.Display.RoleGradientAnimations = true
	cfg.Media.AnimateGIFs = true

	got := ApplySSHAnimationPolicy(cfg, func(string) string { return "present" })
	if !got.Media.AnimateGIFs || !got.Display.RoleGradientAnimations {
		t.Fatalf("opt-out policy changed animation settings: %+v", got)
	}
}
