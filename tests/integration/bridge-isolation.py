#!/usr/bin/env python3
"""Isolated bridge policy and rollback tests; no production Docker access."""
import subprocess,json,pathlib,sys,time,os
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[2]/'deploy/isolation'));from bridge_guard import render
assert os.geteuid()==0, 'Run on a disposable Linux test host as root'
if os.environ.get('FLEETDECK_ISOLATION_TEST_NS') != '1':
 os.execvpe('unshare',['unshare','--net','--',sys.executable,__file__],dict(os.environ,FLEETDECK_ISOLATION_TEST_NS='1'))
prefix='fdi'+format(os.getpid(),'x');bridge=prefix+'br';table='fleetdeck_tenant_fixture_'+format(os.getpid(),'x');children=[];names=[];created_bridge=False
def run(a,**kw):return subprocess.run(a,check=True,capture_output=True,text=True,**kw).stdout
try:
 run(['ip','link','add',bridge,'type','bridge']);created_bridge=True;run(['ip','link','set',bridge,'up'])
 ports=[]
 for i,group in enumerate(['tenant-a','tenant-a','tenant-b','proxy'],1):
  ns=prefix+str(i);v=prefix+'v'+str(i)
  run(['ip','netns','add',ns]);names.append(ns);run(['ip','link','add',v,'type','veth','peer','name','eth0','netns',ns]);run(['ip','link','set',v,'master',bridge]);run(['ip','link','set',v,'up'])
  run(['ip','-n',ns,'link','set','lo','up']);run(['ip','-n',ns,'link','set','eth0','up']);ip='198.18.19.'+str(i)
  run(['ip','-n',ns,'addr','add',ip+'/24','dev','eth0']);mac=json.loads(run(['ip','-n',ns,'-j','link','show','eth0']))[0]['address']
  ports.append(dict(port=v,mac=mac,ip=ip,group=group,proxy=group=='proxy'))
  children.append(subprocess.Popen(['ip','netns','exec',ns,'python3','-m','http.server','8080','--bind',ip],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL))
 time.sleep(1)
 def connect(src,dst,expected=True,source=None):
  s="import socket;s=socket.socket();s.settimeout(1);"+("s.bind(("+repr(source)+",0));" if source else '')+"raise SystemExit(0 if s.connect_ex(('198.18.19."+str(dst)+"',8080))==0 else 1)"
  rc=subprocess.run(['ip','netns','exec',prefix+str(src),'python3','-c',s],capture_output=True).returncode
  assert (rc==0)==expected,(src,dst,rc,expected)
 for src,dst in [(1,2),(1,3),(3,1),(4,1),(4,3),(1,4)]:connect(src,dst)
 rules=render(bridge,ports,table);run(['nft','--check','-f','-'],input=rules);run(['nft','-f','-'],input=rules)
 for src,dst in [(1,2),(2,1),(4,1),(4,3),(1,4),(3,4)]:connect(src,dst)
 for src,dst in [(1,3),(3,1),(2,3)]:connect(src,dst,False)
 run(['ip','-n',prefix+'1','addr','add','198.18.19.99/24','dev','eth0']);connect(1,4,False,'198.18.19.99')
 # An unclassified port is denied even when communicating with the proxy.
 partial=render(bridge,ports[1:],table);run(['nft','-f','-'],input=partial);connect(1,4,False);connect(2,4)
 run(['nft','delete','table','bridge',table]);connect(1,3);connect(3,1)
 print(json.dumps({'baseline_connectivity':True,'same_tenant_allowed':True,'proxy_allowed':True,'cross_tenant_blocked':True,'spoofed_source_blocked':True,'unknown_port_blocked':True,'rollback_restored_connectivity':True}))
finally:
 subprocess.run(['nft','delete','table','bridge',table],capture_output=True)
 for p in children:p.terminate()
 for p in children:
  try:p.wait(timeout=3)
  except subprocess.TimeoutExpired:p.kill();p.wait()
 for n in names:subprocess.run(['ip','netns','delete',n],capture_output=True)
 if created_bridge:subprocess.run(['ip','link','delete',bridge],capture_output=True)
