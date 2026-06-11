package energy

// STUB(h-gbo48) file: minimal seams for the energy-pack red suite (feature
// h-vlmt8, spec h-avt0u). Implemented in h-sjoev (impl-cli). Zero logic by
// design — the tests in energy_pack_test.go fail on assertions until impl.

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

// Deps injects auth + API calls for tests (house pattern, see
// cmd/deploy/deploy_test.go).
type Deps struct {
	GetToken           func() (string, error)
	CreatePackCheckout func(token string) (*api.EnergyPackCheckoutResponse, error)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		CreatePackCheckout: func(token string) (*api.EnergyPackCheckoutResponse, error) {
			return api.NewClient(token).EnergyPackCheckout()
		},
	}
}

var deps = defaultDeps()

// writeAccountEnergy renders the account energy summary — all four buckets
// (daily, weekly, bonus where applicable, pack) — to w. showAccountEnergy
// adopts this seam in impl so the rendering is testable.
// STUB(h-gbo48): implemented in h-sjoev
func writeAccountEnergy(w io.Writer, energy *api.EnergyStatus) {
}

// runBuy executes `hatch energy buy`: creates a pack checkout session and
// prints the URL (mirrors cmd/boost's flow).
// STUB(h-gbo48): implemented in h-sjoev
func runBuy(cmd *cobra.Command, args []string) error {
	return nil
}

func newBuyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "buy",
		Short: "Buy an energy pack (1000 min, non-expiring)",
		RunE:  runBuy,
	}
}
