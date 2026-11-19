"""Collect license texts for resolved Go modules without bundling their source."""
import json,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[1]
text=subprocess.check_output(['go','list','-deps','-json','.'],cwd=root,text=True,encoding="utf-8");decoder=json.JSONDecoder();modules=[]
while text.strip():
    text=text.lstrip();value,end=decoder.raw_decode(text);modules.append(value['Module']) if value.get('Module') else None;text=text[end:]
modules=list({m['Path']:m for m in modules}.values())
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
if missing:raise SystemExit('Review missing license texts: '+', '.join(missing))
goroot=pathlib.Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip());parts.append('\n=== Go standard library ===\n'+(goroot/'LICENSE').read_text())
(root/'THIRD-PARTY-GO-NOTICES.txt').write_text(''.join(parts),encoding='utf-8');print('Collected Go module license texts:',len(modules)-1)
