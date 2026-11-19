"""Smoke-test native torrent startup without any aria2 executable."""
import argparse,hashlib,json,pathlib,subprocess,tempfile,urllib.request,urllib.error,http.server,threading,time
p=argparse.ArgumentParser();p.add_argument('--exe',required=True);a=p.parse_args()
def encode(value):
    if isinstance(value,int):return b'i'+str(value).encode()+b'e'
    if isinstance(value,str):value=value.encode()
    if isinstance(value,bytes):return str(len(value)).encode()+b':'+value
    if isinstance(value,dict):return b'd'+b''.join(encode(k)+encode(value[k]) for k in sorted(value))+b'e'
    raise TypeError(value)
with tempfile.TemporaryDirectory(prefix='odm-native-') as temp:
    http_payload=b'original built-in HTTP fixture\n'*4096
    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args):pass
        def do_GET(self):
            self.send_response(200);self.send_header('Content-Length',str(len(http_payload)));self.end_headers();self.wfile.write(http_payload)
    server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Fixture);threading.Thread(target=server.serve_forever,daemon=True).start()
    root=pathlib.Path(temp);child=subprocess.Popen([str(pathlib.Path(a.exe).resolve()),'--no-browser','--engine','odm-deliberately-missing-aria2','--data-dir',str(root/'data'),'--downloads',str(root/'files')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
    line=child.stdout.readline();assert line.startswith('Open Download Manager: '),line
    base,token=line.split(': ',1)[1].strip().split('/#');headers={'Authorization':'Bearer '+token}
    try:
        payload=b'legal native-mode fixture'
        metadata=encode({'info':{'name':'native-mode.bin','length':len(payload),'piece length':16384,'pieces':hashlib.sha1(payload).digest()}})
        req=urllib.request.Request(base+'/api/torrent-file',data=metadata,headers={**headers,'Content-Type':'application/x-bittorrent'})
        gid=json.load(urllib.request.urlopen(req,timeout=15))['result']
        status=json.load(urllib.request.urlopen(urllib.request.Request(base+'/api/status',headers=headers),timeout=15))
        assert any(d['gid']==gid and d['native'] for d in status['downloads'])
        req=urllib.request.Request(base+'/api/add',data=json.dumps({'url':f'http://127.0.0.1:{server.server_port}/native-http.bin'}).encode(),headers={**headers,'Content-Type':'application/json'})
        http_gid=json.load(urllib.request.urlopen(req,timeout=15))['result'];deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            current=json.load(urllib.request.urlopen(urllib.request.Request(base+'/api/status',headers=headers),timeout=15))
            completed=next((d for d in current['downloads'] if d['gid']==http_gid and d['status']=='complete'),None)
            if completed:
                assert completed['engine']=='native-http'
                assert pathlib.Path(completed['files'][0]['path']).read_bytes()==http_payload
                break
            time.sleep(.05)
        else:raise AssertionError('built-in HTTP did not complete')
        req=urllib.request.Request(base+'/api/add',data=b'{"url":"ftp://127.0.0.1:65530/file.zip"}',headers={**headers,'Content-Type':'application/json'})
        try:urllib.request.urlopen(req,timeout=15);raise AssertionError('optional FTP engine unexpectedly available')
        except urllib.error.HTTPError as error:assert error.code==400
        print('PASS: built-in HTTP download and native torrent import/status work without aria2; FTP reports missing optional engine')
    finally:
        urllib.request.urlopen(urllib.request.Request(base+'/api/quit',data=b'{}',headers={**headers,'Content-Type':'application/json'}),timeout=15).close();child.wait(timeout=15);server.shutdown()
