package remote

import (
	"encoding/json"
	"regexp"
)

type VersionInfo struct {
	SourceDeployProtocol string `json:"source_deploy_protocol"`
	Version              string `json:"version"`
	Commit               string `json:"commit"`
}

var safeVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`)
var safeCommit = regexp.MustCompile(`^[a-f0-9]{7,64}$`)

// ReadVersion is an observation, not proof that the host is secure or that its
// deployment features are compatible. Old or absent binaries remain unknown.
func ReadVersion(runner interface{ Run(string) (string, error) }) VersionInfo {
	out, err := runner.Run("fleetdeck version --json")
	if err != nil || len(out) > 4096 {
		return VersionInfo{}
	}
	var info VersionInfo
	if json.Unmarshal([]byte(out), &info) != nil || !safeVersion.MatchString(info.Version) {
		return VersionInfo{}
	}
	if info.Commit != "" && !safeCommit.MatchString(info.Commit) {
		return VersionInfo{}
	}
	return info
}
