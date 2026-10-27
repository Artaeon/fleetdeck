package backup

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

func TestDatabaseDumpFailureReachesBackupCaller(t *testing.T) {
	for _, image := range []string{"postgres:17", "mysql:8", "mariadb:11"} {
		t.Run(image, func(t *testing.T) {
			putFakeDockerOnPath(t, fakeDatabaseEnvironment+"printf 'partial SQL output'\nexit 23\n")
			projectDir, backupDir := t.TempDir(), t.TempDir()
			os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services:\n  db:\n    image: "+image+"\n"), 0600)
			os.WriteFile(filepath.Join(projectDir, ".env"), []byte("MYSQL_DATABASE=fixture\n"), 0600)
			components, err := BackupDatabases(projectDir, backupDir)
			if err == nil || len(components) != 0 || !strings.Contains(err.Error(), "database service db") {
				t.Fatalf("failed dump reported as successful: components=%v error=%v", components, err)
			}
		})
	}
}

func TestInvalidComposeCannotHideDatabaseServices(t *testing.T) {
	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: [invalid"), 0600)
	// Do not silently fall back to another file after failing to parse the first.
	os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}"), 0600)
	if _, err := BackupDatabases(projectDir, t.TempDir()); err == nil {
		t.Fatal("invalid Compose accepted")
	}
}

func TestFailedDatabaseBackupLeavesNoRecordOrSnapshot(t *testing.T) {
	putFakeDockerOnPath(t, "printf 'partial dump'\nexit 23\n")
	database := newTestDB(t)
	project := createProject(t, database, "incomplete")
	project.ProjectPath = t.TempDir()
	os.WriteFile(filepath.Join(project.ProjectPath, "docker-compose.yml"), []byte("services:\n  db:\n    image: postgres:17\n"), 0600)
	cfg := config.DefaultConfig()
	cfg.Backup.BasePath = t.TempDir()
	record, err := CreateBackup(cfg, database, project, "snapshot", "pre-rollback", Options{SkipVolumes: true})
	if err == nil || record != nil {
		t.Fatal("failed snapshot was accepted")
	}
	records, err := database.ListBackupRecords(project.ID, 100)
	if err != nil || len(records) != 0 {
		t.Fatalf("unexpected recorded backup: %v (%v)", records, err)
	}
	entries, err := os.ReadDir(filepath.Join(cfg.Backup.BasePath, project.Name))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial snapshot left behind: %v (%v)", entries, err)
	}
}

func TestFailedDumpDeletesPartialArchive(t *testing.T) {
	putFakeDockerOnPath(t, "printf 'partial SQL'\nexit 23\n")
	for _, mysql := range []bool{false, true} {
		dir := t.TempDir()
		var err error
		if mysql {
			_, err = dumpMySQL("fixture", "db", map[string]string{"MYSQL_DATABASE": "fixture"}, dir)
		} else {
			_, err = dumpPostgres("fixture", "db", nil, dir)
		}
		if err == nil {
			t.Fatal("partial dump accepted")
		}
		entries, readErr := os.ReadDir(dir)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("partial dump remains: %v (%v)", entries, readErr)
		}
	}
}

func TestDatabaseDumpsArePrivate(t *testing.T) {
	withFakeDocker(t)
	for _, mysql := range []bool{false, true} {
		dir := t.TempDir()
		var err error
		if mysql {
			_, err = dumpMySQL("fixture", "db", map[string]string{"MYSQL_DATABASE": "fixture"}, dir)
		} else {
			_, err = dumpPostgres("fixture", "db", nil, dir)
		}
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(dir, "db.sql.gz"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("dump not private: %v (%v)", info, err)
		}
	}
}

func TestRestoreRejectsFailedDecompressionEvenWhenClientSucceeds(t *testing.T) {
	putFakeDockerOnPath(t, fakeDatabaseEnvironment+"cat >/dev/null\nexit 0\n")
	for _, engine := range []string{"PostgreSQL", "MySQL"} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, ".env"), []byte("MYSQL_DATABASE=fixture\n"), 0600)
			archive := filepath.Join(dir, "broken.sql.gz")
			os.WriteFile(archive, []byte("not gzip"), 0600)
			if err := restoreDatabase(archive, dir, "db ("+engine+")"); err == nil {
				t.Fatal("gzip failure was hidden by successful client")
			}
		})
	}
}

func TestPostgresRestoreDisablesStartupFilesAndStopsOnSQLError(t *testing.T) {
	// Simulate psql's default success after a SQL error. It returns error 3
	// only when error-stop is set and a startup file cannot override it.
	putFakeDockerOnPath(t, fakeDatabaseEnvironment+`cat >/dev/null
case " $* " in
  *" psql -X --set=ON_ERROR_STOP=on "*) exit 3 ;;
  *) exit 0 ;;
esac
`)
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "dump.sql.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	_, _ = gz.Write([]byte("SELECT missing_column;"))
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := restoreDatabase(f.Name(), dir, "db (PostgreSQL)"); err == nil {
		t.Fatal("SQL error reported as success")
	}
}
