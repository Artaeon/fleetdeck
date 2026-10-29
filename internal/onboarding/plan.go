package onboarding

// Plan separates discovery from changes. Steps are deliberately operator-owned:
// an inventory cannot establish backup recoverability or schema compatibility.
type Step struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
}
type Plan struct {
	Mode          string    `json:"mode"`
	Inventory     Inventory `json:"inventory"`
	FreshBlockers []string  `json:"fresh_blockers"`
	Steps         []Step    `json:"steps"`
}

func MakePlan(i Inventory, port string) Plan {
	p := Plan{Mode: "existing", Inventory: i, FreshBlockers: i.FreshBlockers(port)}
	if !i.Complete {
		p.Mode = "unknown"
	} else if len(p.FreshBlockers) == 0 {
		p.Mode = "fresh-candidate"
	}
	p.Steps = []Step{
		{"access", "Verify a new administrator SSH login, MFA, and an independent recovery console; retain the current connection.", ""},
		{"recovery", "Record services, routing and data ownership privately. Encrypt an independent backup and restore it in isolation before making changes.", ""},
		{"registry", "Initialize only the FleetDeck registry, keeping the existing proxy, firewall and services.", "fleetdeck setup --initialize"},
		{"discovery", "Inspect Compose projects. Native services require an explicit containerization/migration plan.", "fleetdeck discover --all"},
		{"adoption", "Import one reviewed Compose project at a time; registration alone does not harden or migrate it.", "fleetdeck import PROJECT --path /absolute/project/path --domain example.invalid"},
		{"isolation", "Review non-root runtime identity, mounts, capabilities and private database networks; test cross-project and host access.", ""},
		{"backups", "Make and restore an application-consistent snapshot before enabling a schedule; include external data and encryption keys.", "fleetdeck backup create PROJECT"},
		{"updates", "Review the host update policy separately from application releases and database migrations.", "fleetdeck server updates plan"},
		{"monitoring", "Configure private mail/Telegram references and run health, origin-firewall and security checks, including alert delivery.", ""},
		{"deployment", "Link this server once on the workstation, then deploy reviewed source with the secret check and pre-update snapshot.", "fleetdeck project link ./app --server production --name app --domain preview.example.invalid"},
	}
	return p
}
