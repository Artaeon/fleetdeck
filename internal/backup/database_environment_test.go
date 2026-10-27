package backup

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseBackupAndRestoreUseRunningServiceEnvironment(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	// A custom Compose filename and conflicting interpolation variables must
	// not override the effective database user/name from the running container.
	os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  db:\n    image: postgres:17\n    environment:\n      POSTGRES_USER: inline_user\n"), 0600)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("POSTGRES_USER=wrong\nPOSTGRES_DB=wrong\n"), 0600)
	log := filepath.Join(t.TempDir(), "commands")
	t.Setenv("DATABASE_COMMAND_LOG", log)
	putFakeDockerOnPath(t, fakeDatabaseEnvironment+`printf '%s\n' "$*" >> "$DATABASE_COMMAND_LOG"
case "$*" in
  "exec fixture-container pg_dump -U fixture_user fixture_db")
    i=0; while [ $i -lt 400 ]; do echo "SELECT $i, 'fixture payload';"; i=$((i+1)); done ;;
  "exec -i fixture-container psql -X --set=ON_ERROR_STOP=on -U fixture_user fixture_db") cat >/dev/null ;;
  *) exit 91 ;;
esac
`)
	components, err := BackupDatabases(dir, out)
	if err != nil || len(components) != 1 {
		t.Fatalf("backup: %v %v", components, err)
	}
	if err := restoreDatabase(filepath.Join(out, components[0].Path), dir, components[0].Name); err != nil {
		t.Fatal(err)
	}
	commands, _ := os.ReadFile(log)
	if !strings.Contains(string(commands), "psql -X") || strings.Contains(string(commands), "wrong") {
		t.Fatalf("unexpected commands: %s", commands)
	}
}

func TestDatabaseEnvironmentFailsClosedWithoutExposingOutput(t *testing.T) {
	for _, body := range []string{
		`echo sensitive-fixture >&2; exit 1`,
		`echo 'one two'`,
		`if [ "$1" = compose ]; then echo fixture; else echo sensitive-fixture; fi`,
		`if [ "$1" = compose ]; then echo fixture; else echo null; fi`,
	} {
		t.Run(body, func(t *testing.T) {
			putFakeDockerOnPath(t, body)
			_, _, err := databaseEnvironment(t.TempDir(), "db")
			if err == nil || strings.Contains(err.Error(), "sensitive-fixture") {
				t.Fatalf("unsafe result: %v", err)
			}
		})
	}
}

func TestMySQLRestoreKeepsEffectivePasswordOutOfArguments(t *testing.T) {
	dir := t.TempDir()
	f, _ := os.Create(filepath.Join(dir, "dump.gz"))
	gz := gzip.NewWriter(f)
	gz.Write([]byte("SELECT 1;"))
	gz.Close()
	f.Close()
	putFakeDockerOnPath(t, fakeDatabaseEnvironment+`case "$*" in
  "exec -i -e MYSQL_PWD fixture-container mysql -u root fixture_db")
    [ "$MYSQL_PWD" = fixture-only ] || exit 92; cat >/dev/null ;;
  *) exit 93 ;;
esac
`)
	if err := restoreDatabase(f.Name(), dir, "db (MySQL)"); err != nil {
		t.Fatal(err)
	}
}
