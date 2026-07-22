package config

// IsSSH reports whether the process appears to be running inside an SSH
// session. The lookup function is injected so callers and tests can use the
// same detection rules without mutating the process environment.
func IsSSH(lookup func(string) string) bool {
	if lookup == nil {
		return false
	}
	return lookup("SSH_CONNECTION") != "" ||
		lookup("SSH_CLIENT") != "" ||
		lookup("SSH_TTY") != ""
}

// ApplySSHAnimationPolicy returns cfg with optional remote-session animation
// suppression applied. Video playback is intentionally unaffected: it is an
// explicit user action rather than a background animation.
func ApplySSHAnimationPolicy(cfg Config, lookup func(string) string) Config {
	if !cfg.Display.NoAnimationsOverSSH || !IsSSH(lookup) {
		return cfg
	}

	cfg.Media.AnimateGIFs = false
	cfg.Display.RoleGradientAnimations = false
	return cfg
}
