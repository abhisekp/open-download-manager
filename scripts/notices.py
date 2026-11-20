"""Collect license texts for resolved Go modules without bundling their source."""
import json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[1]
modules={};decoder=json.JSONDecoder()
for system,arch in [('windows','386'),('windows','amd64'),('windows','arm64'),
                    ('linux','386'),('linux','amd64'),('linux','arm'),('linux','arm64'),
                    ('darwin','amd64'),('darwin','arm64')]:
    env={**os.environ,'GOOS':system,'GOARCH':arch,'CGO_ENABLED':'0','GOARM':'7'}
    text=subprocess.check_output(['go','list','-deps','-json','.'],cwd=root,env=env,text=True,encoding='utf-8')
    while text.strip():
        text=text.lstrip();value,end=decoder.raw_decode(text);text=text[end:]
        if value.get('Module'):modules[value['Module']['Path']]=value['Module']
modules=[modules[name] for name in sorted(modules)]
parts=['Go dependency license notices\nGenerated from the resolved go.mod/go.sum module graph. ODM original source remains MIT.\n'];missing=[]
for module in modules:
    if module.get('Main'):continue
    path=pathlib.Path(module.get('Replace',{}).get('Dir',module.get('Dir','')));found=[]
    if module.get('Replace',{}).get('Dir') or module.get('Dir'):
        for f in path.iterdir():
            if f.is_file() and (f.name.upper().startswith('LICENSE') or f.name.upper() in ['COPYING','NOTICE']):found.append(f)
    if not found:missing.append(module['Path']);continue
    parts.append('\n=== '+module['Path']+' '+module.get('Version','')+' ===\n')
    for f in sorted(found):parts.append(f.name+'\n'+f.read_text(encoding='utf-8',errors='replace')+'\n')
    if module['Path']=='github.com/zalando/go-keyring':
        # This platform implementation has an Apache header in addition to the
        # module's root MIT license. Preserve its attribution without source.
        source=(path/'keyring_darwin.go').read_text(encoding='utf-8')
        header=source.split('package keyring',1)[0]
        parts.append('keyring_darwin.go file attribution\n'+header+'\n')
if missing:raise SystemExit('Review missing license texts: '+', '.join(missing))
goroot=pathlib.Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip());parts.append('\n=== Go standard library ===\n'+(goroot/'LICENSE').read_text())
(root/'THIRD-PARTY-GO-NOTICES.txt').write_text(''.join(parts),encoding='utf-8');print('Collected Go module license texts:',len(modules)-1)
