package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/fleetexport"
	"github.com/fleetdeck/fleetdeck/internal/server"
	"github.com/spf13/cobra"
)

var fleetCmd = &cobra.Command{Use: "fleet", Short: "Export and serve read-only observations for a private workstation overview"}
var fleetSnapshotCmd = &cobra.Command{Use: "snapshot", Short: "Export sanitized local registry and monitoring observations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
	node, _ := cmd.Flags().GetString("node-id")
	out, _ := cmd.Flags().GetString("output")
	security, _ := cmd.Flags().GetString("security-state")
	if !fleet.ValidID(node) || out == "" {
		return fmt.Errorf("--node-id and --output are required")
	}
	d, err := db.OpenReadOnly(cfg.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()
	s, err := fleetexport.Export(d, node, Version, Commit, cfg.Monitoring.StateFile, security, time.Now())
	if err != nil {
		return err
	}
	return fleetexport.WriteSnapshot(out, s)
}}
var fleetAgentCmd = &cobra.Command{Use: "agent", Short: "Serve a sanitized snapshot over mutually authenticated TLS", Args: cobra.NoArgs,
	// The unprivileged observer must not open the registry, load host secrets,
	// start Docker probes, load templates or initialize root audit files.
	PersistentPreRun: func(*cobra.Command, []string) {},
	RunE: func(cmd *cobra.Command, _ []string) error {
		node, _ := cmd.Flags().GetString("node-id")
		path, _ := cmd.Flags().GetString("snapshot")
		addr, _ := cmd.Flags().GetString("addr")
		if !fleet.ValidID(node) || path == "" || !fleet.PrivateAddress(addr) {
			return fmt.Errorf("agent requires node ID, snapshot file and a literal private/loopback IP:port")
		}
		cert, _ := cmd.Flags().GetString("tls-cert")
		key, _ := cmd.Flags().GetString("tls-key")
		ca, _ := cmd.Flags().GetString("client-ca")
		clients, _ := cmd.Flags().GetStringArray("allow-client-cert")
		if cert == "" || key == "" || ca == "" || len(clients) == 0 {
			return fmt.Errorf("agent requires TLS and explicitly authorized client certificates")
		}
		if err := fleet.PrivateKey(key); err != nil {
			return err
		}
		tlsConfig, err := (server.ListenerOptions{CertFile: cert, KeyFile: key, ClientCAFile: ca, AllowedClients: clients}).TLSConfig(addr)
		if err != nil {
			return err
		}
		allowed := map[[32]byte]bool{}
		for _, p := range clients {
			leaf, err := fleet.Leaf(p)
			if err != nil {
				return err
			}
			allowed[sha256.Sum256(leaf.Raw)] = true
		}
		s := &http.Server{Addr: addr, TLSConfig: tlsConfig, Handler: fleet.Agent(node, path, allowed), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16384}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-ctx.Done()
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.Shutdown(c)
		}()
		fmt.Fprintln(cmd.OutOrStdout(), "Read-only observer; no shell, Docker or deployment access.")
		err = server.ServeHTTP(s)
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	},
}

func init() {
	fleetSnapshotCmd.Flags().String("node-id", "", "Stable enrolled node identifier")
	fleetSnapshotCmd.Flags().String("output", "", "Private export file in an existing operator-owned directory")
	fleetSnapshotCmd.Flags().String("security-state", "", "Existing security watcher state file (optional)")
	fleetAgentCmd.Flags().String("node-id", "", "Stable enrolled node identifier")
	fleetAgentCmd.Flags().String("snapshot", "", "Sanitized export file; never a host configuration or raw report")
	fleetAgentCmd.Flags().String("addr", "127.0.0.1:9443", "Private management IP:port")
	fleetAgentCmd.Flags().String("tls-cert", "", "Observer server certificate")
	fleetAgentCmd.Flags().String("tls-key", "", "Observer private key")
	fleetAgentCmd.Flags().String("client-ca", "", "Management client CA")
	fleetAgentCmd.Flags().StringArray("allow-client-cert", nil, "Authorized workstation client leaf certificate (repeatable)")
	fleetCmd.AddCommand(fleetSnapshotCmd, fleetAgentCmd)
	rootCmd.AddCommand(fleetCmd)
}
