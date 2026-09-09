import{B as e,C as t,E as n,G as r,Tt as i,U as a,_ as o,b as s,d as c,dt as l,et as u,g as d,k as f,n as p,nt as m,pt as h,v as g,w as _,xt as v}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Cr as y,Dn as b,F as x,I as S,Jr as C,Jt as w,Kt as T,Nr as E,On as D,Or as O,S as k,ht as A,ir as j,jr as M,kr as N,pt as P,qt as F,un as I,w as L,xt as R}from"./router-DKQW0l-j.js";import{n as z,t as B}from"./FormItem-B8fK6zg_.js";import{c as V,r as H}from"./index-CM9f15Db.js";import{t as U}from"./PageHeader-BcwmmS7V.js";var W=O([O(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),N(`spin-container`,`
 position: relative;
 `,[N(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[T()])]),N(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),N(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[M(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),N(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),N(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[M(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),G={small:20,medium:18,large:16},K=n({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},I.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),w),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=D(e),r=I(`Spin`,`-spin`,W,H,e,t),i=d(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{opacitySpinning:a,color:o,textColor:s}=i;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?y(t):i[E(`size`,t)],"--n-color":o,"--n-text-color":s}}),a=n?b(`spin`,d(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),i,e):void 0,o=j(e,[`spinning`,`show`]),s=h(!1);return u(t=>{let n;if(o.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{s.value=!0},r),t(()=>{clearTimeout(n)});return}}s.value=o.value}),{mergedClsPrefix:t,active:s,mergedStrokeWidth:d(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return G[typeof n==`number`?`medium`:n]}),cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{$slots:t,mergedClsPrefix:n,description:r}=this,i=t.icon&&this.rotate,a=(r||t.description)&&f(`div`,{class:`${n}-spin-description`},r||t.description?.call(t)),o=t.icon?f(`div`,{class:[`${n}-spin-body`,this.themeClass]},f(`div`,{class:[`${n}-spin`,i&&`${n}-spin--rotate`],style:t.default?``:this.cssVars},t.icon()),a):f(`div`,{class:[`${n}-spin-body`,this.themeClass]},f(F,{clsPrefix:n,style:t.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${n}-spin`}),a);return(e=this.onRender)==null||e.call(this),t.default?f(`div`,{class:[`${n}-spin-container`,this.themeClass],style:this.cssVars},f(`div`,{class:[`${n}-spin-content`,this.active&&`${n}-spin-content--spinning`,this.contentClass],style:this.contentStyle},t),f(p,{name:`fade-in-transition`},{default:()=>this.active?o:null})):o}});function q(e){return k(`/system/settings`,e)}function J(e,t){return L(`/system/settings/${e}`,{value:t})}var Y={style:{color:`var(--n-text-color-3)`,"font-size":`12px`}},X=n({name:`settings_system`,__name:`index`,setup(n){let{t:u}=C(),d=V();function f(e){let t=u(`settings.items.${e}.label`);return t===`settings.items.${e}.label`?e:t}function p(e){let t=u(`settings.items.${e.key}.desc`);return t===`settings.items.${e.key}.desc`?e.description:t}let y=h(!1),b=h([]),w=l({}),T=[`server_dark_mode`,`dns_enable_logging`,`dhcp_enable_failover`],E=[`dns_default_ttl`,`dns_cache_max_entries`,`dhcp_default_lease_time`,`dhcp_max_lease_time`];function D(e){return e.type===`bool`||e.type===`int`||e.type===`string`?e.type:T.includes(e.key)||e.value===`true`||e.value===`false`?`bool`:E.includes(e.key)||typeof e.value==`string`&&/^\d+$/.test(e.value)?`int`:`string`}function O(e){return D(e)===`int`}function k(e){return!!e&&e.length>100}let j=l({});async function M(){y.value=!0;try{let e=await q();b.value=e.data;for(let t of e.data)O(t)&&(j[t.key]=Number(t.value))}catch(e){d.error(e instanceof Error?e.message:u(`common.failed`))}finally{y.value=!1}}async function N(e){w[e.key]=!0;try{let t=O(e)?String(j[e.key]??e.value):e.value;await J(e.key,t),d.success(u(`settings.updateSuccess`))}catch(e){d.error(e instanceof Error?e.message:u(`common.failed`))}finally{w[e.key]=!1}}return e(M),(e,n)=>{let l=x,d=S,h=R,C=A,T=B,E=z,D=P,O=K;return a(),s(`div`,null,[_(U,{title:v(u)(`settings.title`)},null,8,[`title`]),_(O,{show:y.value},{default:m(()=>[_(D,null,{default:m(()=>[_(E,{"label-placement":`left`,"label-width":`200px`},{default:m(()=>[(a(!0),s(c,null,r(b.value,e=>(a(),g(T,{key:e.key,label:f(e.key)},{feedback:m(()=>[o(`span`,Y,i(p(e)),1)]),default:m(()=>[e.type===`bool`?(a(),g(l,{key:0,value:e.value,"onUpdate:value":t=>e.value=t,"checked-value":`true`,"unchecked-value":`false`},null,8,[`value`,`onUpdate:value`])):e.type===`int`?(a(),g(d,{key:1,value:j[e.key],"onUpdate:value":t=>j[e.key]=t,min:0,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`])):(a(),g(h,{key:2,value:e.value,"onUpdate:value":t=>e.value=t,type:k(String(e.value))?`textarea`:`text`,rows:3,style:{"max-width":`500px`}},null,8,[`value`,`onUpdate:value`,`type`])),_(C,{type:`primary`,size:`small`,style:{"margin-left":`8px`},loading:w[e.key],onClick:t=>N(e)},{default:m(()=>[t(i(v(u)(`common.save`)),1)]),_:1},8,[`loading`,`onClick`])]),_:2},1032,[`label`]))),128))]),_:1})]),_:1})]),_:1},8,[`show`])])}}});export{X as default};