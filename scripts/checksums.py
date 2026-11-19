import hashlib,pathlib
dist=pathlib.Path(__file__).resolve().parents[1]/'dist'
files=sorted(f for f in dist.iterdir() if f.is_file() and (f.name.endswith(('.zip','.tar.gz','.deb','.rpm','.AppImage','.msix'))))
(dist/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n' for f in files))
print('Checksummed',len(files),'artifacts')
