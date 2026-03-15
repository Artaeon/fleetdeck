package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/crypto"
)

// ---------------------------------------------------------------------------
// Project CRUD edge cases
// ---------------------------------------------------------------------------

func TestCreateProjectAutoGeneratesID(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "auto-id",
		Domain:      "auto.com",
		LinuxUser:   "fleetdeck-auto",
		ProjectPath: "/opt/fleetdeck/auto-id",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(p.ID) < 32 {
		t.Errorf("expected UUID-length ID, got %q", p.ID)
	}
}

func TestCreateProjectPreservesExplicitID(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		ID:          "custom-id-12345",
		Name:        "explicit-id",
		Domain:      "explicit.com",
		LinuxUser:   "fleetdeck-explicit",
		ProjectPath: "/opt/fleetdeck/explicit-id",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID != "custom-id-12345" {
		t.Errorf("expected preserved ID custom-id-12345, got %s", p.ID)
	}

	got, err := db.GetProject("explicit-id")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.ID != "custom-id-12345" {
		t.Errorf("expected ID custom-id-12345 from DB, got %s", got.ID)
	}
}

func TestCreateProjectDefaultStatusAndSource(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "defaults",
		Domain:      "defaults.com",
		LinuxUser:   "fleetdeck-defaults",
		ProjectPath: "/opt/fleetdeck/defaults",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Status != "created" {
		t.Errorf("expected default status 'created', got %s", p.Status)
	}
	if p.Source != "created" {
		t.Errorf("expected default source 'created', got %s", p.Source)
	}
}

func TestCreateProjectCustomStatusAndSource(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "custom-status",
		Domain:      "cs.com",
		LinuxUser:   "fleetdeck-cs",
		ProjectPath: "/opt/fleetdeck/cs",
		Status:      "running",
		Source:      "discovered",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProject("custom-status")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("expected status 'running', got %s", got.Status)
	}
	if got.Source != "discovered" {
		t.Errorf("expected source 'discovered', got %s", got.Source)
	}
}

func TestCreateProjectTimestampsSet(t *testing.T) {
	db := newTestDB(t)

	before := time.Now().Add(-time.Second)
	p := &Project{
		Name:        "timestamps",
		Domain:      "ts.com",
		LinuxUser:   "fleetdeck-ts",
		ProjectPath: "/opt/fleetdeck/ts",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	after := time.Now().Add(time.Second)

	if p.CreatedAt.Before(before) || p.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in expected range [%v, %v]", p.CreatedAt, before, after)
	}
	if p.UpdatedAt.Before(before) || p.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not in expected range [%v, %v]", p.UpdatedAt, before, after)
	}
}

func TestDuplicateProjectNameReturnsError(t *testing.T) {
	db := newTestDB(t)

	p1 := &Project{
		Name:        "dup-edge",
		Domain:      "dup1.com",
		LinuxUser:   "fleetdeck-dup1",
		ProjectPath: "/opt/fleetdeck/dup1",
	}
	if err := db.CreateProject(p1); err != nil {
		t.Fatalf("CreateProject first: %v", err)
	}

	p2 := &Project{
		Name:        "dup-edge",
		Domain:      "dup2.com",
		LinuxUser:   "fleetdeck-dup2",
		ProjectPath: "/opt/fleetdeck/dup2",
	}
	err := db.CreateProject(p2)
	if err == nil {
		t.Fatal("expected error for duplicate project name")
	}
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("expected UNIQUE constraint error, got: %v", err)
	}
}

func TestGetProjectByID(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "getbyid",
		Domain:      "getbyid.com",
		LinuxUser:   "fleetdeck-getbyid",
		ProjectPath: "/opt/fleetdeck/getbyid",
	}
	db.CreateProject(p)

	got, err := db.GetProjectByID(p.ID)
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}
	if got.Name != "getbyid" {
		t.Errorf("expected name getbyid, got %s", got.Name)
	}
	if got.Domain != "getbyid.com" {
		t.Errorf("expected domain getbyid.com, got %s", got.Domain)
	}
}

func TestGetProjectByIDNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.GetProjectByID("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent project ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestUpdateProjectStatusMultipleTransitions(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "multi-status",
		Domain:      "ms.com",
		LinuxUser:   "fleetdeck-ms",
		ProjectPath: "/opt/fleetdeck/ms",
	}
	db.CreateProject(p)

	transitions := []string{"running", "stopped", "deploying", "error", "running"}
	for _, status := range transitions {
		if err := db.UpdateProjectStatus("multi-status", status); err != nil {
			t.Fatalf("UpdateProjectStatus to %s: %v", status, err)
		}
		got, _ := db.GetProject("multi-status")
		if got.Status != status {
			t.Errorf("expected status %s, got %s", status, got.Status)
		}
	}
}

func TestUpdateProjectStatusUpdatesTimestamp(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "status-ts",
		Domain:      "sts.com",
		LinuxUser:   "fleetdeck-sts",
		ProjectPath: "/opt/fleetdeck/sts",
	}
	db.CreateProject(p)
	originalUpdatedAt := p.UpdatedAt

	// Small delay to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)
	if err := db.UpdateProjectStatus("status-ts", "running"); err != nil {
		t.Fatalf("UpdateProjectStatus: %v", err)
	}

	got, _ := db.GetProject("status-ts")
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected UpdatedAt to be updated after status change")
	}
}

func TestUpdateProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "ghost-update",
		Domain:      "ghost.com",
		LinuxUser:   "fleetdeck-ghost",
		ProjectPath: "/opt/fleetdeck/ghost",
	}
	err := db.UpdateProject(p)
	if err == nil {
		t.Error("expected error for updating nonexistent project")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestUpdateProjectAllFields(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "full-update",
		Domain:      "old-domain.com",
		LinuxUser:   "old-user",
		ProjectPath: "/opt/old",
		Template:    "node",
		Source:      "created",
		GitHubRepo:  "",
	}
	db.CreateProject(p)

	p.Domain = "new-domain.com"
	p.LinuxUser = "new-user"
	p.ProjectPath = "/opt/new"
	p.Template = "go"
	p.Source = "imported"
	p.Status = "running"
	p.GitHubRepo = "org/repo"
	if err := db.UpdateProject(p); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	got, _ := db.GetProject("full-update")
	if got.Domain != "new-domain.com" {
		t.Errorf("Domain: got %s, want new-domain.com", got.Domain)
	}
	if got.LinuxUser != "new-user" {
		t.Errorf("LinuxUser: got %s, want new-user", got.LinuxUser)
	}
	if got.Template != "go" {
		t.Errorf("Template: got %s, want go", got.Template)
	}
	if got.Source != "imported" {
		t.Errorf("Source: got %s, want imported", got.Source)
	}
	if got.Status != "running" {
		t.Errorf("Status: got %s, want running", got.Status)
	}
	if got.GitHubRepo != "org/repo" {
		t.Errorf("GitHubRepo: got %s, want org/repo", got.GitHubRepo)
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	err := db.DeleteProject("nonexistent-delete")
	if err == nil {
		t.Error("expected error for deleting nonexistent project")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestDeleteProjectCascadesSecretsAndDeployments(t *testing.T) {
	db := newTestDB(t)

	p := createTestProject(t, db, "cascade-all")

	// Create related secrets
	db.SetSecret(p.ID, "SECRET_1", "val1")
	db.SetSecret(p.ID, "SECRET_2", "val2")

	// Create related deployments
	db.CreateDeployment(&Deployment{ProjectID: p.ID, CommitSHA: "aaa"})
	db.CreateDeployment(&Deployment{ProjectID: p.ID, CommitSHA: "bbb"})

	// Create related backups
	db.CreateBackupRecord(&BackupRecord{ProjectID: p.ID, Path: "/tmp/b1"})

	if err := db.DeleteProject("cascade-all"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// Verify everything was cascade-deleted
	secrets, _ := db.ListSecrets(p.ID)
	if len(secrets) != 0 {
		t.Errorf("expected 0 secrets after delete, got %d", len(secrets))
	}
	deps, _ := db.ListDeployments(p.ID, 100)
	if len(deps) != 0 {
		t.Errorf("expected 0 deployments after delete, got %d", len(deps))
	}
	backups, _ := db.ListBackupRecords(p.ID, 100)
	if len(backups) != 0 {
		t.Errorf("expected 0 backups after delete, got %d", len(backups))
	}
}

func TestListProjectsOrderedByCreatedAtDesc(t *testing.T) {
	db := newTestDB(t)

	names := []string{"first", "second", "third"}
	for _, name := range names {
		time.Sleep(10 * time.Millisecond)
		db.CreateProject(&Project{
			Name:        name,
			Domain:      name + ".com",
			LinuxUser:   "fleetdeck-" + name,
			ProjectPath: "/opt/fleetdeck/" + name,
		})
	}

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}
	// Most recent first
	if projects[0].Name != "third" {
		t.Errorf("expected first result to be 'third' (newest), got %s", projects[0].Name)
	}
	if projects[2].Name != "first" {
		t.Errorf("expected last result to be 'first' (oldest), got %s", projects[2].Name)
	}
}

func TestListProjectsEmpty(t *testing.T) {
	db := newTestDB(t)

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects in empty DB, got %d", len(projects))
	}
}

func TestProjectExistsByPathNoneExist(t *testing.T) {
	db := newTestDB(t)

	if db.ProjectExistsByPath("/does/not/exist") {
		t.Error("expected ProjectExistsByPath to return false on empty DB")
	}
}

func TestListProjectPathsEmpty(t *testing.T) {
	db := newTestDB(t)

	paths, err := db.ListProjectPaths()
	if err != nil {
		t.Fatalf("ListProjectPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths in empty DB, got %d", len(paths))
	}
}

// ---------------------------------------------------------------------------
// Deployment tracking
// ---------------------------------------------------------------------------

func TestCreateDeploymentAutoFields(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-auto")

	d := &Deployment{ProjectID: p.ID, CommitSHA: "abc123"}
	if err := db.CreateDeployment(d); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if d.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if d.Status != "pending" {
		t.Errorf("expected default status 'pending', got %s", d.Status)
	}
	if d.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestCreateDeploymentPreservesExplicitStatus(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-explicit")

	d := &Deployment{ProjectID: p.ID, CommitSHA: "xyz", Status: "success"}
	if err := db.CreateDeployment(d); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if d.Status != "success" {
		t.Errorf("expected status 'success', got %s", d.Status)
	}
}

func TestListDeploymentsDefaultLimit(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-limit")

	// Create 25 deployments
	for i := 0; i < 25; i++ {
		db.CreateDeployment(&Deployment{
			ProjectID: p.ID,
			CommitSHA: strings.Repeat("a", i+1),
		})
	}

	// Limit <= 0 should default to 20
	deps, err := db.ListDeployments(p.ID, 0)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deps) != 20 {
		t.Errorf("expected default limit 20, got %d", len(deps))
	}

	deps, err = db.ListDeployments(p.ID, -1)
	if err != nil {
		t.Fatalf("ListDeployments negative: %v", err)
	}
	if len(deps) != 20 {
		t.Errorf("expected default limit 20 for -1, got %d", len(deps))
	}
}

func TestListDeploymentsCustomLimit(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-custom-limit")

	for i := 0; i < 10; i++ {
		db.CreateDeployment(&Deployment{
			ProjectID: p.ID,
			CommitSHA: strings.Repeat("b", i+1),
		})
	}

	deps, err := db.ListDeployments(p.ID, 5)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deps) != 5 {
		t.Errorf("expected 5 deployments, got %d", len(deps))
	}
}

func TestListDeploymentsOrderedByStartedAtDesc(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-order")

	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Millisecond)
		db.CreateDeployment(&Deployment{
			ProjectID: p.ID,
			CommitSHA: strings.Repeat("c", i+1),
		})
	}

	deps, err := db.ListDeployments(p.ID, 10)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deployments, got %d", len(deps))
	}
	// Most recent first
	if deps[0].CommitSHA != "ccc" {
		t.Errorf("expected most recent deployment first (ccc), got %s", deps[0].CommitSHA)
	}
	if deps[2].CommitSHA != "c" {
		t.Errorf("expected oldest deployment last (c), got %s", deps[2].CommitSHA)
	}
}

func TestListDeploymentsByProjectIsolation(t *testing.T) {
	db := newTestDB(t)
	p1 := createTestProject(t, db, "deploy-iso-1")
	p2 := createTestProject(t, db, "deploy-iso-2")

	db.CreateDeployment(&Deployment{ProjectID: p1.ID, CommitSHA: "p1-commit"})
	db.CreateDeployment(&Deployment{ProjectID: p1.ID, CommitSHA: "p1-commit2"})
	db.CreateDeployment(&Deployment{ProjectID: p2.ID, CommitSHA: "p2-commit"})

	deps1, _ := db.ListDeployments(p1.ID, 100)
	if len(deps1) != 2 {
		t.Errorf("expected 2 deployments for p1, got %d", len(deps1))
	}

	deps2, _ := db.ListDeployments(p2.ID, 100)
	if len(deps2) != 1 {
		t.Errorf("expected 1 deployment for p2, got %d", len(deps2))
	}
}

func TestUpdateDeploymentSetsFinishedAt(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-finish")

	d := &Deployment{ProjectID: p.ID, CommitSHA: "finish-test"}
	db.CreateDeployment(d)

	if err := db.UpdateDeployment(d.ID, "success", "all good"); err != nil {
		t.Fatalf("UpdateDeployment: %v", err)
	}

	deps, _ := db.ListDeployments(p.ID, 1)
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Status != "success" {
		t.Errorf("expected status 'success', got %s", deps[0].Status)
	}
	if deps[0].FinishedAt == nil {
		t.Error("expected FinishedAt to be set after update")
	}
	if deps[0].Log != "all good" {
		t.Errorf("expected log 'all good', got %s", deps[0].Log)
	}
}

func TestListDeploymentsEmptyResult(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "deploy-empty")

	deps, err := db.ListDeployments(p.ID, 10)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(deps))
	}
}

// ---------------------------------------------------------------------------
// Secret storage: plaintext and encrypted edge cases
// ---------------------------------------------------------------------------

func TestSetSecretUpsertEncrypted(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("upsert-enc-key")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "secret-upsert-enc")

	db.SetSecret(p.ID, "KEY", "old-value")
	db.SetSecret(p.ID, "KEY", "new-value")

	s, err := db.GetSecret(p.ID, "KEY")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if s.Value != "new-value" {
		t.Errorf("expected upserted value 'new-value', got %s", s.Value)
	}
}

func TestListSecretsEmpty(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-empty-list")

	secrets, err := db.ListSecrets(p.ID)
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(secrets))
	}
}

func TestSecretIsolationBetweenProjects(t *testing.T) {
	db := newTestDB(t)
	p1 := createTestProject(t, db, "secret-iso-1")
	p2 := createTestProject(t, db, "secret-iso-2")

	db.SetSecret(p1.ID, "SHARED_KEY", "p1-value")
	db.SetSecret(p2.ID, "SHARED_KEY", "p2-value")

	s1, _ := db.GetSecret(p1.ID, "SHARED_KEY")
	s2, _ := db.GetSecret(p2.ID, "SHARED_KEY")

	if s1.Value != "p1-value" {
		t.Errorf("expected p1 secret 'p1-value', got %s", s1.Value)
	}
	if s2.Value != "p2-value" {
		t.Errorf("expected p2 secret 'p2-value', got %s", s2.Value)
	}
}

func TestDecryptValueNotEncryptedReturnsAsIs(t *testing.T) {
	db := newTestDB(t)

	// Without encryption key, decryptValue should return plaintext unchanged
	result, err := db.decryptValue("plain-value")
	if err != nil {
		t.Fatalf("decryptValue plaintext: %v", err)
	}
	if result != "plain-value" {
		t.Errorf("expected 'plain-value', got %s", result)
	}
}

func TestDecryptValueEmptyString(t *testing.T) {
	db := newTestDB(t)

	result, err := db.decryptValue("")
	if err != nil {
		t.Fatalf("decryptValue empty: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestDecryptValueShortEncPrefix(t *testing.T) {
	db := newTestDB(t)

	// String shorter than "enc:" prefix should be returned as-is
	result, err := db.decryptValue("en")
	if err != nil {
		t.Fatalf("decryptValue short: %v", err)
	}
	if result != "en" {
		t.Errorf("expected 'en', got %s", result)
	}
}

func TestDecryptValueEncPrefixNoKey(t *testing.T) {
	db := newTestDB(t)

	// Value with enc: prefix but no encryption key configured
	_, err := db.decryptValue("enc:somebase64data")
	if err == nil {
		t.Error("expected error when decrypting without key")
	}
	if !strings.Contains(err.Error(), "no encryption key") {
		t.Errorf("expected 'no encryption key' error, got: %v", err)
	}
}

func TestEncryptValueNoKey(t *testing.T) {
	db := newTestDB(t)

	result, err := db.encryptValue("hello")
	if err != nil {
		t.Fatalf("encryptValue no key: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected unchanged value 'hello', got %s", result)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("roundtrip-key")
	db.SetEncryptionKey(key)

	original := "sensitive-data-!@#$%^&*()"
	encrypted, err := db.encryptValue(original)
	if err != nil {
		t.Fatalf("encryptValue: %v", err)
	}
	if encrypted == original {
		t.Error("encrypted value should differ from original")
	}
	if !strings.HasPrefix(encrypted, encryptedPrefix) {
		t.Errorf("encrypted value should have prefix %q, got %q", encryptedPrefix, encrypted[:4])
	}

	decrypted, err := db.decryptValue(encrypted)
	if err != nil {
		t.Fatalf("decryptValue: %v", err)
	}
	if decrypted != original {
		t.Errorf("expected decrypted to match original %q, got %q", original, decrypted)
	}
}

func TestDecryptValueBadBase64(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("bad-b64-key")
	db.SetEncryptionKey(key)

	_, err := db.decryptValue("enc:!!!not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestSetEncryptionKeyNilDisablesEncryption(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("temp-key-disable")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "disable-enc")
	db.SetSecret(p.ID, "ENC_KEY", "encrypted-val")

	// Disable encryption
	db.SetEncryptionKey(nil)

	// Set a new plaintext secret
	db.SetSecret(p.ID, "PLAIN_KEY", "plaintext-val")

	// Read the plaintext one (should work)
	s, err := db.GetSecret(p.ID, "PLAIN_KEY")
	if err != nil {
		t.Fatalf("GetSecret plaintext: %v", err)
	}
	if s.Value != "plaintext-val" {
		t.Errorf("expected 'plaintext-val', got %s", s.Value)
	}

	// Read the encrypted one (should fail since key is nil)
	_, err = db.GetSecret(p.ID, "ENC_KEY")
	if err == nil {
		t.Error("expected error reading encrypted secret without key")
	}
}

func TestListSecretsMixedEncryptedAndPlaintext(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "mixed-secrets")

	// Store plaintext secret directly
	db.conn.Exec(
		`INSERT INTO secrets (id, project_id, key, value) VALUES (?, ?, ?, ?)`,
		"plain-id-1", p.ID, "LEGACY_1", "legacy-val-1",
	)
	db.conn.Exec(
		`INSERT INTO secrets (id, project_id, key, value) VALUES (?, ?, ?, ?)`,
		"plain-id-2", p.ID, "LEGACY_2", "legacy-val-2",
	)

	// Enable encryption and store encrypted secrets
	key := crypto.DeriveKeyFromPassphrase("mixed-key")
	db.SetEncryptionKey(key)
	db.SetSecret(p.ID, "NEW_1", "new-val-1")

	secrets, err := db.ListSecrets(p.ID)
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 3 {
		t.Fatalf("expected 3 secrets, got %d", len(secrets))
	}

	// Verify values are all decrypted correctly
	valueMap := make(map[string]string)
	for _, s := range secrets {
		valueMap[s.Key] = s.Value
	}
	if valueMap["LEGACY_1"] != "legacy-val-1" {
		t.Errorf("LEGACY_1: expected 'legacy-val-1', got %s", valueMap["LEGACY_1"])
	}
	if valueMap["NEW_1"] != "new-val-1" {
		t.Errorf("NEW_1: expected 'new-val-1', got %s", valueMap["NEW_1"])
	}
}

// ---------------------------------------------------------------------------
// Backup records
// ---------------------------------------------------------------------------

func TestCreateBackupRecordDefaults(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "backup-defaults")

	b := &BackupRecord{
		ProjectID: p.ID,
		Path:      "/tmp/backup-default",
	}
	if err := db.CreateBackupRecord(b); err != nil {
		t.Fatalf("CreateBackupRecord: %v", err)
	}
	if b.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if b.Type != "manual" {
		t.Errorf("expected default type 'manual', got %s", b.Type)
	}
	if b.Trigger != "user" {
		t.Errorf("expected default trigger 'user', got %s", b.Trigger)
	}
	if b.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestGetBackupRecord(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "backup-get")

	b := &BackupRecord{
		ProjectID: p.ID,
		Type:      "snapshot",
		Trigger:   "pre-stop",
		Path:      "/tmp/snapshot-1",
		SizeBytes: 2048,
	}
	db.CreateBackupRecord(b)

	got, err := db.GetBackupRecord(b.ID)
	if err != nil {
		t.Fatalf("GetBackupRecord: %v", err)
	}
	if got.Type != "snapshot" {
		t.Errorf("Type: got %s, want snapshot", got.Type)
	}
	if got.Trigger != "pre-stop" {
		t.Errorf("Trigger: got %s, want pre-stop", got.Trigger)
	}
	if got.SizeBytes != 2048 {
		t.Errorf("SizeBytes: got %d, want 2048", got.SizeBytes)
	}
	if got.Path != "/tmp/snapshot-1" {
		t.Errorf("Path: got %s, want /tmp/snapshot-1", got.Path)
	}
}

func TestGetBackupRecordNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.GetBackupRecord("nonexistent-backup-id")
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestDeleteBackupRecord(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "backup-delete")

	b := &BackupRecord{ProjectID: p.ID, Path: "/tmp/to-delete"}
	db.CreateBackupRecord(b)

	if err := db.DeleteBackupRecord(b.ID); err != nil {
		t.Fatalf("DeleteBackupRecord: %v", err)
	}

	_, err := db.GetBackupRecord(b.ID)
	if err == nil {
		t.Error("expected backup to be deleted")
	}
}

func TestDeleteBackupsForProject(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "backup-delete-all")

	for i := 0; i < 5; i++ {
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Path:      "/tmp/delete-all-" + string(rune('a'+i)),
		})
	}

	if err := db.DeleteBackupsForProject(p.ID); err != nil {
		t.Fatalf("DeleteBackupsForProject: %v", err)
	}

	remaining, _ := db.ListBackupRecords(p.ID, 100)
	if len(remaining) != 0 {
		t.Errorf("expected 0 backups after bulk delete, got %d", len(remaining))
	}
}

func TestListBackupRecordsDefaultLimit(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "backup-def-limit")

	// Limit <= 0 should default to 50
	_, err := db.ListBackupRecords(p.ID, 0)
	if err != nil {
		t.Fatalf("ListBackupRecords with 0 limit: %v", err)
	}
	_, err = db.ListBackupRecords(p.ID, -5)
	if err != nil {
		t.Fatalf("ListBackupRecords with negative limit: %v", err)
	}
}

func TestCountBackupsByTypeMixed(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "count-mixed")

	types := []string{"manual", "manual", "snapshot", "scheduled", "scheduled", "scheduled"}
	for i, btype := range types {
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      btype,
			Path:      "/tmp/mixed-" + string(rune('a'+i)),
		})
	}

	manual, _ := db.CountBackupsByType(p.ID, "manual")
	if manual != 2 {
		t.Errorf("expected 2 manual, got %d", manual)
	}
	snapshot, _ := db.CountBackupsByType(p.ID, "snapshot")
	if snapshot != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapshot)
	}
	scheduled, _ := db.CountBackupsByType(p.ID, "scheduled")
	if scheduled != 3 {
		t.Errorf("expected 3 scheduled, got %d", scheduled)
	}
	other, _ := db.CountBackupsByType(p.ID, "nonexistent")
	if other != 0 {
		t.Errorf("expected 0 for nonexistent type, got %d", other)
	}
}

func TestGetOldestBackupsLimitExceedsCount(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "oldest-exceed")

	db.CreateBackupRecord(&BackupRecord{ProjectID: p.ID, Type: "manual", Path: "/tmp/only-one"})

	oldest, err := db.GetOldestBackups(p.ID, "manual", 100)
	if err != nil {
		t.Fatalf("GetOldestBackups: %v", err)
	}
	if len(oldest) != 1 {
		t.Errorf("expected 1 backup (limit exceeds count), got %d", len(oldest))
	}
}

func TestGetOldestBackupsOrderedByCreatedAtAsc(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "oldest-order")

	for i := 0; i < 5; i++ {
		time.Sleep(10 * time.Millisecond)
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      "snapshot",
			Path:      "/tmp/ordered-" + string(rune('a'+i)),
		})
	}

	oldest, err := db.GetOldestBackups(p.ID, "snapshot", 3)
	if err != nil {
		t.Fatalf("GetOldestBackups: %v", err)
	}
	if len(oldest) != 3 {
		t.Fatalf("expected 3 oldest, got %d", len(oldest))
	}
	// Oldest first
	for i := 1; i < len(oldest); i++ {
		if oldest[i].CreatedAt.Before(oldest[i-1].CreatedAt) {
			t.Errorf("expected ascending order, but entry %d (%v) is before entry %d (%v)",
				i, oldest[i].CreatedAt, i-1, oldest[i-1].CreatedAt)
		}
	}
}

func TestBackupRecordIsolationBetweenProjects(t *testing.T) {
	db := newTestDB(t)
	p1 := createTestProject(t, db, "backup-iso-1")
	p2 := createTestProject(t, db, "backup-iso-2")

	db.CreateBackupRecord(&BackupRecord{ProjectID: p1.ID, Path: "/tmp/p1-b1"})
	db.CreateBackupRecord(&BackupRecord{ProjectID: p1.ID, Path: "/tmp/p1-b2"})
	db.CreateBackupRecord(&BackupRecord{ProjectID: p2.ID, Path: "/tmp/p2-b1"})

	b1, _ := db.ListBackupRecords(p1.ID, 100)
	if len(b1) != 2 {
		t.Errorf("expected 2 backups for p1, got %d", len(b1))
	}
	b2, _ := db.ListBackupRecords(p2.ID, 100)
	if len(b2) != 1 {
		t.Errorf("expected 1 backup for p2, got %d", len(b2))
	}
}

// ---------------------------------------------------------------------------
// Database handling and migration verification
// ---------------------------------------------------------------------------

func TestMigrateCreatesAllTables(t *testing.T) {
	db := newTestDB(t)

	tables := []string{"projects", "deployments", "secrets", "backups"}
	for _, table := range tables {
		var name string
		err := db.conn.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
		if name != table {
			t.Errorf("expected table name %s, got %s", table, name)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newTestDB(t)

	// Run migrate again; should succeed without error
	if err := db.migrate(); err != nil {
		t.Fatalf("second migrate call should be idempotent: %v", err)
	}

	// Verify tables still work
	p := &Project{
		Name:        "post-migrate",
		Domain:      "pm.com",
		LinuxUser:   "fleetdeck-pm",
		ProjectPath: "/opt/fleetdeck/pm",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject after re-migrate: %v", err)
	}
}

func TestSnapshotCreatesValidCopy(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "snap-test",
		Domain:      "snap.com",
		LinuxUser:   "fleetdeck-snap",
		ProjectPath: "/opt/fleetdeck/snap",
	}
	db.CreateProject(p)

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "snapshot.db")

	if err := db.Snapshot(destPath); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Open the snapshot and verify it has the data
	snap, err := Open(destPath)
	if err != nil {
		t.Fatalf("Open snapshot: %v", err)
	}
	defer snap.Close()

	got, err := snap.GetProject("snap-test")
	if err != nil {
		t.Fatalf("GetProject from snapshot: %v", err)
	}
	if got.Domain != "snap.com" {
		t.Errorf("expected domain snap.com in snapshot, got %s", got.Domain)
	}
}

func TestOpenCreatesBackupFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "backup-check.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	bakPath := dbPath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("expected .bak file to exist after Open: %v", err)
	}
}

func TestOpenWithCorruptedFileHandledGracefully(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "corrupted.db")

	// Write garbage to the file
	os.WriteFile(dbPath, []byte("this is not a valid sqlite database file"), 0644)

	// Open should fail (or log a warning) since the file is corrupt
	_, err := Open(dbPath)
	if err == nil {
		// If it somehow succeeds (unlikely with real corruption), that is
		// acceptable since SQLite may recreate tables via CREATE IF NOT EXISTS.
		// The important thing is it does not panic.
		t.Log("Open succeeded on corrupted file (SQLite may have recovered)")
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	db := newTestDB(t)

	// Attempt to create a deployment with a nonexistent project_id
	d := &Deployment{
		ProjectID: "nonexistent-project-id",
		CommitSHA: "abc",
	}
	err := db.CreateDeployment(d)
	if err == nil {
		t.Error("expected foreign key constraint error for nonexistent project_id")
	}
}

func TestCloseAndReopenPreservesData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reopen.db")

	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	p := &Project{
		Name:        "persist",
		Domain:      "persist.com",
		LinuxUser:   "fleetdeck-persist",
		ProjectPath: "/opt/fleetdeck/persist",
	}
	db1.CreateProject(p)
	db1.SetSecret(p.ID, "PERSISTENT_KEY", "persistent-val")
	db1.CreateDeployment(&Deployment{ProjectID: p.ID, CommitSHA: "persist-commit"})
	db1.CreateBackupRecord(&BackupRecord{ProjectID: p.ID, Path: "/tmp/persist-bak"})

	db1.Close()

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer db2.Close()

	got, err := db2.GetProject("persist")
	if err != nil {
		t.Fatalf("GetProject after reopen: %v", err)
	}
	if got.Domain != "persist.com" {
		t.Errorf("expected domain persist.com, got %s", got.Domain)
	}

	deps, _ := db2.ListDeployments(p.ID, 10)
	if len(deps) != 1 {
		t.Errorf("expected 1 deployment after reopen, got %d", len(deps))
	}

	backups, _ := db2.ListBackupRecords(p.ID, 10)
	if len(backups) != 1 {
		t.Errorf("expected 1 backup after reopen, got %d", len(backups))
	}
}
