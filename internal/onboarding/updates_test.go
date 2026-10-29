package onboarding

import (
	"os/exec"
	"testing"
)

func TestUpdateArgumentsBeforeHostAccess(t *testing.T) {
	r := &fakeRunner{}
	for _, w := range []string{"25:00", "4:00", "00:60", "04:00; reboot"} {
		if _, err := ConfigureUpdates(r, "enable", w); err == nil {
			t.Fatal("invalid time accepted")
		}
	}
	if len(r.commands) != 0 {
		t.Fatal("host accessed for invalid input")
	}
}
func TestSecurityOnlyPolicy(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 unavailable")
	}
	// Load definitions only; the real script's entry point must not run in tests.
	program := "__name__='fixture'\noptions={}\n" + updatesScript + `
for osname,version,code in [('ubuntu','24.04','noble'),('debian','12','bookworm'),('debian','13','trixie')]:
 origin,policy=policy_for(osname,version)
 assert code+'-security' in origin and '-updates' not in origin
 assert b'#clear Unattended-Upgrade::Allowed-Origins;' in policy
 assert b'#clear Unattended-Upgrade::Origins-Pattern;' in policy
 assert b'Automatic-Reboot "false"' in policy
 assert b'Remove-Unused-Dependencies "false"' in policy
try: policy_for('ubuntu','16.04')
except RuntimeError: pass
else: raise AssertionError('unsupported distribution accepted')
`
	if out, err := exec.Command(python, "-c", program).CombinedOutput(); err != nil {
		t.Fatalf("update policy failed: %v\n%s", err, out)
	}
}

func TestUpdateActivationRollbackAndDisable(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 unavailable")
	}
	fixture := `
from unittest.mock import patch
import types,sys
with tempfile.TemporaryDirectory() as directory:
 root=pathlib.Path(directory)
 POLICY=root/'apt/policy';TIMER=root/'systemd/timer';STATE=root/'state'
 require_real_parents=lambda path: None
 enabled=lambda name: True
 active=lambda name: True
 verify_policy=lambda origin: None
 sys.modules['apt_pkg']=types.SimpleNamespace()
 original_read=pathlib.Path.read_text
 def read(path,*args,**kwargs):
  if str(path)=='/etc/os-release':return 'ID=ubuntu\nVERSION_ID="24.04"'
  return original_read(path,*args,**kwargs)
 commands=[]
 def command(argv,timeout=60):
  commands.append(argv)
  return ''
 run=command
 class Result:
  def __init__(self,code): self.returncode=code
 def fail_dry_run(args,**kwargs):return Result(1)
 options={'mode':'enable','window':'04:00'}
 with patch('os.geteuid',return_value=0),patch('pathlib.Path.read_text',read),patch('subprocess.run',fail_dry_run):
  try: main()
  except RuntimeError:pass
  else:raise AssertionError('failed dry run accepted')
  assert not POLICY.exists() and not TIMER.exists() and not (STATE/'current.json').exists()
  assert ['systemctl','enable','apt-daily-upgrade.timer'] in commands
  assert ['systemctl','start','apt-daily-upgrade.timer'] in commands
 def good_dry_run(args,**kwargs):return Result(0 if args[0]=='unattended-upgrade' else 1)
 with patch('os.geteuid',return_value=0),patch('pathlib.Path.read_text',read),patch('subprocess.run',good_dry_run):
  main()
  assert POLICY.exists() and TIMER.exists() and (STATE/'current.json').exists()
  assert (STATE/'current.json').stat().st_mode&0o777==0o600
  original=POLICY.read_bytes();POLICY.write_bytes(original+b'// operator change\n')
  options['mode']='disable'
  try:main()
  except RuntimeError:pass
  else:raise AssertionError('operator policy overwritten')
  assert POLICY.read_bytes().endswith(b'// operator change\n')
  POLICY.write_bytes(original)
  main()
  assert not POLICY.exists() and not TIMER.exists() and not (STATE/'current.json').exists()
`
	if out, err := exec.Command(python, "-c", "__name__='fixture'\n"+updatesScript+fixture).CombinedOutput(); err != nil {
		t.Fatalf("update lifecycle failed: %v\n%s", err, out)
	}
}
