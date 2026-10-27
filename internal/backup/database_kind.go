package backup

import (
	"fmt"
	"strings"
)

// DatabaseLabel explicitly selects the logical dump engine for custom images.
const DatabaseLabel = "io.fleetdeck.backup.database"

// DatabaseKind is shared by logical backups and writer quiescing. Image IDs and
// private registry names cannot establish the database engine by themselves.
func DatabaseKind(image string, labels any) (string, error) {
	var value any
	found := false
	set := func(v any) error {
		if found {
			return fmt.Errorf("duplicate database backup label")
		}
		value, found = v, true
		return nil
	}
	switch values := labels.(type) {
	case nil:
	case map[string]string:
		if v, ok := values[DatabaseLabel]; ok {
			value, found = v, true
		}
	case map[string]any:
		value, found = values[DatabaseLabel]
	case map[any]any:
		value, found = values[DatabaseLabel]
	case []any:
		for _, entry := range values {
			label, ok := entry.(string)
			if !ok {
				return "", fmt.Errorf("invalid Compose label list")
			}
			key, v, _ := strings.Cut(label, "=")
			if key == DatabaseLabel {
				if err := set(v); err != nil {
					return "", err
				}
			}
		}
	default:
		return "", fmt.Errorf("invalid Compose labels")
	}
	if found {
		engine, ok := value.(string)
		if ok && (engine == "postgres" || engine == "mysql" || engine == "mariadb") {
			return engine, nil
		}
		return "", fmt.Errorf("database backup label must be postgres, mysql or mariadb")
	}
	name := strings.SplitN(strings.ToLower(image), "@", 2)[0]
	for _, prefix := range []string{"docker.io/library/", "index.docker.io/library/", "library/"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	name = strings.SplitN(name, ":", 2)[0]
	switch name {
	case "postgres", "mysql", "mariadb":
		return name, nil
	default:
		// Preserve failure rather than silently replacing a previously attempted
		// logical dump with a live file copy when the image name is ambiguous.
		for _, prefix := range []string{"postgres", "mysql", "mariadb"} {
			if strings.HasPrefix(name, prefix) {
				return "", fmt.Errorf("ambiguous database image requires an explicit database backup label")
			}
		}
		return "", nil
	}
}
