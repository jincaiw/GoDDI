import{C as e,E as t,H as n,Q as r,W as i,Z as a,_ as o,b as s,ct as c,d as l,g as u,k as d,kt as f,mt as p,ot as m,v as h,w as g,z as _}from"./echarts-Cw2yHLaZ.js";import{W as v,a as y,b,s as x,u as S,x as C,y as w}from"./auth-CpMNBSoS.js";import{j as T,o as E}from"./vue-core-BDTvi3xZ.js";import{G as D,K as O,N as k,P as A,Y as j,Z as M,i as N}from"./light-BluJl8SD.js";import{t as P}from"./use-compitable-OuJoh8Ky.js";import{n as F,t as I}from"./FormItem-naKoOago.js";import{t as L}from"./Input-CXaR7Brf.js";import{t as R}from"./InputNumber-CbX73_my.js";import{t as z}from"./Switch-yIi-9Ihy.js";import{O as B,o as V,p as H}from"./index-BUYVDbRn.js";import{t as U}from"./PageHeader-ClyZphgg.js";var W=D([D(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),O(`spin-container`,`
 position: relative;
 `,[O(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[w()])]),O(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),O(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[j(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),O(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),O(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[j(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),G={small:20,medium:18,large:16},K=t({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},N.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),C),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=A(e),r=N(`Spin`,`-spin`,W,V,e,t),i=u(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{opacitySpinning:a,color:o,textColor:s}=i;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?T(t):i[M(`size`,t)],"--n-color":o,"--n-text-color":s}}),o=n?k(`spin`,u(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),i,e):void 0,s=P(e,[`spinning`,`show`]),l=c(!1);return a(t=>{let n;if(s.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{l.value=!0},r),t(()=>{clearTimeout(n)});return}}l.value=s.value}),{mergedClsPrefix:t,active:l,mergedStrokeWidth:u(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return G[typeof n==`number`?`medium`:n]}),cssVars:n?void 0:i,themeClass:o?.themeClass,onRender:o?.onRender}},render(){var e;let{$slots:t,mergedClsPrefix:n,description:r}=this,i=t.icon&&this.rotate,a=(r||t.description)&&d(`div`,{class:`${n}-spin-description`},r||t.description?.call(t)),o=t.icon?d(`div`,{class:[`${n}-spin-body`,this.themeClass]},d(`div`,{class:[`${n}-spin`,i&&`${n}-spin--rotate`],style:t.default?``:this.cssVars},t.icon()),a):d(`div`,{class:[`${n}-spin-body`,this.themeClass]},d(b,{clsPrefix:n,style:t.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${n}-spin`}),a);return(e=this.onRender)==null||e.call(this),t.default?d(`div`,{class:[`${n}-spin-container`,this.themeClass],style:this.cssVars},d(`div`,{class:[`${n}-spin-content`,this.active&&`${n}-spin-content--spinning`,this.contentClass],style:this.contentStyle},t),d(v,{name:`fade-in-transition`},{default:()=>this.active?o:null})):o}});function q(e){return y(`/system/settings`,e)}function J(e,t){return x(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=t({__name:`SystemSettingsView`,setup(t){let{t:a}=E(),u=H();function d(e){let t=a(`settings.items.${e}.label`);return t===`settings.items.${e}.label`?e:t}function v(e){let t=a(`settings.items.${e.key}.desc`);return t===`settings.items.${e.key}.desc`?e.description:t}let y=c(!1),b=c([]),x=m({}),C=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],w=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function T(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:C.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:w.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function D(e){return T(e)===`int`}function O(e){return!!e&&e.length>100}let k=m({});async function A(){y.value=!0;try{let e=await q();b.value=e.data;for(let t of e.data)D(t)&&(k[t.key]=Number(t.value))}catch(e){u.error(e instanceof Error?e.message:a(`common.failed`))}finally{y.value=!1}}async function j(e){x[e.key]=!0;try{let t=D(e)?String(k[e.key]??e.value):e.value;await J(e.key,t),u.success(a(`settings.updateSuccess`))}catch(e){u.error(e instanceof Error?e.message:a(`common.failed`))}finally{x[e.key]=!1}}return _(A),(t,c)=>{let u=z,m=R,_=L,C=S,w=I,T=F,E=B,D=K;return n(),s(`div`,null,[g(U,{title:p(a)(`settings.title`)},null,8,[`title`]),g(D,{show:y.value},{default:r(()=>[g(E,null,{default:r(()=>[g(T,{"label-placement":`left`,"label-width":`200px`},{default:r(()=>[(n(!0),s(l,null,i(b.value,t=>(n(),h(w,{key:t.key,label:d(t.key)},{feedback:r(()=>[o(`span`,Y,f(v(t)),1)]),default:r(()=>[t.type===`bool`?(n(),h(u,{key:0,value:t.value,"onUpdate:value":e=>t.value=e,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):t.type===`int`?(n(),h(m,{key:1,value:k[t.key],"onUpdate:value":e=>k[t.key]=e,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(n(),h(_,{key:2,value:t.value,"onUpdate:value":e=>t.value=e,type:O(String(t.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),g(C,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:x[t.key],onClick:e=>j(t)},{default:r(()=>[e(f(p(a)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};