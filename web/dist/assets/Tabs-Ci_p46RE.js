import{A as e,B as t,D as n,F as r,M as i,P as a,Q as o,W as s,Z as c,_ as l,et as u,f as d,g as f,lt as p,pt as m}from"./echarts-Bmuv2i_G.js";import{B as h,H as g,M as _,T as v,Y as y,q as b}from"./auth-BTeG-_sB.js";import{A as x,O as S,b as ee,d as C,l as w}from"./vue-core-bFnGnCI4.js";import{D as T,G as E,J as D,K as O,L as k,M as A,N as te,P as ne,X as re,Y as j,Z as M,i as ie,n as ae,z as oe}from"./light-B4Ik6WNX.js";import{c as se}from"./_plugin-vue_export-helper-BHHysIWo.js";import{t as ce}from"./use-compitable-BMUbnpEM.js";import{n as N}from"./InputNumber-BlEi_M1y.js";import{it as P,r as le,rt as F,st as I}from"./index-Yh_aUYzb.js";var L=/\s/;function R(e){for(var t=e.length;t--&&L.test(e.charAt(t)););return t}var z=/^\s+/;function ue(e){return e&&e.slice(0,R(e)+1).replace(z,``)}var B=NaN,de=/^[-+]0x[0-9a-f]+$/i,V=/^0b[01]+$/i,H=/^0o[0-7]+$/i,fe=parseInt;function U(e){if(typeof e==`number`)return e;if(_(e))return B;if(T(e)){var t=typeof e.valueOf==`function`?e.valueOf():e;e=T(t)?t+``:t}if(typeof e!=`string`)return e===0?e:+e;e=ue(e);var n=V.test(e);return n||H.test(e)?fe(e.slice(2),n?2:8):de.test(e)?B:+e}var W=function(){return A.Date.now()},G=`Expected a function`,K=Math.max,q=Math.min;function J(e,t,n){var r,i,a,o,s,c,l=0,u=!1,d=!1,f=!0;if(typeof e!=`function`)throw TypeError(G);t=U(t)||0,T(n)&&(u=!!n.leading,d=`maxWait`in n,a=d?K(U(n.maxWait)||0,t):a,f=`trailing`in n?!!n.trailing:f);function p(t){var n=r,a=i;return r=i=void 0,l=t,o=e.apply(a,n),o}function m(e){return l=e,s=setTimeout(_,t),u?p(e):o}function h(e){var n=e-c,r=e-l,i=t-n;return d?q(i,a-r):i}function g(e){var n=e-c,r=e-l;return c===void 0||n>=t||n<0||d&&r>=a}function _(){var e=W();if(g(e))return v(e);s=setTimeout(_,h(e))}function v(e){return s=void 0,f&&r?p(e):(r=i=void 0,o)}function y(){s!==void 0&&clearTimeout(s),l=0,r=c=i=s=void 0}function b(){return s===void 0?o:v(W())}function x(){var e=W(),n=g(e);if(r=arguments,i=this,c=e,n){if(s===void 0)return m(c);if(d)return clearTimeout(s),s=setTimeout(_,t),p(c)}return s===void 0&&(s=setTimeout(_,t)),o}return x.cancel=y,x.flush=b,x}var Y=`Expected a function`;function pe(e,t,n){var r=!0,i=!0;if(typeof e!=`function`)throw TypeError(Y);return T(n)&&(r=`leading`in n?!!n.leading:r,i=`trailing`in n?!!n.trailing:i),J(e,t,{leading:r,maxWait:t,trailing:i})}var me=oe(`n-tabs`),he={tab:[String,Number,Object,Function],name:{type:[String,Number],required:!0},disabled:Boolean,displayDirective:{type:String,default:`if`},closable:{type:Boolean,default:void 0},tabProps:Object,label:[String,Number,Object,Function]},X=n({__TAB_PANE__:!0,name:`TabPane`,alias:[`TabPanel`],props:he,slots:Object,setup(e){let t=i(me,null);return t||k(`tab-pane`,"`n-tab-pane` must be placed inside `n-tabs`."),{style:t.paneStyleRef,class:t.paneClassRef,mergedClsPrefix:t.mergedClsPrefixRef}},render(){return e(`div`,{class:[`${this.mergedClsPrefix}-tab-pane`,this.class],style:this.style},this.$slots)}}),Z=n({__TAB__:!0,inheritAttrs:!1,name:`Tab`,props:Object.assign({internalLeftPadded:Boolean,internalAddable:Boolean,internalCreatedByPane:Boolean},P(he,[`displayDirective`])),setup(e){let{mergedClsPrefixRef:t,valueRef:n,typeRef:r,closableRef:a,tabStyleRef:o,addTabStyleRef:s,tabClassRef:c,addTabClassRef:u,tabChangeIdRef:d,onBeforeLeaveRef:f,triggerRef:p,handleAdd:m,activateTab:h,handleClose:g}=i(me);return{trigger:p,mergedClosable:l(()=>{if(e.internalAddable)return!1;let{closable:t}=e;return t===void 0?a.value:t}),style:o,addStyle:s,tabClass:c,addTabClass:u,clsPrefix:t,value:n,type:r,handleClose(t){t.stopPropagation(),!e.disabled&&g(e.name)},activateTab(){if(e.disabled)return;if(e.internalAddable){m();return}let{name:t}=e,r=++d.id;if(t!==n.value){let{value:i}=f;i?Promise.resolve(i(e.name,n.value)).then(e=>{e&&d.id===r&&h(t)}):h(t)}}}},render(){let{internalAddable:t,clsPrefix:n,name:r,disabled:i,label:o,tab:s,value:c,mergedClosable:l,trigger:u,$slots:{default:f}}=this,p=o??s;return e(`div`,{class:`${n}-tabs-tab-wrapper`},this.internalLeftPadded?e(`div`,{class:`${n}-tabs-tab-pad`}):null,e(`div`,Object.assign({key:r,"data-name":r,"data-disabled":i?!0:void 0},a({class:[`${n}-tabs-tab`,c===r&&`${n}-tabs-tab--active`,i&&`${n}-tabs-tab--disabled`,l&&`${n}-tabs-tab--closable`,t&&`${n}-tabs-tab--addable`,t?this.addTabClass:this.tabClass],onClick:u===`click`?this.activateTab:void 0,onMouseenter:u===`hover`?this.activateTab:void 0,style:t?this.addStyle:this.style},this.internalCreatedByPane?this.tabProps||{}:this.$attrs)),e(`span`,{class:`${n}-tabs-tab__label`},t?e(d,null,e(`div`,{class:`${n}-tabs-tab__height-placeholder`},`\xA0`),e(ae,{clsPrefix:n},{default:()=>e(N,null)})):f?f():typeof p==`object`?p:F(p??r)),l&&this.type===`card`?e(v,{clsPrefix:n,class:`${n}-tabs-tab__close`,onClick:this.handleClose,disabled:i}):null))}}),ge=O(`tabs`,`
 box-sizing: border-box;
 width: 100%;
 display: flex;
 flex-direction: column;
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
`,[j(`segment-type`,[O(`tabs-rail`,[E(`&.transition-disabled`,[O(`tabs-capsule`,`
 transition: none;
 `)])])]),j(`top`,[O(`tab-pane`,`
 padding: var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left);
 `)]),j(`left`,[O(`tab-pane`,`
 padding: var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left) var(--n-pane-padding-top);
 `)]),j(`left, right`,`
 flex-direction: row;
 `,[O(`tabs-bar`,`
 width: 2px;
 right: 0;
 transition:
 top .2s var(--n-bezier),
 max-height .2s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),O(`tabs-tab`,`
 padding: var(--n-tab-padding-vertical); 
 `)]),j(`right`,`
 flex-direction: row-reverse;
 `,[O(`tab-pane`,`
 padding: var(--n-pane-padding-left) var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom);
 `),O(`tabs-bar`,`
 left: 0;
 `)]),j(`bottom`,`
 flex-direction: column-reverse;
 justify-content: flex-end;
 `,[O(`tab-pane`,`
 padding: var(--n-pane-padding-bottom) var(--n-pane-padding-right) var(--n-pane-padding-top) var(--n-pane-padding-left);
 `),O(`tabs-bar`,`
 top: 0;
 `)]),O(`tabs-rail`,`
 position: relative;
 padding: 3px;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 background-color: var(--n-color-segment);
 transition: background-color .3s var(--n-bezier);
 display: flex;
 align-items: center;
 `,[O(`tabs-capsule`,`
 border-radius: var(--n-tab-border-radius);
 position: absolute;
 pointer-events: none;
 background-color: var(--n-tab-color-segment);
 box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .08);
 transition: transform 0.3s var(--n-bezier);
 `),O(`tabs-tab-wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[O(`tabs-tab`,`
 overflow: hidden;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[j(`active`,`
 font-weight: var(--n-font-weight-strong);
 color: var(--n-tab-text-color-active);
 `),E(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])])]),j(`flex`,[O(`tabs-nav`,`
 width: 100%;
 position: relative;
 `,[O(`tabs-wrapper`,`
 width: 100%;
 `,[O(`tabs-tab`,`
 margin-right: 0;
 `)])])]),O(`tabs-nav`,`
 box-sizing: border-box;
 line-height: 1.5;
 display: flex;
 transition: border-color .3s var(--n-bezier);
 `,[D(`prefix, suffix`,`
 display: flex;
 align-items: center;
 `),D(`prefix`,`padding-right: 16px;`),D(`suffix`,`padding-left: 16px;`)]),j(`top, bottom`,[E(`>`,[O(`tabs-nav`,[O(`tabs-nav-scroll-wrapper`,[E(`&::before`,`
 top: 0;
 bottom: 0;
 left: 0;
 width: 20px;
 `),E(`&::after`,`
 top: 0;
 bottom: 0;
 right: 0;
 width: 20px;
 `),j(`shadow-start`,[E(`&::before`,`
 box-shadow: inset 10px 0 8px -8px rgba(0, 0, 0, .12);
 `)]),j(`shadow-end`,[E(`&::after`,`
 box-shadow: inset -10px 0 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),j(`left, right`,[O(`tabs-nav-scroll-content`,`
 flex-direction: column;
 `),E(`>`,[O(`tabs-nav`,[O(`tabs-nav-scroll-wrapper`,[E(`&::before`,`
 top: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),E(`&::after`,`
 bottom: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),j(`shadow-start`,[E(`&::before`,`
 box-shadow: inset 0 10px 8px -8px rgba(0, 0, 0, .12);
 `)]),j(`shadow-end`,[E(`&::after`,`
 box-shadow: inset 0 -10px 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),O(`tabs-nav-scroll-wrapper`,`
 flex: 1;
 position: relative;
 overflow: hidden;
 `,[O(`tabs-nav-y-scroll`,`
 height: 100%;
 width: 100%;
 overflow-y: auto; 
 scrollbar-width: none;
 `,[E(`&::-webkit-scrollbar, &::-webkit-scrollbar-track-piece, &::-webkit-scrollbar-thumb`,`
 width: 0;
 height: 0;
 display: none;
 `)]),E(`&::before, &::after`,`
 transition: box-shadow .3s var(--n-bezier);
 pointer-events: none;
 content: "";
 position: absolute;
 z-index: 1;
 `)]),O(`tabs-nav-scroll-content`,`
 display: flex;
 position: relative;
 min-width: 100%;
 min-height: 100%;
 width: fit-content;
 box-sizing: border-box;
 `),O(`tabs-wrapper`,`
 display: inline-flex;
 flex-wrap: nowrap;
 position: relative;
 `),O(`tabs-tab-wrapper`,`
 display: flex;
 flex-wrap: nowrap;
 flex-shrink: 0;
 flex-grow: 0;
 `),O(`tabs-tab`,`
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
 `,[j(`disabled`,{cursor:`not-allowed`}),D(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `),D(`label`,`
 display: flex;
 align-items: center;
 z-index: 1;
 `)]),O(`tabs-bar`,`
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
 `,[E(`&.transition-disabled`,`
 transition: none;
 `),j(`disabled`,`
 background-color: var(--n-tab-text-color-disabled)
 `)]),O(`tabs-pane-wrapper`,`
 position: relative;
 overflow: hidden;
 transition: max-height .2s var(--n-bezier);
 `),O(`tab-pane`,`
 color: var(--n-pane-text-color);
 width: 100%;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 opacity .2s var(--n-bezier);
 left: 0;
 right: 0;
 top: 0;
 `,[E(`&.next-transition-leave-active, &.prev-transition-leave-active, &.next-transition-enter-active, &.prev-transition-enter-active`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 transform .2s var(--n-bezier),
 opacity .2s var(--n-bezier);
 `),E(`&.next-transition-leave-active, &.prev-transition-leave-active`,`
 position: absolute;
 `),E(`&.next-transition-enter-from, &.prev-transition-leave-to`,`
 transform: translateX(32px);
 opacity: 0;
 `),E(`&.next-transition-leave-to, &.prev-transition-enter-from`,`
 transform: translateX(-32px);
 opacity: 0;
 `),E(`&.next-transition-leave-from, &.next-transition-enter-to, &.prev-transition-leave-from, &.prev-transition-enter-to`,`
 transform: translateX(0);
 opacity: 1;
 `)]),O(`tabs-tab-pad`,`
 box-sizing: border-box;
 width: var(--n-tab-gap);
 flex-grow: 0;
 flex-shrink: 0;
 `),j(`line-type, bar-type`,[O(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 box-sizing: border-box;
 vertical-align: bottom;
 `,[E(`&:hover`,{color:`var(--n-tab-text-color-hover)`}),j(`active`,`
 color: var(--n-tab-text-color-active);
 font-weight: var(--n-tab-font-weight-active);
 `),j(`disabled`,{color:`var(--n-tab-text-color-disabled)`})])]),O(`tabs-nav`,[j(`line-type`,[j(`top`,[D(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),O(`tabs-nav-scroll-content`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),O(`tabs-bar`,`
 bottom: -1px;
 `)]),j(`left`,[D(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),O(`tabs-nav-scroll-content`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),O(`tabs-bar`,`
 right: -1px;
 `)]),j(`right`,[D(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),O(`tabs-nav-scroll-content`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),O(`tabs-bar`,`
 left: -1px;
 `)]),j(`bottom`,[D(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),O(`tabs-nav-scroll-content`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),O(`tabs-bar`,`
 top: -1px;
 `)]),D(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),O(`tabs-nav-scroll-content`,`
 transition: border-color .3s var(--n-bezier);
 `),O(`tabs-bar`,`
 border-radius: 0;
 `)]),j(`card-type`,[D(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),O(`tabs-pad`,`
 flex-grow: 1;
 transition: border-color .3s var(--n-bezier);
 `),O(`tabs-tab-pad`,`
 transition: border-color .3s var(--n-bezier);
 `),O(`tabs-tab`,`
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
 `,[j(`addable`,`
 padding-left: 8px;
 padding-right: 8px;
 font-size: 16px;
 justify-content: center;
 `,[D(`height-placeholder`,`
 width: 0;
 font-size: var(--n-tab-font-size);
 `),re(`disabled`,[E(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])]),j(`closable`,`padding-right: 8px;`),j(`active`,`
 background-color: #0000;
 font-weight: var(--n-tab-font-weight-active);
 color: var(--n-tab-text-color-active);
 `),j(`disabled`,`color: var(--n-tab-text-color-disabled);`)])]),j(`left, right`,`
 flex-direction: column; 
 `,[D(`prefix, suffix`,`
 padding: var(--n-tab-padding-vertical);
 `),O(`tabs-wrapper`,`
 flex-direction: column;
 `),O(`tabs-tab-wrapper`,`
 flex-direction: column;
 `,[O(`tabs-tab-pad`,`
 height: var(--n-tab-gap-vertical);
 width: 100%;
 `)])]),j(`top`,[j(`card-type`,[O(`tabs-scroll-padding`,`border-bottom: 1px solid var(--n-tab-border-color);`),D(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),O(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-top-right-radius: var(--n-tab-border-radius);
 `,[j(`active`,`
 border-bottom: 1px solid #0000;
 `)]),O(`tabs-tab-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),O(`tabs-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `)])]),j(`left`,[j(`card-type`,[O(`tabs-scroll-padding`,`border-right: 1px solid var(--n-tab-border-color);`),D(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),O(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-bottom-left-radius: var(--n-tab-border-radius);
 `,[j(`active`,`
 border-right: 1px solid #0000;
 `)]),O(`tabs-tab-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),O(`tabs-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `)])]),j(`right`,[j(`card-type`,[O(`tabs-scroll-padding`,`border-left: 1px solid var(--n-tab-border-color);`),D(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),O(`tabs-tab`,`
 border-top-right-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[j(`active`,`
 border-left: 1px solid #0000;
 `)]),O(`tabs-tab-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),O(`tabs-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `)])]),j(`bottom`,[j(`card-type`,[O(`tabs-scroll-padding`,`border-top: 1px solid var(--n-tab-border-color);`),D(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),O(`tabs-tab`,`
 border-bottom-left-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[j(`active`,`
 border-top: 1px solid #0000;
 `)]),O(`tabs-tab-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),O(`tabs-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `)])])])]),_e=pe,ve=n({name:`Tabs`,props:Object.assign(Object.assign({},ie.props),{value:[String,Number],defaultValue:[String,Number],trigger:{type:String,default:`click`},type:{type:String,default:`bar`},closable:Boolean,justifyContent:String,size:String,placement:{type:String,default:`top`},tabStyle:[String,Object],tabClass:String,addTabStyle:[String,Object],addTabClass:String,barWidth:Number,paneClass:String,paneStyle:[String,Object],paneWrapperClass:String,paneWrapperStyle:[String,Object],addable:[Boolean,Object],tabsPadding:{type:Number,default:0},animated:Boolean,onBeforeLeave:Function,onAdd:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onClose:[Function,Array],labelSize:String,activeName:[String,Number],onActiveNameChange:[Function,Array]}),slots:Object,setup(e,{slots:n}){let{mergedClsPrefixRef:i,inlineThemeDisabled:a,mergedComponentPropsRef:u}=ne(e),d=ie(`Tabs`,`-tabs`,ge,le,e,i),f=p(null),h=p(null),_=p(null),v=p(null),y=p(null),b=p(null),C=p(!0),w=p(!0),T=ce(e,[`labelSize`,`size`]),E=l(()=>T.value?T.value:u?.value?.Tabs?.size||`medium`),D=ce(e,[`activeName`,`value`]),O=p(D.value??e.defaultValue??(n.default?I(n.default())[0]?.props?.name:null)),k=se(D,O),A={id:0},re=l(()=>{if(!(!e.justifyContent||e.type===`card`))return{display:`flex`,justifyContent:e.justifyContent}});c(k,()=>{A.id=0,P(),F()});function j(){let{value:e}=k;return e===null?null:f.value?.querySelector(`[data-name="${e}"]`)}function ae(t){if(e.type===`card`)return;let{value:n}=h;if(!n)return;let r=n.style.opacity===`0`;if(t){let a=`${i.value}-tabs-bar--disabled`,{barWidth:o,placement:s}=e;if(t.dataset.disabled===`true`?n.classList.add(a):n.classList.remove(a),[`top`,`bottom`].includes(s)){if(N([`top`,`maxHeight`,`height`]),typeof o==`number`&&t.offsetWidth>=o){let e=Math.floor((t.offsetWidth-o)/2)+t.offsetLeft;n.style.left=`${e}px`,n.style.maxWidth=`${o}px`}else n.style.left=`${t.offsetLeft}px`,n.style.maxWidth=`${t.offsetWidth}px`;n.style.width=`8192px`,r&&(n.style.transition=`none`),n.offsetWidth,r&&(n.style.transition=``,n.style.opacity=`1`)}else{if(N([`left`,`maxWidth`,`width`]),typeof o==`number`&&t.offsetHeight>=o){let e=Math.floor((t.offsetHeight-o)/2)+t.offsetTop;n.style.top=`${e}px`,n.style.maxHeight=`${o}px`}else n.style.top=`${t.offsetTop}px`,n.style.maxHeight=`${t.offsetHeight}px`;n.style.height=`8192px`,r&&(n.style.transition=`none`),n.offsetHeight,r&&(n.style.transition=``,n.style.opacity=`1`)}}}function oe(){if(e.type===`card`)return;let{value:t}=h;t&&(t.style.opacity=`0`)}function N(e){let{value:t}=h;if(t)for(let n of e)t.style[n]=``}function P(){if(e.type===`card`)return;let t=j();t?ae(t):oe()}function F(){let e=y.value?.$el;if(!e)return;let t=j();if(!t)return;let{scrollLeft:n,offsetWidth:r}=e,{offsetLeft:i,offsetWidth:a}=t;n>i?e.scrollTo({top:0,left:i,behavior:`smooth`}):i+a>n+r&&e.scrollTo({top:0,left:i+a-r,behavior:`smooth`})}let L=p(null),R=0,z=null;function ue(e){let t=L.value;if(t){R=e.getBoundingClientRect().height;let n=`${R}px`,r=()=>{t.style.height=n,t.style.maxHeight=n};z?(r(),z(),z=null):z=r}}function B(e){let t=L.value;if(t){let n=e.getBoundingClientRect().height,r=()=>{document.body.offsetHeight,t.style.maxHeight=`${n}px`,t.style.height=`${Math.max(R,n)}px`};z?(z(),z=null,r()):z=r}}function de(){let t=L.value;if(t){t.style.maxHeight=``,t.style.height=``;let{paneWrapperStyle:n}=e;if(typeof n==`string`)t.style.cssText=n;else if(n){let{maxHeight:e,height:r}=n;e!==void 0&&(t.style.maxHeight=e),r!==void 0&&(t.style.height=r)}}}let V={value:[]},H=p(`next`);function fe(e){let t=k.value,n=`next`;for(let r of V.value){if(r===t)break;if(r===e){n=`prev`;break}}H.value=n,U(e)}function U(t){let{onActiveNameChange:n,onUpdateValue:r,"onUpdate:value":i}=e;n&&g(n,t),r&&g(r,t),i&&g(i,t),O.value=t}function W(t){let{onClose:n}=e;n&&g(n,t)}let G=!0;function K(){let{value:e}=h;if(!e)return;G||=!1;let t=`transition-disabled`;e.classList.add(t),P(),e.classList.remove(t)}let q=p(null);function J({transitionDisabled:e}){let t=f.value;if(!t)return;e&&t.classList.add(`transition-disabled`);let n=j();n&&q.value&&(q.value.style.width=`${n.offsetWidth}px`,q.value.style.height=`${n.offsetHeight}px`,q.value.style.transform=`translateX(${n.offsetLeft-S(getComputedStyle(t).paddingLeft)}px)`,e&&q.value.offsetWidth),e&&t.classList.remove(`transition-disabled`)}c([k],()=>{e.type===`segment`&&r(()=>{J({transitionDisabled:!1})})}),t(()=>{e.type===`segment`&&J({transitionDisabled:!0})});let Y=0;function pe(t){if(t.contentRect.width===0&&t.contentRect.height===0||Y===t.contentRect.width)return;Y=t.contentRect.width;let{type:n}=e;if((n===`line`||n===`bar`)&&(G||e.justifyContent?.startsWith(`space`))&&K(),n!==`segment`){let{placement:t}=e;Q((t===`top`||t===`bottom`?y.value?.$el:b.value)||null)}}let he=_e(pe,64);c([()=>e.justifyContent,()=>e.size],()=>{r(()=>{let{type:t}=e;(t===`line`||t===`bar`)&&K()})});let X=p(!1);function Z(t){let{target:n,contentRect:{width:r,height:i}}=t,a=n.parentElement.parentElement.offsetWidth,o=n.parentElement.parentElement.offsetHeight,{placement:s}=e;if(!X.value)s===`top`||s===`bottom`?a<r&&(X.value=!0):o<i&&(X.value=!0);else{let{value:e}=v;if(!e)return;s===`top`||s===`bottom`?a-r>e.$el.offsetWidth&&(X.value=!1):o-i>e.$el.offsetHeight&&(X.value=!1)}Q(y.value?.$el||null)}let ve=_e(Z,64);function ye(){let{onAdd:t}=e;t&&t(),r(()=>{let e=j(),{value:t}=y;!e||!t||t.scrollTo({left:e.offsetLeft,top:0,behavior:`smooth`})})}function Q(t){if(!t)return;let{placement:n}=e;if(n===`top`||n===`bottom`){let{scrollLeft:e,scrollWidth:n,offsetWidth:r}=t;C.value=e<=0,w.value=e+r>=n}else{let{scrollTop:e,scrollHeight:n,offsetHeight:r}=t;C.value=e<=0,w.value=e+r>=n}}let be=_e(e=>{Q(e.target)},64);s(me,{triggerRef:m(e,`trigger`),tabStyleRef:m(e,`tabStyle`),tabClassRef:m(e,`tabClass`),addTabStyleRef:m(e,`addTabStyle`),addTabClassRef:m(e,`addTabClass`),paneClassRef:m(e,`paneClass`),paneStyleRef:m(e,`paneStyle`),mergedClsPrefixRef:i,typeRef:m(e,`type`),closableRef:m(e,`closable`),valueRef:k,tabChangeIdRef:A,onBeforeLeaveRef:m(e,`onBeforeLeave`),activateTab:fe,handleClose:W,handleAdd:ye}),ee(()=>{P(),F()}),o(()=>{let{value:e}=_;if(!e)return;let{value:t}=i,n=`${t}-tabs-nav-scroll-wrapper--shadow-start`,r=`${t}-tabs-nav-scroll-wrapper--shadow-end`;C.value?e.classList.remove(n):e.classList.add(n),w.value?e.classList.remove(r):e.classList.add(r)});let $={syncBarPosition:()=>{P()}},xe=()=>{J({transitionDisabled:!0})},Se=l(()=>{let{value:t}=E,{type:n}=e,r=`${t}${{card:`Card`,bar:`Bar`,line:`Line`,segment:`Segment`}[n]}`,{self:{barColor:i,closeIconColor:a,closeIconColorHover:o,closeIconColorPressed:s,tabColor:c,tabBorderColor:l,paneTextColor:u,tabFontWeight:f,tabBorderRadius:p,tabFontWeightActive:m,colorSegment:h,fontWeightStrong:g,tabColorSegment:_,closeSize:v,closeIconSize:y,closeColorHover:b,closeColorPressed:S,closeBorderRadius:ee,[M(`panePadding`,t)]:C,[M(`tabPadding`,r)]:w,[M(`tabPaddingVertical`,r)]:T,[M(`tabGap`,r)]:D,[M(`tabGap`,`${r}Vertical`)]:O,[M(`tabTextColor`,n)]:k,[M(`tabTextColorActive`,n)]:A,[M(`tabTextColorHover`,n)]:te,[M(`tabTextColorDisabled`,n)]:ne,[M(`tabFontSize`,t)]:re},common:{cubicBezierEaseInOut:j}}=d.value;return{"--n-bezier":j,"--n-color-segment":h,"--n-bar-color":i,"--n-tab-font-size":re,"--n-tab-text-color":k,"--n-tab-text-color-active":A,"--n-tab-text-color-disabled":ne,"--n-tab-text-color-hover":te,"--n-pane-text-color":u,"--n-tab-border-color":l,"--n-tab-border-radius":p,"--n-close-size":v,"--n-close-icon-size":y,"--n-close-color-hover":b,"--n-close-color-pressed":S,"--n-close-border-radius":ee,"--n-close-icon-color":a,"--n-close-icon-color-hover":o,"--n-close-icon-color-pressed":s,"--n-tab-color":c,"--n-tab-font-weight":f,"--n-tab-font-weight-active":m,"--n-tab-padding":w,"--n-tab-padding-vertical":T,"--n-tab-gap":D,"--n-tab-gap-vertical":O,"--n-pane-padding-left":x(C,`left`),"--n-pane-padding-right":x(C,`right`),"--n-pane-padding-top":x(C,`top`),"--n-pane-padding-bottom":x(C,`bottom`),"--n-font-weight-strong":g,"--n-tab-color-segment":_}}),Ce=a?te(`tabs`,l(()=>`${E.value[0]}${e.type[0]}`),Se,e):void 0;return Object.assign({mergedClsPrefix:i,mergedValue:k,renderedNames:new Set,segmentCapsuleElRef:q,tabsPaneWrapperRef:L,tabsElRef:f,barElRef:h,addTabInstRef:v,xScrollInstRef:y,scrollWrapperElRef:_,addTabFixed:X,tabWrapperStyle:re,handleNavResize:he,mergedSize:E,handleScroll:be,handleTabsResize:ve,cssVars:a?void 0:Se,themeClass:Ce?.themeClass,animationDirection:H,renderNameListRef:V,yScrollElRef:b,handleSegmentResize:xe,onAnimationBeforeLeave:ue,onAnimationEnter:B,onAnimationAfterEnter:de,onRender:Ce?.onRender},$)},render(){let{mergedClsPrefix:t,type:n,placement:r,addTabFixed:i,addable:a,mergedSize:o,renderNameListRef:s,onRender:c,paneWrapperClass:l,paneWrapperStyle:u,$slots:{default:d,prefix:f,suffix:p}}=this;c?.();let m=d?I(d()).filter(e=>e.type.__TAB_PANE__===!0):[],g=d?I(d()).filter(e=>e.type.__TAB__===!0):[],_=!g.length,v=n===`card`,y=n===`segment`,b=!v&&!y&&this.justifyContent;s.value=[];let x=()=>{let n=e(`div`,{style:this.tabWrapperStyle,class:`${t}-tabs-wrapper`},b?null:e(`div`,{class:`${t}-tabs-scroll-padding`,style:r===`top`||r===`bottom`?{width:`${this.tabsPadding}px`}:{height:`${this.tabsPadding}px`}}),_?m.map((t,n)=>(s.value.push(t.props.name),$(e(Z,Object.assign({},t.props,{internalCreatedByPane:!0,internalLeftPadded:n!==0&&(!b||b===`center`||b===`start`||b===`end`)}),t.children?{default:t.children.tab}:void 0)))):g.map((e,t)=>(s.value.push(e.props.name),$(t!==0&&!b?be(e):e))),!i&&a&&v?Q(a,(_?m.length:g.length)!==0):null,b?null:e(`div`,{class:`${t}-tabs-scroll-padding`,style:{width:`${this.tabsPadding}px`}}));return e(`div`,{ref:`tabsElRef`,class:`${t}-tabs-nav-scroll-content`},v&&a?e(C,{onResize:this.handleTabsResize},{default:()=>n}):n,v?e(`div`,{class:`${t}-tabs-pad`}):null,v?null:e(`div`,{ref:`barElRef`,class:`${t}-tabs-bar`}))},S=y?`top`:r;return e(`div`,{class:[`${t}-tabs`,this.themeClass,`${t}-tabs--${n}-type`,`${t}-tabs--${o}-size`,b&&`${t}-tabs--flex`,`${t}-tabs--${S}`],style:this.cssVars},e(`div`,{class:[`${t}-tabs-nav--${n}-type`,`${t}-tabs-nav--${S}`,`${t}-tabs-nav`]},h(f,n=>n&&e(`div`,{class:`${t}-tabs-nav__prefix`},n)),y?e(C,{onResize:this.handleSegmentResize},{default:()=>e(`div`,{class:`${t}-tabs-rail`,ref:`tabsElRef`},e(`div`,{class:`${t}-tabs-capsule`,ref:`segmentCapsuleElRef`},e(`div`,{class:`${t}-tabs-wrapper`},e(`div`,{class:`${t}-tabs-tab`}))),_?m.map((t,n)=>(s.value.push(t.props.name),e(Z,Object.assign({},t.props,{internalCreatedByPane:!0,internalLeftPadded:n!==0}),t.children?{default:t.children.tab}:void 0))):g.map((e,t)=>(s.value.push(e.props.name),t===0?e:be(e))))}):e(C,{onResize:this.handleNavResize},{default:()=>e(`div`,{class:`${t}-tabs-nav-scroll-wrapper`,ref:`scrollWrapperElRef`},[`top`,`bottom`].includes(S)?e(w,{ref:`xScrollInstRef`,onScroll:this.handleScroll},{default:x}):e(`div`,{class:`${t}-tabs-nav-y-scroll`,onScroll:this.handleScroll,ref:`yScrollElRef`},x()))}),i&&a&&v?Q(a,!0):null,h(p,n=>n&&e(`div`,{class:`${t}-tabs-nav__suffix`},n))),_&&(this.animated&&(S===`top`||S===`bottom`)?e(`div`,{ref:`tabsPaneWrapperRef`,style:u,class:[`${t}-tabs-pane-wrapper`,l]},ye(m,this.mergedValue,this.renderedNames,this.onAnimationBeforeLeave,this.onAnimationEnter,this.onAnimationAfterEnter,this.animationDirection)):ye(m,this.mergedValue,this.renderedNames)))}});function ye(t,n,r,i,a,o,s){let c=[];return t.forEach(e=>{let{name:t,displayDirective:i,"display-directive":a}=e.props,o=e=>i===e||a===e,s=n===t;if(e.key!==void 0&&(e.key=t),s||o(`show`)||o(`show:lazy`)&&r.has(t)){r.has(t)||r.add(t);let n=!o(`if`);c.push(n?u(e,[[y,s]]):e)}}),s?e(b,{name:`${s}-transition`,onBeforeLeave:i,onEnter:a,onAfterEnter:o},{default:()=>c}):c}function Q(t,n){return e(Z,{ref:`addTabInstRef`,key:`__addable`,name:`__addable`,internalCreatedByPane:!0,internalAddable:!0,internalLeftPadded:n,disabled:typeof t==`object`&&t.disabled})}function be(e){let t=f(e);return t.props?t.props.internalLeftPadded=!0:t.props={internalLeftPadded:!0},t}function $(e){return Array.isArray(e.dynamicProps)?e.dynamicProps.includes(`internalLeftPadded`)||e.dynamicProps.push(`internalLeftPadded`):e.dynamicProps=[`internalLeftPadded`],e}export{Z as n,X as r,ve as t};