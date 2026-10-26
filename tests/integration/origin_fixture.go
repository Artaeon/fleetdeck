//go:build integration

// Render the production policy with fixed, public provider prefixes. No network.
package main

import (
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/bootstrap"
	"net/netip"
	"os"
)

func main() {
	rules, err := bootstrap.RenderOriginFirewall([]string{"wan"}, []netip.Prefix{netip.MustParsePrefix("173.245.48.0/20"), netip.MustParsePrefix("2606:4700::/32")})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(rules)
}
