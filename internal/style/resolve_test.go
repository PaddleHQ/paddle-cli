package style_test

import (
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestResolve_Precedence(t *testing.T) {
	cases := []struct {
		name string
		in   style.Inputs
		want bool
	}{
		// 1. Agent forces off, even with FORCE_COLOR.
		{"agent_beats_force", style.Inputs{AgentFlag: true, ForceColor: true, AutoDetected: true}, false},
		// 2. --no-color beats env force, config, auto.
		{"flag_no_color_beats_force", style.Inputs{NoColorFlag: true, ForceColor: true, AutoDetected: true}, false},
		// 3. FORCE_COLOR beats NO_COLOR and config never.
		{"force_beats_no_color_env", style.Inputs{ForceColor: true, NoColorEnv: true, ConfigPref: "never"}, true},
		// 4. NO_COLOR beats config always.
		{"no_color_env_beats_always", style.Inputs{NoColorEnv: true, ConfigPref: "always"}, false},
		// 5. config=always beats auto-detect-off.
		{"config_always_beats_auto_off", style.Inputs{ConfigPref: "always", AutoDetected: false}, true},
		// 6. config=never beats auto-detect-on.
		{"config_never_beats_auto_on", style.Inputs{ConfigPref: "never", AutoDetected: true}, false},
		// 7. config=auto, auto-detect on → on.
		{"auto_on", style.Inputs{ConfigPref: "auto", AutoDetected: true}, true},
		// 8. config=auto, auto-detect off → off.
		{"auto_off", style.Inputs{ConfigPref: "auto", AutoDetected: false}, false},
		// 9. Empty config string is treated as "auto".
		{"empty_config_treated_as_auto", style.Inputs{AutoDetected: true}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := style.Resolve(c.in).IsEnabled()
			if got != c.want {
				t.Errorf("Resolve(%+v).IsEnabled() = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
