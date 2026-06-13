import{A as e,H as t,M as n,N as r,O as i,Q as a,R as o,T as s,X as c,Y as l,d as u,dt as d,g as f,h as p,st as m}from"./echarts-DxBJA66o.js";import{C as h,D as g,Dt as _,Gt as v,It as y,Lt as b,Nt as x,Pt as S,Rt as C,St as w,Ut as T,dt as ee,et as E,k as D,rt as O,st as k,ut as te,xt as A,yt as j,zt as M}from"./auth-DEMSBBAb.js";import{A as N,O as ne,b as re,d as P,l as ie}from"./vue-core-RsSxNVS3.js";import{s as ae}from"./get-D5hjkymV.js";import{t as oe}from"./use-compitable-mYvAsDS6.js";import{n as se}from"./InputNumber-CyIzv-Z5.js";import{it as F,r as ce,rt as I,st as L}from"./index-lJkVLtdC.js";var R=/\s/;function z(e){for(var t=e.length;t--&&R.test(e.charAt(t)););return t}var B=/^\s+/;function le(e){return e&&e.slice(0,z(e)+1).replace(B,``)}var V=NaN,ue=/^[-+]0x[0-9a-f]+$/i,H=/^0b[01]+$/i,U=/^0o[0-7]+$/i,de=parseInt;function W(e){if(typeof e==`number`)return e;if(O(e))return V;if(E(e)){var t=typeof e.valueOf==`function`?e.valueOf():e;e=E(t)?t+``:t}if(typeof e!=`string`)return e===0?e:+e;e=le(e);var n=H.test(e);return n||U.test(e)?de(e.slice(2),n?2:8):ue.test(e)?V:+e}var G=function(){return k.Date.now()},K=`Expected a function`,q=Math.max,J=Math.min;function Y(e,t,n){var r,i,a,o,s,c,l=0,u=!1,d=!1,f=!0;if(typeof e!=`function`)throw TypeError(K);t=W(t)||0,E(n)&&(u=!!n.leading,d=`maxWait`in n,a=d?q(W(n.maxWait)||0,t):a,f=`trailing`in n?!!n.trailing:f);function p(t){var n=r,a=i;return r=i=void 0,l=t,o=e.apply(a,n),o}function m(e){return l=e,s=setTimeout(_,t),u?p(e):o}function h(e){var n=e-c,r=e-l,i=t-n;return d?J(i,a-r):i}function g(e){var n=e-c,r=e-l;return c===void 0||n>=t||n<0||d&&r>=a}function _(){var e=G();if(g(e))return v(e);s=setTimeout(_,h(e))}function v(e){return s=void 0,f&&r?p(e):(r=i=void 0,o)}function y(){s!==void 0&&clearTimeout(s),l=0,r=c=i=s=void 0}function b(){return s===void 0?o:v(G())}function x(){var e=G(),n=g(e);if(r=arguments,i=this,c=e,n){if(s===void 0)return m(c);if(d)return clearTimeout(s),s=setTimeout(_,t),p(c)}return s===void 0&&(s=setTimeout(_,t)),o}return x.cancel=y,x.flush=b,x}var X=`Expected a function`;function fe(e,t,n){var r=!0,i=!0;if(typeof e!=`function`)throw TypeError(X);return E(n)&&(r=`leading`in n?!!n.leading:r,i=`trailing`in n?!!n.trailing:i),Y(e,t,{leading:r,maxWait:t,trailing:i})}var pe=_(`n-tabs`),me={tab:[String,Number,Object,Function],name:{type:[String,Number],required:!0},disabled:Boolean,displayDirective:{type:String,default:`if`},closable:{type:Boolean,default:void 0},tabProps:Object,label:[String,Number,Object,Function]},Z=s({__TAB_PANE__:!0,name:`TabPane`,alias:[`TabPanel`],props:me,slots:Object,setup(t){let n=e(pe,null);return n||w(`tab-pane`,"`n-tab-pane` must be placed inside `n-tabs`."),{style:n.paneStyleRef,class:n.paneClassRef,mergedClsPrefix:n.mergedClsPrefixRef}},render(){return i(`div`,{class:[`${this.mergedClsPrefix}-tab-pane`,this.class],style:this.style},this.$slots)}}),Q=s({__TAB__:!0,inheritAttrs:!1,name:`Tab`,props:Object.assign({internalLeftPadded:Boolean,internalAddable:Boolean,internalCreatedByPane:Boolean},F(me,[`displayDirective`])),setup(t){let{mergedClsPrefixRef:n,valueRef:r,typeRef:i,closableRef:a,tabStyleRef:o,addTabStyleRef:s,tabClassRef:c,addTabClassRef:l,tabChangeIdRef:u,onBeforeLeaveRef:d,triggerRef:p,handleAdd:m,activateTab:h,handleClose:g}=e(pe);return{trigger:p,mergedClosable:f(()=>{if(t.internalAddable)return!1;let{closable:e}=t;return e===void 0?a.value:e}),style:o,addStyle:s,tabClass:c,addTabClass:l,clsPrefix:n,value:r,type:i,handleClose(e){e.stopPropagation(),!t.disabled&&g(t.name)},activateTab(){if(t.disabled)return;if(t.internalAddable){m();return}let{name:e}=t,n=++u.id;if(e!==r.value){let{value:i}=d;i?Promise.resolve(i(t.name,r.value)).then(t=>{t&&u.id===n&&h(e)}):h(e)}}}},render(){let{internalAddable:e,clsPrefix:t,name:r,disabled:a,label:o,tab:s,value:c,mergedClosable:l,trigger:d,$slots:{default:f}}=this,p=o??s;return i(`div`,{class:`${t}-tabs-tab-wrapper`},this.internalLeftPadded?i(`div`,{class:`${t}-tabs-tab-pad`}):null,i(`div`,Object.assign({key:r,"data-name":r,"data-disabled":a?!0:void 0},n({class:[`${t}-tabs-tab`,c===r&&`${t}-tabs-tab--active`,a&&`${t}-tabs-tab--disabled`,l&&`${t}-tabs-tab--closable`,e&&`${t}-tabs-tab--addable`,e?this.addTabClass:this.tabClass],onClick:d===`click`?this.activateTab:void 0,onMouseenter:d===`hover`?this.activateTab:void 0,style:e?this.addStyle:this.style},this.internalCreatedByPane?this.tabProps||{}:this.$attrs)),i(`span`,{class:`${t}-tabs-tab__label`},e?i(u,null,i(`div`,{class:`${t}-tabs-tab__height-placeholder`},`\xA0`),i(g,{clsPrefix:t},{default:()=>i(se,null)})):f?f():typeof p==`object`?p:I(p??r)),l&&this.type===`card`?i(h,{clsPrefix:t,class:`${t}-tabs-tab__close`,onClick:this.handleClose,disabled:a}):null))}}),he=S(`tabs`,`
 box-sizing: border-box;
 width: 100%;
 display: flex;
 flex-direction: column;
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
`,[b(`segment-type`,[S(`tabs-rail`,[x(`&.transition-disabled`,[S(`tabs-capsule`,`
 transition: none;
 `)])])]),b(`top`,[S(`tab-pane`,`
 padding: var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left);
 `)]),b(`left`,[S(`tab-pane`,`
 padding: var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left) var(--n-pane-padding-top);
 `)]),b(`left, right`,`
 flex-direction: row;
 `,[S(`tabs-bar`,`
 width: 2px;
 right: 0;
 transition:
 top .2s var(--n-bezier),
 max-height .2s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),S(`tabs-tab`,`
 padding: var(--n-tab-padding-vertical); 
 `)]),b(`right`,`
 flex-direction: row-reverse;
 `,[S(`tab-pane`,`
 padding: var(--n-pane-padding-left) var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom);
 `),S(`tabs-bar`,`
 left: 0;
 `)]),b(`bottom`,`
 flex-direction: column-reverse;
 justify-content: flex-end;
 `,[S(`tab-pane`,`
 padding: var(--n-pane-padding-bottom) var(--n-pane-padding-right) var(--n-pane-padding-top) var(--n-pane-padding-left);
 `),S(`tabs-bar`,`
 top: 0;
 `)]),S(`tabs-rail`,`
 position: relative;
 padding: 3px;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 background-color: var(--n-color-segment);
 transition: background-color .3s var(--n-bezier);
 display: flex;
 align-items: center;
 `,[S(`tabs-capsule`,`
 border-radius: var(--n-tab-border-radius);
 position: absolute;
 pointer-events: none;
 background-color: var(--n-tab-color-segment);
 box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .08);
 transition: transform 0.3s var(--n-bezier);
 `),S(`tabs-tab-wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[S(`tabs-tab`,`
 overflow: hidden;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[b(`active`,`
 font-weight: var(--n-font-weight-strong);
 color: var(--n-tab-text-color-active);
 `),x(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])])]),b(`flex`,[S(`tabs-nav`,`
 width: 100%;
 position: relative;
 `,[S(`tabs-wrapper`,`
 width: 100%;
 `,[S(`tabs-tab`,`
 margin-right: 0;
 `)])])]),S(`tabs-nav`,`
 box-sizing: border-box;
 line-height: 1.5;
 display: flex;
 transition: border-color .3s var(--n-bezier);
 `,[y(`prefix, suffix`,`
 display: flex;
 align-items: center;
 `),y(`prefix`,`padding-right: 16px;`),y(`suffix`,`padding-left: 16px;`)]),b(`top, bottom`,[x(`>`,[S(`tabs-nav`,[S(`tabs-nav-scroll-wrapper`,[x(`&::before`,`
 top: 0;
 bottom: 0;
 left: 0;
 width: 20px;
 `),x(`&::after`,`
 top: 0;
 bottom: 0;
 right: 0;
 width: 20px;
 `),b(`shadow-start`,[x(`&::before`,`
 box-shadow: inset 10px 0 8px -8px rgba(0, 0, 0, .12);
 `)]),b(`shadow-end`,[x(`&::after`,`
 box-shadow: inset -10px 0 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),b(`left, right`,[S(`tabs-nav-scroll-content`,`
 flex-direction: column;
 `),x(`>`,[S(`tabs-nav`,[S(`tabs-nav-scroll-wrapper`,[x(`&::before`,`
 top: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),x(`&::after`,`
 bottom: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),b(`shadow-start`,[x(`&::before`,`
 box-shadow: inset 0 10px 8px -8px rgba(0, 0, 0, .12);
 `)]),b(`shadow-end`,[x(`&::after`,`
 box-shadow: inset 0 -10px 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),S(`tabs-nav-scroll-wrapper`,`
 flex: 1;
 position: relative;
 overflow: hidden;
 `,[S(`tabs-nav-y-scroll`,`
 height: 100%;
 width: 100%;
 overflow-y: auto; 
 scrollbar-width: none;
 `,[x(`&::-webkit-scrollbar, &::-webkit-scrollbar-track-piece, &::-webkit-scrollbar-thumb`,`
 width: 0;
 height: 0;
 display: none;
 `)]),x(`&::before, &::after`,`
 transition: box-shadow .3s var(--n-bezier);
 pointer-events: none;
 content: "";
 position: absolute;
 z-index: 1;
 `)]),S(`tabs-nav-scroll-content`,`
 display: flex;
 position: relative;
 min-width: 100%;
 min-height: 100%;
 width: fit-content;
 box-sizing: border-box;
 `),S(`tabs-wrapper`,`
 display: inline-flex;
 flex-wrap: nowrap;
 position: relative;
 `),S(`tabs-tab-wrapper`,`
 display: flex;
 flex-wrap: nowrap;
 flex-shrink: 0;
 flex-grow: 0;
 `),S(`tabs-tab`,`
 cursor: pointer;
 white-space: nowrap;
 flex-wrap: nowrap;
 display: inline-flex;
 align-items: center;
 color: var(--n-tab-text-color);
 font-size: var(--n-tab-font-size);
 background-clip: padding-box;
 padding: var(--n-tab-padding);
 transition:
 box-shadow .3s var(--n-bezier),
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[b(`disabled`,{cursor:`not-allowed`}),y(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `),y(`label`,`
 display: flex;
 align-items: center;
 z-index: 1;
 `)]),S(`tabs-bar`,`
 position: absolute;
 bottom: 0;
 height: 2px;
 border-radius: 1px;
 background-color: var(--n-bar-color);
 transition:
 left .2s var(--n-bezier),
 max-width .2s var(--n-bezier),
 opacity .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `,[x(`&.transition-disabled`,`
 transition: none;
 `),b(`disabled`,`
 background-color: var(--n-tab-text-color-disabled)
 `)]),S(`tabs-pane-wrapper`,`
 position: relative;
 overflow: hidden;
 transition: max-height .2s var(--n-bezier);
 `),S(`tab-pane`,`
 color: var(--n-pane-text-color);
 width: 100%;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 opacity .2s var(--n-bezier);
 left: 0;
 right: 0;
 top: 0;
 `,[x(`&.next-transition-leave-active, &.prev-transition-leave-active, &.next-transition-enter-active, &.prev-transition-enter-active`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 transform .2s var(--n-bezier),
 opacity .2s var(--n-bezier);
 `),x(`&.next-transition-leave-active, &.prev-transition-leave-active`,`
 position: absolute;
 `),x(`&.next-transition-enter-from, &.prev-transition-leave-to`,`
 transform: translateX(32px);
 opacity: 0;
 `),x(`&.next-transition-leave-to, &.prev-transition-enter-from`,`
 transform: translateX(-32px);
 opacity: 0;
 `),x(`&.next-transition-leave-from, &.next-transition-enter-to, &.prev-transition-leave-from, &.prev-transition-enter-to`,`
 transform: translateX(0);
 opacity: 1;
 `)]),S(`tabs-tab-pad`,`
 box-sizing: border-box;
 width: var(--n-tab-gap);
 flex-grow: 0;
 flex-shrink: 0;
 `),b(`line-type, bar-type`,[S(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 box-sizing: border-box;
 vertical-align: bottom;
 `,[x(`&:hover`,{color:`var(--n-tab-text-color-hover)`}),b(`active`,`
 color: var(--n-tab-text-color-active);
 font-weight: var(--n-tab-font-weight-active);
 `),b(`disabled`,{color:`var(--n-tab-text-color-disabled)`})])]),S(`tabs-nav`,[b(`line-type`,[b(`top`,[y(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),S(`tabs-nav-scroll-content`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),S(`tabs-bar`,`
 bottom: -1px;
 `)]),b(`left`,[y(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),S(`tabs-nav-scroll-content`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),S(`tabs-bar`,`
 right: -1px;
 `)]),b(`right`,[y(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),S(`tabs-nav-scroll-content`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),S(`tabs-bar`,`
 left: -1px;
 `)]),b(`bottom`,[y(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),S(`tabs-nav-scroll-content`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),S(`tabs-bar`,`
 top: -1px;
 `)]),y(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),S(`tabs-nav-scroll-content`,`
 transition: border-color .3s var(--n-bezier);
 `),S(`tabs-bar`,`
 border-radius: 0;
 `)]),b(`card-type`,[y(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),S(`tabs-pad`,`
 flex-grow: 1;
 transition: border-color .3s var(--n-bezier);
 `),S(`tabs-tab-pad`,`
 transition: border-color .3s var(--n-bezier);
 `),S(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 border: 1px solid var(--n-tab-border-color);
 background-color: var(--n-tab-color);
 box-sizing: border-box;
 position: relative;
 vertical-align: bottom;
 display: flex;
 justify-content: space-between;
 font-size: var(--n-tab-font-size);
 color: var(--n-tab-text-color);
 `,[b(`addable`,`
 padding-left: 8px;
 padding-right: 8px;
 font-size: 16px;
 justify-content: center;
 `,[y(`height-placeholder`,`
 width: 0;
 font-size: var(--n-tab-font-size);
 `),C(`disabled`,[x(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])]),b(`closable`,`padding-right: 8px;`),b(`active`,`
 background-color: #0000;
 font-weight: var(--n-tab-font-weight-active);
 color: var(--n-tab-text-color-active);
 `),b(`disabled`,`color: var(--n-tab-text-color-disabled);`)])]),b(`left, right`,`
 flex-direction: column; 
 `,[y(`prefix, suffix`,`
 padding: var(--n-tab-padding-vertical);
 `),S(`tabs-wrapper`,`
 flex-direction: column;
 `),S(`tabs-tab-wrapper`,`
 flex-direction: column;
 `,[S(`tabs-tab-pad`,`
 height: var(--n-tab-gap-vertical);
 width: 100%;
 `)])]),b(`top`,[b(`card-type`,[S(`tabs-scroll-padding`,`border-bottom: 1px solid var(--n-tab-border-color);`),y(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),S(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-top-right-radius: var(--n-tab-border-radius);
 `,[b(`active`,`
 border-bottom: 1px solid #0000;
 `)]),S(`tabs-tab-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),S(`tabs-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `)])]),b(`left`,[b(`card-type`,[S(`tabs-scroll-padding`,`border-right: 1px solid var(--n-tab-border-color);`),y(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),S(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-bottom-left-radius: var(--n-tab-border-radius);
 `,[b(`active`,`
 border-right: 1px solid #0000;
 `)]),S(`tabs-tab-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),S(`tabs-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `)])]),b(`right`,[b(`card-type`,[S(`tabs-scroll-padding`,`border-left: 1px solid var(--n-tab-border-color);`),y(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),S(`tabs-tab`,`
 border-top-right-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[b(`active`,`
 border-left: 1px solid #0000;
 `)]),S(`tabs-tab-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),S(`tabs-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `)])]),b(`bottom`,[b(`card-type`,[S(`tabs-scroll-padding`,`border-top: 1px solid var(--n-tab-border-color);`),y(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),S(`tabs-tab`,`
 border-bottom-left-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[b(`active`,`
 border-top: 1px solid #0000;
 `)]),S(`tabs-tab-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),S(`tabs-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `)])])])]),ge=fe,_e=s({name:`Tabs`,props:Object.assign(Object.assign({},D.props),{value:[String,Number],defaultValue:[String,Number],trigger:{type:String,default:`click`},type:{type:String,default:`bar`},closable:Boolean,justifyContent:String,size:String,placement:{type:String,default:`top`},tabStyle:[String,Object],tabClass:String,addTabStyle:[String,Object],addTabClass:String,barWidth:Number,paneClass:String,paneStyle:[String,Object],paneWrapperClass:String,paneWrapperStyle:[String,Object],addable:[Boolean,Object],tabsPadding:{type:Number,default:0},animated:Boolean,onBeforeLeave:Function,onAdd:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onClose:[Function,Array],labelSize:String,activeName:[String,Number],onActiveNameChange:[Function,Array]}),slots:Object,setup(e,{slots:n}){let{mergedClsPrefixRef:i,inlineThemeDisabled:a,mergedComponentPropsRef:s}=ee(e),u=D(`Tabs`,`-tabs`,he,ce,e,i),p=m(null),h=m(null),g=m(null),_=m(null),v=m(null),y=m(null),b=m(!0),x=m(!0),S=oe(e,[`labelSize`,`size`]),C=f(()=>S.value?S.value:s?.value?.Tabs?.size||`medium`),w=oe(e,[`activeName`,`value`]),T=m(w.value??e.defaultValue??(n.default?L(n.default())[0]?.props?.name:null)),E=ae(w,T),O={id:0},k=f(()=>{if(!(!e.justifyContent||e.type===`card`))return{display:`flex`,justifyContent:e.justifyContent}});l(E,()=>{O.id=0,F(),I()});function j(){let{value:e}=E;return e===null?null:p.value?.querySelector(`[data-name="${e}"]`)}function P(t){if(e.type===`card`)return;let{value:n}=h;if(!n)return;let r=n.style.opacity===`0`;if(t){let a=`${i.value}-tabs-bar--disabled`,{barWidth:o,placement:s}=e;if(t.dataset.disabled===`true`?n.classList.add(a):n.classList.remove(a),[`top`,`bottom`].includes(s)){if(se([`top`,`maxHeight`,`height`]),typeof o==`number`&&t.offsetWidth>=o){let e=Math.floor((t.offsetWidth-o)/2)+t.offsetLeft;n.style.left=`${e}px`,n.style.maxWidth=`${o}px`}else n.style.left=`${t.offsetLeft}px`,n.style.maxWidth=`${t.offsetWidth}px`;n.style.width=`8192px`,r&&(n.style.transition=`none`),n.offsetWidth,r&&(n.style.transition=``,n.style.opacity=`1`)}else{if(se([`left`,`maxWidth`,`width`]),typeof o==`number`&&t.offsetHeight>=o){let e=Math.floor((t.offsetHeight-o)/2)+t.offsetTop;n.style.top=`${e}px`,n.style.maxHeight=`${o}px`}else n.style.top=`${t.offsetTop}px`,n.style.maxHeight=`${t.offsetHeight}px`;n.style.height=`8192px`,r&&(n.style.transition=`none`),n.offsetHeight,r&&(n.style.transition=``,n.style.opacity=`1`)}}}function ie(){if(e.type===`card`)return;let{value:t}=h;t&&(t.style.opacity=`0`)}function se(e){let{value:t}=h;if(t)for(let n of e)t.style[n]=``}function F(){if(e.type===`card`)return;let t=j();t?P(t):ie()}function I(){let e=v.value?.$el;if(!e)return;let t=j();if(!t)return;let{scrollLeft:n,offsetWidth:r}=e,{offsetLeft:i,offsetWidth:a}=t;n>i?e.scrollTo({top:0,left:i,behavior:`smooth`}):i+a>n+r&&e.scrollTo({top:0,left:i+a-r,behavior:`smooth`})}let R=m(null),z=0,B=null;function le(e){let t=R.value;if(t){z=e.getBoundingClientRect().height;let n=`${z}px`,r=()=>{t.style.height=n,t.style.maxHeight=n};B?(r(),B(),B=null):B=r}}function V(e){let t=R.value;if(t){let n=e.getBoundingClientRect().height,r=()=>{document.body.offsetHeight,t.style.maxHeight=`${n}px`,t.style.height=`${Math.max(z,n)}px`};B?(B(),B=null,r()):B=r}}function ue(){let t=R.value;if(t){t.style.maxHeight=``,t.style.height=``;let{paneWrapperStyle:n}=e;if(typeof n==`string`)t.style.cssText=n;else if(n){let{maxHeight:e,height:r}=n;e!==void 0&&(t.style.maxHeight=e),r!==void 0&&(t.style.height=r)}}}let H={value:[]},U=m(`next`);function de(e){let t=E.value,n=`next`;for(let r of H.value){if(r===t)break;if(r===e){n=`prev`;break}}U.value=n,W(e)}function W(t){let{onActiveNameChange:n,onUpdateValue:r,"onUpdate:value":i}=e;n&&A(n,t),r&&A(r,t),i&&A(i,t),T.value=t}function G(t){let{onClose:n}=e;n&&A(n,t)}let K=!0;function q(){let{value:e}=h;if(!e)return;K||=!1;let t=`transition-disabled`;e.classList.add(t),F(),e.classList.remove(t)}let J=m(null);function Y({transitionDisabled:e}){let t=p.value;if(!t)return;e&&t.classList.add(`transition-disabled`);let n=j();n&&J.value&&(J.value.style.width=`${n.offsetWidth}px`,J.value.style.height=`${n.offsetHeight}px`,J.value.style.transform=`translateX(${n.offsetLeft-ne(getComputedStyle(t).paddingLeft)}px)`,e&&J.value.offsetWidth),e&&t.classList.remove(`transition-disabled`)}l([E],()=>{e.type===`segment`&&r(()=>{Y({transitionDisabled:!1})})}),o(()=>{e.type===`segment`&&Y({transitionDisabled:!0})});let X=0;function fe(t){if(t.contentRect.width===0&&t.contentRect.height===0||X===t.contentRect.width)return;X=t.contentRect.width;let{type:n}=e;if((n===`line`||n===`bar`)&&(K||e.justifyContent?.startsWith(`space`))&&q(),n!==`segment`){let{placement:t}=e;$((t===`top`||t===`bottom`?v.value?.$el:y.value)||null)}}let me=ge(fe,64);l([()=>e.justifyContent,()=>e.size],()=>{r(()=>{let{type:t}=e;(t===`line`||t===`bar`)&&q()})});let Z=m(!1);function Q(t){let{target:n,contentRect:{width:r,height:i}}=t,a=n.parentElement.parentElement.offsetWidth,o=n.parentElement.parentElement.offsetHeight,{placement:s}=e;if(!Z.value)s===`top`||s===`bottom`?a<r&&(Z.value=!0):o<i&&(Z.value=!0);else{let{value:e}=_;if(!e)return;s===`top`||s===`bottom`?a-r>e.$el.offsetWidth&&(Z.value=!1):o-i>e.$el.offsetHeight&&(Z.value=!1)}$(v.value?.$el||null)}let _e=ge(Q,64);function ve(){let{onAdd:t}=e;t&&t(),r(()=>{let e=j(),{value:t}=v;!e||!t||t.scrollTo({left:e.offsetLeft,top:0,behavior:`smooth`})})}function $(t){if(!t)return;let{placement:n}=e;if(n===`top`||n===`bottom`){let{scrollLeft:e,scrollWidth:n,offsetWidth:r}=t;b.value=e<=0,x.value=e+r>=n}else{let{scrollTop:e,scrollHeight:n,offsetHeight:r}=t;b.value=e<=0,x.value=e+r>=n}}let ye=ge(e=>{$(e.target)},64);t(pe,{triggerRef:d(e,`trigger`),tabStyleRef:d(e,`tabStyle`),tabClassRef:d(e,`tabClass`),addTabStyleRef:d(e,`addTabStyle`),addTabClassRef:d(e,`addTabClass`),paneClassRef:d(e,`paneClass`),paneStyleRef:d(e,`paneStyle`),mergedClsPrefixRef:i,typeRef:d(e,`type`),closableRef:d(e,`closable`),valueRef:E,tabChangeIdRef:O,onBeforeLeaveRef:d(e,`onBeforeLeave`),activateTab:de,handleClose:G,handleAdd:ve}),re(()=>{F(),I()}),c(()=>{let{value:e}=g;if(!e)return;let{value:t}=i,n=`${t}-tabs-nav-scroll-wrapper--shadow-start`,r=`${t}-tabs-nav-scroll-wrapper--shadow-end`;b.value?e.classList.remove(n):e.classList.add(n),x.value?e.classList.remove(r):e.classList.add(r)});let be={syncBarPosition:()=>{F()}},xe=()=>{Y({transitionDisabled:!0})},Se=f(()=>{let{value:t}=C,{type:n}=e,r=`${t}${{card:`Card`,bar:`Bar`,line:`Line`,segment:`Segment`}[n]}`,{self:{barColor:i,closeIconColor:a,closeIconColorHover:o,closeIconColorPressed:s,tabColor:c,tabBorderColor:l,paneTextColor:d,tabFontWeight:f,tabBorderRadius:p,tabFontWeightActive:m,colorSegment:h,fontWeightStrong:g,tabColorSegment:_,closeSize:v,closeIconSize:y,closeColorHover:b,closeColorPressed:x,closeBorderRadius:S,[M(`panePadding`,t)]:w,[M(`tabPadding`,r)]:T,[M(`tabPaddingVertical`,r)]:ee,[M(`tabGap`,r)]:E,[M(`tabGap`,`${r}Vertical`)]:D,[M(`tabTextColor`,n)]:O,[M(`tabTextColorActive`,n)]:k,[M(`tabTextColorHover`,n)]:te,[M(`tabTextColorDisabled`,n)]:A,[M(`tabFontSize`,t)]:j},common:{cubicBezierEaseInOut:ne}}=u.value;return{"--n-bezier":ne,"--n-color-segment":h,"--n-bar-color":i,"--n-tab-font-size":j,"--n-tab-text-color":O,"--n-tab-text-color-active":k,"--n-tab-text-color-disabled":A,"--n-tab-text-color-hover":te,"--n-pane-text-color":d,"--n-tab-border-color":l,"--n-tab-border-radius":p,"--n-close-size":v,"--n-close-icon-size":y,"--n-close-color-hover":b,"--n-close-color-pressed":x,"--n-close-border-radius":S,"--n-close-icon-color":a,"--n-close-icon-color-hover":o,"--n-close-icon-color-pressed":s,"--n-tab-color":c,"--n-tab-font-weight":f,"--n-tab-font-weight-active":m,"--n-tab-padding":T,"--n-tab-padding-vertical":ee,"--n-tab-gap":E,"--n-tab-gap-vertical":D,"--n-pane-padding-left":N(w,`left`),"--n-pane-padding-right":N(w,`right`),"--n-pane-padding-top":N(w,`top`),"--n-pane-padding-bottom":N(w,`bottom`),"--n-font-weight-strong":g,"--n-tab-color-segment":_}}),Ce=a?te(`tabs`,f(()=>`${C.value[0]}${e.type[0]}`),Se,e):void 0;return Object.assign({mergedClsPrefix:i,mergedValue:E,renderedNames:new Set,segmentCapsuleElRef:J,tabsPaneWrapperRef:R,tabsElRef:p,barElRef:h,addTabInstRef:_,xScrollInstRef:v,scrollWrapperElRef:g,addTabFixed:Z,tabWrapperStyle:k,handleNavResize:me,mergedSize:C,handleScroll:ye,handleTabsResize:_e,cssVars:a?void 0:Se,themeClass:Ce?.themeClass,animationDirection:U,renderNameListRef:H,yScrollElRef:y,handleSegmentResize:xe,onAnimationBeforeLeave:le,onAnimationEnter:V,onAnimationAfterEnter:ue,onRender:Ce?.onRender},be)},render(){let{mergedClsPrefix:e,type:t,placement:n,addTabFixed:r,addable:a,mergedSize:o,renderNameListRef:s,onRender:c,paneWrapperClass:l,paneWrapperStyle:u,$slots:{default:d,prefix:f,suffix:p}}=this;c?.();let m=d?L(d()).filter(e=>e.type.__TAB_PANE__===!0):[],h=d?L(d()).filter(e=>e.type.__TAB__===!0):[],g=!h.length,_=t===`card`,v=t===`segment`,y=!_&&!v&&this.justifyContent;s.value=[];let b=()=>{let t=i(`div`,{style:this.tabWrapperStyle,class:`${e}-tabs-wrapper`},y?null:i(`div`,{class:`${e}-tabs-scroll-padding`,style:n===`top`||n===`bottom`?{width:`${this.tabsPadding}px`}:{height:`${this.tabsPadding}px`}}),g?m.map((e,t)=>(s.value.push(e.props.name),be(i(Q,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0&&(!y||y===`center`||y===`start`||y===`end`)}),e.children?{default:e.children.tab}:void 0)))):h.map((e,t)=>(s.value.push(e.props.name),be(t!==0&&!y?ye(e):e))),!r&&a&&_?$(a,(g?m.length:h.length)!==0):null,y?null:i(`div`,{class:`${e}-tabs-scroll-padding`,style:{width:`${this.tabsPadding}px`}}));return i(`div`,{ref:`tabsElRef`,class:`${e}-tabs-nav-scroll-content`},_&&a?i(P,{onResize:this.handleTabsResize},{default:()=>t}):t,_?i(`div`,{class:`${e}-tabs-pad`}):null,_?null:i(`div`,{ref:`barElRef`,class:`${e}-tabs-bar`}))},x=v?`top`:n;return i(`div`,{class:[`${e}-tabs`,this.themeClass,`${e}-tabs--${t}-type`,`${e}-tabs--${o}-size`,y&&`${e}-tabs--flex`,`${e}-tabs--${x}`],style:this.cssVars},i(`div`,{class:[`${e}-tabs-nav--${t}-type`,`${e}-tabs-nav--${x}`,`${e}-tabs-nav`]},j(f,t=>t&&i(`div`,{class:`${e}-tabs-nav__prefix`},t)),v?i(P,{onResize:this.handleSegmentResize},{default:()=>i(`div`,{class:`${e}-tabs-rail`,ref:`tabsElRef`},i(`div`,{class:`${e}-tabs-capsule`,ref:`segmentCapsuleElRef`},i(`div`,{class:`${e}-tabs-wrapper`},i(`div`,{class:`${e}-tabs-tab`}))),g?m.map((e,t)=>(s.value.push(e.props.name),i(Q,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0}),e.children?{default:e.children.tab}:void 0))):h.map((e,t)=>(s.value.push(e.props.name),t===0?e:ye(e))))}):i(P,{onResize:this.handleNavResize},{default:()=>i(`div`,{class:`${e}-tabs-nav-scroll-wrapper`,ref:`scrollWrapperElRef`},[`top`,`bottom`].includes(x)?i(ie,{ref:`xScrollInstRef`,onScroll:this.handleScroll},{default:b}):i(`div`,{class:`${e}-tabs-nav-y-scroll`,onScroll:this.handleScroll,ref:`yScrollElRef`},b()))}),r&&a&&_?$(a,!0):null,j(p,t=>t&&i(`div`,{class:`${e}-tabs-nav__suffix`},t))),g&&(this.animated&&(x===`top`||x===`bottom`)?i(`div`,{ref:`tabsPaneWrapperRef`,style:u,class:[`${e}-tabs-pane-wrapper`,l]},ve(m,this.mergedValue,this.renderedNames,this.onAnimationBeforeLeave,this.onAnimationEnter,this.onAnimationAfterEnter,this.animationDirection)):ve(m,this.mergedValue,this.renderedNames)))}});function ve(e,t,n,r,o,s,c){let l=[];return e.forEach(e=>{let{name:r,displayDirective:i,"display-directive":o}=e.props,s=e=>i===e||o===e,c=t===r;if(e.key!==void 0&&(e.key=r),c||s(`show`)||s(`show:lazy`)&&n.has(r)){n.has(r)||n.add(r);let t=!s(`if`);l.push(t?a(e,[[v,c]]):e)}}),c?i(T,{name:`${c}-transition`,onBeforeLeave:r,onEnter:o,onAfterEnter:s},{default:()=>l}):l}function $(e,t){return i(Q,{ref:`addTabInstRef`,key:`__addable`,name:`__addable`,internalCreatedByPane:!0,internalAddable:!0,internalLeftPadded:t,disabled:typeof e==`object`&&e.disabled})}function ye(e){let t=p(e);return t.props?t.props.internalLeftPadded=!0:t.props={internalLeftPadded:!0},t}function be(e){return Array.isArray(e.dynamicProps)?e.dynamicProps.includes(`internalLeftPadded`)||e.dynamicProps.push(`internalLeftPadded`):e.dynamicProps=[`internalLeftPadded`],e}export{Z as n,_e as t};