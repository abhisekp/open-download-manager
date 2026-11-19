importScripts('connection.js');
chrome.runtime.onInstalled.addListener(()=>{chrome.contextMenus.create({id:'odm-send-link',title:'Download link with ODM',contexts:['link']})});
chrome.contextMenus.onClicked.addListener(async(info)=>{
  if(info.menuItemId!=='odm-send-link')return;
  try{const {connection}=await chrome.storage.local.get('connection');if(!connection)throw new Error('Pair with ODM first');await ODMConnection.send(connection,info.linkUrl);await chrome.action.setBadgeText({text:'OK'});await chrome.action.setBadgeBackgroundColor({color:'#1267de'})}
  catch{await chrome.action.setBadgeText({text:'!'});await chrome.action.setBadgeBackgroundColor({color:'#a23026'})}
});
