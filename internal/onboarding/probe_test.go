package onboarding

import (
	"os/exec"
	"testing"
)

// Execute the actual Python collector with synthetic command output. The fixture
// proves stopped containers, IPv6 listeners and enabled native units are retained,
// and permission errors stay unknown without echoing sensitive diagnostics.
func TestLinuxProbeParsing(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 not installed")
	}
	fixture := `
from unittest.mock import patch
outputs = {
 ('docker','ps','-a','--format','{{.ID}}'): 'aabbccddeeff\n',
 ('systemctl','list-units','--type=service','--state=running','--no-legend','--plain','--no-pager'): 'ssh.service loaded active running OpenSSH\n',
 ('systemctl','list-unit-files','--type=service','--state=enabled','--no-legend','--no-pager'): 'database.service enabled enabled\n',
 ('ss','-H','-lntu'): 'tcp LISTEN 0 128 [::1]:5432 [::]:*\ntcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\n',
 ('nft','-j','list','ruleset'): '{"nftables":[{"metainfo":{}},{"table":{"name":"operator"}}]}',
 ('iptables-save',): '', ('ip6tables-save',): '',
 ('systemctl','list-unit-files','--type=timer','--state=enabled','--no-legend','--no-pager'): 'backup.timer enabled enabled\n',
}
def fake_run(argv): return outputs[tuple(argv)]
with patch('os.geteuid',return_value=0), patch('pathlib.Path.read_text',return_value='ID=ubuntu\nVERSION_ID="24.04"'), patch('pathlib.Path.is_dir',return_value=True), patch('pathlib.Path.exists',return_value=False), patch('pathlib.Path.is_symlink',return_value=False), patch('shutil.which',return_value='/usr/bin/tool'):
 run=fake_run
 result=collect()
 assert result['complete'], result
 assert result['containers']==1
 assert result['services']==['database.service','ssh.service']
 assert {'protocol':'tcp','port':5432,'loopback':True} in result['listeners']
 assert result['firewall_rules']==1
 assert result['timers']==['backup.timer']
 def broken(argv):
  if argv[0]=='nft': raise RuntimeError('private diagnostic')
  return fake_run(argv)
 run=broken
 result=collect()
 assert not result['complete'] and result['unknown']==['firewall']
 assert 'private diagnostic' not in json.dumps(result)
`
	script := "__name__='fixture'\n" + inventoryScript + fixture
	if out, err := exec.Command(python, "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("probe fixture failed: %v\n%s", err, out)
	}
}
