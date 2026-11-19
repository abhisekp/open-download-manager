"""Launch an isolated app instance and run the real local transfer suite."""
import argparse,json,pathlib,subprocess,tempfile,urllib.request
p=argparse.ArgumentParser();p.add_argument('--exe',required=True);a=p.parse_args()
with tempfile.TemporaryDirectory(prefix='odm-smoke-') as temp:
    root=pathlib.Path(temp);log=root/'launch.log'
    child=subprocess.Popen([str(pathlib.Path(a.exe).resolve()),'--no-browser','--data-dir',str(root/'data'),'--downloads',str(root/'files')],stdout=subprocess.PIPE,text=True,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
    line=child.stdout.readline();assert line.startswith('Open Download Manager: '),line;log.write_text(line)
    base,token=line.split(': ',1)[1].strip().split('/#')
    try:
        subprocess.run(['python',str(pathlib.Path(__file__).with_name('integration.py')),'--launch-log',str(log),'--downloads',str(root/'files')],check=True)
    finally:
        req=urllib.request.Request(base+'/api/quit',data=b'{}',headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
        urllib.request.urlopen(req,timeout=15).close();child.wait(timeout=15)
