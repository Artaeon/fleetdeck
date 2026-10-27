package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseKindUsesExplicitEngineForOpaqueImages(t *testing.T) {
	for _, tc := range []struct {
		image   string
		labels  any
		want    string
		invalid bool
	}{
		{"postgres:17", nil, "postgres", false},
		{"docker.io/library/postgres@sha256:fixture", nil, "postgres", false},
		{"library/mysql:8", nil, "mysql", false},
		{"index.docker.io/library/mariadb:11", nil, "mariadb", false},
		{"postgres-exporter:1", nil, "", true},
		{"registry.example/postgres:17", nil, "", false},
		{"sha256:fixture", map[string]string{DatabaseLabel: "postgres"}, "postgres", false},
		{"${DATABASE_IMAGE}", map[string]any{DatabaseLabel: "mysql"}, "mysql", false},
		{"example/custom:1", []any{DatabaseLabel + "=mariadb"}, "mariadb", false},
		{"postgres:17", map[string]any{DatabaseLabel: 42}, "", true},
		{"postgres:17", []any{DatabaseLabel + "="}, "", true},
		{"postgres:17", []any{DatabaseLabel + "=postgres", DatabaseLabel + "=mysql"}, "", true},
		{"postgres:17", map[string]string{DatabaseLabel: "${ENGINE}"}, "", true},
	} {
		got, err := DatabaseKind(tc.image, tc.labels)
		if (err != nil) != tc.invalid || got != tc.want {
			t.Fatalf("image %s: got %q, %v; want %q invalid=%v", tc.image, got, err, tc.want, tc.invalid)
		}
	}
}

func TestCustomDatabaseDumpFailureCannotBecomeFileOnlyBackup(t *testing.T) {
	putFakeDockerOnPath(t, fakeDatabaseEnvironment+"printf 'partial dump'; exit 23\n")
	for _, labels := range []string{"labels:\n      " + DatabaseLabel + ": postgres", "labels: ['" + DatabaseLabel + "=postgres']"} {
		project := t.TempDir()
		os.WriteFile(filepath.Join(project, "compose.yaml"), []byte("services:\n  db:\n    image: sha256:fixture\n    "+labels+"\n"), 0600)
		if _, err := BackupDatabases(project, t.TempDir()); err == nil {
			t.Fatal("custom database dump failure was ignored")
		}
	}
}
