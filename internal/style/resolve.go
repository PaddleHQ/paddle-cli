package style

// Inputs is the full set of signals Resolve considers.
// The root command populates each field before invoking Resolve.
type Inputs struct {
	NoColorFlag  bool   // --no-color
	AgentFlag    bool   // --agent or PADDLE_AGENT=1
	NoColorEnv   bool   // NO_COLOR set to any non-empty value
	ForceColor   bool   // FORCE_COLOR set to any non-empty value
	ConfigPref   string // "auto" | "always" | "never"; empty treated as "auto"
	AutoDetected bool   // result of TTY auto-detection (delivered separately)
}

// Resolve decides whether color output should be enabled for this CLI run.
// Precedence (first match wins):
//
//  1. agent → off
//  2. --no-color → off
//  3. FORCE_COLOR → on
//  4. NO_COLOR → off
//  5. config.color = always → on
//  6. config.color = never → off
//  7. config.color = auto (default) → AutoDetected
func Resolve(in Inputs) Profile {
	switch {
	case in.AgentFlag:
		return Profile{}
	case in.NoColorFlag:
		return Profile{}
	case in.ForceColor:
		return Enabled()
	case in.NoColorEnv:
		return Profile{}
	case in.ConfigPref == "always":
		return Enabled()
	case in.ConfigPref == "never":
		return Profile{}
	}
	// auto / empty
	if in.AutoDetected {
		return Enabled()
	}
	return Profile{}
}
