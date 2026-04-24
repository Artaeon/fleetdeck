package backup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Environment interface{}       `yaml:"environment"`
	Container   string            `yaml:"container_name"`
	Labels      interface{}       `yaml:"labels"`
	Volumes     []string          `yaml:"volumes"`
}

func BackupDatabases(projectPath, backupDir string) ([]ComponentInfo, error) {
	dbDir := filepath.Join(backupDir, "databases")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, err
	}

	compose, err := parseComposeFile(projectPath)
	if err != nil {
		return nil, nil // no compose file or parse error, skip silently
	}

	envVars := loadEnvFile(projectPath)

	var components []ComponentInfo

	for serviceName, svc := range compose.Services {
		image := strings.ToLower(svc.Image)
		containerName := svc.Container
		if containerName == "" {
			containerName = filepath.Base(projectPath) + "-" + serviceName + "-1"
		}

		switch {
		case strings.HasPrefix(image, "postgres"):
			comp, err := dumpPostgres(containerName, serviceName, envVars, dbDir)
			if err != nil {
				continue
			}
			if comp != nil {
				components = append(components, *comp)
			}
		case strings.HasPrefix(image, "mysql") || strings.HasPrefix(image, "mariadb"):
			comp, err := dumpMySQL(containerName, serviceName, envVars, dbDir)
			if err != nil {
				continue
			}
			if comp != nil {
				components = append(components, *comp)
			}
		}
	}

	return components, nil
}

func dumpPostgres(containerName, serviceName string, envVars map[string]string, dbDir string) (*ComponentInfo, error) {
	user := envVars["POSTGRES_USER"]
	dbName := envVars["POSTGRES_DB"]
	if user == "" {
		user = "postgres"
	}
	if dbName == "" {
		dbName = user
	}

	dumpFile := filepath.Join(dbDir, serviceName+".sql.gz")

	cmd := exec.Command("bash", "-c",
		shellQuote("docker", "exec", containerName, "pg_dump", "-U", user, dbName)+
			" | gzip > "+shellQuote(dumpFile))

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pg_dump failed for %s: %s: %w", containerName, strings.TrimSpace(string(out)), err)
	}

	info, err := os.Stat(dumpFile)
	if err != nil {
		return nil, err
	}

	return &ComponentInfo{
		Type:      "database",
		Name:      serviceName + " (PostgreSQL)",
		Path:      filepath.Join("databases", serviceName+".sql.gz"),
		SizeBytes: info.Size(),
	}, nil
}

func dumpMySQL(containerName, serviceName string, envVars map[string]string, dbDir string) (*ComponentInfo, error) {
	password := envVars["MYSQL_ROOT_PASSWORD"]
	dbName := envVars["MYSQL_DATABASE"]
	if dbName == "" {
		dbName = envVars["MYSQL_DB"]
	}
	if dbName == "" {
		return nil, fmt.Errorf("no MySQL database name found")
	}

	dumpFile := filepath.Join(dbDir, serviceName+".sql.gz")

	// Pass the password through the environment rather than argv. `docker exec
	// -e MYSQL_PWD` without an `=value` tells docker to forward the variable
	// from the parent process, so the password never appears in `ps aux` or
	// in the Docker API call's CLI form. The previous `-p<password>` argv was
	// visible to every local user via /proc during the dump.
	dumpCmd := "docker exec"
	if password != "" {
		dumpCmd += " -e MYSQL_PWD"
	}
	dumpCmd += " " + shellQuote(containerName) + " mysqldump -u root " + shellQuote(dbName) +
		" | gzip > " + shellQuote(dumpFile)

	cmd := exec.Command("bash", "-c", dumpCmd)
	if password != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+password)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mysqldump failed for %s: %s: %w", containerName, strings.TrimSpace(string(out)), err)
	}

	info, err := os.Stat(dumpFile)
	if err != nil {
		return nil, err
	}

	return &ComponentInfo{
		Type:      "database",
		Name:      serviceName + " (MySQL)",
		Path:      filepath.Join("databases", serviceName+".sql.gz"),
		SizeBytes: info.Size(),
	}, nil
}

// shellQuote safely quotes arguments for shell execution.
func shellQuote(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func parseComposeFile(projectPath string) (*composeFile, error) {
	names := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	for _, name := range names {
		path := filepath.Join(projectPath, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cf composeFile
		if err := yaml.Unmarshal(data, &cf); err != nil {
			continue
		}
		return &cf, nil
	}
	return nil, fmt.Errorf("no compose file found")
}

func loadEnvFile(projectPath string) map[string]string {
	envVars := make(map[string]string)
	f, err := os.Open(filepath.Join(projectPath, ".env"))
	if err != nil {
		return envVars
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}
	return envVars
}
