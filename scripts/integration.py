"""Real loopback transfer test; run against a separately started ODM test instance."""
import argparse, hashlib, http.server, json, pathlib, threading, time, urllib.request
p=argparse.ArgumentParser();p.add_argument('--launch-log',required=True);p.add_argument('--downloads',required=True);a=p.parse_args()
launch=pathlib.Path(a.launch_log).read_text().split('Open Download Manager: ')[1].strip();base,token=launch.split('/#')
payload=bytes(range(256))*32768
class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_GET(self):
        start=int(self.headers.get('Range','bytes=0-').split('=')[1].split('-')[0]);body=payload[start:]
        self.send_response(206 if start else 200);self.send_header('Content-Length',str(len(body)));self.send_header('Accept-Ranges','bytes')
        if start:self.send_header('Content-Range',f'bytes {start}-{len(payload)-1}/{len(payload)}')
        self.end_headers()
        try:
            for offset in range(0,len(body),32768):self.wfile.write(body[offset:offset+32768]);self.wfile.flush();time.sleep(.015)
        except (ConnectionError,OSError):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
def api(path,data=None):
    req=urllib.request.Request(base+'/api/'+path,data=json.dumps(data).encode() if data is not None else None,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
    return json.load(urllib.request.urlopen(req,timeout=20))
def wait_for(gid,state):
    until=time.monotonic()+30
    while time.monotonic()<until:
        row=next((d for d in api('status')['downloads'] if d['gid']==gid),None)
        if row and row['status']==state:return row
        time.sleep(.1)
    raise AssertionError('Timed out waiting for '+state)
gid=api('add',{'url':f'http://127.0.0.1:{server.server_port}/qa-transfer.bin'})['result']
wait_for(gid,'active');time.sleep(.3);api('control',{'gid':gid,'action':'pause'});wait_for(gid,'paused')
api('control',{'gid':gid,'action':'resume'});row=wait_for(gid,'complete')
assert hashlib.sha256(pathlib.Path(row['files'][0]['path']).read_bytes()).digest()==hashlib.sha256(payload).digest()
expected=hashlib.sha256(payload).hexdigest()
assert api('verify',{'gid':gid,'checksum':expected})['matches'] is True
assert api('verify',{'gid':gid,'checksum':'0'*64})['matches'] is False
link=f'http://127.0.0.1:{server.server_port}/qa-batch.bin'
batch=api('batch',{'urls':[link,link]})['results'];assert len(batch)==1
wait_for(batch[0]['gid'],'complete')
origin='chrome-extension://'+'a'*32
pair=api('pair',{'extensionOrigin':origin})
bridge_request=urllib.request.Request(base+'/bridge/add',data=json.dumps({'url':f'http://127.0.0.1:{server.server_port}/qa-browser.bin'}).encode(),headers={'Authorization':'Bearer '+pair['token'],'Origin':origin,'X-ODM-Extension-Origin':origin,'Content-Type':'application/json'})
bridge=json.load(urllib.request.urlopen(bridge_request,timeout=15));wait_for(bridge['gid'],'complete')
api('revoke-browsers',{})
try:urllib.request.urlopen(bridge_request,timeout=15);raise AssertionError('revoked browser token worked')
except urllib.error.HTTPError as error:assert error.code==403
api('limits',{'limit':'1048576'});api('limits',{'limit':'0'})
server.shutdown();print('PASS: real transfer, pause/resume, checksum match/mismatch, batch deduplication, paired browser submission/revocation, speed limits')
