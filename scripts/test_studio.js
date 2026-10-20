#!/usr/bin/env node
'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const {StudioProtocol:protocol}=require('../server/public/studio-protocol.js');
const p={text:'你好 < & "世界"\u0001',voice:'zh-CN-XiaoxiaoNeural',rate:'-10%',volume:'-20%',pitch:'+5Hz',format:'audio-24khz-48kbitrate-mono-mp3',style:'cheerful',styleDegree:'1.5',role:'Narrator',secondaryLocale:'zh-CN'};
const azure=protocol.buildRequest('azure',p,'test-token');
assert.equal(azure.url,'/api/azure');assert.equal(azure.headers.Token,'test-token');assert.equal(azure.headers.Format,p.format);assert.equal(azure.headers['Content-Type'],'application/ssml+xml');
assert(azure.body.includes('你好 &lt; &amp; &quot;世界&quot; '));assert(azure.body.includes('style="cheerful"'));assert(azure.body.includes('role="Narrator"'));assert(azure.body.includes('<lang xml:lang="zh-CN">'));assert(azure.body.includes('pitch="+5Hz"'));
assert(!protocol.azureSSML({...p,style:'general',role:'default',secondaryLocale:''}).includes('<mstts:express-as'));
const edge=protocol.buildRequest('edge',{text:'你好',write_subtitles:true},'');assert.equal(edge.url,'/api/ra');assert(!('Token' in edge.headers));assert.equal(JSON.parse(edge.body).write_subtitles,true);
assert.equal(protocol.filename('riff-24khz-16bit-mono-pcm'),'audio.wav');assert.equal(protocol.filename('raw-16khz-16bit-mono-pcm'),'audio.pcm');
// Execute generated POSIX commands with a fake curl to check quoting and UTF-8.
const os=require('node:os');
const {execFileSync}=require('node:child_process');
const shellDir=fs.mkdtempSync(path.join(os.tmpdir(),'tts-shell-'));
try {
 fs.writeFileSync(path.join(shellDir,'curl'),'#!/bin/sh\nexit 0\n',{mode:0o755});
 const special={...p,text:"你好 ' $HOME `whoami` \n第二行",response_format:'audio'};
 for(const channel of ['edge','azure']) {
  const examples=protocol.shellExamples(channel,special,'https://tts.example.com');
  execFileSync('/bin/sh',['-c',examples.mac],{cwd:shellDir,env:{...process.env,PATH:shellDir+':'+process.env.PATH}});
  const expected=protocol.buildRequest(channel,special,'');
  assert.equal(fs.readFileSync(path.join(shellDir,channel==='edge'?'request.json':'request.ssml'),'utf8'),expected.body);
  assert(examples.mac.includes('https://tts.example.com'+expected.url));
  assert(examples.windows.includes('UTF8.GetBytes($body)'));
  assert(examples.windows.includes(expected.body.replace(/'/g,"''")));
  assert(examples.windows.includes("$token = 'YOUR_TOKEN'"));
 }
 assert.equal(protocol.shellExamples('edge',{...special,response_format:'json'},'https://tts.example.com').output,'result.json');
} finally {fs.rmSync(shellDir,{recursive:true,force:true});}
// Execute the UI controller with a small DOM fixture and controlled HTTP responses.
// This checks channel isolation and asynchronous voice-list races, without a browser.
const html=fs.readFileSync(path.join(__dirname,'../server/public/index.html'),'utf8');
class Element {
 constructor(id,tag,attrs){this.id=id;this.tag=tag;this._value=attrs.value||'';this.hidden='hidden' in attrs;this.checked=false;this.disabled=false;this.textContent='';this.options=[];this.listeners={};this.classList={toggle(){},add(){},remove(){}};this.attributes={...attrs};this.paused=true;}
 get value(){return this._value;}
 set value(v){this._value=String(v);}
 replaceChildren(...options){this.options=options;if(this.tag==='select')this._value=options[0]?.value||'';}
 addEventListener(name,fn){this.listeners[name]=fn;}
 setAttribute(name,value){this.attributes[name]=value;}
 removeAttribute(name){delete this.attributes[name];}
 pause(){this.paused=true;} play(){this.paused=false;this.listeners.play?.();return Promise.resolve();} getAttribute(name){return this.attributes[name]||null;} load(){} focus(){} closest(){return {open:false};}
}
const elements={};
for(const match of html.matchAll(/<([a-z]+)\b([^>]*\bid="([^"]+)"[^>]*)>/g)){
 const attrs={};for(const a of match[2].matchAll(/([\w-]+)(?:="([^"]*)")?/g))attrs[a[1]]=a[2]||'';
 elements[match[3]]=new Element(match[3],match[1],attrs);
}
elements.previewText.value='测试文字';
const calls=[];const pending=[];const windowEvents={};
const savedStorage=new Map([["token","remembered-user-token"]]);
const sandbox={console,AbortController,setTimeout,clearTimeout,URL,URLSearchParams,Option:class {constructor(text,value){this.text=text;this.value=value;}},document:{getElementById:id=>{assert(elements[id],'missing '+id);return elements[id];},querySelectorAll:selector=>selector==='audio[data-voice-sample]'?Object.values(elements).filter(el=>el.tag==='audio'&&el.attributes['data-voice-sample']):[],addEventListener(){}},window:{addEventListener:(name,fn)=>windowEvents[name]=fn},location:{hash:'#edge',origin:'http://localhost:1233'},history:{replaceState(){}},localStorage:{getItem:key=>savedStorage.get(key)||null,setItem:(key,value)=>savedStorage.set(key,value),removeItem:key=>savedStorage.delete(key)},fetch:(url,options)=>{calls.push({url,options});return new Promise(resolve=>pending.push(resolve));}};
sandbox.StudioProtocol=protocol;
vm.createContext(sandbox);vm.runInContext(fs.readFileSync(path.join(__dirname,'../server/public/config.js'),'utf8'),sandbox);vm.runInContext(fs.readFileSync(path.join(__dirname,'../server/public/studio.js'),'utf8'),sandbox);
const edgeList=[{ShortName:'zh-CN-XiaoxiaoNeural',Locale:'zh-CN',Gender:'Female'}];
const azureList=[{...edgeList[0],LocalName:'晓晓',StyleList:['cheerful'],RolePlayList:['Narrator'],SecondaryLocaleList:['en-US']}];
const response=data=>({ok:true,json:async()=>data});
const flush=()=>new Promise(resolve=>setImmediate(resolve));
(async()=>{
 const samplePlayers=Object.values(elements).filter(el=>el.tag==='audio'&&el.attributes['data-voice-sample']);
 assert.equal(samplePlayers.length,6);
 assert(samplePlayers.every(el=>el.src.startsWith('/voice-samples/edge/')));
 const initialCalls=calls.length;
 elements.audioPlayer.play();samplePlayers[0].play();assert(elements.audioPlayer.paused);
 samplePlayers[1].play();assert(samplePlayers[0].paused);assert(!samplePlayers[1].paused);
 elements.audioPlayer.play();assert(samplePlayers.every(el=>el.paused));
 assert.equal(calls.length,initialCalls,'static previews must not call synthesis APIs');
 assert.equal(elements.token.value,'remembered-user-token');
 assert.equal(calls[0].options.headers.Token,'remembered-user-token');
 elements.token.value='new-user-token';elements.token.listeners.input();
 assert.equal(savedStorage.get('token'),'new-user-token');
 assert(!elements.macShell.textContent.includes('new-user-token'));
 elements.token.value='';elements.token.listeners.input();
 assert(!savedStorage.has('token'));
 const originalSetItem=sandbox.localStorage.setItem;
 sandbox.localStorage.setItem=()=>{throw new Error('Storage blocked');};
 elements.token.value='session-only-token';assert.doesNotThrow(()=>elements.token.listeners.input());
 assert(elements.notice.textContent.includes('禁止保存'));
 sandbox.localStorage.setItem=originalSetItem;
 elements.token.value='';elements.token.listeners.change();
 assert.equal(calls[0].url,'/api/ra');assert.equal(JSON.parse(calls[0].options.body).list_voices,true);
 // A slow Edge response must not overwrite Azure's voice list after switching.
 sandbox.location.hash='#azure';windowEvents.hashchange();
 assert(samplePlayers.every(el=>el.paused&&el.src.startsWith('/voice-samples/azure/')));
 assert.equal(calls[1].url,'/api/azure/voices');assert.equal(elements.endpointPath.textContent,'/api/azure');assert(elements.subtitlesControl.hidden);
 pending[1](response(azureList));await flush();pending[0](response({voices:edgeList}));await flush();
 assert.equal(elements.styleName.disabled,false);assert.equal(elements.roleName.disabled,false);assert.equal(elements.secondaryLocaleField.hidden,false);
 elements.styleName.value='cheerful';elements.styleName.listeners.change();elements.roleName.value='Narrator';elements.roleName.listeners.change();
 elements.secondaryLocale.value='en-US';elements.secondaryLocale.listeners.change();sandbox.updateRequest();
 assert(elements.macShell.textContent.includes('https://tts.cookup.cn/api/azure'));assert(elements.windowsShell.textContent.includes('Format ='));
 assert(elements.requestPreview.textContent.includes('style="cheerful"'));assert(elements.requestPreview.textContent.includes('xml:lang="en-US"'));
 sandbox.location.hash='#edge';windowEvents.hashchange();assert(!elements.subtitlesControl.hidden);assert(elements.azureOptions.hidden);pending[2](response({voices:edgeList}));await flush();
 assert(elements.macShell.textContent.includes('https://tts.cookup.cn/api/ra'));assert(!elements.windowsShell.textContent.includes('Format ='));
 assert.equal(JSON.parse(elements.requestPreview.textContent).voice,'zh-CN-XiaoxiaoNeural');
 sandbox.location.hash='#azure';windowEvents.hashchange();pending[3](response(azureList));await flush();assert.equal(elements.styleName.value,'cheerful');assert.equal(elements.roleName.value,'Narrator');
 console.log('Studio checks passed: protocol, XML escaping, Azure capabilities, channel switching, stale responses, saved settings, configured API domain, Token persistence and storage denial.');
})().catch(error=>{console.error(error);process.exitCode=1;});
