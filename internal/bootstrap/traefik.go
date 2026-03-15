package bootstrap

import "fmt"

const traefikDir = "/opt/traefik"

// setupTraefik creates a Traefik reverse proxy configuration with Let's
// Encrypt TLS and starts it via Docker Compose. Idempotent: recreates the
// config and restarts the stack each time.
func setupTraefik(runner CommandRunner, domain, email, network string) error {
	if _, err := runner.Run(fmt.Sprintf("mkdir -p %s", traefikDir)); err != nil {
		return fmt.Errorf("creating traefik dir: %w", err)
	}

	// Create Docker network if it doesn't exist.
	runner.Run(fmt.Sprintf("docker network create %s 2>/dev/null || true", network))

	// Create acme.json with correct permissions.
	if _, err := runner.Run(fmt.Sprintf("touch %s/acme.json && chmod 600 %s/acme.json", traefikDir, traefikDir)); err != nil {
		return fmt.Errorf("creating acme.json: %w", err)
	}

	compose := composeFile(domain, email, network)
	if _, err := runner.Run(fmt.Sprintf("cat > %s/docker-compose.yml << 'FLEETDECK_EOF'\n%s\nFLEETDECK_EOF", traefikDir, compose)); err != nil {
		return fmt.Errorf("writing docker-compose.yml: %w", err)
	}

	// Start (or restart) Traefik.
	if _, err := runner.Run(fmt.Sprintf("cd %s && docker compose up -d --force-recreate", traefikDir)); err != nil {
		return fmt.Errorf("starting traefik: %w", err)
	}

	return nil
}

func composeFile(domain, email, network string) string {
	return fmt.Sprintf(`services:
  traefik:
    image: traefik:v3
    restart: unless-stopped
    command:
      - "--api.dashboard=true"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--entrypoints.web.http.redirections.entrypoint.to=websecure"
      - "--entrypoints.web.http.redirections.entrypoint.scheme=https"
      - "--certificatesresolvers.letsencrypt.acme.email=%s"
      - "--certificatesresolvers.letsencrypt.acme.storage=/acme.json"
      - "--certificatesresolvers.letsencrypt.acme.tlschallenge=true"
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--providers.docker.network=%s"
      - "--accesslog=true"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - "./acme.json:/acme.json"
    networks:
      - %s
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.dashboard.rule=Host(%s) && (PathPrefix(%s) || PathPrefix(%s))"
      - "traefik.http.routers.dashboard.entrypoints=websecure"
      - "traefik.http.routers.dashboard.tls.certresolver=letsencrypt"
      - "traefik.http.routers.dashboard.service=api@internal"

networks:
  %s:
    external: true`,
		email,
		network,
		network,
		"`"+domain+"`",
		"`/api`",
		"`/dashboard`",
		network,
	)
}
