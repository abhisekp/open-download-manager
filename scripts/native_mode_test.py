"""Smoke-test native torrent startup without any aria2 executable."""
import argparse,hashlib,json,pathlib,subprocess,tempfile,urllib.request,urllib.error
p=argparse.ArgumentParser();p.add_argument('--exe',required=True);a=p.parse_args()
def encode(value):
    if isinstance(value,int):return b'i'+str(value).encode()+b'e'
    if isinstance(value,str):value=value.encode()
    if isinstance(value,bytes):return str(len(value)).encode()+b':'+value
    if isinstance(value,dict):return b'd'+b''.join(encode(k)+encode(value[k]) for k in sorted(value))+b'e'
    raise TypeError(value)
with tempfile.TemporaryDirectory(prefix='odm-native-') as temp:
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
        req=urllib.request.Request(base+'/api/add',data=b'{"url":"https://example.com/file.zip"}',headers={**headers,'Content-Type':'application/json'})
        try:urllib.request.urlopen(req,timeout=15);raise AssertionError('non-torrent engine unexpectedly available')
        except urllib.error.HTTPError as error:assert error.code==400
        print('PASS: native torrent import and status work without aria2; other protocols report missing engine')
    finally:
        urllib.request.urlopen(urllib.request.Request(base+'/api/quit',data=b'{}',headers={**headers,'Content-Type':'application/json'}),timeout=15).close();child.wait(timeout=15)
