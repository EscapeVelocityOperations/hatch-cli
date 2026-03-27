package energy

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

// CheckAfterCommand checks energy for a specific app and prints a warning if
// energy is low (< 20%) or critical (< 10%). Intended to be called after
// deploy, restart, etc. Errors are silently ignored — this is best-effort.
func CheckAfterCommand(client *api.Client, slug string) {
	energy, err := client.GetAppEnergy(slug)
	if err != nil {
		return
	}

	if energy.AlwaysOn || energy.Boosted {
		return
	}

	dailyPct := pctOf(energy.DailyRemainingMin, energy.DailyLimitMin)
	weeklyPct := pctOf(energy.WeeklyRemainingMin, energy.WeeklyLimitMin)

	if dailyPct <= 10 || weeklyPct <= 10 {
		fmt.Fprintf(os.Stderr, "\n  %s Energy critically low (%d%% daily, %d%% weekly).\n",
			ui.Red("!!"), dailyPct, weeklyPct)
		fmt.Fprintf(os.Stderr, "  Your egg will sleep when energy runs out.\n")
		fmt.Fprintf(os.Stderr, "  Run %s to boost it.\n",
			ui.Bold("hatch boost "+slug))
	} else if dailyPct <= 20 || weeklyPct <= 20 {
		fmt.Fprintf(os.Stderr, "\n  %s Energy low (%d%% daily, %d%% weekly). Run %s to boost.\n",
			ui.Yellow("!"), dailyPct, weeklyPct, ui.Bold("hatch boost "+slug))
	}
}

func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
