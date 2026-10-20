'use strict';
const byId = id => document.getElementById(id);
const presets = [
  {ShortName:'zh-CN-XiaoxiaoNeural',Locale:'zh-CN',Gender:'Female'},
  {ShortName:'zh-CN-XiaoyiNeural',Locale:'zh-CN',Gender:'Female'},
  {ShortName:'zh-CN-YunxiNeural',Locale:'zh-CN',Gender:'Male'},
  {ShortName:'zh-CN-YunjianNeural',Locale:'zh-CN',Gender:'Male'},
  {ShortName:'zh-CN-YunxiaNeural',Locale:'zh-CN',Gender:'Male'},
  {ShortName:'zh-CN-YunyangNeural',Locale:'zh-CN',Gender:'Male'},
  {ShortName:'en-US-EmmaMultilingualNeural',Locale:'en-US',Gender:'Female'},
  {ShortName:'en-US-JennyNeural',Locale:'en-US',Gender:'Female'},
  {ShortName:'zh-HK-HiuGaaiNeural',Locale:'zh-HK',Gender:'Female'},
  {ShortName:'zh-TW-HsiaoChenNeural',Locale:'zh-TW',Gender:'Female'}
];
const edgeFormats=['audio-24khz-48kbitrate-mono-mp3','audio-24khz-96kbitrate-mono-mp3','webm-24khz-16bit-mono-opus'];
const azureFormats=["raw-16khz-16bit-mono-pcm", "raw-48khz-16bit-mono-pcm", "raw-8khz-8bit-mono-mulaw", "raw-8khz-8bit-mono-alaw", "raw-16khz-16bit-mono-truesilk", "raw-24khz-16bit-mono-truesilk", "riff-16khz-16bit-mono-pcm", "riff-24khz-16bit-mono-pcm", "riff-48khz-16bit-mono-pcm", "riff-8khz-8bit-mono-mulaw", "riff-8khz-8bit-mono-alaw", "audio-16khz-32kbitrate-mono-mp3", "audio-16khz-64kbitrate-mono-mp3", "audio-16khz-128kbitrate-mono-mp3", "audio-24khz-48kbitrate-mono-mp3", "audio-24khz-96kbitrate-mono-mp3", "audio-24khz-160kbitrate-mono-mp3", "audio-48khz-96kbitrate-mono-mp3", "audio-48khz-192kbitrate-mono-mp3", "webm-16khz-16bit-mono-opus", "webm-24khz-16bit-24kbps-mono-opus", "webm-24khz-16bit-mono-opus", "ogg-16khz-16bit-mono-opus", "ogg-24khz-16bit-mono-opus", "ogg-48khz-16bit-mono-opus"];
let channel='edge';
const channelSettings={edge:{},azure:{}};
const voiceCache={};
let voiceController;
let voiceSequence=0;
let voices = presets;
let voiceSource = '预设';
let audioURL = '';
let subtitleURL = '';
let busy = false;
let voicesBusy = false;
const samples = {
  '中文旁白':{locale:'zh-CN',voice:'zh-CN-XiaoxiaoNeural',text:'你好，欢迎来到语音工作台。选择喜欢的声音，让每一段文字都拥有自己的表达。'},
  '英文介绍':{locale:'en-US',voice:'en-US-EmmaMultilingualNeural',text:'Every great idea starts with a few words. Give your story a voice, and let the world hear it.'},
  '粤语问候':{locale:'zh-HK',voice:'zh-HK-HiuGaaiNeural',text:'你好，歡迎嚟到語音工作台。揀一個你鍾意嘅聲音，一齊聽見文字嘅溫度。'}
};
const settingIDs=['language','voiceName','rate','volume','pitch','voiceFormat','styleName','styleDegree','roleName','secondaryLocale'];
function saveChannelSettings() {
  for(const id of settingIDs) channelSettings[channel][id]=byId(id).value;
  channelSettings[channel].subtitles=byId('subtitles').checked;
}
function formatLabel(format) {
  if(format===edgeFormats[0])return 'MP3 · 24 kHz / 48 kbps';
  if(format===edgeFormats[1])return 'MP3 · 24 kHz / 96 kbps';
  if(format===edgeFormats[2])return 'WebM · 24 kHz / Opus';
  return format;
}
function switchChannel(next,initial=false) {
  if(!['edge','azure'].includes(next)||(!initial&&next===channel))return;
  if(busy){notice('请等待当前合成完成后切换渠道。','warning');history.replaceState(null,'','#'+channel);return;}
  if(!initial)saveChannelSettings();
  if(voiceController)voiceController.abort();
  ++voiceSequence;voicesBusy=false;byId('reloadVoices').disabled=false;
  channel=next;
  const azure=channel==='azure';
  byId('voiceSampleChannel').textContent=azure?'Azure':'Edge';
  for(const player of document.querySelectorAll('audio[data-voice-sample]')) {
    player.pause();
    player.src='/voice-samples/'+channel+'/'+player.getAttribute('data-voice-sample')+'.mp3';
    player.load();
  }
  for(const name of ['edge','azure']) {const el=byId(name+'Channel');el.classList.toggle('active',name===channel);if(name===channel)el.setAttribute('aria-current','page');else el.removeAttribute('aria-current');}
  byId('channelTitle').textContent=azure?'Azure 语音':'Edge 语音';
  byId('channelTag').textContent=azure?'Azure 演示服务':'Microsoft Edge';
  byId('endpointPath').textContent=azure?'/api/azure':'/api/ra';
  byId('requestTitle').textContent=azure?'查看 SSML 请求':'查看 JSON 请求';
  byId('azureOptions').hidden=!azure;byId('subtitlesControl').hidden=azure;
  byId('voiceFormat').replaceChildren(...(azure?azureFormats:edgeFormats).map(f=>new Option(formatLabel(f),f)));
  const settings=channelSettings[channel];
  for(const id of ['rate','volume','pitch'])byId(id).value=settings[id]||'0';
  byId('styleDegree').value=settings.styleDegree||'1';
  byId('voiceFormat').value=settings.voiceFormat||(edgeFormats[0]);
  byId('subtitles').checked=!azure&&!!settings.subtitles;
  voices=voiceCache[channel]||presets;voiceSource=voiceCache[channel]?'在线':'预设';
  refreshLanguages(settings.language||'zh-CN',settings.voiceName||'zh-CN-XiaoxiaoNeural');
  byId('audioPlayer').pause();byId('audioPlayer').removeAttribute('src');byId('audioPlayer').load();
  if(audioURL)URL.revokeObjectURL(audioURL);if(subtitleURL)URL.revokeObjectURL(subtitleURL);audioURL='';subtitleURL='';
  byId('readyResult').hidden=true;byId('emptyResult').hidden=false;byId('resultStatus').textContent='等待生成';byId('downloadSRT').hidden=true;
  notice(azure?'Azure 使用演示服务。风格、角色和朗读语言会随音色切换；此渠道不提供字幕。':'');
  updateRequest();loadVoices();
}
function updateAzureOptions() {
  if(channel!=='azure')return;
  const voice=voices.find(v=>v.ShortName===byId('voiceName').value)||{};
  const saved=channelSettings.azure;
  const preferredStyle=byId('styleName').value==='general'?(saved.styleName||'general'):byId('styleName').value;
  const preferredRole=byId('roleName').value==='default'?(saved.roleName||'default'):byId('roleName').value;
  const preferredLocale=byId('secondaryLocale').value||saved.secondaryLocale||'';
  const styles=Array.isArray(voice.StyleList)?voice.StyleList:[];
  const roles=Array.isArray(voice.RolePlayList)?voice.RolePlayList:[];
  const locales=Array.isArray(voice.SecondaryLocaleList)?voice.SecondaryLocaleList:[];
  const localized=value=>(typeof cnLocalStyleAndRole!=='undefined'&&cnLocalStyleAndRole[value])||value;
  byId('styleName').replaceChildren(new Option('普通','general'),...styles.filter(x=>x!=='general').map(x=>new Option(localized(x),x)));
  if(styles.includes(preferredStyle))byId('styleName').value=preferredStyle;
  byId('styleName').disabled=!styles.length;
  byId('styleHint').textContent=styles.length?styles.length+' 种风格':'仅普通风格';
  byId('styleDegree').disabled=byId('styleName').value==='general';
  byId('roleName').replaceChildren(new Option('默认','default'),...roles.map(x=>new Option(localized(x),x)));
  if(roles.includes(preferredRole))byId('roleName').value=preferredRole;
  byId('roleName').disabled=!roles.length;
  byId('secondaryLocale').replaceChildren(new Option('音色默认语言',''),...locales.map(x=>new Option((typeof cnLocalLanguage!=='undefined'&&cnLocalLanguage[x])||x,x)));
  if(locales.includes(preferredLocale))byId('secondaryLocale').value=preferredLocale;
  byId('secondaryLocaleField').hidden=!locales.length;
}
function notice(message,type='info') {
  const el=byId('notice'); el.textContent=message; el.className='notice'+(type==='info'?'':' '+type); el.hidden=!message;
}
function voiceLabel(voice) {
  return voice.LocalName || (typeof cnLocalVoice!=='undefined' && cnLocalVoice[voice.ShortName]) || voice.ShortName.replace(/^.*-([^-]+)Neural$/,'$1');
}
function refreshLanguages(preferredLocale=byId('language').value, preferredVoice=byId('voiceName').value) {
  const locales=[...new Set(voices.map(v=>v.Locale))].sort((a,b)=>a==='zh-CN'?-1:b==='zh-CN'?1:a.localeCompare(b));
  byId('language').replaceChildren(...locales.map(locale=>new Option((typeof cnLocalLanguage!=='undefined' && cnLocalLanguage[locale]) || locale,locale)));
  if (locales.includes(preferredLocale)) byId('language').value=preferredLocale;
  refreshVoices(preferredVoice);
}
function refreshVoices(preferred=byId('voiceName').value) {
  const filtered=voices.filter(v=>v.Locale===byId('language').value);
  byId('voiceName').replaceChildren(...filtered.map(v=>new Option(voiceLabel(v)+' · '+(v.Gender==='Male'?'男声':'女声'),v.ShortName)));
  if (filtered.some(v=>v.ShortName===preferred)) byId('voiceName').value=preferred;
  byId('voiceCount').textContent=voiceSource+' · '+filtered.length+' 个音色';
  updateVoice();
}
function updateVoice() {
  const voice=voices.find(v=>v.ShortName===byId('voiceName').value);
  if (!voice) return;
  const name=voiceLabel(voice);
  byId('voiceTitle').textContent=name;
  byId('voiceAvatar').textContent=[...name][0]||'声';
  byId('voiceDetail').textContent=voice.ShortName;
  byId('voiceGender').textContent=voice.Gender==='Male'?'男声':'女声';
  updateAzureOptions();updateRequest();
}
function headers() {
  const result={'Content-Type':'application/json'};
  if (byId('token').value) result.Token=byId('token').value;
  return result;
}
async function requestError(response) {
  const text=await response.text();
  let message=text;
  try { message=JSON.parse(text).error || text; } catch (_) {}
  if (response.status===401) {
    byId('token').closest('details').open=true;
    return 'Token 验证失败。请在「连接与认证」中填写有效的用户 Token，管理 Token 不能用于语音合成。';
  }
  if (response.status===429 || String(message).includes('429')) return '微软服务暂时限流，请稍后重试（429）。';
  if (response.status===503 || response.status===504) return '合成超时，请缩短文本或稍后重试。';
  return String(message).slice(0,500) || '请求失败（HTTP '+response.status+'）。';
}
async function loadVoices() {
  if(voicesBusy)return;
  voicesBusy=true;byId('reloadVoices').disabled=true;byId('voiceCount').textContent='加载中…';
  const selected=channel;const sequence=++voiceSequence;
  const controller=new AbortController();voiceController=controller;const timer=setTimeout(()=>controller.abort(),35000);
  try {
    let response;
    if(selected==='azure') response=await fetch('/api/azure/voices',{signal:controller.signal});
    else {const request={list_voices:true};response=await fetch('/api/ra',{method:'POST',headers:headers(),body:JSON.stringify(request),signal:controller.signal});}
    if(sequence!==voiceSequence)return;
    if(!response.ok)throw new Error(await requestError(response));
    const result=await response.json();const list=selected==='azure'?result:result.voices;
    if(sequence!==voiceSequence)return;
    if(!Array.isArray(list)||!list.length)throw new Error('声音列表为空，已保留预设音色。');
    const valid=list.filter(v=>v.ShortName&&v.Locale);if(!valid.length)throw new Error('声音列表格式不正确。');
    voiceCache[selected]=valid;voices=valid;voiceSource='在线';refreshLanguages();notice('已加载 '+valid.length+' 个 '+(selected==='azure'?'Azure':'Edge')+' 在线音色。');
  }catch(error){if(sequence!==voiceSequence)return;refreshVoices();notice(error.name==='AbortError'?'音色列表加载超时，仍可使用预设音色。':error.message+' 已保留预设音色，可点击 ↻ 重试。','warning');}
  finally{clearTimeout(timer);if(sequence===voiceSequence){voicesBusy=false;byId('reloadVoices').disabled=false;}}
}
function signed(value,unit) {return (Number(value)>=0?'+':'')+value+unit;}
function payload() {
  const result={text:byId('previewText').value,voice:byId('voiceName').value,rate:signed(byId('rate').value,'%'),volume:signed(byId('volume').value,'%'),pitch:signed(byId('pitch').value,'Hz'),format:byId('voiceFormat').value,response_format:byId('subtitles').checked?'json':'audio'};
  if(channel==='azure')return {...result,response_format:'audio',style:byId('styleName').value||'general',styleDegree:byId('styleDegree').value,role:byId('roleName').value||'default',secondaryLocale:byId('secondaryLocale').value};
  if (byId('subtitles').checked) result.write_subtitles=true;
  return result;
}
function updateRequest() {
  byId('charCount').textContent=[...byId('previewText').value].length.toLocaleString();
  for (const name of ['rate','volume','pitch']) byId(name+'Value').textContent=(Number(byId(name).value)>0?'+':'')+byId(name).value+(name==='pitch'?' Hz':'%');
  byId('requestPreview').textContent=channel==='azure'?StudioProtocol.azureSSML(payload()):JSON.stringify(payload(),null,2);
  const examples=StudioProtocol.shellExamples(channel,payload(),window.VOICE_LAB_CONFIG.apiBaseURL.replace(/\/+$/,''));
  byId('shellChannel').textContent=channel==='azure'?'Azure':'Edge';
  byId('macShell').textContent=examples.mac;
  byId('windowsShell').textContent=examples.windows;
  byId('shellOutput').textContent=examples.output==='result.json'?'输出 result.json：audio 为 Base64 音频，subtitles 为 SRT 文本；只需音频时取消「同时生成 SRT 字幕」。':'输出文件：'+examples.output+'（保存在终端当前目录）。';
}
function sizeLabel(bytes) {return bytes<1024?bytes+' B':bytes<1048576?(bytes/1024).toFixed(1)+' KB':(bytes/1048576).toFixed(2)+' MB';}
async function generate() {
  if (busy) return;
  const body=payload();
  if (!body.text.trim()) {notice('先写一段文字，再让它发声。','warning');byId('previewText').focus();return;}
  if(channel==='azure' && (!Number.isFinite(Number(body.styleDegree))||Number(body.styleDegree)<0.01||Number(body.styleDegree)>2)){notice('风格强度需要在 0.01 到 2 之间。','warning');byId('styleDegree').focus();return;}
  const selectedChannel=channel;const selectedVoice=voices.find(v=>v.ShortName===body.voice)||{ShortName:body.voice};
  busy=true;byId('edgeChannel').setAttribute('aria-disabled','true');byId('azureChannel').setAttribute('aria-disabled','true');byId('previewButton').disabled=true;byId('generateLabel').textContent='正在合成…';byId('generateIcon').textContent='◌';byId('resultStatus').textContent='合成中';byId('resultStatus').className='status-tag';byId('emptyResult').classList.add('busy');notice('');
  const start=performance.now();const controller=new AbortController();const timer=setTimeout(()=>controller.abort(),35000);
  try {
    const request=StudioProtocol.buildRequest(selectedChannel,body,byId('token').value);
    const response=await fetch(request.url,{method:'POST',headers:request.headers,body:request.body,signal:controller.signal});
    if (!response.ok) throw new Error(await requestError(response));
    let blob,subtitles='';
    if (selectedChannel==='edge' && body.response_format==='json') {
      const result=await response.json();
      if (typeof result.audio!=='string') throw new Error('响应缺少音频数据。');
      blob=new Blob([Uint8Array.from(atob(result.audio),c=>c.charCodeAt(0))],{type:result.content_type||'audio/mpeg'});
      subtitles=result.subtitles||'';
    } else blob=await response.blob();
    if (!blob.size) throw new Error('服务返回了空音频。');
    byId('audioPlayer').pause();
    if (audioURL) URL.revokeObjectURL(audioURL);
    if (subtitleURL) URL.revokeObjectURL(subtitleURL);
    audioURL=URL.createObjectURL(blob);subtitleURL='';
    const name=StudioProtocol.filename(body.format);
    byId('audioFilename').textContent=name;byId('downloadAudio').href=audioURL;byId('downloadAudio').download=name;
    byId('audioInfoLabel').textContent=sizeLabel(blob.size)+' · '+((performance.now()-start)/1000).toFixed(2)+' 秒 · '+voiceLabel(selectedVoice)+' · '+(selectedChannel==='azure'?'Azure':'Edge');
    byId('downloadSRT').hidden=!subtitles;
    if (subtitles) {subtitleURL=URL.createObjectURL(new Blob([subtitles],{type:'text/plain;charset=utf-8'}));byId('downloadSRT').href=subtitleURL;}
    byId('audioPlayer').src=audioURL;byId('emptyResult').hidden=true;byId('readyResult').hidden=false;byId('resultStatus').textContent='已生成';
    if(body.format.startsWith('raw'))notice('原始音频已生成，可下载文件；浏览器通常无法直接播放此格式。');
    else await byId('audioPlayer').play().catch(()=>notice('音频已生成。点击播放器即可试听。'));
  } catch (error) {byId('resultStatus').textContent='生成失败';notice(error.name==='AbortError'?'请求超时，请检查服务连接或缩短文本。':error.message,'error');}
  finally {clearTimeout(timer);busy=false;byId('edgeChannel').removeAttribute('aria-disabled');byId('azureChannel').removeAttribute('aria-disabled');byId('previewButton').disabled=false;byId('generateLabel').textContent='生成并试听';byId('generateIcon').textContent='▶';byId('emptyResult').classList.remove('busy');}
}
async function copyText(text,button) {
  const original=button.textContent;
  try {
    if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(text);
    else {const input=document.createElement('textarea');input.value=text;input.style.position='fixed';input.style.opacity='0';document.body.append(input);input.select();const ok=document.execCommand('copy');input.remove();if (!ok) throw new Error('请手动选择并复制。');}
    button.textContent='已复制 ✓';
  } catch (_) {button.textContent='请手动复制';}
  setTimeout(()=>button.textContent=original,1800);
}
byId('previewText').addEventListener('input',updateRequest);
byId('clearText').addEventListener('click',()=>{byId('previewText').value='';updateRequest();byId('previewText').focus();});
byId('language').addEventListener('change',()=>refreshVoices());byId('voiceName').addEventListener('change',updateVoice);
for (const id of ['rate','volume','pitch','styleDegree']) byId(id).addEventListener('input',updateRequest);
byId('voiceFormat').addEventListener('change',()=>{if (byId('subtitles').checked && byId('voiceFormat').value!=='audio-24khz-48kbitrate-mono-mp3') {byId('subtitles').checked=false;notice('此格式不支持字幕，已关闭 SRT 生成。','warning');}updateRequest();});
byId('subtitles').addEventListener('change',()=>{if (byId('subtitles').checked){byId('voiceFormat').value='audio-24khz-48kbitrate-mono-mp3';}updateRequest();});
byId('resetSettings').addEventListener('click',()=>{for (const id of ['rate','volume','pitch']) byId(id).value=0;byId('voiceFormat').value='audio-24khz-48kbitrate-mono-mp3';byId('subtitles').checked=false;byId('styleName').value='general';byId('styleDegree').value='1';byId('styleDegree').disabled=true;byId('roleName').value='default';byId('secondaryLocale').value='';saveChannelSettings();updateRequest();notice('声音参数已恢复默认值。');});
for (const button of document.querySelectorAll('[data-sample]')) button.addEventListener('click',()=>{const sample=samples[button.dataset.sample];byId('previewText').value=sample.text;if (voices.some(v=>v.Locale===sample.locale)){byId('language').value=sample.locale;refreshVoices(sample.voice);}updateRequest();});
byId('previewButton').addEventListener('click',generate);byId('reloadVoices').addEventListener('click',loadVoices);
function saveToken() {
  try {
    const token=byId('token').value;
    if(token) localStorage.setItem('token',token);
    else localStorage.removeItem('token');
    return true;
  } catch (_) {
    notice('浏览器禁止保存 Token，本次页面仍可使用；刷新后需重新填写。','warning');
    return false;
  }
}
byId('token').addEventListener('input',saveToken);
byId('token').addEventListener('change',()=>{if(saveToken())notice('Token 已更新，浏览器会记住；清空可移除。可点击 ↻ 刷新音色列表。');});
byId('toggleToken').addEventListener('click',()=>{const hidden=byId('token').type==='password';byId('token').type=hidden?'text':'password';byId('toggleToken').textContent=hidden?'隐藏':'显示';byId('toggleToken').setAttribute('aria-label',hidden?'隐藏 Token':'显示 Token');});
byId('copyMacShell').addEventListener('click',event=>copyText(byId('macShell').textContent,event.currentTarget));
byId('copyWindowsShell').addEventListener('click',event=>copyText(byId('windowsShell').textContent,event.currentTarget));
byId('copyRequest').addEventListener('click',event=>copyText(byId('requestPreview').textContent,event.currentTarget));
byId('textFile').addEventListener('change',async event=>{const file=event.target.files[0];if(!file)return;try{if(file.size>2*1024*1024)throw new Error('文本文件不能超过 2 MiB。');byId('previewText').value=await file.text();updateRequest();notice('已导入 '+file.name+'。');}catch(error){notice(error.message,'error');}finally{event.target.value='';}});
document.addEventListener('keydown',event=>{if((event.metaKey||event.ctrlKey)&&event.key==='Enter'){event.preventDefault();generate();}});
const voiceSamplePlayers=[...document.querySelectorAll('audio[data-voice-sample]')];
for(const player of voiceSamplePlayers) {
  player.addEventListener('play',()=>{
    byId('audioPlayer').pause();
    for(const other of voiceSamplePlayers)if(other!==player)other.pause();
  });
  player.addEventListener('error',()=>notice('试听音频暂时无法播放，请稍后重试。','warning'));
}
byId('audioPlayer').addEventListener('play',()=>{for(const player of voiceSamplePlayers)player.pause();});
byId('audioPlayer').addEventListener('error',()=>notice('浏览器无法播放此格式，可下载音频或切换到 MP3 重试。','warning'));
window.addEventListener('pagehide',()=>{if(audioURL)URL.revokeObjectURL(audioURL);if(subtitleURL)URL.revokeObjectURL(subtitleURL);});
try{byId('token').value=localStorage.getItem('token')||'';}catch(_){}
byId('styleName').addEventListener('change',()=>{byId('styleDegree').disabled=byId('styleName').value==='general';saveChannelSettings();updateRequest();});
for(const id of ['roleName','secondaryLocale'])byId(id).addEventListener('change',()=>{saveChannelSettings();updateRequest();});
for(const name of ['edge','azure'])byId(name+'Channel').addEventListener('click',event=>{if(busy){event.preventDefault();notice('请等待当前合成完成后切换渠道。','warning');}});
window.addEventListener('hashchange',()=>switchChannel(location.hash==='#azure'?'azure':'edge'));
switchChannel(location.hash==='#azure'?'azure':'edge',true);
