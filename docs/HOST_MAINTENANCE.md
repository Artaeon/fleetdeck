# Host security maintenance

A clean APT upgrade simulation means no upgrades are offered by the configured
repositories. It does not prove that every CVE has a patch, that every installed
service uses current libraries, or that application container images are current.
The security watcher reports available updates and never installs them. A separate,
explicit [OS security-update policy](SERVER_SETUP.md#automatic-os-security-updates)
can configure unattended-upgrades in a maintenance window, without automatic
reboots. Follow the procedure below for manual maintenance and recovery.

1. Verify a fresh administrative login with the configured MFA and a separate
   recovery console. Keep the authenticated session open. Record the active SSH
   policy **for the administrator's user/address** (`sshd -T -C ...`); a global
   `sshd -T` alone omits `Match` rules. Record firewall rules and project health.
2. Save package versions and host configuration in a private encrypted archive,
   copy it independently and verify its checksum. Separately verify application
   backups and restoration. An `/etc` backup cannot restore a disk or downgrade
   arbitrary packages; use a provider/filesystem snapshot when rollback requires it.
3. Refresh signed package indexes and refuse partial refresh failures. Review
   `apt-get --simulate upgrade`, removals, held packages, repository origins,
   kernel changes and service restarts. Pin the approved package versions for
   the maintenance run; stop and review if the package plan changes.
4. Install only the reviewed changes, preserving locally managed SSH, firewall
   and identity configuration. Check package-manager exit status and `dpkg --audit`.
   Do not disable MFA or weaken firewall rules to make an update easier.
5. Review `needrestart` and `/var/run/reboot-required`. Restart affected services
   deliberately, then test their consumers. A reboot needs a maintenance window,
   tested recovery access and a new MFA login afterward. Never infer that it is
   unnecessary solely because no further APT updates remain.
6. Compare the effective SSH policy, firewall, service state and application
   behavior with the baseline. Verify public routes externally as well as local
   health checks, including unauthenticated denial on protected endpoints. Re-run
   the watcher and confirm notification delivery without disclosing private logs.

Container filesystems do not inherit host library fixes. Rebuild or replace each
reviewed application image separately, then scan its immutable ID and test its
startup/migrations/recovery. Optional [maintenance image recipes](../deploy/runtime-maintenance/README.md)
can remove unused package installers or patch selected Alpine libraries while
preserving an existing application bundle. Keep all host inventories, addresses,
accounts, backup archives and maintenance logs outside the public repository.
