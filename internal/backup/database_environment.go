package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Read the running service's effective environment. Compose .env is only an
// interpolation source; it need not contain inline or env_file credentials.
// Resolve through Compose so custom project/container names work as well.
func databaseEnvironment(projectPath, service string) (string, map[string]string, error) {
	if !dockerObjectName.MatchString(service) {
		return "", nil, fmt.Errorf("invalid database service name")
	}
	ids, err := privateDockerOutput(projectPath, "compose", "ps", "-q", service)
	if err != nil {
		return "", nil, fmt.Errorf("locating running database service: %w", err)
	}
	containers := strings.Fields(string(ids))
	if len(containers) != 1 || !dockerObjectName.MatchString(containers[0]) {
		return "", nil, fmt.Errorf("database service must have exactly one running container")
	}
	data, err := privateDockerOutput(projectPath, "inspect", "--format", "{{json .Config.Env}}", containers[0])
	if err != nil {
		return "", nil, fmt.Errorf("reading database environment: %w", err)
	}
	var entries []string
	if json.Unmarshal(data, &entries) != nil || entries == nil {
		return "", nil, fmt.Errorf("invalid database environment response")
	}
	env := make(map[string]string)
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return containers[0], env, nil
}

type privateOutput struct{ bytes.Buffer }

func (b *privateOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("Docker response exceeds size limit")
	}
	return b.Buffer.Write(p)
}

func privateDockerOutput(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	var output privateOutput
	cmd.Stdout = &output
	// Neither stdout nor stderr may enter errors: they can contain credentials.
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Docker operation failed: %w", err)
	}
	return output.Bytes(), nil
}
