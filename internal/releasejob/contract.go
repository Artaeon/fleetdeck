// Package releasejob defines the bounded, product-neutral contract used by
// trusted control planes to ask FleetDeck to apply immutable container images.
// It intentionally contains no shell command or operator-specific routing.
package releasejob

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	SchemaVersion  = 1
	MaxRequestSize = 256 * 1024
)

var (
	identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
	digestPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	imagePattern      = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+@sha256:[a-f0-9]{64}$`)
)

type Request struct {
	SchemaVersion  int             `json:"schema_version"`
	JobID          string          `json:"job_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	ReleaseID      string          `json:"release_id"`
	ManifestDigest string          `json:"manifest_digest"`
	Target         Target          `json:"target"`
	Images         []Image         `json:"images"`
	Backup         BackupPolicy    `json:"backup"`
	Migration      MigrationPolicy `json:"migration"`
	HealthProfile  string          `json:"health_profile"`
}

type Target struct {
	ID                       string  `json:"id"`
	Environment              string  `json:"environment"`
	Profile                  string  `json:"profile"`
	ExpectedCurrentReleaseID *string `json:"expected_current_release_id"`
}

type Image struct {
	Component string `json:"component"`
	Reference string `json:"reference"`
}

type BackupPolicy struct {
	Required bool `json:"required"`
}

type MigrationPolicy struct {
	Mode string `json:"mode"`
}

func Decode(reader io.Reader) (Request, error) {
	limited := &io.LimitedReader{R: reader, N: MaxRequestSize + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()

	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("decode release job: %w", err)
	}
	if limited.N <= 0 {
		return Request{}, fmt.Errorf("release job exceeds %d bytes", MaxRequestSize)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Request{}, errors.New("release job contains trailing JSON")
		}
		return Request{}, fmt.Errorf("decode trailing release job data: %w", err)
	}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (r Request) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported release job schema_version %d", r.SchemaVersion)
	}
	for name, value := range map[string]string{
		"job_id":          r.JobID,
		"idempotency_key": r.IdempotencyKey,
		"release_id":      r.ReleaseID,
		"target.id":       r.Target.ID,
		"target.profile":  r.Target.Profile,
		"health_profile":  r.HealthProfile,
	} {
		if !identifierPattern.MatchString(value) {
			return fmt.Errorf("%s is not a valid bounded identifier", name)
		}
	}
	if !digestPattern.MatchString(r.ManifestDigest) {
		return errors.New("manifest_digest must be a lowercase sha256 digest")
	}
	if r.Target.Environment != "staging" && r.Target.Environment != "production" {
		return errors.New("target.environment must be staging or production")
	}
	if r.Target.ExpectedCurrentReleaseID != nil &&
		!identifierPattern.MatchString(*r.Target.ExpectedCurrentReleaseID) {
		return errors.New("target.expected_current_release_id is invalid")
	}
	if r.Target.Environment == "production" && !r.Backup.Required {
		return errors.New("production release jobs must require a backup")
	}
	if r.Migration.Mode != "preflight-and-apply" && r.Migration.Mode != "none" {
		return errors.New("migration.mode must be preflight-and-apply or none")
	}
	if len(r.Images) == 0 || len(r.Images) > 32 {
		return errors.New("images must contain between 1 and 32 components")
	}

	components := make(map[string]struct{}, len(r.Images))
	for i, image := range r.Images {
		if !identifierPattern.MatchString(image.Component) {
			return fmt.Errorf("images[%d].component is invalid", i)
		}
		if _, exists := components[image.Component]; exists {
			return fmt.Errorf("images contains duplicate component %q", image.Component)
		}
		components[image.Component] = struct{}{}
		if strings.Contains(image.Reference, ":latest") || !imagePattern.MatchString(image.Reference) {
			return fmt.Errorf("images[%d].reference must be an immutable lowercase registry digest", i)
		}
	}
	return nil
}
