"""Verify graceful stop and restore using isolated data and paused local-only URI."""
import argparse,json,pathlib,subprocess,time,urllib.request,http.server,threading,hashlib
p=argparse.ArgumentParser();p.add_argument('--exe',required=True);p.add_argument('--root',required=True);a=p.parse_args()
root=pathlib.Path(a.root).resolve();root.mkdir(parents=True,exist_ok=True)
def start():
    child=subprocess.Popen([str(pathlib.Path(a.exe).resolve()),'--no-browser','--data-dir',str(root/'data'),'--downloads',str(root/'files')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
    line=child.stdout.readline().strip()
    if not line.startswith('Open Download Manager: '):raise RuntimeError(child.stderr.read())
    base,token=line.split(': ',1)[1].split('/#');return child,base,token
def post(base,token,path,data):
    return json.load(urllib.request.urlopen(urllib.request.Request(base+path,data=json.dumps(data).encode(),headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'}),timeout=15))
child,base,token=start()
try:
    payload=b'restored history and checksum'
    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args):pass
        def do_GET(self):
            self.send_response(200);self.send_header('Content-Length',str(len(payload)));self.end_headers();self.wfile.write(payload)
    server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
    completed=post(base,token,'/api/add',{'url':f'http://127.0.0.1:{server.server_port}/history-test.bin'})['result']
    deadline=time.monotonic()+20
    while True:
        req=urllib.request.Request(base+'/api/status',headers={'Authorization':'Bearer '+token})
        rows=json.load(urllib.request.urlopen(req,timeout=15))['downloads'] or []
        if any(d['gid']==completed and d['status']=='complete' for d in rows):break
        assert time.monotonic()<deadline,'completion timed out';time.sleep(.1)
    server.shutdown()
    conf=dict(line.split('=',1) for line in (root/'data/engine.conf').read_text().splitlines() if '=' in line)
    rpc='http://127.0.0.1:'+conf['rpc-listen-port']+'/jsonrpc'
    params=['token:'+conf['rpc-secret'],['http://127.0.0.1:65530/recovery-test.bin'],{'pause':'true'}]
    req=urllib.request.Request(rpc,data=json.dumps({'jsonrpc':'2.0','id':1,'method':'aria2.addUri','params':params}).encode(),headers={'Content-Type':'application/json'})
    result=json.load(urllib.request.urlopen(req,timeout=10));assert 'result' in result,result
    post(base,token,'/api/quit',{});assert child.wait(timeout=15)==0
    assert not (root/'data/running.lock').exists();assert not (root/'data/engine.conf').exists()
    child,base,token=start()
    req=urllib.request.Request(base+'/api/status',headers={'Authorization':'Bearer '+token})
    rows=json.load(urllib.request.urlopen(req,timeout=15))['downloads']
    assert any(d['status']=='paused' and any('recovery-test.bin' in u['uri'] for f in d['files'] for u in f['uris']) for d in rows),rows
    assert any(d['gid']==completed and d.get('archived') for d in rows),'completed history missing'
    assert post(base,token,'/api/verify',{'gid':completed,'checksum':hashlib.sha256(payload).hexdigest()})['matches']
    post(base,token,'/api/control',{'gid':completed,'action':'forget'})
    post(base,token,'/api/quit',{});assert child.wait(timeout=15)==0
    print('PASS: paused session and completed history survive restart; archived checksum and forget work; lock/token config cleaned')
finally:
    if child.poll() is None:child.kill();child.wait()
