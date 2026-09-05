import{$ as e,E as t,N as n,P as r,U as i,X as a,Z as o,ct as s,d as c,ft as l,g as u,h as d,j as f,k as p,z as m}from"./echarts-Cw2yHLaZ.js";import{A as h,Bt as g,Ct as _,Ft as v,Kt as y,Lt as b,O as x,Ot as S,Pt as C,Rt as w,St as T,Wt as E,bt as D,ct as O,dt as ee,ft as te,it as k,tt as A,w as ne,zt as j}from"./auth-CYMAsBzV.js";import{A as M,O as re,b as ie,d as N,l as ae}from"./vue-core-BDTvi3xZ.js";import{s as oe}from"./get-CsENiSKu.js";import{t as se}from"./use-compitable-OuJoh8Ky.js";import{n as P}from"./InputNumber-CsSdg2CO.js";import{it as F,r as ce,rt as le,st as ue}from"./index-ChsYpqrV.js";var I=/\s/;function L(e){for(var t=e.length;t--&&I.test(e.charAt(t)););return t}var R=/^\s+/;function de(e){return e&&e.slice(0,L(e)+1).replace(R,``)}var z=NaN,fe=/^[-+]0x[0-9a-f]+$/i,B=/^0b[01]+$/i,V=/^0o[0-7]+$/i,pe=parseInt;function H(e){if(typeof e==`number`)return e;if(k(e))return z;if(A(e)){var t=typeof e.valueOf==`function`?e.valueOf():e;e=A(t)?t+``:t}if(typeof e!=`string`)return e===0?e:+e;e=de(e);var n=B.test(e);return n||V.test(e)?pe(e.slice(2),n?2:8):fe.test(e)?z:+e}var U=function(){return O.Date.now()},W=`Expected a function`,G=Math.max,K=Math.min;function q(e,t,n){var r,i,a,o,s,c,l=0,u=!1,d=!1,f=!0;if(typeof e!=`function`)throw TypeError(W);t=H(t)||0,A(n)&&(u=!!n.leading,d=`maxWait`in n,a=d?G(H(n.maxWait)||0,t):a,f=`trailing`in n?!!n.trailing:f);function p(t){var n=r,a=i;return r=i=void 0,l=t,o=e.apply(a,n),o}function m(e){return l=e,s=setTimeout(_,t),u?p(e):o}function h(e){var n=e-c,r=e-l,i=t-n;return d?K(i,a-r):i}function g(e){var n=e-c,r=e-l;return c===void 0||n>=t||n<0||d&&r>=a}function _(){var e=U();if(g(e))return v(e);s=setTimeout(_,h(e))}function v(e){return s=void 0,f&&r?p(e):(r=i=void 0,o)}function y(){s!==void 0&&clearTimeout(s),l=0,r=c=i=s=void 0}function b(){return s===void 0?o:v(U())}function x(){var e=U(),n=g(e);if(r=arguments,i=this,c=e,n){if(s===void 0)return m(c);if(d)return clearTimeout(s),s=setTimeout(_,t),p(c)}return s===void 0&&(s=setTimeout(_,t)),o}return x.cancel=y,x.flush=b,x}var J=`Expected a function`;function me(e,t,n){var r=!0,i=!0;if(typeof e!=`function`)throw TypeError(J);return A(n)&&(r=`leading`in n?!!n.leading:r,i=`trailing`in n?!!n.trailing:i),q(e,t,{leading:r,maxWait:t,trailing:i})}var Y=S(`n-tabs`),X={tab:[String,Number,Object,Function],name:{type:[String,Number],required:!0},disabled:Boolean,displayDirective:{type:String,default:`if`},closable:{type:Boolean,default:void 0},tabProps:Object,label:[String,Number,Object,Function]},Z=t({__TAB_PANE__:!0,name:`TabPane`,alias:[`TabPanel`],props:X,slots:Object,setup(e){let t=f(Y,null);return t||_(`tab-pane`,"`n-tab-pane` must be placed inside `n-tabs`."),{style:t.paneStyleRef,class:t.paneClassRef,mergedClsPrefix:t.mergedClsPrefixRef}},render(){return p(`div`,{class:[`${this.mergedClsPrefix}-tab-pane`,this.class],style:this.style},this.$slots)}}),Q=t({__TAB__:!0,inheritAttrs:!1,name:`Tab`,props:Object.assign({internalLeftPadded:Boolean,internalAddable:Boolean,internalCreatedByPane:Boolean},F(X,[`displayDirective`])),setup(e){let{mergedClsPrefixRef:t,valueRef:n,typeRef:r,closableRef:i,tabStyleRef:a,addTabStyleRef:o,tabClassRef:s,addTabClassRef:c,tabChangeIdRef:l,onBeforeLeaveRef:d,triggerRef:p,handleAdd:m,activateTab:h,handleClose:g}=f(Y);return{trigger:p,mergedClosable:u(()=>{if(e.internalAddable)return!1;let{closable:t}=e;return t===void 0?i.value:t}),style:a,addStyle:o,tabClass:s,addTabClass:c,clsPrefix:t,value:n,type:r,handleClose(t){t.stopPropagation(),!e.disabled&&g(e.name)},activateTab(){if(e.disabled)return;if(e.internalAddable){m();return}let{name:t}=e,r=++l.id;if(t!==n.value){let{value:i}=d;i?Promise.resolve(i(e.name,n.value)).then(e=>{e&&l.id===r&&h(t)}):h(t)}}}},render(){let{internalAddable:e,clsPrefix:t,name:r,disabled:i,label:a,tab:o,value:s,mergedClosable:l,trigger:u,$slots:{default:d}}=this,f=a??o;return p(`div`,{class:`${t}-tabs-tab-wrapper`},this.internalLeftPadded?p(`div`,{class:`${t}-tabs-tab-pad`}):null,p(`div`,Object.assign({key:r,"data-name":r,"data-disabled":i?!0:void 0},n({class:[`${t}-tabs-tab`,s===r&&`${t}-tabs-tab--active`,i&&`${t}-tabs-tab--disabled`,l&&`${t}-tabs-tab--closable`,e&&`${t}-tabs-tab--addable`,e?this.addTabClass:this.tabClass],onClick:u===`click`?this.activateTab:void 0,onMouseenter:u===`hover`?this.activateTab:void 0,style:e?this.addStyle:this.style},this.internalCreatedByPane?this.tabProps||{}:this.$attrs)),p(`span`,{class:`${t}-tabs-tab__label`},e?p(c,null,p(`div`,{class:`${t}-tabs-tab__height-placeholder`},`\xA0`),p(x,{clsPrefix:t},{default:()=>p(P,null)})):d?d():typeof f==`object`?f:le(f??r)),l&&this.type===`card`?p(ne,{clsPrefix:t,class:`${t}-tabs-tab__close`,onClick:this.handleClose,disabled:i}):null))}}),he=v(`tabs`,`
 box-sizing: border-box;
 width: 100%;
 display: flex;
 flex-direction: column;
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
`,[w(`segment-type`,[v(`tabs-rail`,[C(`&.transition-disabled`,[v(`tabs-capsule`,`
 transition: none;
 `)])])]),w(`top`,[v(`tab-pane`,`
 padding: var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left);
 `)]),w(`left`,[v(`tab-pane`,`
 padding: var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left) var(--n-pane-padding-top);
 `)]),w(`left, right`,`
 flex-direction: row;
 `,[v(`tabs-bar`,`
 width: 2px;
 right: 0;
 transition:
 top .2s var(--n-bezier),
 max-height .2s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),v(`tabs-tab`,`
 padding: var(--n-tab-padding-vertical); 
 `)]),w(`right`,`
 flex-direction: row-reverse;
 `,[v(`tab-pane`,`
 padding: var(--n-pane-padding-left) var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom);
 `),v(`tabs-bar`,`
 left: 0;
 `)]),w(`bottom`,`
 flex-direction: column-reverse;
 justify-content: flex-end;
 `,[v(`tab-pane`,`
 padding: var(--n-pane-padding-bottom) var(--n-pane-padding-right) var(--n-pane-padding-top) var(--n-pane-padding-left);
 `),v(`tabs-bar`,`
 top: 0;
 `)]),v(`tabs-rail`,`
 position: relative;
 padding: 3px;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 background-color: var(--n-color-segment);
 transition: background-color .3s var(--n-bezier);
 display: flex;
 align-items: center;
 `,[v(`tabs-capsule`,`
 border-radius: var(--n-tab-border-radius);
 position: absolute;
 pointer-events: none;
 background-color: var(--n-tab-color-segment);
 box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .08);
 transition: transform 0.3s var(--n-bezier);
 `),v(`tabs-tab-wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[v(`tabs-tab`,`
 overflow: hidden;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[w(`active`,`
 font-weight: var(--n-font-weight-strong);
 color: var(--n-tab-text-color-active);
 `),C(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])])]),w(`flex`,[v(`tabs-nav`,`
 width: 100%;
 position: relative;
 `,[v(`tabs-wrapper`,`
 width: 100%;
 `,[v(`tabs-tab`,`
 margin-right: 0;
 `)])])]),v(`tabs-nav`,`
 box-sizing: border-box;
 line-height: 1.5;
 display: flex;
 transition: border-color .3s var(--n-bezier);
 `,[b(`prefix, suffix`,`
 display: flex;
 align-items: center;
 `),b(`prefix`,`padding-right: 16px;`),b(`suffix`,`padding-left: 16px;`)]),w(`top, bottom`,[C(`>`,[v(`tabs-nav`,[v(`tabs-nav-scroll-wrapper`,[C(`&::before`,`
 top: 0;
 bottom: 0;
 left: 0;
 width: 20px;
 `),C(`&::after`,`
 top: 0;
 bottom: 0;
 right: 0;
 width: 20px;
 `),w(`shadow-start`,[C(`&::before`,`
 box-shadow: inset 10px 0 8px -8px rgba(0, 0, 0, .12);
 `)]),w(`shadow-end`,[C(`&::after`,`
 box-shadow: inset -10px 0 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),w(`left, right`,[v(`tabs-nav-scroll-content`,`
 flex-direction: column;
 `),C(`>`,[v(`tabs-nav`,[v(`tabs-nav-scroll-wrapper`,[C(`&::before`,`
 top: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),C(`&::after`,`
 bottom: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),w(`shadow-start`,[C(`&::before`,`
 box-shadow: inset 0 10px 8px -8px rgba(0, 0, 0, .12);
 `)]),w(`shadow-end`,[C(`&::after`,`
 box-shadow: inset 0 -10px 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),v(`tabs-nav-scroll-wrapper`,`
 flex: 1;
 position: relative;
 overflow: hidden;
 `,[v(`tabs-nav-y-scroll`,`
 height: 100%;
 width: 100%;
 overflow-y: auto; 
 scrollbar-width: none;
 `,[C(`&::-webkit-scrollbar, &::-webkit-scrollbar-track-piece, &::-webkit-scrollbar-thumb`,`
 width: 0;
 height: 0;
 display: none;
 `)]),C(`&::before, &::after`,`
 transition: box-shadow .3s var(--n-bezier);
 pointer-events: none;
 content: "";
 position: absolute;
 z-index: 1;
 `)]),v(`tabs-nav-scroll-content`,`
 display: flex;
 position: relative;
 min-width: 100%;
 min-height: 100%;
 width: fit-content;
 box-sizing: border-box;
 `),v(`tabs-wrapper`,`
 display: inline-flex;
 flex-wrap: nowrap;
 position: relative;
 `),v(`tabs-tab-wrapper`,`
 display: flex;
 flex-wrap: nowrap;
 flex-shrink: 0;
 flex-grow: 0;
 `),v(`tabs-tab`,`
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
 `,[w(`disabled`,{cursor:`not-allowed`}),b(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `),b(`label`,`
 display: flex;
 align-items: center;
 z-index: 1;
 `)]),v(`tabs-bar`,`
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
 `,[C(`&.transition-disabled`,`
 transition: none;
 `),w(`disabled`,`
 background-color: var(--n-tab-text-color-disabled)
 `)]),v(`tabs-pane-wrapper`,`
 position: relative;
 overflow: hidden;
 transition: max-height .2s var(--n-bezier);
 `),v(`tab-pane`,`
 color: var(--n-pane-text-color);
 width: 100%;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 opacity .2s var(--n-bezier);
 left: 0;
 right: 0;
 top: 0;
 `,[C(`&.next-transition-leave-active, &.prev-transition-leave-active, &.next-transition-enter-active, &.prev-transition-enter-active`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 transform .2s var(--n-bezier),
 opacity .2s var(--n-bezier);
 `),C(`&.next-transition-leave-active, &.prev-transition-leave-active`,`
 position: absolute;
 `),C(`&.next-transition-enter-from, &.prev-transition-leave-to`,`
 transform: translateX(32px);
 opacity: 0;
 `),C(`&.next-transition-leave-to, &.prev-transition-enter-from`,`
 transform: translateX(-32px);
 opacity: 0;
 `),C(`&.next-transition-leave-from, &.next-transition-enter-to, &.prev-transition-leave-from, &.prev-transition-enter-to`,`
 transform: translateX(0);
 opacity: 1;
 `)]),v(`tabs-tab-pad`,`
 box-sizing: border-box;
 width: var(--n-tab-gap);
 flex-grow: 0;
 flex-shrink: 0;
 `),w(`line-type, bar-type`,[v(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 box-sizing: border-box;
 vertical-align: bottom;
 `,[C(`&:hover`,{color:`var(--n-tab-text-color-hover)`}),w(`active`,`
 color: var(--n-tab-text-color-active);
 font-weight: var(--n-tab-font-weight-active);
 `),w(`disabled`,{color:`var(--n-tab-text-color-disabled)`})])]),v(`tabs-nav`,[w(`line-type`,[w(`top`,[b(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),v(`tabs-nav-scroll-content`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),v(`tabs-bar`,`
 bottom: -1px;
 `)]),w(`left`,[b(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),v(`tabs-nav-scroll-content`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),v(`tabs-bar`,`
 right: -1px;
 `)]),w(`right`,[b(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),v(`tabs-nav-scroll-content`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),v(`tabs-bar`,`
 left: -1px;
 `)]),w(`bottom`,[b(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),v(`tabs-nav-scroll-content`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),v(`tabs-bar`,`
 top: -1px;
 `)]),b(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),v(`tabs-nav-scroll-content`,`
 transition: border-color .3s var(--n-bezier);
 `),v(`tabs-bar`,`
 border-radius: 0;
 `)]),w(`card-type`,[b(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),v(`tabs-pad`,`
 flex-grow: 1;
 transition: border-color .3s var(--n-bezier);
 `),v(`tabs-tab-pad`,`
 transition: border-color .3s var(--n-bezier);
 `),v(`tabs-tab`,`
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
 `,[w(`addable`,`
 padding-left: 8px;
 padding-right: 8px;
 font-size: 16px;
 justify-content: center;
 `,[b(`height-placeholder`,`
 width: 0;
 font-size: var(--n-tab-font-size);
 `),j(`disabled`,[C(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])]),w(`closable`,`padding-right: 8px;`),w(`active`,`
 background-color: #0000;
 font-weight: var(--n-tab-font-weight-active);
 color: var(--n-tab-text-color-active);
 `),w(`disabled`,`color: var(--n-tab-text-color-disabled);`)])]),w(`left, right`,`
 flex-direction: column; 
 `,[b(`prefix, suffix`,`
 padding: var(--n-tab-padding-vertical);
 `),v(`tabs-wrapper`,`
 flex-direction: column;
 `),v(`tabs-tab-wrapper`,`
 flex-direction: column;
 `,[v(`tabs-tab-pad`,`
 height: var(--n-tab-gap-vertical);
 width: 100%;
 `)])]),w(`top`,[w(`card-type`,[v(`tabs-scroll-padding`,`border-bottom: 1px solid var(--n-tab-border-color);`),b(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),v(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-top-right-radius: var(--n-tab-border-radius);
 `,[w(`active`,`
 border-bottom: 1px solid #0000;
 `)]),v(`tabs-tab-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),v(`tabs-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `)])]),w(`left`,[w(`card-type`,[v(`tabs-scroll-padding`,`border-right: 1px solid var(--n-tab-border-color);`),b(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),v(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-bottom-left-radius: var(--n-tab-border-radius);
 `,[w(`active`,`
 border-right: 1px solid #0000;
 `)]),v(`tabs-tab-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),v(`tabs-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `)])]),w(`right`,[w(`card-type`,[v(`tabs-scroll-padding`,`border-left: 1px solid var(--n-tab-border-color);`),b(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),v(`tabs-tab`,`
 border-top-right-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[w(`active`,`
 border-left: 1px solid #0000;
 `)]),v(`tabs-tab-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),v(`tabs-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `)])]),w(`bottom`,[w(`card-type`,[v(`tabs-scroll-padding`,`border-top: 1px solid var(--n-tab-border-color);`),b(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),v(`tabs-tab`,`
 border-bottom-left-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[w(`active`,`
 border-top: 1px solid #0000;
 `)]),v(`tabs-tab-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),v(`tabs-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `)])])])]),ge=me,_e=t({name:`Tabs`,props:Object.assign(Object.assign({},h.props),{value:[String,Number],defaultValue:[String,Number],trigger:{type:String,default:`click`},type:{type:String,default:`bar`},closable:Boolean,justifyContent:String,size:String,placement:{type:String,default:`top`},tabStyle:[String,Object],tabClass:String,addTabStyle:[String,Object],addTabClass:String,barWidth:Number,paneClass:String,paneStyle:[String,Object],paneWrapperClass:String,paneWrapperStyle:[String,Object],addable:[Boolean,Object],tabsPadding:{type:Number,default:0},animated:Boolean,onBeforeLeave:Function,onAdd:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onClose:[Function,Array],labelSize:String,activeName:[String,Number],onActiveNameChange:[Function,Array]}),slots:Object,setup(e,{slots:t}){let{mergedClsPrefixRef:n,inlineThemeDisabled:c,mergedComponentPropsRef:d}=te(e),f=h(`Tabs`,`-tabs`,he,ce,e,n),p=s(null),_=s(null),v=s(null),y=s(null),b=s(null),x=s(null),S=s(!0),C=s(!0),w=se(e,[`labelSize`,`size`]),E=u(()=>w.value?w.value:d?.value?.Tabs?.size||`medium`),D=se(e,[`activeName`,`value`]),O=s(D.value??e.defaultValue??(t.default?ue(t.default())[0]?.props?.name:null)),k=oe(D,O),A={id:0},ne=u(()=>{if(!(!e.justifyContent||e.type===`card`))return{display:`flex`,justifyContent:e.justifyContent}});a(k,()=>{A.id=0,F(),le()});function j(){let{value:e}=k;return e===null?null:p.value?.querySelector(`[data-name="${e}"]`)}function N(t){if(e.type===`card`)return;let{value:r}=_;if(!r)return;let i=r.style.opacity===`0`;if(t){let a=`${n.value}-tabs-bar--disabled`,{barWidth:o,placement:s}=e;if(t.dataset.disabled===`true`?r.classList.add(a):r.classList.remove(a),[`top`,`bottom`].includes(s)){if(P([`top`,`maxHeight`,`height`]),typeof o==`number`&&t.offsetWidth>=o){let e=Math.floor((t.offsetWidth-o)/2)+t.offsetLeft;r.style.left=`${e}px`,r.style.maxWidth=`${o}px`}else r.style.left=`${t.offsetLeft}px`,r.style.maxWidth=`${t.offsetWidth}px`;r.style.width=`8192px`,i&&(r.style.transition=`none`),r.offsetWidth,i&&(r.style.transition=``,r.style.opacity=`1`)}else{if(P([`left`,`maxWidth`,`width`]),typeof o==`number`&&t.offsetHeight>=o){let e=Math.floor((t.offsetHeight-o)/2)+t.offsetTop;r.style.top=`${e}px`,r.style.maxHeight=`${o}px`}else r.style.top=`${t.offsetTop}px`,r.style.maxHeight=`${t.offsetHeight}px`;r.style.height=`8192px`,i&&(r.style.transition=`none`),r.offsetHeight,i&&(r.style.transition=``,r.style.opacity=`1`)}}}function ae(){if(e.type===`card`)return;let{value:t}=_;t&&(t.style.opacity=`0`)}function P(e){let{value:t}=_;if(t)for(let n of e)t.style[n]=``}function F(){if(e.type===`card`)return;let t=j();t?N(t):ae()}function le(){let e=b.value?.$el;if(!e)return;let t=j();if(!t)return;let{scrollLeft:n,offsetWidth:r}=e,{offsetLeft:i,offsetWidth:a}=t;n>i?e.scrollTo({top:0,left:i,behavior:`smooth`}):i+a>n+r&&e.scrollTo({top:0,left:i+a-r,behavior:`smooth`})}let I=s(null),L=0,R=null;function de(e){let t=I.value;if(t){L=e.getBoundingClientRect().height;let n=`${L}px`,r=()=>{t.style.height=n,t.style.maxHeight=n};R?(r(),R(),R=null):R=r}}function z(e){let t=I.value;if(t){let n=e.getBoundingClientRect().height,r=()=>{document.body.offsetHeight,t.style.maxHeight=`${n}px`,t.style.height=`${Math.max(L,n)}px`};R?(R(),R=null,r()):R=r}}function fe(){let t=I.value;if(t){t.style.maxHeight=``,t.style.height=``;let{paneWrapperStyle:n}=e;if(typeof n==`string`)t.style.cssText=n;else if(n){let{maxHeight:e,height:r}=n;e!==void 0&&(t.style.maxHeight=e),r!==void 0&&(t.style.height=r)}}}let B={value:[]},V=s(`next`);function pe(e){let t=k.value,n=`next`;for(let r of B.value){if(r===t)break;if(r===e){n=`prev`;break}}V.value=n,H(e)}function H(t){let{onActiveNameChange:n,onUpdateValue:r,"onUpdate:value":i}=e;n&&T(n,t),r&&T(r,t),i&&T(i,t),O.value=t}function U(t){let{onClose:n}=e;n&&T(n,t)}let W=!0;function G(){let{value:e}=_;if(!e)return;W||=!1;let t=`transition-disabled`;e.classList.add(t),F(),e.classList.remove(t)}let K=s(null);function q({transitionDisabled:e}){let t=p.value;if(!t)return;e&&t.classList.add(`transition-disabled`);let n=j();n&&K.value&&(K.value.style.width=`${n.offsetWidth}px`,K.value.style.height=`${n.offsetHeight}px`,K.value.style.transform=`translateX(${n.offsetLeft-re(getComputedStyle(t).paddingLeft)}px)`,e&&K.value.offsetWidth),e&&t.classList.remove(`transition-disabled`)}a([k],()=>{e.type===`segment`&&r(()=>{q({transitionDisabled:!1})})}),m(()=>{e.type===`segment`&&q({transitionDisabled:!0})});let J=0;function me(t){if(t.contentRect.width===0&&t.contentRect.height===0||J===t.contentRect.width)return;J=t.contentRect.width;let{type:n}=e;if((n===`line`||n===`bar`)&&(W||e.justifyContent?.startsWith(`space`))&&G(),n!==`segment`){let{placement:t}=e;$((t===`top`||t===`bottom`?b.value?.$el:x.value)||null)}}let X=ge(me,64);a([()=>e.justifyContent,()=>e.size],()=>{r(()=>{let{type:t}=e;(t===`line`||t===`bar`)&&G()})});let Z=s(!1);function Q(t){let{target:n,contentRect:{width:r,height:i}}=t,a=n.parentElement.parentElement.offsetWidth,o=n.parentElement.parentElement.offsetHeight,{placement:s}=e;if(!Z.value)s===`top`||s===`bottom`?a<r&&(Z.value=!0):o<i&&(Z.value=!0);else{let{value:e}=y;if(!e)return;s===`top`||s===`bottom`?a-r>e.$el.offsetWidth&&(Z.value=!1):o-i>e.$el.offsetHeight&&(Z.value=!1)}$(b.value?.$el||null)}let _e=ge(Q,64);function ve(){let{onAdd:t}=e;t&&t(),r(()=>{let e=j(),{value:t}=b;!e||!t||t.scrollTo({left:e.offsetLeft,top:0,behavior:`smooth`})})}function $(t){if(!t)return;let{placement:n}=e;if(n===`top`||n===`bottom`){let{scrollLeft:e,scrollWidth:n,offsetWidth:r}=t;S.value=e<=0,C.value=e+r>=n}else{let{scrollTop:e,scrollHeight:n,offsetHeight:r}=t;S.value=e<=0,C.value=e+r>=n}}let ye=ge(e=>{$(e.target)},64);i(Y,{triggerRef:l(e,`trigger`),tabStyleRef:l(e,`tabStyle`),tabClassRef:l(e,`tabClass`),addTabStyleRef:l(e,`addTabStyle`),addTabClassRef:l(e,`addTabClass`),paneClassRef:l(e,`paneClass`),paneStyleRef:l(e,`paneStyle`),mergedClsPrefixRef:n,typeRef:l(e,`type`),closableRef:l(e,`closable`),valueRef:k,tabChangeIdRef:A,onBeforeLeaveRef:l(e,`onBeforeLeave`),activateTab:pe,handleClose:U,handleAdd:ve}),ie(()=>{F(),le()}),o(()=>{let{value:e}=v;if(!e)return;let{value:t}=n,r=`${t}-tabs-nav-scroll-wrapper--shadow-start`,i=`${t}-tabs-nav-scroll-wrapper--shadow-end`;S.value?e.classList.remove(r):e.classList.add(r),C.value?e.classList.remove(i):e.classList.add(i)});let be={syncBarPosition:()=>{F()}},xe=()=>{q({transitionDisabled:!0})},Se=u(()=>{let{value:t}=E,{type:n}=e,r=`${t}${{card:`Card`,bar:`Bar`,line:`Line`,segment:`Segment`}[n]}`,{self:{barColor:i,closeIconColor:a,closeIconColorHover:o,closeIconColorPressed:s,tabColor:c,tabBorderColor:l,paneTextColor:u,tabFontWeight:d,tabBorderRadius:p,tabFontWeightActive:m,colorSegment:h,fontWeightStrong:_,tabColorSegment:v,closeSize:y,closeIconSize:b,closeColorHover:x,closeColorPressed:S,closeBorderRadius:C,[g(`panePadding`,t)]:w,[g(`tabPadding`,r)]:T,[g(`tabPaddingVertical`,r)]:D,[g(`tabGap`,r)]:O,[g(`tabGap`,`${r}Vertical`)]:ee,[g(`tabTextColor`,n)]:te,[g(`tabTextColorActive`,n)]:k,[g(`tabTextColorHover`,n)]:A,[g(`tabTextColorDisabled`,n)]:ne,[g(`tabFontSize`,t)]:j},common:{cubicBezierEaseInOut:re}}=f.value;return{"--n-bezier":re,"--n-color-segment":h,"--n-bar-color":i,"--n-tab-font-size":j,"--n-tab-text-color":te,"--n-tab-text-color-active":k,"--n-tab-text-color-disabled":ne,"--n-tab-text-color-hover":A,"--n-pane-text-color":u,"--n-tab-border-color":l,"--n-tab-border-radius":p,"--n-close-size":y,"--n-close-icon-size":b,"--n-close-color-hover":x,"--n-close-color-pressed":S,"--n-close-border-radius":C,"--n-close-icon-color":a,"--n-close-icon-color-hover":o,"--n-close-icon-color-pressed":s,"--n-tab-color":c,"--n-tab-font-weight":d,"--n-tab-font-weight-active":m,"--n-tab-padding":T,"--n-tab-padding-vertical":D,"--n-tab-gap":O,"--n-tab-gap-vertical":ee,"--n-pane-padding-left":M(w,`left`),"--n-pane-padding-right":M(w,`right`),"--n-pane-padding-top":M(w,`top`),"--n-pane-padding-bottom":M(w,`bottom`),"--n-font-weight-strong":_,"--n-tab-color-segment":v}}),Ce=c?ee(`tabs`,u(()=>`${E.value[0]}${e.type[0]}`),Se,e):void 0;return Object.assign({mergedClsPrefix:n,mergedValue:k,renderedNames:new Set,segmentCapsuleElRef:K,tabsPaneWrapperRef:I,tabsElRef:p,barElRef:_,addTabInstRef:y,xScrollInstRef:b,scrollWrapperElRef:v,addTabFixed:Z,tabWrapperStyle:ne,handleNavResize:X,mergedSize:E,handleScroll:ye,handleTabsResize:_e,cssVars:c?void 0:Se,themeClass:Ce?.themeClass,animationDirection:V,renderNameListRef:B,yScrollElRef:x,handleSegmentResize:xe,onAnimationBeforeLeave:de,onAnimationEnter:z,onAnimationAfterEnter:fe,onRender:Ce?.onRender},be)},render(){let{mergedClsPrefix:e,type:t,placement:n,addTabFixed:r,addable:i,mergedSize:a,renderNameListRef:o,onRender:s,paneWrapperClass:c,paneWrapperStyle:l,$slots:{default:u,prefix:d,suffix:f}}=this;s?.();let m=u?ue(u()).filter(e=>e.type.__TAB_PANE__===!0):[],h=u?ue(u()).filter(e=>e.type.__TAB__===!0):[],g=!h.length,_=t===`card`,v=t===`segment`,y=!_&&!v&&this.justifyContent;o.value=[];let b=()=>{let t=p(`div`,{style:this.tabWrapperStyle,class:`${e}-tabs-wrapper`},y?null:p(`div`,{class:`${e}-tabs-scroll-padding`,style:n===`top`||n===`bottom`?{width:`${this.tabsPadding}px`}:{height:`${this.tabsPadding}px`}}),g?m.map((e,t)=>(o.value.push(e.props.name),be(p(Q,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0&&(!y||y===`center`||y===`start`||y===`end`)}),e.children?{default:e.children.tab}:void 0)))):h.map((e,t)=>(o.value.push(e.props.name),be(t!==0&&!y?ye(e):e))),!r&&i&&_?$(i,(g?m.length:h.length)!==0):null,y?null:p(`div`,{class:`${e}-tabs-scroll-padding`,style:{width:`${this.tabsPadding}px`}}));return p(`div`,{ref:`tabsElRef`,class:`${e}-tabs-nav-scroll-content`},_&&i?p(N,{onResize:this.handleTabsResize},{default:()=>t}):t,_?p(`div`,{class:`${e}-tabs-pad`}):null,_?null:p(`div`,{ref:`barElRef`,class:`${e}-tabs-bar`}))},x=v?`top`:n;return p(`div`,{class:[`${e}-tabs`,this.themeClass,`${e}-tabs--${t}-type`,`${e}-tabs--${a}-size`,y&&`${e}-tabs--flex`,`${e}-tabs--${x}`],style:this.cssVars},p(`div`,{class:[`${e}-tabs-nav--${t}-type`,`${e}-tabs-nav--${x}`,`${e}-tabs-nav`]},D(d,t=>t&&p(`div`,{class:`${e}-tabs-nav__prefix`},t)),v?p(N,{onResize:this.handleSegmentResize},{default:()=>p(`div`,{class:`${e}-tabs-rail`,ref:`tabsElRef`},p(`div`,{class:`${e}-tabs-capsule`,ref:`segmentCapsuleElRef`},p(`div`,{class:`${e}-tabs-wrapper`},p(`div`,{class:`${e}-tabs-tab`}))),g?m.map((e,t)=>(o.value.push(e.props.name),p(Q,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0}),e.children?{default:e.children.tab}:void 0))):h.map((e,t)=>(o.value.push(e.props.name),t===0?e:ye(e))))}):p(N,{onResize:this.handleNavResize},{default:()=>p(`div`,{class:`${e}-tabs-nav-scroll-wrapper`,ref:`scrollWrapperElRef`},[`top`,`bottom`].includes(x)?p(ae,{ref:`xScrollInstRef`,onScroll:this.handleScroll},{default:b}):p(`div`,{class:`${e}-tabs-nav-y-scroll`,onScroll:this.handleScroll,ref:`yScrollElRef`},b()))}),r&&i&&_?$(i,!0):null,D(f,t=>t&&p(`div`,{class:`${e}-tabs-nav__suffix`},t))),g&&(this.animated&&(x===`top`||x===`bottom`)?p(`div`,{ref:`tabsPaneWrapperRef`,style:l,class:[`${e}-tabs-pane-wrapper`,c]},ve(m,this.mergedValue,this.renderedNames,this.onAnimationBeforeLeave,this.onAnimationEnter,this.onAnimationAfterEnter,this.animationDirection)):ve(m,this.mergedValue,this.renderedNames)))}});function ve(t,n,r,i,a,o,s){let c=[];return t.forEach(t=>{let{name:i,displayDirective:a,"display-directive":o}=t.props,s=e=>a===e||o===e,l=n===i;if(t.key!==void 0&&(t.key=i),l||s(`show`)||s(`show:lazy`)&&r.has(i)){r.has(i)||r.add(i);let n=!s(`if`);c.push(n?e(t,[[y,l]]):t)}}),s?p(E,{name:`${s}-transition`,onBeforeLeave:i,onEnter:a,onAfterEnter:o},{default:()=>c}):c}function $(e,t){return p(Q,{ref:`addTabInstRef`,key:`__addable`,name:`__addable`,internalCreatedByPane:!0,internalAddable:!0,internalLeftPadded:t,disabled:typeof e==`object`&&e.disabled})}function ye(e){let t=d(e);return t.props?t.props.internalLeftPadded=!0:t.props={internalLeftPadded:!0},t}function be(e){return Array.isArray(e.dynamicProps)?e.dynamicProps.includes(`internalLeftPadded`)||e.dynamicProps.push(`internalLeftPadded`):e.dynamicProps=[`internalLeftPadded`],e}export{Z as n,_e as t};