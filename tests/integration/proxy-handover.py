#!/usr/bin/env python3
"""Real IPv4/IPv6 NAT handover and rollback in isolated network namespaces."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import select
import shutil
import subprocess
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument("--routes", default=str(Path(__file__).resolve().parents[2] / "deploy/proxy-handover/routes.py"))
parser.add_argument("--systemd-timer", action="store_true")
options = parser.parse_args()
if os.geteuid() != 0:
    raise SystemExit("Run as root on disposable Linux; all network changes stay in new namespaces.")
spec = importlib.util.spec_from_file_location("handover_routes", options.routes)
routes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(routes)
prefix = f"fdph{os.getpid()}"
names = {x: prefix + "-" + x for x in ("client", "edge", "old", "new")}
created = []
children = []
timer = prefix + "-rollback"
params = ("front", "198.18.0.1", "2001:db8:1::1", "10.123.0.3", 18080, 18443)


def run(args, namespace=None, data=None, check=True):
    command = (["ip", "netns", "exec", names[namespace]] if namespace else []) + args
    result = subprocess.run(command, input=data, capture_output=True, text=True, timeout=15)
    if check and result.returncode:
        raise RuntimeError(f"{args[0]} failed: {result.stderr.strip()}")
    return result


def nsip(namespace, *args):
    return run(["ip", *args], namespace)


def link(left, left_if, right, right_if, index):
    a, b = f"p{os.getpid()}a{index}", f"p{os.getpid()}b{index}"
    run(["ip", "link", "add", a, "type", "veth", "peer", "name", b])
    for interface, namespace, name in ((a, left, left_if), (b, right, right_if)):
        run(["ip", "link", "set", interface, "netns", names[namespace]])
        nsip(namespace, "link", "set", interface, "name", name)
        nsip(namespace, "link", "set", name, "up")


server_code = r'''
import socket,threading,json,sys
def serve(port,family,label):
 s=socket.socket(family);s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
 if family==socket.AF_INET6:s.setsockopt(socket.IPPROTO_IPV6,socket.IPV6_V6ONLY,1)
 s.bind(('::' if family==socket.AF_INET6 else '0.0.0.0',port));s.listen()
 def client(c,a):
  with c,c.makefile('rb') as f:
   for line in f:c.sendall((label+'|'+a[0]+'\n').encode())
 while True:
  c,a=s.accept();threading.Thread(target=client,args=(c,a),daemon=True).start()
threads=[]
for port,fam,label in json.loads(sys.argv[1]):
 t=threading.Thread(target=serve,args=(port,socket.AF_INET6 if fam==6 else socket.AF_INET,label));t.start();threads.append(t)
for t in threads:t.join()
'''
client_code = r'''
import socket,sys
s=socket.socket(socket.AF_INET6 if sys.argv[1]=='6' else socket.AF_INET);s.settimeout(2)
s.connect((sys.argv[2],int(sys.argv[3])));print('connected',flush=True)
with s,s.makefile('rb') as f:
 for line in sys.stdin:
  s.sendall(b'ping\n');print(f.readline().decode().strip(),flush=True)
'''


def spawn(namespace, args):
    child = subprocess.Popen(["ip", "netns", "exec", names[namespace], *args],
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    children.append(child)
    return child


def line(child):
    if not select.select([child.stdout], [], [], 5)[0]:
        raise RuntimeError("Test connection timeout")
    value = child.stdout.readline().strip()
    if not value:
        raise RuntimeError("Test connection closed unexpectedly")
    return value


def connect(family, port):
    host = params[1 if family == 4 else 2]
    child = spawn("client", [sys.executable, "-u", "-c", client_code, str(family), host, str(port)])
    assert line(child) == "connected"
    return child


def expect(child, label, family):
    child.stdin.write("ping\n")
    child.stdin.flush()
    response = line(child)
    assert response.split("|")[0] == label, (response, label)
    if family == 4:
        assert response.split("|")[1] == "198.18.0.2", "Client IPv4 was not preserved"


def fresh(family, port, label):
    child = connect(family, port)
    expect(child, label, family)
    child.stdin.close()
    child.wait(timeout=5)


def apply():
    run(["nft", "-f", "-"], "edge", routes.render(*params))


def clear():
    # Keep the NAT hooks while existing redirected flows drain. Removing the
    # last family-specific NAT hook can destroy their translations.
    run(["nft", "flush", "table", "inet", routes.TABLE], "edge")


try:
    for role, name in names.items():
        run(["ip", "netns", "add", name]); created.append(name)
        nsip(role, "link", "set", "lo", "up")
    link("client", "front", "edge", "front", 0)
    for role, last in (("edge", 1), ("client", 2)):
        nsip(role, "addr", "add", f"198.18.0.{last}/24", "dev", "front")
        nsip(role, "-6", "addr", "add", f"2001:db8:1::{last}/64", "dev", "front", "nodad")
    nsip("edge", "link", "add", "back", "type", "bridge")
    nsip("edge", "link", "set", "back", "up")
    nsip("edge", "addr", "add", "10.123.0.1/24", "dev", "back")
    for index, role in enumerate(("old", "new"), 1):
        link("edge", role, role, "back", index)
        nsip("edge", "link", "set", role, "master", "back")
        nsip(role, "addr", "add", f"10.123.0.{index+1}/24", "dev", "back")
        nsip(role, "route", "add", "default", "via", "10.123.0.1")
        spawn(role, [sys.executable, "-u", "-c", server_code, json.dumps([[80, 4, role], [443, 4, role]])])
    run(["sysctl", "-w", "net.ipv4.ip_forward=1"], "edge")
    spawn("edge", [sys.executable, "-u", "-c", server_code,
                   json.dumps([[80, 6, "old"], [443, 6, "old"], [18080, 6, "new"], [18443, 6, "new"]])])
    run(["nft", "-f", "-"], "edge", """table ip baseline {
 chain incoming { type nat hook prerouting priority -100; policy accept;
 ip daddr 198.18.0.1 tcp dport {80,443} dnat to 10.123.0.2
 }
}
""")
    run(["iptables", "-P", "FORWARD", "DROP"], "edge")
    run(["iptables", "-A", "FORWARD", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"], "edge")
    run(["iptables", "-A", "FORWARD", "-i", "front", "-d", "10.123.0.0/24", "-p", "tcp", "-m", "multiport", "--dports", "80,443", "-j", "ACCEPT"], "edge")
    run(["ip6tables", "-P", "INPUT", "DROP"], "edge")
    run(["ip6tables", "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"], "edge")
    run(["ip6tables", "-A", "INPUT", "-p", "ipv6-icmp", "-j", "ACCEPT"], "edge")
    run(["ip6tables", "-A", "INPUT", "-p", "tcp", "-m", "multiport", "--dports", "80,443", "-j", "ACCEPT"], "edge")
    for rule in routes.ipv6_input_rules(*params):
        run(["ip6tables", "-A", "INPUT", *rule], "edge")
    time.sleep(0.3)
    old = {(family, port): connect(family, port) for family in (4, 6) for port in (80, 443)}
    for (family, _), child in old.items(): expect(child, "old", family)
    apply()
    new = {(family, port): connect(family, port) for family in (4, 6) for port in (80, 443)}
    for (family, _), child in old.items(): expect(child, "old", family)
    for (family, _), child in new.items(): expect(child, "new", family)
    # A malformed nft batch must leave the active routing transaction intact.
    bad = run(["nft", "-f", "-"], "edge", routes.render(*params) + "invalid syntax\n", check=False)
    assert bad.returncode != 0
    for family in (4, 6): fresh(family, 443, "new")
    # Direct public access to alternate IPv6 listener ports is never allowed.
    for port in (18080, 18443):
        blocked = run([sys.executable, "-c", "import socket; s=socket.socket(socket.AF_INET6); s.settimeout(0.5); s.connect(('2001:db8:1::1'," + str(port) + "))"], "client", check=False)
        assert blocked.returncode != 0, "Alternate port became public"
    if options.systemd_timer:
        run(["systemd-run", "--quiet", "--unit=" + timer, "--on-active=2s", "--timer-property=AccuracySec=100ms",
             shutil.which("ip"), "netns", "exec", names["edge"], shutil.which("nft"), "flush", "table", "inet", routes.TABLE])
        for _ in range(50):
            if "dnat" not in run(["nft", "list", "table", "inet", routes.TABLE], "edge").stdout: break
            time.sleep(0.1)
        else: raise RuntimeError("Timed rollback did not run")
    else:
        clear()
    for (family, _), child in old.items(): expect(child, "old", family)
    for (family, _), child in new.items(): expect(child, "new", family)
    for family in (4, 6):
        for port in (80, 443): fresh(family, port, "old")
    # Repeated handovers replace the owned table without touching other rules.
    apply(); apply(); clear()
    assert run(["nft", "list", "table", "ip", "baseline"], "edge").returncode == 0
    print(json.dumps({"ipv4": "passed", "ipv6": "passed", "established_connections": "preserved", "rollback": "passed",
                      "alternate_ports": "blocked", "ipv4_client_address": "preserved", "malformed_transaction": "rejected",
                      "systemd_rollback": options.systemd_timer}))
finally:
    if options.systemd_timer:
        subprocess.run(["systemctl", "stop", timer + ".timer", timer + ".service"], capture_output=True)
        subprocess.run(["systemctl", "reset-failed", timer + ".service"], capture_output=True)
    for child in children:
        if child.poll() is None: child.terminate()
    for child in children:
        try: child.communicate(timeout=3)
        except (subprocess.TimeoutExpired, ValueError):
            child.kill(); child.wait()
    for name in reversed(created):
        subprocess.run(["ip", "netns", "delete", name], capture_output=True)
