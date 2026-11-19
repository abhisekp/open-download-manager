"""Create Debian packages with standard ar/tar containers; no install hooks."""
import hashlib,io,pathlib,tarfile,time
root=pathlib.Path(__file__).resolve().parents[1];dist=root/'dist'
def tar(items):
    b=io.BytesIO()
    with tarfile.open(fileobj=b,mode='w:gz') as t:
        directories=set()
        for name,_,_ in items:
            directories.update(str(parent) for parent in pathlib.PurePosixPath(name).parents if str(parent)!='.')
        for name in sorted(directories,key=lambda value:(value.count('/'),value)):
            info=tarfile.TarInfo(name+'/');info.type=tarfile.DIRTYPE;info.mode=0o755;info.mtime=0;t.addfile(info)
        for name,body,mode in items:
            info=tarfile.TarInfo(name);info.size=len(body);info.mode=mode;info.mtime=0;t.addfile(info,io.BytesIO(body))
    return b.getvalue()
def ar(items):
    b=bytearray(b'!<arch>\n')
    for name,data in items:
        header=f'{name+"/":<16}{0:<12}{0:<6}{0:<6}{"100644":<8}{len(data):<10}`\n'.encode()
        assert len(header)==60
        b.extend(header);b.extend(data)
        if len(data)%2:b.extend(b'\n')
    return b
def main():
    for goarch,debarch in [('386','i386'),('amd64','amd64'),('arm','armhf'),('arm64','arm64')]:
        folder=dist/f'odm-0.1.0-dev-linux-{goarch}'
        control=f'Package: open-download-manager\nVersion: 0.1.0~dev\nArchitecture: {debarch}\nMaintainer: Open Download Manager contributors\nSection: net\nPriority: optional\nDepends: aria2, xdg-utils\nDescription: Local browser-based download manager (development)\n Uses a separately installed aria2 engine. Not a native desktop shell.\n'
        data=tar([('usr/bin/odm',(folder/'odm').read_bytes(),0o755),('usr/share/icons/hicolor/128x128/apps/open-download-manager.png',(root/'browser-extension/assets/icon-128.png').read_bytes(),0o644),('usr/share/doc/open-download-manager/copyright',(root/'LICENSE').read_bytes(),0o644),*((f'usr/share/doc/open-download-manager/{name}',(root/name).read_bytes(),0o644) for name in ['THIRD-PARTY-NOTICES.txt','THIRD-PARTY-GO-NOTICES.txt']),('usr/share/applications/open-download-manager.desktop',b'[Desktop Entry]\nType=Application\nName=Open Download Manager\nExec=odm\nIcon=open-download-manager\nTerminal=true\nCategories=Network;FileTransfer;\n',0o644)])
        (dist/f'open-download-manager_0.1.0-dev_{debarch}.deb').write_bytes(ar([('debian-binary',b'2.0\n'),('control.tar.gz',tar([('control',control.encode(),0o644)])),('data.tar.gz',data)]))
    files=sorted(f for f in dist.iterdir() if f.suffix in ['.zip','.gz','.deb','.xz'])
    (dist/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n' for f in files))
    print('Created four Debian packages; install/runtime verification still required on Linux.')

if __name__=="__main__":main()
