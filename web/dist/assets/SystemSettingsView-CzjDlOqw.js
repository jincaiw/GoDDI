import{$ as e,A as t,At as n,B as r,D as i,G as a,Q as o,T as s,U as c,_ as l,f as u,ht as d,lt as f,st as p,v as m,w as h,x as g,y as _}from"./echarts-Bmuv2i_G.js";import{C as v,K as y,S as b,f as x,l as S,s as C,x as w}from"./auth-BTeG-_sB.js";import{j as T,o as E}from"./vue-core-bFnGnCI4.js";import{G as D,K as O,N as k,P as A,Y as j,Z as M,i as N}from"./light-B4Ik6WNX.js";import{t as P}from"./use-compitable-BMUbnpEM.js";import{n as F,t as I}from"./FormItem-OrlOSWkT.js";import{t as L}from"./Input-CJN0Uu5u.js";import{t as R}from"./InputNumber-BlEi_M1y.js";import{t as z}from"./Switch-BydDc8YI.js";import{O as B,o as V,p as H}from"./index-Yh_aUYzb.js";import{t as U}from"./PageHeader-CdWoZL3J.js";var W=D([D(`@keyframes spin-rotate`,`
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
 `)])]),G={small:20,medium:18,large:16},K=i({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},N.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),v),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=A(e),r=N(`Spin`,`-spin`,W,V,e,t),i=l(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{opacitySpinning:a,color:o,textColor:s}=i;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?T(t):i[M(`size`,t)],"--n-color":o,"--n-text-color":s}}),a=n?k(`spin`,l(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),i,e):void 0,s=P(e,[`spinning`,`show`]),c=f(!1);return o(t=>{let n;if(s.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{c.value=!0},r),t(()=>{clearTimeout(n)});return}}c.value=s.value}),{mergedClsPrefix:t,active:c,mergedStrokeWidth:l(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return G[typeof n==`number`?`medium`:n]}),cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{$slots:n,mergedClsPrefix:r,description:i}=this,a=n.icon&&this.rotate,o=(i||n.description)&&t(`div`,{class:`${r}-spin-description`},i||n.description?.call(n)),s=n.icon?t(`div`,{class:[`${r}-spin-body`,this.themeClass]},t(`div`,{class:[`${r}-spin`,a&&`${r}-spin--rotate`],style:n.default?``:this.cssVars},n.icon()),o):t(`div`,{class:[`${r}-spin-body`,this.themeClass]},t(b,{clsPrefix:r,style:n.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${r}-spin`}),o);return(e=this.onRender)==null||e.call(this),n.default?t(`div`,{class:[`${r}-spin-container`,this.themeClass],style:this.cssVars},t(`div`,{class:[`${r}-spin-content`,this.active&&`${r}-spin-content--spinning`,this.contentClass],style:this.contentStyle},n),t(y,{name:`fade-in-transition`},{default:()=>this.active?s:null})):s}});function q(e){return C(`/system/settings`,e)}function J(e,t){return S(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=i({__name:`SystemSettingsView`,setup(t){let{t:i}=E(),o=H();function l(e){let t=i(`settings.items.${e}.label`);return t===`settings.items.${e}.label`?e:t}function v(e){let t=i(`settings.items.${e.key}.desc`);return t===`settings.items.${e.key}.desc`?e.description:t}let y=f(!1),b=f([]),S=p({}),C=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],w=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function T(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:C.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:w.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function D(e){return T(e)===`int`}function O(e){return!!e&&e.length>100}let k=p({});async function A(){y.value=!0;try{let e=await q();b.value=e.data;for(let t of e.data)D(t)&&(k[t.key]=Number(t.value))}catch(e){o.error(e instanceof Error?e.message:i(`common.failed`))}finally{y.value=!1}}async function j(e){S[e.key]=!0;try{let t=D(e)?String(k[e.key]??e.value):e.value;await J(e.key,t),o.success(i(`settings.updateSuccess`))}catch(e){o.error(e instanceof Error?e.message:i(`common.failed`))}finally{S[e.key]=!1}}return r(A),(t,r)=>{let o=z,f=R,p=L,C=x,w=I,T=F,E=B,D=K;return c(),g(`div`,null,[s(U,{title:d(i)(`settings.title`)},null,8,[`title`]),s(D,{show:y.value},{default:e(()=>[s(E,null,{default:e(()=>[s(T,{"label-placement":`left`,"label-width":`200px`},{default:e(()=>[(c(!0),g(u,null,a(b.value,t=>(c(),_(w,{key:t.key,label:l(t.key)},{feedback:e(()=>[m(`span`,Y,n(v(t)),1)]),default:e(()=>[t.type===`bool`?(c(),_(o,{key:0,value:t.value,"onUpdate:value":e=>t.value=e,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):t.type===`int`?(c(),_(f,{key:1,value:k[t.key],"onUpdate:value":e=>k[t.key]=e,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(c(),_(p,{key:2,value:t.value,"onUpdate:value":e=>t.value=e,type:O(String(t.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),s(C,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:S[t.key],onClick:e=>j(t)},{default:e(()=>[h(n(d(i)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};