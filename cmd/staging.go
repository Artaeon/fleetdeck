package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/staging"
	"github.com/spf13/cobra"
)

var stagingCmd = &cobra.Command{Use: "staging", Short: "Prepare isolated, password-gated staging deployments"}
var stagingInitCmd = &cobra.Command{Use: "init <new-directory>", Short: "Create a hardened image-based preview without copying production secrets", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	get := func(key string) string { v, _ := cmd.Flags().GetString(key); return v }
	passwordFile := get("password-file")
	info, err := os.Lstat(passwordFile)
	if err != nil {
		return fmt.Errorf("read a private password file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 74 {
		return fmt.Errorf("password file must be a private regular file of at most 74 bytes")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		return err
	}
	password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"))
	port, _ := cmd.Flags().GetInt("port")
	o := staging.Options{Name: get("name"), Domain: get("domain"), Image: get("image"), Username: get("username"), User: get("user"), Port: port}
	if err := staging.Create(args[0], o, password); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Staging prepared. Review compose.yaml, configure DNS and deploy with the private .env as the first-deploy --env-file. Credentials were not printed.")
	return nil
}}

func init() {
	for _, key := range []string{"name", "domain", "image", "username", "password-file"} {
		stagingInitCmd.Flags().String(key, "", "Required staging "+key)
	}
	stagingInitCmd.Flags().String("user", "10001:10001", "Non-root container UID:GID; match the image's supported user")
	stagingInitCmd.Flags().Int("port", 3000, "Application port inside the container (1024-65535)")
	stagingCmd.AddCommand(stagingInitCmd)
	rootCmd.AddCommand(stagingCmd)
}
