/* Shared request builders: browser UI and Node validation. */
(function(root) {
  'use strict';
  function escapeXML(value) {
    return String(value).replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\ufffe\uffff]/g,' ').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&apos;');
  }
  function azureSSML(p) {
    let speech='<prosody rate="'+escapeXML(p.rate)+'" volume="'+escapeXML(p.volume)+'" pitch="'+escapeXML(p.pitch)+'">'+escapeXML(p.text)+'</prosody>';
    if (p.style!=='general'||p.role!=='default') speech='<mstts:express-as style="'+escapeXML(p.style)+'" styledegree="'+escapeXML(p.styleDegree)+'"'+(p.role!=='default'?' role="'+escapeXML(p.role)+'"':'')+'>'+speech+'</mstts:express-as>';
    if(p.secondaryLocale) speech='<lang xml:lang="'+escapeXML(p.secondaryLocale)+'">'+speech+'</lang>';
    return '<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts" xml:lang="en-US"><voice name="'+escapeXML(p.voice)+'">'+speech+'</voice></speak>';
  }
  function buildRequest(channel,p,token) {
    const headers={};if(token)headers.Token=token;
    if(channel==='azure') {headers['Content-Type']='application/ssml+xml';headers.Format=p.format;return {url:'/api/azure',headers,body:azureSSML(p)};}
    headers['Content-Type']='application/json';return {url:'/api/ra',headers,body:JSON.stringify(p)};
  }
  function filename(format) {
    if(format.startsWith('webm'))return 'audio.webm';
    if(format.startsWith('riff'))return 'audio.wav';
    if(format.startsWith('ogg'))return 'audio.ogg';
    if(format.startsWith('raw'))return format.endsWith('pcm')?'audio.pcm':'audio.raw';
    return 'audio.mp3';
  }
  root.StudioProtocol={escapeXML,azureSSML,buildRequest,filename};
})(typeof module==='object'&&module.exports?module.exports:window);
