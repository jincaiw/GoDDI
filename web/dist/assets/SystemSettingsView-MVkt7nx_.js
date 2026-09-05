import{C as e,E as t,H as n,Q as r,W as i,Z as a,_ as o,b as s,ct as c,d as l,g as u,k as d,kt as f,mt as p,ot as m,v as h,w as g,z as _}from"./echarts-Cw2yHLaZ.js";import{A as v,Bt as y,Ft as b,Pt as x,Rt as S,S as C,Ut as w,a as T,b as E,dt as D,ft as O,s as k,u as A,x as j}from"./auth-CYMAsBzV.js";import{j as M,o as N}from"./vue-core-BDTvi3xZ.js";import{t as P}from"./use-compitable-OuJoh8Ky.js";import{n as F,t as I}from"./FormItem-BxhCRETX.js";import{t as L}from"./Input-BkdkO5EE.js";import{t as R}from"./InputNumber-CsSdg2CO.js";import{t as z}from"./use-message-CPIKa8Ng.js";import{t as B}from"./Switch-BqlmE47V.js";import{O as V,o as H}from"./index-ChsYpqrV.js";import{t as U}from"./PageHeader-BpKOrg0_.js";var W=x([x(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),b(`spin-container`,`
 position: relative;
 `,[b(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[E()])]),b(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),b(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[S(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),b(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),b(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[S(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),G={small:20,medium:18,large:16},K=t({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},v.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),C),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=O(e),r=v(`Spin`,`-spin`,W,H,e,t),i=u(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{opacitySpinning:a,color:o,textColor:s}=i;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?M(t):i[y(`size`,t)],"--n-color":o,"--n-text-color":s}}),o=n?D(`spin`,u(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),i,e):void 0,s=P(e,[`spinning`,`show`]),l=c(!1);return a(t=>{let n;if(s.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{l.value=!0},r),t(()=>{clearTimeout(n)});return}}l.value=s.value}),{mergedClsPrefix:t,active:l,mergedStrokeWidth:u(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return G[typeof n==`number`?`medium`:n]}),cssVars:n?void 0:i,themeClass:o?.themeClass,onRender:o?.onRender}},render(){var e;let{$slots:t,mergedClsPrefix:n,description:r}=this,i=t.icon&&this.rotate,a=(r||t.description)&&d(`div`,{class:`${n}-spin-description`},r||t.description?.call(t)),o=t.icon?d(`div`,{class:[`${n}-spin-body`,this.themeClass]},d(`div`,{class:[`${n}-spin`,i&&`${n}-spin--rotate`],style:t.default?``:this.cssVars},t.icon()),a):d(`div`,{class:[`${n}-spin-body`,this.themeClass]},d(j,{clsPrefix:n,style:t.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${n}-spin`}),a);return(e=this.onRender)==null||e.call(this),t.default?d(`div`,{class:[`${n}-spin-container`,this.themeClass],style:this.cssVars},d(`div`,{class:[`${n}-spin-content`,this.active&&`${n}-spin-content--spinning`,this.contentClass],style:this.contentStyle},t),d(w,{name:`fade-in-transition`},{default:()=>this.active?o:null})):o}});function q(e){return T(`/system/settings`,e)}function J(e,t){return k(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=t({__name:`SystemSettingsView`,setup(t){let{t:a}=N(),u=z(),d=c(!1),v=c([]),y=m({}),b=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],x=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function S(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:b.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:x.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function C(e){return S(e)===`int`}function w(e){return!!e&&e.length>100}let T=m({});async function E(){d.value=!0;try{let e=await q();v.value=e.data;for(let t of e.data)C(t)&&(T[t.key]=Number(t.value))}catch(e){u.error(e instanceof Error?e.message:a(`common.failed`))}finally{d.value=!1}}async function D(e){y[e.key]=!0;try{let t=C(e)?String(T[e.key]??e.value):e.value;await J(e.key,t),u.success(a(`settings.updateSuccess`))}catch(e){u.error(e instanceof Error?e.message:a(`common.failed`))}finally{y[e.key]=!1}}return _(E),(t,c)=>{let u=B,m=R,_=L,b=A,x=I,S=F,C=V,E=K;return n(),s(`div`,null,[g(U,{title:p(a)(`settings.title`)},null,8,[`title`]),g(E,{show:d.value},{default:r(()=>[g(C,null,{default:r(()=>[g(S,{"label-placement":`left`,"label-width":`200px`},{default:r(()=>[(n(!0),s(l,null,i(v.value,t=>(n(),h(x,{key:t.key,label:t.key},{feedback:r(()=>[o(`span`,Y,f(t.description),1)]),default:r(()=>[t.type===`bool`?(n(),h(u,{key:0,value:t.value,"onUpdate:value":e=>t.value=e,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):t.type===`int`?(n(),h(m,{key:1,value:T[t.key],"onUpdate:value":e=>T[t.key]=e,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(n(),h(_,{key:2,value:t.value,"onUpdate:value":e=>t.value=e,type:w(String(t.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),g(b,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:y[t.key],onClick:e=>D(t)},{default:r(()=>[e(f(p(a)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};