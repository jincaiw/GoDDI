import{$ as e,E as t,N as n,P as r,U as i,X as a,Z as o,ct as s,d as c,ft as l,g as u,h as d,j as f,k as p,z as m}from"./echarts-Cw2yHLaZ.js";import{A as h,B as g,C as _,G as v,R as y,q as b}from"./auth-CpMNBSoS.js";import{A as x,O as S,b as ee,d as C,l as w}from"./vue-core-BDTvi3xZ.js";import{D as T,G as E,J as D,K as O,L as k,M as A,N as te,P as ne,X as re,Y as j,Z as M,i as ie,n as ae,z as oe}from"./light-BluJl8SD.js";import{c as se}from"./_plugin-vue_export-helper-DtQO_WtX.js";import{t as ce}from"./use-compitable-OuJoh8Ky.js";import{n as N}from"./InputNumber-CbX73_my.js";import{it as P,r as le,rt as F,st as I}from"./index-BUYVDbRn.js";var L=/\s/;function R(e){for(var t=e.length;t--&&L.test(e.charAt(t)););return t}var z=/^\s+/;function ue(e){return e&&e.slice(0,R(e)+1).replace(z,``)}var B=NaN,de=/^[-+]0x[0-9a-f]+$/i,V=/^0b[01]+$/i,H=/^0o[0-7]+$/i,fe=parseInt;function U(e){if(typeof e==`number`)return e;if(h(e))return B;if(T(e)){var t=typeof e.valueOf==`function`?e.valueOf():e;e=T(t)?t+``:t}if(typeof e!=`string`)return e===0?e:+e;e=ue(e);var n=V.test(e);return n||H.test(e)?fe(e.slice(2),n?2:8):de.test(e)?B:+e}var W=function(){return A.Date.now()},G=`Expected a function`,K=Math.max,q=Math.min;function J(e,t,n){var r,i,a,o,s,c,l=0,u=!1,d=!1,f=!0;if(typeof e!=`function`)throw TypeError(G);t=U(t)||0,T(n)&&(u=!!n.leading,d=`maxWait`in n,a=d?K(U(n.maxWait)||0,t):a,f=`trailing`in n?!!n.trailing:f);function p(t){var n=r,a=i;return r=i=void 0,l=t,o=e.apply(a,n),o}function m(e){return l=e,s=setTimeout(_,t),u?p(e):o}function h(e){var n=e-c,r=e-l,i=t-n;return d?q(i,a-r):i}function g(e){var n=e-c,r=e-l;return c===void 0||n>=t||n<0||d&&r>=a}function _(){var e=W();if(g(e))return v(e);s=setTimeout(_,h(e))}function v(e){return s=void 0,f&&r?p(e):(r=i=void 0,o)}function y(){s!==void 0&&clearTimeout(s),l=0,r=c=i=s=void 0}function b(){return s===void 0?o:v(W())}function x(){var e=W(),n=g(e);if(r=arguments,i=this,c=e,n){if(s===void 0)return m(c);if(d)return clearTimeout(s),s=setTimeout(_,t),p(c)}return s===void 0&&(s=setTimeout(_,t)),o}return x.cancel=y,x.flush=b,x}var Y=`Expected a function`;function pe(e,t,n){var r=!0,i=!0;if(typeof e!=`function`)throw TypeError(Y);return T(n)&&(r=`leading`in n?!!n.leading:r,i=`trailing`in n?!!n.trailing:i),J(e,t,{leading:r,maxWait:t,trailing:i})}var me=oe(`n-tabs`),he={tab:[String,Number,Object,Function],name:{type:[String,Number],required:!0},disabled:Boolean,displayDirective:{type:String,default:`if`},closable:{type:Boolean,default:void 0},tabProps:Object,label:[String,Number,Object,Function]},X=t({__TAB_PANE__:!0,name:`TabPane`,alias:[`TabPanel`],props:he,slots:Object,setup(e){let t=f(me,null);return t||k(`tab-pane`,"`n-tab-pane` must be placed inside `n-tabs`."),{style:t.paneStyleRef,class:t.paneClassRef,mergedClsPrefix:t.mergedClsPrefixRef}},render(){return p(`div`,{class:[`${this.mergedClsPrefix}-tab-pane`,this.class],style:this.style},this.$slots)}}),Z=t({__TAB__:!0,inheritAttrs:!1,name:`Tab`,props:Object.assign({internalLeftPadded:Boolean,internalAddable:Boolean,internalCreatedByPane:Boolean},P(he,[`displayDirective`])),setup(e){let{mergedClsPrefixRef:t,valueRef:n,typeRef:r,closableRef:i,tabStyleRef:a,addTabStyleRef:o,tabClassRef:s,addTabClassRef:c,tabChangeIdRef:l,onBeforeLeaveRef:d,triggerRef:p,handleAdd:m,activateTab:h,handleClose:g}=f(me);return{trigger:p,mergedClosable:u(()=>{if(e.internalAddable)return!1;let{closable:t}=e;return t===void 0?i.value:t}),style:a,addStyle:o,tabClass:s,addTabClass:c,clsPrefix:t,value:n,type:r,handleClose(t){t.stopPropagation(),!e.disabled&&g(e.name)},activateTab(){if(e.disabled)return;if(e.internalAddable){m();return}let{name:t}=e,r=++l.id;if(t!==n.value){let{value:i}=d;i?Promise.resolve(i(e.name,n.value)).then(e=>{e&&l.id===r&&h(t)}):h(t)}}}},render(){let{internalAddable:e,clsPrefix:t,name:r,disabled:i,label:a,tab:o,value:s,mergedClosable:l,trigger:u,$slots:{default:d}}=this,f=a??o;return p(`div`,{class:`${t}-tabs-tab-wrapper`},this.internalLeftPadded?p(`div`,{class:`${t}-tabs-tab-pad`}):null,p(`div`,Object.assign({key:r,"data-name":r,"data-disabled":i?!0:void 0},n({class:[`${t}-tabs-tab`,s===r&&`${t}-tabs-tab--active`,i&&`${t}-tabs-tab--disabled`,l&&`${t}-tabs-tab--closable`,e&&`${t}-tabs-tab--addable`,e?this.addTabClass:this.tabClass],onClick:u===`click`?this.activateTab:void 0,onMouseenter:u===`hover`?this.activateTab:void 0,style:e?this.addStyle:this.style},this.internalCreatedByPane?this.tabProps||{}:this.$attrs)),p(`span`,{class:`${t}-tabs-tab__label`},e?p(c,null,p(`div`,{class:`${t}-tabs-tab__height-placeholder`},`\xA0`),p(ae,{clsPrefix:t},{default:()=>p(N,null)})):d?d():typeof f==`object`?f:F(f??r)),l&&this.type===`card`?p(_,{clsPrefix:t,class:`${t}-tabs-tab__close`,onClick:this.handleClose,disabled:i}):null))}}),ge=O(`tabs`,`
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
 `)])])])]),_e=pe,ve=t({name:`Tabs`,props:Object.assign(Object.assign({},ie.props),{value:[String,Number],defaultValue:[String,Number],trigger:{type:String,default:`click`},type:{type:String,default:`bar`},closable:Boolean,justifyContent:String,size:String,placement:{type:String,default:`top`},tabStyle:[String,Object],tabClass:String,addTabStyle:[String,Object],addTabClass:String,barWidth:Number,paneClass:String,paneStyle:[String,Object],paneWrapperClass:String,paneWrapperStyle:[String,Object],addable:[Boolean,Object],tabsPadding:{type:Number,default:0},animated:Boolean,onBeforeLeave:Function,onAdd:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onClose:[Function,Array],labelSize:String,activeName:[String,Number],onActiveNameChange:[Function,Array]}),slots:Object,setup(e,{slots:t}){let{mergedClsPrefixRef:n,inlineThemeDisabled:c,mergedComponentPropsRef:d}=ne(e),f=ie(`Tabs`,`-tabs`,ge,le,e,n),p=s(null),h=s(null),_=s(null),v=s(null),y=s(null),b=s(null),C=s(!0),w=s(!0),T=ce(e,[`labelSize`,`size`]),E=u(()=>T.value?T.value:d?.value?.Tabs?.size||`medium`),D=ce(e,[`activeName`,`value`]),O=s(D.value??e.defaultValue??(t.default?I(t.default())[0]?.props?.name:null)),k=se(D,O),A={id:0},re=u(()=>{if(!(!e.justifyContent||e.type===`card`))return{display:`flex`,justifyContent:e.justifyContent}});a(k,()=>{A.id=0,P(),F()});function j(){let{value:e}=k;return e===null?null:p.value?.querySelector(`[data-name="${e}"]`)}function ae(t){if(e.type===`card`)return;let{value:r}=h;if(!r)return;let i=r.style.opacity===`0`;if(t){let a=`${n.value}-tabs-bar--disabled`,{barWidth:o,placement:s}=e;if(t.dataset.disabled===`true`?r.classList.add(a):r.classList.remove(a),[`top`,`bottom`].includes(s)){if(N([`top`,`maxHeight`,`height`]),typeof o==`number`&&t.offsetWidth>=o){let e=Math.floor((t.offsetWidth-o)/2)+t.offsetLeft;r.style.left=`${e}px`,r.style.maxWidth=`${o}px`}else r.style.left=`${t.offsetLeft}px`,r.style.maxWidth=`${t.offsetWidth}px`;r.style.width=`8192px`,i&&(r.style.transition=`none`),r.offsetWidth,i&&(r.style.transition=``,r.style.opacity=`1`)}else{if(N([`left`,`maxWidth`,`width`]),typeof o==`number`&&t.offsetHeight>=o){let e=Math.floor((t.offsetHeight-o)/2)+t.offsetTop;r.style.top=`${e}px`,r.style.maxHeight=`${o}px`}else r.style.top=`${t.offsetTop}px`,r.style.maxHeight=`${t.offsetHeight}px`;r.style.height=`8192px`,i&&(r.style.transition=`none`),r.offsetHeight,i&&(r.style.transition=``,r.style.opacity=`1`)}}}function oe(){if(e.type===`card`)return;let{value:t}=h;t&&(t.style.opacity=`0`)}function N(e){let{value:t}=h;if(t)for(let n of e)t.style[n]=``}function P(){if(e.type===`card`)return;let t=j();t?ae(t):oe()}function F(){let e=y.value?.$el;if(!e)return;let t=j();if(!t)return;let{scrollLeft:n,offsetWidth:r}=e,{offsetLeft:i,offsetWidth:a}=t;n>i?e.scrollTo({top:0,left:i,behavior:`smooth`}):i+a>n+r&&e.scrollTo({top:0,left:i+a-r,behavior:`smooth`})}let L=s(null),R=0,z=null;function ue(e){let t=L.value;if(t){R=e.getBoundingClientRect().height;let n=`${R}px`,r=()=>{t.style.height=n,t.style.maxHeight=n};z?(r(),z(),z=null):z=r}}function B(e){let t=L.value;if(t){let n=e.getBoundingClientRect().height,r=()=>{document.body.offsetHeight,t.style.maxHeight=`${n}px`,t.style.height=`${Math.max(R,n)}px`};z?(z(),z=null,r()):z=r}}function de(){let t=L.value;if(t){t.style.maxHeight=``,t.style.height=``;let{paneWrapperStyle:n}=e;if(typeof n==`string`)t.style.cssText=n;else if(n){let{maxHeight:e,height:r}=n;e!==void 0&&(t.style.maxHeight=e),r!==void 0&&(t.style.height=r)}}}let V={value:[]},H=s(`next`);function fe(e){let t=k.value,n=`next`;for(let r of V.value){if(r===t)break;if(r===e){n=`prev`;break}}H.value=n,U(e)}function U(t){let{onActiveNameChange:n,onUpdateValue:r,"onUpdate:value":i}=e;n&&g(n,t),r&&g(r,t),i&&g(i,t),O.value=t}function W(t){let{onClose:n}=e;n&&g(n,t)}let G=!0;function K(){let{value:e}=h;if(!e)return;G||=!1;let t=`transition-disabled`;e.classList.add(t),P(),e.classList.remove(t)}let q=s(null);function J({transitionDisabled:e}){let t=p.value;if(!t)return;e&&t.classList.add(`transition-disabled`);let n=j();n&&q.value&&(q.value.style.width=`${n.offsetWidth}px`,q.value.style.height=`${n.offsetHeight}px`,q.value.style.transform=`translateX(${n.offsetLeft-S(getComputedStyle(t).paddingLeft)}px)`,e&&q.value.offsetWidth),e&&t.classList.remove(`transition-disabled`)}a([k],()=>{e.type===`segment`&&r(()=>{J({transitionDisabled:!1})})}),m(()=>{e.type===`segment`&&J({transitionDisabled:!0})});let Y=0;function pe(t){if(t.contentRect.width===0&&t.contentRect.height===0||Y===t.contentRect.width)return;Y=t.contentRect.width;let{type:n}=e;if((n===`line`||n===`bar`)&&(G||e.justifyContent?.startsWith(`space`))&&K(),n!==`segment`){let{placement:t}=e;Q((t===`top`||t===`bottom`?y.value?.$el:b.value)||null)}}let he=_e(pe,64);a([()=>e.justifyContent,()=>e.size],()=>{r(()=>{let{type:t}=e;(t===`line`||t===`bar`)&&K()})});let X=s(!1);function Z(t){let{target:n,contentRect:{width:r,height:i}}=t,a=n.parentElement.parentElement.offsetWidth,o=n.parentElement.parentElement.offsetHeight,{placement:s}=e;if(!X.value)s===`top`||s===`bottom`?a<r&&(X.value=!0):o<i&&(X.value=!0);else{let{value:e}=v;if(!e)return;s===`top`||s===`bottom`?a-r>e.$el.offsetWidth&&(X.value=!1):o-i>e.$el.offsetHeight&&(X.value=!1)}Q(y.value?.$el||null)}let ve=_e(Z,64);function ye(){let{onAdd:t}=e;t&&t(),r(()=>{let e=j(),{value:t}=y;!e||!t||t.scrollTo({left:e.offsetLeft,top:0,behavior:`smooth`})})}function Q(t){if(!t)return;let{placement:n}=e;if(n===`top`||n===`bottom`){let{scrollLeft:e,scrollWidth:n,offsetWidth:r}=t;C.value=e<=0,w.value=e+r>=n}else{let{scrollTop:e,scrollHeight:n,offsetHeight:r}=t;C.value=e<=0,w.value=e+r>=n}}let be=_e(e=>{Q(e.target)},64);i(me,{triggerRef:l(e,`trigger`),tabStyleRef:l(e,`tabStyle`),tabClassRef:l(e,`tabClass`),addTabStyleRef:l(e,`addTabStyle`),addTabClassRef:l(e,`addTabClass`),paneClassRef:l(e,`paneClass`),paneStyleRef:l(e,`paneStyle`),mergedClsPrefixRef:n,typeRef:l(e,`type`),closableRef:l(e,`closable`),valueRef:k,tabChangeIdRef:A,onBeforeLeaveRef:l(e,`onBeforeLeave`),activateTab:fe,handleClose:W,handleAdd:ye}),ee(()=>{P(),F()}),o(()=>{let{value:e}=_;if(!e)return;let{value:t}=n,r=`${t}-tabs-nav-scroll-wrapper--shadow-start`,i=`${t}-tabs-nav-scroll-wrapper--shadow-end`;C.value?e.classList.remove(r):e.classList.add(r),w.value?e.classList.remove(i):e.classList.add(i)});let $={syncBarPosition:()=>{P()}},xe=()=>{J({transitionDisabled:!0})},Se=u(()=>{let{value:t}=E,{type:n}=e,r=`${t}${{card:`Card`,bar:`Bar`,line:`Line`,segment:`Segment`}[n]}`,{self:{barColor:i,closeIconColor:a,closeIconColorHover:o,closeIconColorPressed:s,tabColor:c,tabBorderColor:l,paneTextColor:u,tabFontWeight:d,tabBorderRadius:p,tabFontWeightActive:m,colorSegment:h,fontWeightStrong:g,tabColorSegment:_,closeSize:v,closeIconSize:y,closeColorHover:b,closeColorPressed:S,closeBorderRadius:ee,[M(`panePadding`,t)]:C,[M(`tabPadding`,r)]:w,[M(`tabPaddingVertical`,r)]:T,[M(`tabGap`,r)]:D,[M(`tabGap`,`${r}Vertical`)]:O,[M(`tabTextColor`,n)]:k,[M(`tabTextColorActive`,n)]:A,[M(`tabTextColorHover`,n)]:te,[M(`tabTextColorDisabled`,n)]:ne,[M(`tabFontSize`,t)]:re},common:{cubicBezierEaseInOut:j}}=f.value;return{"--n-bezier":j,"--n-color-segment":h,"--n-bar-color":i,"--n-tab-font-size":re,"--n-tab-text-color":k,"--n-tab-text-color-active":A,"--n-tab-text-color-disabled":ne,"--n-tab-text-color-hover":te,"--n-pane-text-color":u,"--n-tab-border-color":l,"--n-tab-border-radius":p,"--n-close-size":v,"--n-close-icon-size":y,"--n-close-color-hover":b,"--n-close-color-pressed":S,"--n-close-border-radius":ee,"--n-close-icon-color":a,"--n-close-icon-color-hover":o,"--n-close-icon-color-pressed":s,"--n-tab-color":c,"--n-tab-font-weight":d,"--n-tab-font-weight-active":m,"--n-tab-padding":w,"--n-tab-padding-vertical":T,"--n-tab-gap":D,"--n-tab-gap-vertical":O,"--n-pane-padding-left":x(C,`left`),"--n-pane-padding-right":x(C,`right`),"--n-pane-padding-top":x(C,`top`),"--n-pane-padding-bottom":x(C,`bottom`),"--n-font-weight-strong":g,"--n-tab-color-segment":_}}),Ce=c?te(`tabs`,u(()=>`${E.value[0]}${e.type[0]}`),Se,e):void 0;return Object.assign({mergedClsPrefix:n,mergedValue:k,renderedNames:new Set,segmentCapsuleElRef:q,tabsPaneWrapperRef:L,tabsElRef:p,barElRef:h,addTabInstRef:v,xScrollInstRef:y,scrollWrapperElRef:_,addTabFixed:X,tabWrapperStyle:re,handleNavResize:he,mergedSize:E,handleScroll:be,handleTabsResize:ve,cssVars:c?void 0:Se,themeClass:Ce?.themeClass,animationDirection:H,renderNameListRef:V,yScrollElRef:b,handleSegmentResize:xe,onAnimationBeforeLeave:ue,onAnimationEnter:B,onAnimationAfterEnter:de,onRender:Ce?.onRender},$)},render(){let{mergedClsPrefix:e,type:t,placement:n,addTabFixed:r,addable:i,mergedSize:a,renderNameListRef:o,onRender:s,paneWrapperClass:c,paneWrapperStyle:l,$slots:{default:u,prefix:d,suffix:f}}=this;s?.();let m=u?I(u()).filter(e=>e.type.__TAB_PANE__===!0):[],h=u?I(u()).filter(e=>e.type.__TAB__===!0):[],g=!h.length,_=t===`card`,v=t===`segment`,b=!_&&!v&&this.justifyContent;o.value=[];let x=()=>{let t=p(`div`,{style:this.tabWrapperStyle,class:`${e}-tabs-wrapper`},b?null:p(`div`,{class:`${e}-tabs-scroll-padding`,style:n===`top`||n===`bottom`?{width:`${this.tabsPadding}px`}:{height:`${this.tabsPadding}px`}}),g?m.map((e,t)=>(o.value.push(e.props.name),$(p(Z,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0&&(!b||b===`center`||b===`start`||b===`end`)}),e.children?{default:e.children.tab}:void 0)))):h.map((e,t)=>(o.value.push(e.props.name),$(t!==0&&!b?be(e):e))),!r&&i&&_?Q(i,(g?m.length:h.length)!==0):null,b?null:p(`div`,{class:`${e}-tabs-scroll-padding`,style:{width:`${this.tabsPadding}px`}}));return p(`div`,{ref:`tabsElRef`,class:`${e}-tabs-nav-scroll-content`},_&&i?p(C,{onResize:this.handleTabsResize},{default:()=>t}):t,_?p(`div`,{class:`${e}-tabs-pad`}):null,_?null:p(`div`,{ref:`barElRef`,class:`${e}-tabs-bar`}))},S=v?`top`:n;return p(`div`,{class:[`${e}-tabs`,this.themeClass,`${e}-tabs--${t}-type`,`${e}-tabs--${a}-size`,b&&`${e}-tabs--flex`,`${e}-tabs--${S}`],style:this.cssVars},p(`div`,{class:[`${e}-tabs-nav--${t}-type`,`${e}-tabs-nav--${S}`,`${e}-tabs-nav`]},y(d,t=>t&&p(`div`,{class:`${e}-tabs-nav__prefix`},t)),v?p(C,{onResize:this.handleSegmentResize},{default:()=>p(`div`,{class:`${e}-tabs-rail`,ref:`tabsElRef`},p(`div`,{class:`${e}-tabs-capsule`,ref:`segmentCapsuleElRef`},p(`div`,{class:`${e}-tabs-wrapper`},p(`div`,{class:`${e}-tabs-tab`}))),g?m.map((e,t)=>(o.value.push(e.props.name),p(Z,Object.assign({},e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0}),e.children?{default:e.children.tab}:void 0))):h.map((e,t)=>(o.value.push(e.props.name),t===0?e:be(e))))}):p(C,{onResize:this.handleNavResize},{default:()=>p(`div`,{class:`${e}-tabs-nav-scroll-wrapper`,ref:`scrollWrapperElRef`},[`top`,`bottom`].includes(S)?p(w,{ref:`xScrollInstRef`,onScroll:this.handleScroll},{default:x}):p(`div`,{class:`${e}-tabs-nav-y-scroll`,onScroll:this.handleScroll,ref:`yScrollElRef`},x()))}),r&&i&&_?Q(i,!0):null,y(f,t=>t&&p(`div`,{class:`${e}-tabs-nav__suffix`},t))),g&&(this.animated&&(S===`top`||S===`bottom`)?p(`div`,{ref:`tabsPaneWrapperRef`,style:l,class:[`${e}-tabs-pane-wrapper`,c]},ye(m,this.mergedValue,this.renderedNames,this.onAnimationBeforeLeave,this.onAnimationEnter,this.onAnimationAfterEnter,this.animationDirection)):ye(m,this.mergedValue,this.renderedNames)))}});function ye(t,n,r,i,a,o,s){let c=[];return t.forEach(t=>{let{name:i,displayDirective:a,"display-directive":o}=t.props,s=e=>a===e||o===e,l=n===i;if(t.key!==void 0&&(t.key=i),l||s(`show`)||s(`show:lazy`)&&r.has(i)){r.has(i)||r.add(i);let n=!s(`if`);c.push(n?e(t,[[b,l]]):t)}}),s?p(v,{name:`${s}-transition`,onBeforeLeave:i,onEnter:a,onAfterEnter:o},{default:()=>c}):c}function Q(e,t){return p(Z,{ref:`addTabInstRef`,key:`__addable`,name:`__addable`,internalCreatedByPane:!0,internalAddable:!0,internalLeftPadded:t,disabled:typeof e==`object`&&e.disabled})}function be(e){let t=d(e);return t.props?t.props.internalLeftPadded=!0:t.props={internalLeftPadded:!0},t}function $(e){return Array.isArray(e.dynamicProps)?e.dynamicProps.includes(`internalLeftPadded`)||e.dynamicProps.push(`internalLeftPadded`):e.dynamicProps=[`internalLeftPadded`],e}export{X as n,ve as t};