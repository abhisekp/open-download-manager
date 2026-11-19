"""Create original MIT WebExtension packages. Does not publish or sign store packages."""
import json,pathlib,shutil,zipfile
root=pathlib.Path(__file__).resolve().parents[1];dist=root/'dist';dist.mkdir(exist_ok=True)
for browser in ['chromium','firefox','safari-source']:
    folder=dist/('extension-'+browser);folder.mkdir(exist_ok=True)
    for p in (root/'browser-extension/shared').iterdir():shutil.copy2(p,folder/p.name)
    manifest={'manifest_version':3,'name':'Open Download Manager','version':'0.1.0','description':'Send selected download links to your paired local Open Download Manager.','permissions':['activeTab','storage','contextMenus'],'host_permissions':['http://127.0.0.1/*'],'action':{'default_popup':'popup.html'},'background':{'service_worker':'background.js'}}
    if browser=='firefox':
        manifest['background']={'scripts':['connection.js','background.js']}
        manifest['browser_specific_settings']={'gecko':{'id':'odm-browser@open-download-manager.local','strict_min_version':'121.0','data_collection_permissions':{'required':['none']}}}
        (folder/'background.js').write_text((folder/'background.js').read_text().replace("importScripts('connection.js');",''))
    shutil.copytree(root/'browser-extension/assets',folder/'icons',dirs_exist_ok=True)
    manifest['icons']={str(size):f'icons/icon-{size}.png' for size in [16,32,48,128]}
    manifest['action']['default_icon']=manifest['icons']
    (folder/'manifest.json').write_text(json.dumps(manifest,indent=2));shutil.copy2(root/'LICENSE',folder/'LICENSE')
    with zipfile.ZipFile(dist/f'odm-extension-{browser}-0.1.0.zip','w',zipfile.ZIP_DEFLATED) as z:
        for p in folder.rglob('*'):
            if p.is_file():z.write(p,p.relative_to(folder))
print('Built Chromium and Firefox development packages plus Safari conversion source; no store listing or signature.')
