"""Build portable archives. Does not fetch or redistribute aria2."""
import hashlib,json,os,pathlib,subprocess,tarfile,zipfile
root=pathlib.Path(__file__).resolve().parents[1];out=root/'dist';out.mkdir(exist_ok=True)
targets=[('windows','386'),('windows','amd64'),('windows','arm64'),('linux','386'),('linux','amd64'),('linux','arm'),('linux','arm64'),('darwin','amd64'),('darwin','arm64')]
for system,arch in targets:
    stem=f'odm-0.1.0-dev-{system}-{arch}';folder=out/stem;folder.mkdir(exist_ok=True)
    name='odm.exe' if system=='windows' else 'odm';env={**os.environ,'GOOS':system,'GOARCH':arch,'CGO_ENABLED':'0','GOARM':'7'}
    subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(folder/name),'.'],cwd=root,env=env,check=True)
    (folder/'INSTALL.txt').write_text('HTTP/HTTPS and torrents are built in. Optional aria2 adds FTP/SFTP and accelerated file transfers; install it for your architecture and put aria2c on PATH.\nThis is a browser-based local interface, not a native window.\nSee project README and release report for testing limitations.\n')
    (folder/name).chmod(0o755)
    for document in ['README.md','LICENSE','THIRD-PARTY-NOTICES.txt','THIRD-PARTY-GO-NOTICES.txt']:(folder/document).write_bytes((root/document).read_bytes())
    if system=='windows':
        with zipfile.ZipFile(out/(stem+'.zip'),'w',zipfile.ZIP_DEFLATED) as z:
            for f in folder.iterdir():z.write(f,stem+'/'+f.name)
    else:
        with tarfile.open(out/(stem+'.tar.gz'),'w:gz') as z:z.add(folder,arcname=stem)
digests={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in out.iterdir() if f.is_file() and f.suffix in ['.zip','.gz']}
(out/'SHA256SUMS').write_text(''.join(f'{v}  {k}\n' for k,v in sorted(digests.items())))
(out/'build-targets.json').write_text(json.dumps({'version':'0.1.0-dev','targets':targets,'runtimeVerified':[],'runtimeEvidence':'See CI runtime and installation job results; this file records cross-compilation only.','aria2Bundled':False},indent=2))
print('Built',len(targets),'portable targets. Cross-compilation does not establish runtime compatibility.')
