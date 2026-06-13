import{C as e,O as t,Ot as n,R as r,T as i,U as a,V as o,X as s,Z as c,_ as l,at as u,b as d,d as f,g as p,pt as m,st as h,v as g,w as _}from"./echarts-DxBJA66o.js";import{Ht as v,Lt as y,Nt as b,Pt as x,a as S,b as C,dt as w,k as T,l as E,s as D,ut as O,x as k,y as A,zt as j}from"./auth-DEMSBBAb.js";import{j as M,o as N}from"./vue-core-RsSxNVS3.js";import{t as P}from"./use-compitable-mYvAsDS6.js";import{n as F,t as I}from"./FormItem-BoUwzbFY.js";import{t as L}from"./Input-B3_w6SX1.js";import{t as R}from"./InputNumber-CyIzv-Z5.js";import{t as z}from"./use-message-CwVCvVSw.js";import{t as B}from"./Switch-CFCaDdRr.js";import{O as V,o as H}from"./index-lJkVLtdC.js";import{t as U}from"./PageHeader-BAc0kMfG.js";var W=b([b(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),x(`spin-container`,`
 position: relative;
 `,[x(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[A()])]),x(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),x(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[y(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),x(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),x(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[y(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),G={small:20,medium:18,large:16},K=i({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},T.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),k),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=w(e),r=T(`Spin`,`-spin`,W,H,e,t),i=p(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{opacitySpinning:a,color:o,textColor:s}=i;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?M(t):i[j(`size`,t)],"--n-color":o,"--n-text-color":s}}),a=n?O(`spin`,p(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),i,e):void 0,o=P(e,[`spinning`,`show`]),c=h(!1);return s(t=>{let n;if(o.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{c.value=!0},r),t(()=>{clearTimeout(n)});return}}c.value=o.value}),{mergedClsPrefix:t,active:c,mergedStrokeWidth:p(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return G[typeof n==`number`?`medium`:n]}),cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{$slots:n,mergedClsPrefix:r,description:i}=this,a=n.icon&&this.rotate,o=(i||n.description)&&t(`div`,{class:`${r}-spin-description`},i||n.description?.call(n)),s=n.icon?t(`div`,{class:[`${r}-spin-body`,this.themeClass]},t(`div`,{class:[`${r}-spin`,a&&`${r}-spin--rotate`],style:n.default?``:this.cssVars},n.icon()),o):t(`div`,{class:[`${r}-spin-body`,this.themeClass]},t(C,{clsPrefix:r,style:n.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${r}-spin`}),o);return(e=this.onRender)==null||e.call(this),n.default?t(`div`,{class:[`${r}-spin-container`,this.themeClass],style:this.cssVars},t(`div`,{class:[`${r}-spin-content`,this.active&&`${r}-spin-content--spinning`,this.contentClass],style:this.contentStyle},n),t(v,{name:`fade-in-transition`},{default:()=>this.active?s:null})):s}});function q(e){return S(`/system/settings`,e)}function J(e,t){return D(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=i({__name:`SystemSettingsView`,setup(t){let{t:i}=N(),s=z(),p=h(!1),v=h([]),y=u({}),b=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],x=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function S(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:b.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:x.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function C(e){return S(e)===`int`}function w(e){return!!e&&e.length>100}let T=u({});async function D(){p.value=!0;try{let e=await q();v.value=e.data;for(let t of e.data)C(t)&&(T[t.key]=Number(t.value))}catch(e){s.error(e instanceof Error?e.message:i(`common.failed`))}finally{p.value=!1}}async function O(e){y[e.key]=!0;try{let t=C(e)?String(T[e.key]??e.value):e.value;await J(e.key,t),s.success(i(`settings.updateSuccess`))}catch(e){s.error(e instanceof Error?e.message:i(`common.failed`))}finally{y[e.key]=!1}}return r(D),(t,r)=>{let s=B,u=R,h=L,b=E,x=I,S=F,C=V,D=K;return o(),d(`div`,null,[_(U,{title:m(i)(`settings.title`)},null,8,[`title`]),_(D,{show:p.value},{default:c(()=>[_(C,null,{default:c(()=>[_(S,{"label-placement":`left`,"label-width":`200px`},{default:c(()=>[(o(!0),d(f,null,a(v.value,t=>(o(),g(x,{key:t.key,label:t.key},{feedback:c(()=>[l(`span`,Y,n(t.description),1)]),default:c(()=>[t.type===`bool`?(o(),g(s,{key:0,value:t.value,"onUpdate:value":e=>t.value=e,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):t.type===`int`?(o(),g(u,{key:1,value:T[t.key],"onUpdate:value":e=>T[t.key]=e,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(o(),g(h,{key:2,value:t.value,"onUpdate:value":e=>t.value=e,type:w(String(t.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),_(b,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:y[t.key],onClick:e=>O(t)},{default:c(()=>[e(n(m(i)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};