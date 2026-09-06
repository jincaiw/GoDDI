import{$ as e,E as t,K as n,O as r,S as i,T as a,V as o,W as s,b as c,ct as l,et as u,gt as d,j as f,jt as p,p as m,ut as h,v as g,y as _}from"./echarts-eUEtiXc8.js";import{C as v,K as y,S as b,f as x,l as S,s as C,x as w}from"./auth-B7OufbA5.js";import{j as T,o as E}from"./vue-core-CcEAoDdX.js";import{G as D,K as O,N as k,P as A,Y as j,Z as M,i as N}from"./light-BKCENzy2.js";import{t as P}from"./use-compitable-DSN36vHZ.js";import{n as F,t as I}from"./FormItem-FNG01EY_.js";import{t as L}from"./Input-D5GLSr6t.js";import{t as R}from"./InputNumber-BpLcbslt.js";import{t as z}from"./Switch-2m67hJ3j.js";import{O as B,o as V,p as H}from"./index-CGmIOu7l.js";import{t as U}from"./PageHeader-B2HNT6BN.js";var W=D([D(`@keyframes spin-rotate`,`
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
 `)])]),G={small:20,medium:18,large:16},K=r({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},N.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),v),slots:Object,setup(t){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=A(t),i=N(`Spin`,`-spin`,W,V,t,n),a=g(()=>{let{size:e}=t,{common:{cubicBezierEaseInOut:n},self:r}=i.value,{opacitySpinning:a,color:o,textColor:s}=r;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof e==`number`?T(e):r[M(`size`,e)],"--n-color":o,"--n-text-color":s}}),o=r?k(`spin`,g(()=>{let{size:e}=t;return typeof e==`number`?String(e):e[0]}),a,t):void 0,s=P(t,[`spinning`,`show`]),c=h(!1);return e(e=>{let n;if(s.value){let{delay:r}=t;if(r){n=window.setTimeout(()=>{c.value=!0},r),e(()=>{clearTimeout(n)});return}}c.value=s.value}),{mergedClsPrefix:n,active:c,mergedStrokeWidth:g(()=>{let{strokeWidth:e}=t;if(e!==void 0)return e;let{size:n}=t;return G[typeof n==`number`?`medium`:n]}),cssVars:r?void 0:a,themeClass:o?.themeClass,onRender:o?.onRender}},render(){var e;let{$slots:t,mergedClsPrefix:n,description:r}=this,i=t.icon&&this.rotate,a=(r||t.description)&&f(`div`,{class:`${n}-spin-description`},r||t.description?.call(t)),o=t.icon?f(`div`,{class:[`${n}-spin-body`,this.themeClass]},f(`div`,{class:[`${n}-spin`,i&&`${n}-spin--rotate`],style:t.default?``:this.cssVars},t.icon()),a):f(`div`,{class:[`${n}-spin-body`,this.themeClass]},f(b,{clsPrefix:n,style:t.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${n}-spin`}),a);return(e=this.onRender)==null||e.call(this),t.default?f(`div`,{class:[`${n}-spin-container`,this.themeClass],style:this.cssVars},f(`div`,{class:[`${n}-spin-content`,this.active&&`${n}-spin-content--spinning`,this.contentClass],style:this.contentStyle},t),f(y,{name:`fade-in-transition`},{default:()=>this.active?o:null})):o}});function q(e){return C(`/system/settings`,e)}function J(e,t){return S(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=r({__name:`SystemSettingsView`,setup(e){let{t:r}=E(),f=H();function g(e){let t=r(`settings.items.${e}.label`);return t===`settings.items.${e}.label`?e:t}function v(e){let t=r(`settings.items.${e.key}.desc`);return t===`settings.items.${e.key}.desc`?e.description:t}let y=h(!1),b=h([]),S=l({}),C=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],w=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function T(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:C.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:w.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function D(e){return T(e)===`int`}function O(e){return!!e&&e.length>100}let k=l({});async function A(){y.value=!0;try{let e=await q();b.value=e.data;for(let t of e.data)D(t)&&(k[t.key]=Number(t.value))}catch(e){f.error(e instanceof Error?e.message:r(`common.failed`))}finally{y.value=!1}}async function j(e){S[e.key]=!0;try{let t=D(e)?String(k[e.key]??e.value):e.value;await J(e.key,t),f.success(r(`settings.updateSuccess`))}catch(e){f.error(e instanceof Error?e.message:r(`common.failed`))}finally{S[e.key]=!1}}return o(A),(e,o)=>{let l=z,f=R,h=L,C=x,w=I,T=F,E=B,D=K;return s(),i(`div`,null,[t(U,{title:d(r)(`settings.title`)},null,8,[`title`]),t(D,{show:y.value},{default:u(()=>[t(E,null,{default:u(()=>[t(T,{"label-placement":`left`,"label-width":`200px`},{default:u(()=>[(s(!0),i(m,null,n(b.value,e=>(s(),c(w,{key:e.key,label:g(e.key)},{feedback:u(()=>[_(`span`,Y,p(v(e)),1)]),default:u(()=>[e.type===`bool`?(s(),c(l,{key:0,value:e.value,"onUpdate:value":t=>e.value=t,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):e.type===`int`?(s(),c(f,{key:1,value:k[e.key],"onUpdate:value":t=>k[e.key]=t,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(s(),c(h,{key:2,value:e.value,"onUpdate:value":t=>e.value=t,type:O(String(e.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),t(C,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:S[e.key],onClick:t=>j(e)},{default:u(()=>[a(p(d(r)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};