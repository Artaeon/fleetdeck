import { chromium } from '/work/npm/node_modules/playwright/index.mjs';
import { readFileSync, existsSync } from 'node:fs';
import assert from 'node:assert/strict';

const auth = 'https://auth.example.test:8443';
const admin = 'https://admin.example.test:8443';
const browser = await chromium.launch({headless: true, args: ['--no-sandbox']});
let stage = 'start';
try {
  // The untrusted test CA is accepted only inside this disposable browser.
  const context = await browser.newContext({ignoreHTTPSErrors: true});
  const page = await context.newPage();
  stage = 'untrusted portal forwarding headers';
  const discovery = await context.request.get(auth+'/.well-known/openid-configuration', {headers: {
    'X-Forwarded-Host':'admin.example.test:8443',
    'X-Forwarded-Proto':'http',
    'X-Original-URL':admin+'/',
    'X-Forwarded-URI':'/untrusted-path',
  }});
  assert.equal(discovery.status(),200);
  assert.equal((await discovery.json()).issuer,auth);
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  await cdp.send('WebAuthn.addVirtualAuthenticator', {options: {
    protocol: 'ctap2', transport: 'usb', hasResidentKey: true,
    hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
  }});
  const api = async (method, path, data) => page.evaluate(async ({method,path,data}) => {
    const response = await fetch(path, {method, headers: {'Content-Type':'application/json'}, body: data ? JSON.stringify(data) : undefined});
    return {status: response.status, body: await response.json()};
  }, {method,path,data});
  await page.goto(auth);
  stage = 'password authentication';
  let result = await api('POST', '/api/firstfactor', {username:'testadmin',password:readFileSync('/work/password','utf8'),keepMeLoggedIn:false});
  assert.equal(result.status,200); assert.equal(result.body.status,'OK');
  stage = 'available factor policy';
  result = await api('GET', '/api/configuration');
  assert.equal(result.status,200);
  assert.deepEqual(result.body.data.available_methods,['webauthn']);
  stage = 'registration verification';
  result = await api('POST','/api/user/session/elevation');
  assert.equal(result.status,200);
  let code;
  for(let i=0;i<100;i++) {
    if(existsSync('/work/data/notifications.txt')) {
      const message=readFileSync('/work/data/notifications.txt','utf8');
      code=message.match(/^([A-Z0-9]{6,32})\s*$/m)?.[1];
      if(code)break;
    }
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  assert.ok(code,'registration verification code missing');
  result=await api('PUT','/api/user/session/elevation',{otc:code});
  assert.equal(result.status,200);
  stage = 'register virtual passkey';
  result=await api('PUT','/api/secondfactor/webauthn/credential/register',{description:'Disposable integration authenticator'});
  assert.equal(result.status,200);
  const registration=await page.evaluate(async options=>{
    const publicKey=PublicKeyCredential.parseCreationOptionsFromJSON(options);
    const credential=await navigator.credentials.create({publicKey});
    return credential.toJSON();
  },result.body.data.publicKey);
  result=await api('POST','/api/secondfactor/webauthn/credential/register',registration);
  assert.ok([200,201].includes(result.status));
  stage = 'second factor assertion';
  result=await api('GET','/api/secondfactor/webauthn');
  assert.equal(result.status,200);
  const assertion=await page.evaluate(async options=>{
    const publicKey=PublicKeyCredential.parseRequestOptionsFromJSON(options);
    return (await navigator.credentials.get({publicKey})).toJSON();
  },result.body.data.publicKey);
  result=await api('POST','/api/secondfactor/webauthn',{response:assertion});
  assert.equal(result.status,200);
  assert.equal((await context.cookies(admin)).filter(cookie=>cookie.name==='authelia_session').length,0,
    'provider session cookie must not reach sibling applications');
  stage = 'OIDC authorization';
  await page.goto(admin+'/login');
  await page.waitForURL(url=>url.hostname==='auth.example.test' || (url.hostname==='admin.example.test' && url.pathname==='/'));
  if(new URL(page.url()).hostname==='auth.example.test') {
    await page.getByRole('button',{name:/accept|authorize|allow/i}).click({timeout:15000});
  }
  await page.waitForURL(admin+'/',{timeout:15000});
  const projects=await context.request.get(admin+'/api/projects');
  assert.equal(projects.status(),200);
  stage = 'untrusted header and token rejection';
  const stranger=await browser.newContext({ignoreHTTPSErrors:true});
  const denied=await stranger.request.get(admin+'/api/projects',{headers:{'Remote-User':'testadmin','Remote-Groups':'fleetdeck-admins','Authorization':'Bearer '+readFileSync('/work/setup/client-secret','utf8').trim()}});
  assert.equal(denied.status(),401);
  const logout=await context.request.post(admin+'/logout',{headers:{Origin:admin}});
  assert.equal(logout.status(),200);
  assert.equal((await context.request.get(admin+'/api/projects')).status(),401);
  await stranger.close(); await context.close();
  console.log('Real Authelia password + virtual passkey login, OIDC authorization, bypass refusal and logout passed.');
} catch(error) {
  console.error('Authelia integration failed at: '+stage);
  console.error(error.message.replace(/https:\/\/\S+/g,'[test URL]'));
  process.exitCode=1;
} finally { await browser.close(); }
