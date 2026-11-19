/* MIT. No content scripts, remote code, page tracking or cookie extraction. */
(function(root){
  function parsePairing(text){
    const input=JSON.parse(text),u=new URL(input.endpoint);
    if(u.protocol!=='http:'||u.hostname!=='127.0.0.1'||u.pathname!=='/'||u.search||u.hash||u.username||u.password||!u.port)throw new Error('Use the pairing JSON from the local ODM Settings page.');
    if(!/^[a-f0-9]{64}$/.test(input.token)||typeof input.origin!=='string')throw new Error('Invalid browser pairing.');
    return {endpoint:u.origin,token:input.token,origin:input.origin};
  }
  function validLink(raw){try{const u=new URL(raw);return ['https:','http:','ftp:','sftp:','magnet:'].includes(u.protocol)}catch{return false}}
  async function send(connection,link,fetcher=fetch){
    if(!validLink(link))throw new Error('Choose a downloadable web URL or magnet link.');
    const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),10000);
    try{const res=await fetcher(connection.endpoint+'/bridge/add',{method:'POST',headers:{'Content-Type':'application/json','Authorization':'Bearer '+connection.token,'X-ODM-Extension-Origin':connection.origin},body:JSON.stringify({url:link}),signal:controller.signal});if(!res.ok)throw new Error('ODM rejected this request. Check pairing and that ODM is running.');return await res.json()}finally{clearTimeout(timer)}
  }
  const api={parsePairing,validLink,send};if(typeof module!=='undefined')module.exports=api;root.ODMConnection=api;
})(globalThis);
