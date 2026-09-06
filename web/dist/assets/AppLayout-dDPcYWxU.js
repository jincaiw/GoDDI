import{$ as e,A as t,At as n,B as r,C as i,D as a,G as o,M as s,P as c,Q as l,R as u,T as d,U as f,V as p,W as m,Z as h,_ as g,b as _,et as v,f as y,ht as b,kt as x,lt as S,pt as C,q as w,v as T,w as ee,x as E,y as D}from"./echarts-Bmuv2i_G.js";import{G as O,H as k,K as A,R as j,T as te,V as M,W as N,Y as P,_ as F,f as ne,k as I,t as L,w as R,x as re,y as ie}from"./auth-BTeG-_sB.js";import{C as ae,E as oe,c as se,d as ce,g as le,i as ue,m as de,o as fe,r as pe,s as me,y as he}from"./vue-core-bFnGnCI4.js";import{G as z,J as B,K as V,L as ge,N as H,P as U,V as _e,X as W,Y as G,i as K,n as ve,o as ye,r as be,t as xe,z as q}from"./light-B4Ik6WNX.js";import{a as Se,n as Ce,s as we,t as Te}from"./Dropdown-QJb02pGj.js";import{c as J,s as Y,t as Ee}from"./_plugin-vue_export-helper-BHHysIWo.js";import{t as De}from"./use-compitable-BMUbnpEM.js";import{t as X}from"./Icon-BJnisG2B.js";import{t as Oe}from"./Switch-BydDc8YI.js";import{at as ke,c as Ae,ct as je,d as Me,dt as Ne,ft as Pe,gt as Fe,ht as Ie,j as Le,k as Re,mt as ze,pt as Be,rt as Z,t as Ve,ut as He}from"./index-Yh_aUYzb.js";import{i as Ue,n as We,r as Ge,t as Ke}from"./ServerOutline-BqDRx0QR.js";import{t as qe}from"./PersonOutline-f3Mzqf3w.js";import{t as Je}from"./SearchOutline-BRqAKFrn.js";var Ye=a({name:`ChevronDownFilled`,render(){return t(`svg`,{viewBox:`0 0 16 16`,fill:`none`,xmlns:`http://www.w3.org/2000/svg`},t(`path`,{d:`M3.20041 5.73966C3.48226 5.43613 3.95681 5.41856 4.26034 5.70041L8 9.22652L11.7397 5.70041C12.0432 5.41856 12.5177 5.43613 12.7996 5.73966C13.0815 6.0432 13.0639 6.51775 12.7603 6.7996L8.51034 10.7996C8.22258 11.0668 7.77743 11.0668 7.48967 10.7996L3.23966 6.7996C2.93613 6.51775 2.91856 6.0432 3.20041 5.73966Z`,fill:`currentColor`}))}}),Xe=V(`breadcrumb`,`
 white-space: nowrap;
 cursor: default;
 line-height: var(--n-item-line-height);
`,[z(`ul`,`
 list-style: none;
 padding: 0;
 margin: 0;
 `),z(`a`,`
 color: inherit;
 text-decoration: inherit;
 `),V(`breadcrumb-item`,`
 font-size: var(--n-font-size);
 transition: color .3s var(--n-bezier);
 display: inline-flex;
 align-items: center;
 `,[V(`icon`,`
 font-size: 18px;
 vertical-align: -.2em;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `),z(`&:not(:last-child)`,[G(`clickable`,[B(`link`,`
 cursor: pointer;
 `,[z(`&:hover`,`
 background-color: var(--n-item-color-hover);
 `),z(`&:active`,`
 background-color: var(--n-item-color-pressed); 
 `)])])]),B(`link`,`
 padding: 4px;
 border-radius: var(--n-item-border-radius);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 position: relative;
 `,[z(`&:hover`,`
 color: var(--n-item-text-color-hover);
 `,[V(`icon`,`
 color: var(--n-item-text-color-hover);
 `)]),z(`&:active`,`
 color: var(--n-item-text-color-pressed);
 `,[V(`icon`,`
 color: var(--n-item-text-color-pressed);
 `)])]),B(`separator`,`
 margin: 0 8px;
 color: var(--n-separator-color);
 transition: color .3s var(--n-bezier);
 user-select: none;
 -webkit-user-select: none;
 `),z(`&:last-child`,[B(`link`,`
 font-weight: var(--n-font-weight-active);
 cursor: unset;
 color: var(--n-item-text-color-active);
 `,[V(`icon`,`
 color: var(--n-item-text-color-active);
 `)]),B(`separator`,`
 display: none;
 `)])])]),Ze=q(`n-breadcrumb`),Qe=a({name:`Breadcrumb`,props:Object.assign(Object.assign({},K.props),{separator:{type:String,default:`/`}}),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=U(e),r=K(`Breadcrumb`,`-breadcrumb`,Xe,Re,e,t);m(Ze,{separatorRef:C(e,`separator`),mergedClsPrefixRef:t});let i=g(()=>{let{common:{cubicBezierEaseInOut:e},self:{separatorColor:t,itemTextColor:n,itemTextColorHover:i,itemTextColorPressed:a,itemTextColorActive:o,fontSize:s,fontWeightActive:c,itemBorderRadius:l,itemColorHover:u,itemColorPressed:d,itemLineHeight:f}}=r.value;return{"--n-font-size":s,"--n-bezier":e,"--n-item-text-color":n,"--n-item-text-color-hover":i,"--n-item-text-color-pressed":a,"--n-item-text-color-active":o,"--n-separator-color":t,"--n-item-color-hover":u,"--n-item-color-pressed":d,"--n-item-border-radius":l,"--n-font-weight-active":c,"--n-item-line-height":f}}),a=n?H(`breadcrumb`,void 0,i,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),t(`nav`,{class:[`${this.mergedClsPrefix}-breadcrumb`,this.themeClass],style:this.cssVars,"aria-label":`Breadcrumb`},t(`ul`,null,this.$slots))}});function $e(e=O?window:null){let t=()=>{let{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}=e?.location||{};return{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}},n=S(t()),i=()=>{n.value=t()};return r(()=>{e&&(e.addEventListener(`popstate`,i),e.addEventListener(`hashchange`,i))}),p(()=>{e&&(e.removeEventListener(`popstate`,i),e.removeEventListener(`hashchange`,i))}),n}var et=a({name:`BreadcrumbItem`,props:{separator:String,href:String,clickable:{type:Boolean,default:!0},showSeparator:{type:Boolean,default:!0},onClick:Function},slots:Object,setup(e,{slots:n}){let r=s(Ze,null);if(!r)return()=>null;let{separatorRef:i,mergedClsPrefixRef:a}=r,o=$e(),c=g(()=>e.href?`a`:`span`),l=g(()=>o.value.href===e.href?`location`:null);return()=>{let{value:r}=a;return t(`li`,{class:[`${r}-breadcrumb-item`,e.clickable&&`${r}-breadcrumb-item--clickable`]},t(c.value,{class:`${r}-breadcrumb-item__link`,"aria-current":l.value,href:e.href,onClick:e.onClick},n),e.showSeparator&&t(`span`,{class:`${r}-breadcrumb-item__separator`,"aria-hidden":`true`},j(n.separator,()=>[e.separator??i.value])))}}}),tt=a({name:`NDrawerContent`,inheritAttrs:!1,props:{blockScroll:Boolean,show:{type:Boolean,default:void 0},displayDirective:{type:String,required:!0},placement:{type:String,required:!0},contentClass:String,contentStyle:[Object,String],nativeScrollbar:{type:Boolean,required:!0},scrollbarProps:Object,trapFocus:{type:Boolean,default:!0},autoFocus:{type:Boolean,default:!0},showMask:{type:[Boolean,String],required:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,onClickoutside:Function,onAfterLeave:Function,onAfterEnter:Function,onEsc:Function},setup(e){let t=S(!!e.show),n=S(null),r=s(Fe),i=0,a=``,o=null,c=S(!1),d=S(!1),f=g(()=>e.placement===`top`||e.placement===`bottom`),{mergedClsPrefixRef:p,mergedRtlRef:_}=U(e),v=I(`Drawer`,_,p),y=O,b=e=>{d.value=!0,i=f.value?e.clientY:e.clientX,a=document.body.style.cursor,document.body.style.cursor=f.value?`ns-resize`:`ew-resize`,document.body.addEventListener(`mousemove`,D),document.body.addEventListener(`mouseleave`,y),document.body.addEventListener(`mouseup`,O)},x=()=>{o!==null&&(window.clearTimeout(o),o=null),d.value?c.value=!0:o=window.setTimeout(()=>{c.value=!0},300)},C=()=>{o!==null&&(window.clearTimeout(o),o=null),c.value=!1},{doUpdateHeight:w,doUpdateWidth:T}=r,ee=t=>{let{maxWidth:n}=e;if(n&&t>n)return n;let{minWidth:r}=e;return r&&t<r?r:t},E=t=>{let{maxHeight:n}=e;if(n&&t>n)return n;let{minHeight:r}=e;return r&&t<r?r:t};function D(t){if(d.value)if(f.value){let r=n.value?.offsetHeight||0,a=i-t.clientY;r+=e.placement===`bottom`?a:-a,r=E(r),w(r),i=t.clientY}else{let r=n.value?.offsetWidth||0,a=i-t.clientX;r+=e.placement===`right`?a:-a,r=ee(r),T(r),i=t.clientX}}function O(){d.value&&(i=0,d.value=!1,document.body.style.cursor=a,document.body.removeEventListener(`mousemove`,D),document.body.removeEventListener(`mouseup`,O),document.body.removeEventListener(`mouseleave`,y))}l(()=>{e.show&&(t.value=!0)}),h(()=>e.show,e=>{e||O()}),u(()=>{O()});let k=g(()=>{let{show:t}=e,n=[[P,t]];return e.showMask||n.push([He,e.onClickoutside,void 0,{capture:!0}]),n});function A(){var n;t.value=!1,(n=e.onAfterLeave)==null||n.call(e)}return Ne(g(()=>e.blockScroll&&t.value)),m(Ie,n),m(Be,null),m(ze,null),{bodyRef:n,rtlEnabled:v,mergedClsPrefix:r.mergedClsPrefixRef,isMounted:r.isMountedRef,mergedTheme:r.mergedThemeRef,displayed:t,transitionName:g(()=>({right:`slide-in-from-right-transition`,left:`slide-in-from-left-transition`,top:`slide-in-from-top-transition`,bottom:`slide-in-from-bottom-transition`})[e.placement]),handleAfterLeave:A,bodyDirectives:k,handleMousedownResizeTrigger:b,handleMouseenterResizeTrigger:x,handleMouseleaveResizeTrigger:C,isDragging:d,isHoverOnResizeTrigger:c}},render(){let{$slots:e,mergedClsPrefix:n}=this;return this.displayDirective===`show`||this.displayed||this.show?v(t(`div`,{role:`none`},t(me,{disabled:!this.showMask||!this.trapFocus,active:this.show,autoFocus:this.autoFocus,onEsc:this.onEsc},{default:()=>t(A,{name:this.transitionName,appear:this.isMounted,onAfterEnter:this.onAfterEnter,onAfterLeave:this.handleAfterLeave},{default:()=>v(t(`div`,c(this.$attrs,{role:`dialog`,ref:`bodyRef`,"aria-modal":`true`,class:[`${n}-drawer`,this.rtlEnabled&&`${n}-drawer--rtl`,`${n}-drawer--${this.placement}-placement`,this.isDragging&&`${n}-drawer--unselectable`,this.nativeScrollbar&&`${n}-drawer--native-scrollbar`]}),[this.resizable?t(`div`,{class:[`${n}-drawer__resize-trigger`,(this.isDragging||this.isHoverOnResizeTrigger)&&`${n}-drawer__resize-trigger--hover`],onMouseenter:this.handleMouseenterResizeTrigger,onMouseleave:this.handleMouseleaveResizeTrigger,onMousedown:this.handleMousedownResizeTrigger}):null,this.nativeScrollbar?t(`div`,{class:[`${n}-drawer-content-wrapper`,this.contentClass],style:this.contentStyle,role:`none`},e):t(F,Object.assign({},this.scrollbarProps,{contentStyle:this.contentStyle,contentClass:[`${n}-drawer-content-wrapper`,this.contentClass],theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar}),e)]),this.bodyDirectives)})})),[[P,this.displayDirective===`if`||this.displayed||this.show]]):null}}),{cubicBezierEaseIn:nt,cubicBezierEaseOut:rt}=ye;function it({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-bottom`}={}){return[z(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${nt}`}),z(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${rt}`}),z(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),z(`&.${n}-transition-enter-from`,{transform:`translateY(100%)`}),z(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),z(`&.${n}-transition-leave-to`,{transform:`translateY(100%)`})]}var{cubicBezierEaseIn:at,cubicBezierEaseOut:ot}=ye;function st({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-left`}={}){return[z(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${at}`}),z(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ot}`}),z(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),z(`&.${n}-transition-enter-from`,{transform:`translateX(-100%)`}),z(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),z(`&.${n}-transition-leave-to`,{transform:`translateX(-100%)`})]}var{cubicBezierEaseIn:ct,cubicBezierEaseOut:lt}=ye;function ut({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-right`}={}){return[z(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ct}`}),z(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${lt}`}),z(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),z(`&.${n}-transition-enter-from`,{transform:`translateX(100%)`}),z(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),z(`&.${n}-transition-leave-to`,{transform:`translateX(100%)`})]}var{cubicBezierEaseIn:dt,cubicBezierEaseOut:ft}=ye;function pt({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-top`}={}){return[z(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${dt}`}),z(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ft}`}),z(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),z(`&.${n}-transition-enter-from`,{transform:`translateY(-100%)`}),z(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),z(`&.${n}-transition-leave-to`,{transform:`translateY(-100%)`})]}var mt=z([V(`drawer`,`
 word-break: break-word;
 line-height: var(--n-line-height);
 position: absolute;
 pointer-events: all;
 box-shadow: var(--n-box-shadow);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 background-color: var(--n-color);
 color: var(--n-text-color);
 box-sizing: border-box;
 `,[ut(),st(),pt(),it(),G(`unselectable`,`
 user-select: none; 
 -webkit-user-select: none;
 `),G(`native-scrollbar`,[V(`drawer-content-wrapper`,`
 overflow: auto;
 height: 100%;
 `)]),B(`resize-trigger`,`
 position: absolute;
 background-color: #0000;
 transition: background-color .3s var(--n-bezier);
 `,[G(`hover`,`
 background-color: var(--n-resize-trigger-color-hover);
 `)]),V(`drawer-content-wrapper`,`
 box-sizing: border-box;
 `),V(`drawer-content`,`
 height: 100%;
 display: flex;
 flex-direction: column;
 `,[G(`native-scrollbar`,[V(`drawer-body-content-wrapper`,`
 height: 100%;
 overflow: auto;
 `)]),V(`drawer-body`,`
 flex: 1 0 0;
 overflow: hidden;
 `),V(`drawer-body-content-wrapper`,`
 box-sizing: border-box;
 padding: var(--n-body-padding);
 `),V(`drawer-header`,`
 font-weight: var(--n-title-font-weight);
 line-height: 1;
 font-size: var(--n-title-font-size);
 color: var(--n-title-text-color);
 padding: var(--n-header-padding);
 transition: border .3s var(--n-bezier);
 border-bottom: 1px solid var(--n-divider-color);
 border-bottom: var(--n-header-border-bottom);
 display: flex;
 justify-content: space-between;
 align-items: center;
 `,[B(`main`,`
 flex: 1;
 `),B(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)]),V(`drawer-footer`,`
 display: flex;
 justify-content: flex-end;
 border-top: var(--n-footer-border-top);
 transition: border .3s var(--n-bezier);
 padding: var(--n-footer-padding);
 `)]),G(`right-placement`,`
 top: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-bottom-left-radius: var(--n-border-radius);
 `,[B(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 left: 0;
 transform: translateX(-1.5px);
 cursor: ew-resize;
 `)]),G(`left-placement`,`
 top: 0;
 bottom: 0;
 left: 0;
 border-top-right-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[B(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 right: 0;
 transform: translateX(1.5px);
 cursor: ew-resize;
 `)]),G(`top-placement`,`
 top: 0;
 left: 0;
 right: 0;
 border-bottom-left-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[B(`resize-trigger`,`
 width: 100%;
 height: 3px;
 bottom: 0;
 left: 0;
 transform: translateY(1.5px);
 cursor: ns-resize;
 `)]),G(`bottom-placement`,`
 left: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-top-right-radius: var(--n-border-radius);
 `,[B(`resize-trigger`,`
 width: 100%;
 height: 3px;
 top: 0;
 left: 0;
 transform: translateY(-1.5px);
 cursor: ns-resize;
 `)])]),z(`body`,[z(`>`,[V(`drawer-container`,`
 position: fixed;
 `)])]),V(`drawer-container`,`
 position: relative;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 `,[z(`> *`,`
 pointer-events: all;
 `)]),V(`drawer-mask`,`
 background-color: rgba(0, 0, 0, .3);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `,[G(`invisible`,`
 background-color: rgba(0, 0, 0, 0)
 `),re({enterDuration:`0.2s`,leaveDuration:`0.2s`,enterCubicBezier:`var(--n-bezier-in)`,leaveCubicBezier:`var(--n-bezier-out)`})])]),ht=a({name:`Drawer`,inheritAttrs:!1,props:Object.assign(Object.assign({},K.props),{show:Boolean,width:[Number,String],height:[Number,String],placement:{type:String,default:`right`},maskClosable:{type:Boolean,default:!0},showMask:{type:[Boolean,String],default:!0},to:[String,Object],displayDirective:{type:String,default:`if`},nativeScrollbar:{type:Boolean,default:!0},zIndex:Number,onMaskClick:Function,scrollbarProps:Object,contentClass:String,contentStyle:[Object,String],trapFocus:{type:Boolean,default:!0},onEsc:Function,autoFocus:{type:Boolean,default:!0},closeOnEsc:{type:Boolean,default:!0},blockScroll:{type:Boolean,default:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,defaultWidth:{type:[Number,String],default:251},defaultHeight:{type:[Number,String],default:251},onUpdateWidth:[Function,Array],onUpdateHeight:[Function,Array],"onUpdate:width":[Function,Array],"onUpdate:height":[Function,Array],"onUpdate:show":[Function,Array],onUpdateShow:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,drawerStyle:[String,Object],drawerClass:String,target:null,onShow:Function,onHide:Function}),setup(e){let{mergedClsPrefixRef:t,namespaceRef:n,inlineThemeDisabled:r}=U(e),i=he(),a=K(`Drawer`,`-drawer`,mt,Me,e,t),o=S(e.defaultWidth),s=S(e.defaultHeight),c=J(C(e,`width`),o),l=J(C(e,`height`),s),u=g(()=>{let{placement:t}=e;return t===`top`||t===`bottom`?``:Y(c.value)}),d=g(()=>{let{placement:t}=e;return t===`left`||t===`right`?``:Y(l.value)}),f=t=>{let{onUpdateWidth:n,"onUpdate:width":r}=e;n&&k(n,t),r&&k(r,t),o.value=t},p=t=>{let{onUpdateHeight:n,"onUpdate:width":r}=e;n&&k(n,t),r&&k(r,t),s.value=t},h=g(()=>[{width:u.value,height:d.value},e.drawerStyle||``]);function _(t){let{onMaskClick:n,maskClosable:r}=e;r&&x(!1),n&&n(t)}function v(e){_(e)}let y=Pe();function b(t){var n;(n=e.onEsc)==null||n.call(e),e.show&&e.closeOnEsc&&je(t)&&(y.value||x(!1))}function x(t){let{onHide:n,onUpdateShow:r,"onUpdate:show":i}=e;r&&k(r,t),i&&k(i,t),n&&!t&&k(n,t)}m(Fe,{isMountedRef:i,mergedThemeRef:a,mergedClsPrefixRef:t,doUpdateShow:x,doUpdateHeight:p,doUpdateWidth:f});let w=g(()=>{let{common:{cubicBezierEaseInOut:e,cubicBezierEaseIn:t,cubicBezierEaseOut:n},self:{color:r,textColor:i,boxShadow:o,lineHeight:s,headerPadding:c,footerPadding:l,borderRadius:u,bodyPadding:d,titleFontSize:f,titleTextColor:p,titleFontWeight:m,headerBorderBottom:h,footerBorderTop:g,closeIconColor:_,closeIconColorHover:v,closeIconColorPressed:y,closeColorHover:b,closeColorPressed:x,closeIconSize:S,closeSize:C,closeBorderRadius:w,resizableTriggerColorHover:T}}=a.value;return{"--n-line-height":s,"--n-color":r,"--n-border-radius":u,"--n-text-color":i,"--n-box-shadow":o,"--n-bezier":e,"--n-bezier-out":n,"--n-bezier-in":t,"--n-header-padding":c,"--n-body-padding":d,"--n-footer-padding":l,"--n-title-text-color":p,"--n-title-font-size":f,"--n-title-font-weight":m,"--n-header-border-bottom":h,"--n-footer-border-top":g,"--n-close-icon-color":_,"--n-close-icon-color-hover":v,"--n-close-icon-color-pressed":y,"--n-close-size":C,"--n-close-color-hover":b,"--n-close-color-pressed":x,"--n-close-icon-size":S,"--n-close-border-radius":w,"--n-resize-trigger-color-hover":T}}),T=r?H(`drawer`,void 0,w,e):void 0;return{mergedClsPrefix:t,namespace:n,mergedBodyStyle:h,handleOutsideClick:v,handleMaskClick:_,handleEsc:b,mergedTheme:a,cssVars:r?void 0:w,themeClass:T?.themeClass,onRender:T?.onRender,isMounted:i}},render(){let{mergedClsPrefix:e}=this;return t(de,{to:this.to,show:this.show},{default:()=>{var n;return(n=this.onRender)==null||n.call(this),v(t(`div`,{class:[`${e}-drawer-container`,this.namespace,this.themeClass],style:this.cssVars,role:`none`},this.showMask?t(A,{name:`fade-in-transition`,appear:this.isMounted},{default:()=>this.show?t(`div`,{"aria-hidden":!0,class:[`${e}-drawer-mask`,this.showMask===`transparent`&&`${e}-drawer-mask--invisible`],onClick:this.handleMaskClick}):null}):null,t(tt,Object.assign({},this.$attrs,{class:[this.drawerClass,this.$attrs.class],style:[this.mergedBodyStyle,this.$attrs.style],blockScroll:this.blockScroll,contentStyle:this.contentStyle,contentClass:this.contentClass,placement:this.placement,scrollbarProps:this.scrollbarProps,show:this.show,displayDirective:this.displayDirective,nativeScrollbar:this.nativeScrollbar,onAfterEnter:this.onAfterEnter,onAfterLeave:this.onAfterLeave,trapFocus:this.trapFocus,autoFocus:this.autoFocus,resizable:this.resizable,maxHeight:this.maxHeight,minHeight:this.minHeight,maxWidth:this.maxWidth,minWidth:this.minWidth,showMask:this.showMask,onEsc:this.handleEsc,onClickoutside:this.handleOutsideClick}),this.$slots)),[[le,{zIndex:this.zIndex,enabled:this.show}]])}})}}),gt=a({name:`DrawerContent`,props:{title:String,headerClass:String,headerStyle:[Object,String],footerClass:String,footerStyle:[Object,String],bodyClass:String,bodyStyle:[Object,String],bodyContentClass:String,bodyContentStyle:[Object,String],nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,closable:Boolean},slots:Object,setup(){let e=s(Fe,null);e||ge(`drawer-content`,"`n-drawer-content` must be placed inside `n-drawer`.");let{doUpdateShow:t}=e;function n(){t(!1)}return{handleCloseClick:n,mergedTheme:e.mergedThemeRef,mergedClsPrefix:e.mergedClsPrefixRef}},render(){let{title:e,mergedClsPrefix:n,nativeScrollbar:r,mergedTheme:i,bodyClass:a,bodyStyle:o,bodyContentClass:s,bodyContentStyle:c,headerClass:l,headerStyle:u,footerClass:d,footerStyle:f,scrollbarProps:p,closable:m,$slots:h}=this;return t(`div`,{role:`none`,class:[`${n}-drawer-content`,r&&`${n}-drawer-content--native-scrollbar`]},h.header||e||m?t(`div`,{class:[`${n}-drawer-header`,l],style:u,role:`none`},t(`div`,{class:`${n}-drawer-header__main`,role:`heading`,"aria-level":`1`},h.header===void 0?e:h.header()),m&&t(te,{onClick:this.handleCloseClick,clsPrefix:n,class:`${n}-drawer-header__close`,absolute:!0})):null,r?t(`div`,{class:[`${n}-drawer-body`,a],style:o,role:`none`},t(`div`,{class:[`${n}-drawer-body-content-wrapper`,s],style:c,role:`none`},h)):t(F,Object.assign({themeOverrides:i.peerOverrides.Scrollbar,theme:i.peers.Scrollbar},p,{class:`${n}-drawer-body`,contentClass:[`${n}-drawer-body-content-wrapper`,s],contentStyle:c}),h),h.footer?t(`div`,{class:[`${n}-drawer-footer`,d],style:f,role:`none`},h.footer()):null)}});function _t(e){let{baseColor:t,textColor2:n,bodyColor:r,cardColor:i,dividerColor:a,actionColor:o,scrollbarColor:s,scrollbarColorHover:c,invertedColor:l}=e;return{textColor:n,textColorInverted:`#FFF`,color:r,colorEmbedded:o,headerColor:i,headerColorInverted:l,footerColor:o,footerColorInverted:l,headerBorderColor:a,headerBorderColorInverted:l,footerBorderColor:a,footerBorderColorInverted:l,siderBorderColor:a,siderBorderColorInverted:l,siderColor:i,siderColorInverted:l,siderToggleButtonBorder:`1px solid ${a}`,siderToggleButtonColor:t,siderToggleButtonIconColor:n,siderToggleButtonIconColorInverted:n,siderToggleBarColor:_e(r,s),siderToggleBarColorHover:_e(r,c),__invertScrollbar:`true`}}var vt=be({name:`Layout`,common:xe,peers:{Scrollbar:ie},self:_t}),yt=q(`n-layout-sider`),bt={type:String,default:`static`},xt=V(`layout`,`
 color: var(--n-text-color);
 background-color: var(--n-color);
 box-sizing: border-box;
 position: relative;
 z-index: auto;
 flex: auto;
 overflow: hidden;
 transition:
 box-shadow .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
`,[V(`layout-scroll-container`,`
 overflow-x: hidden;
 box-sizing: border-box;
 height: 100%;
 `),G(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),St={embedded:Boolean,position:bt,nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,onScroll:Function,contentClass:String,contentStyle:{type:[String,Object],default:``},hasSider:Boolean,siderPlacement:{type:String,default:`left`}},Ct=q(`n-layout`);function wt(e){return a({name:e?`LayoutContent`:`Layout`,props:Object.assign(Object.assign({},K.props),St),setup(e){let t=S(null),n=S(null),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=U(e),a=K(`Layout`,`-layout`,xt,vt,e,r);function o(r,i){if(e.nativeScrollbar){let{value:e}=t;e&&(i===void 0?e.scrollTo(r):e.scrollTo(r,i))}else{let{value:e}=n;e&&e.scrollTo(r,i)}}m(Ct,e);let s=0,c=0,l=t=>{var n;let r=t.target;s=r.scrollLeft,c=r.scrollTop,(n=e.onScroll)==null||n.call(e,t)};N(()=>{if(e.nativeScrollbar){let e=t.value;e&&(e.scrollTop=c,e.scrollLeft=s)}});let u={display:`flex`,flexWrap:`nowrap`,width:`100%`,flexDirection:`row`},d={scrollTo:o},f=g(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=a.value;return{"--n-bezier":t,"--n-color":e.embedded?n.colorEmbedded:n.color,"--n-text-color":n.textColor}}),p=i?H(`layout`,g(()=>e.embedded?`e`:``),f,e):void 0;return Object.assign({mergedClsPrefix:r,scrollableElRef:t,scrollbarInstRef:n,hasSiderStyle:u,mergedTheme:a,handleNativeElScroll:l,cssVars:i?void 0:f,themeClass:p?.themeClass,onRender:p?.onRender},d)},render(){var n;let{mergedClsPrefix:r,hasSider:i}=this;(n=this.onRender)==null||n.call(this);let a=i?this.hasSiderStyle:void 0;return t(`div`,{class:[this.themeClass,e&&`${r}-layout-content`,`${r}-layout`,`${r}-layout--${this.position}-positioned`],style:this.cssVars},this.nativeScrollbar?t(`div`,{ref:`scrollableElRef`,class:[`${r}-layout-scroll-container`,this.contentClass],style:[this.contentStyle,a],onScroll:this.handleNativeElScroll},this.$slots):t(F,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,contentClass:this.contentClass,contentStyle:[this.contentStyle,a]}),this.$slots))}})}var Tt=wt(!1),Et=wt(!0),Dt=V(`layout-header`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 box-sizing: border-box;
 width: 100%;
 background-color: var(--n-color);
 color: var(--n-text-color);
`,[G(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 `),G(`bordered`,`
 border-bottom: solid 1px var(--n-border-color);
 `)]),Ot={position:bt,inverted:Boolean,bordered:{type:Boolean,default:!1}},kt=a({name:`LayoutHeader`,props:Object.assign(Object.assign({},K.props),Ot),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=U(e),r=K(`Layout`,`-layout-header`,Dt,vt,e,t),i=g(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=r.value,i={"--n-bezier":t};return e.inverted?(i[`--n-color`]=n.headerColorInverted,i[`--n-text-color`]=n.textColorInverted,i[`--n-border-color`]=n.headerBorderColorInverted):(i[`--n-color`]=n.headerColor,i[`--n-text-color`]=n.textColor,i[`--n-border-color`]=n.headerBorderColor),i}),a=n?H(`layout-header`,g(()=>e.inverted?`a`:`b`),i,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{mergedClsPrefix:n}=this;return(e=this.onRender)==null||e.call(this),t(`div`,{class:[`${n}-layout-header`,this.themeClass,this.position&&`${n}-layout-header--${this.position}-positioned`,this.bordered&&`${n}-layout-header--bordered`],style:this.cssVars},this.$slots)}}),At=V(`layout-sider`,`
 flex-shrink: 0;
 box-sizing: border-box;
 position: relative;
 z-index: 1;
 color: var(--n-text-color);
 transition:
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 min-width .3s var(--n-bezier),
 max-width .3s var(--n-bezier),
 transform .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 display: flex;
 justify-content: flex-end;
`,[G(`bordered`,[B(`border`,`
 content: "";
 position: absolute;
 top: 0;
 bottom: 0;
 width: 1px;
 background-color: var(--n-border-color);
 transition: background-color .3s var(--n-bezier);
 `)]),B(`left-placement`,[G(`bordered`,[B(`border`,`
 right: 0;
 `)])]),G(`right-placement`,`
 justify-content: flex-start;
 `,[G(`bordered`,[B(`border`,`
 left: 0;
 `)]),G(`collapsed`,[V(`layout-toggle-button`,[V(`base-icon`,`
 transform: rotate(180deg);
 `)]),V(`layout-toggle-bar`,[z(`&:hover`,[B(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),B(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])])]),V(`layout-toggle-button`,`
 left: 0;
 transform: translateX(-50%) translateY(-50%);
 `,[V(`base-icon`,`
 transform: rotate(0);
 `)]),V(`layout-toggle-bar`,`
 left: -28px;
 transform: rotate(180deg);
 `,[z(`&:hover`,[B(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),B(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})])])]),G(`collapsed`,[V(`layout-toggle-bar`,[z(`&:hover`,[B(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),B(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])]),V(`layout-toggle-button`,[V(`base-icon`,`
 transform: rotate(0);
 `)])]),V(`layout-toggle-button`,`
 transition:
 color .3s var(--n-bezier),
 right .3s var(--n-bezier),
 left .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 cursor: pointer;
 width: 24px;
 height: 24px;
 position: absolute;
 top: 50%;
 right: 0;
 border-radius: 50%;
 display: flex;
 align-items: center;
 justify-content: center;
 font-size: 18px;
 color: var(--n-toggle-button-icon-color);
 border: var(--n-toggle-button-border);
 background-color: var(--n-toggle-button-color);
 box-shadow: 0 2px 4px 0px rgba(0, 0, 0, .06);
 transform: translateX(50%) translateY(-50%);
 z-index: 1;
 `,[V(`base-icon`,`
 transition: transform .3s var(--n-bezier);
 transform: rotate(180deg);
 `)]),V(`layout-toggle-bar`,`
 cursor: pointer;
 height: 72px;
 width: 32px;
 position: absolute;
 top: calc(50% - 36px);
 right: -28px;
 `,[B(`top, bottom`,`
 position: absolute;
 width: 4px;
 border-radius: 2px;
 height: 38px;
 left: 14px;
 transition: 
 background-color .3s var(--n-bezier),
 transform .3s var(--n-bezier);
 `),B(`bottom`,`
 position: absolute;
 top: 34px;
 `),z(`&:hover`,[B(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),B(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})]),B(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color)`}),z(`&:hover`,[B(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color-hover)`})])]),B(`border`,`
 position: absolute;
 top: 0;
 right: 0;
 bottom: 0;
 width: 1px;
 transition: background-color .3s var(--n-bezier);
 `),V(`layout-sider-scroll-container`,`
 flex-grow: 1;
 flex-shrink: 0;
 box-sizing: border-box;
 height: 100%;
 opacity: 0;
 transition: opacity .3s var(--n-bezier);
 max-width: 100%;
 `),G(`show-content`,[V(`layout-sider-scroll-container`,{opacity:1})]),G(`absolute-positioned`,`
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 `)]),jt=a({props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return t(`div`,{onClick:this.onClick,class:`${e}-layout-toggle-bar`},t(`div`,{class:`${e}-layout-toggle-bar__top`}),t(`div`,{class:`${e}-layout-toggle-bar__bottom`}))}}),Mt=a({name:`LayoutToggleButton`,props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return t(`div`,{class:`${e}-layout-toggle-button`,onClick:this.onClick},t(ve,{clsPrefix:e},{default:()=>t(we,null)}))}}),Nt={position:bt,bordered:Boolean,collapsedWidth:{type:Number,default:48},width:{type:[Number,String],default:272},contentClass:String,contentStyle:{type:[String,Object],default:``},collapseMode:{type:String,default:`transform`},collapsed:{type:Boolean,default:void 0},defaultCollapsed:Boolean,showCollapsedContent:{type:Boolean,default:!0},showTrigger:{type:[Boolean,String],default:!1},nativeScrollbar:{type:Boolean,default:!0},inverted:Boolean,scrollbarProps:Object,triggerClass:String,triggerStyle:[String,Object],collapsedTriggerClass:String,collapsedTriggerStyle:[String,Object],"onUpdate:collapsed":[Function,Array],onUpdateCollapsed:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,onExpand:[Function,Array],onCollapse:[Function,Array],onScroll:Function},Pt=a({name:`LayoutSider`,props:Object.assign(Object.assign({},K.props),Nt),setup(e){let t=s(Ct),n=S(null),r=S(null),i=S(e.defaultCollapsed),a=J(C(e,`collapsed`),i),o=g(()=>Y(a.value?e.collapsedWidth:e.width)),c=g(()=>e.collapseMode===`transform`?{minWidth:Y(e.width)}:{}),l=g(()=>t?t.siderPlacement:`left`);function u(t,i){if(e.nativeScrollbar){let{value:e}=n;e&&(i===void 0?e.scrollTo(t):e.scrollTo(t,i))}else{let{value:e}=r;e&&e.scrollTo(t,i)}}function d(){let{"onUpdate:collapsed":t,onUpdateCollapsed:n,onExpand:r,onCollapse:o}=e,{value:s}=a;n&&k(n,!s),t&&k(t,!s),i.value=!s,s?r&&k(r):o&&k(o)}let f=0,p=0,h=t=>{var n;let r=t.target;f=r.scrollLeft,p=r.scrollTop,(n=e.onScroll)==null||n.call(e,t)};N(()=>{if(e.nativeScrollbar){let e=n.value;e&&(e.scrollTop=p,e.scrollLeft=f)}}),m(yt,{collapsedRef:a,collapseModeRef:C(e,`collapseMode`)});let{mergedClsPrefixRef:_,inlineThemeDisabled:v}=U(e),y=K(`Layout`,`-layout-sider`,At,vt,e,_);function b(t){var n,r;t.propertyName===`max-width`&&(a.value?(n=e.onAfterLeave)==null||n.call(e):(r=e.onAfterEnter)==null||r.call(e))}let x={scrollTo:u},w=g(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=y.value,{siderToggleButtonColor:r,siderToggleButtonBorder:i,siderToggleBarColor:a,siderToggleBarColorHover:o}=n,s={"--n-bezier":t,"--n-toggle-button-color":r,"--n-toggle-button-border":i,"--n-toggle-bar-color":a,"--n-toggle-bar-color-hover":o};return e.inverted?(s[`--n-color`]=n.siderColorInverted,s[`--n-text-color`]=n.textColorInverted,s[`--n-border-color`]=n.siderBorderColorInverted,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColorInverted,s.__invertScrollbar=n.__invertScrollbar):(s[`--n-color`]=n.siderColor,s[`--n-text-color`]=n.textColor,s[`--n-border-color`]=n.siderBorderColor,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColor),s}),T=v?H(`layout-sider`,g(()=>e.inverted?`a`:`b`),w,e):void 0;return Object.assign({scrollableElRef:n,scrollbarInstRef:r,mergedClsPrefix:_,mergedTheme:y,styleMaxWidth:o,mergedCollapsed:a,scrollContainerStyle:c,siderPlacement:l,handleNativeElScroll:h,handleTransitionend:b,handleTriggerClick:d,inlineThemeDisabled:v,cssVars:w,themeClass:T?.themeClass,onRender:T?.onRender},x)},render(){var e;let{mergedClsPrefix:n,mergedCollapsed:r,showTrigger:i}=this;return(e=this.onRender)==null||e.call(this),t(`aside`,{class:[`${n}-layout-sider`,this.themeClass,`${n}-layout-sider--${this.position}-positioned`,`${n}-layout-sider--${this.siderPlacement}-placement`,this.bordered&&`${n}-layout-sider--bordered`,r&&`${n}-layout-sider--collapsed`,(!r||this.showCollapsedContent)&&`${n}-layout-sider--show-content`],onTransitionend:this.handleTransitionend,style:[this.inlineThemeDisabled?void 0:this.cssVars,{maxWidth:this.styleMaxWidth,width:Y(this.width)}]},this.nativeScrollbar?t(`div`,{class:[`${n}-layout-sider-scroll-container`,this.contentClass],onScroll:this.handleNativeElScroll,style:[this.scrollContainerStyle,{overflow:`auto`},this.contentStyle],ref:`scrollableElRef`},this.$slots):t(F,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,style:this.scrollContainerStyle,contentStyle:this.contentStyle,contentClass:this.contentClass,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,builtinThemeOverrides:this.inverted&&this.cssVars.__invertScrollbar===`true`?{colorHover:`rgba(255, 255, 255, .4)`,color:`rgba(255, 255, 255, .3)`}:void 0}),this.$slots),i?t(i===`bar`?jt:Mt,{clsPrefix:n,class:r?this.collapsedTriggerClass:this.triggerClass,style:r?this.collapsedTriggerStyle:this.triggerStyle,onClick:this.handleTriggerClick}):null,this.bordered?t(`div`,{class:`${n}-layout-sider__border`}):null)}}),Q=q(`n-menu`),Ft=q(`n-submenu`),It=q(`n-menu-item-group`),Lt=[z(`&::before`,`background-color: var(--n-item-color-hover);`),B(`arrow`,`
 color: var(--n-arrow-color-hover);
 `),B(`icon`,`
 color: var(--n-item-icon-color-hover);
 `),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover);
 `,[z(`a`,`
 color: var(--n-item-text-color-hover);
 `),B(`extra`,`
 color: var(--n-item-text-color-hover);
 `)])],Rt=[B(`icon`,`
 color: var(--n-item-icon-color-hover-horizontal);
 `),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover-horizontal);
 `,[z(`a`,`
 color: var(--n-item-text-color-hover-horizontal);
 `),B(`extra`,`
 color: var(--n-item-text-color-hover-horizontal);
 `)])],zt=z([V(`menu`,`
 background-color: var(--n-color);
 color: var(--n-item-text-color);
 overflow: hidden;
 transition: background-color .3s var(--n-bezier);
 box-sizing: border-box;
 font-size: var(--n-font-size);
 padding-bottom: 6px;
 `,[G(`horizontal`,`
 max-width: 100%;
 width: 100%;
 display: flex;
 overflow: hidden;
 padding-bottom: 0;
 `,[V(`submenu`,`margin: 0;`),V(`menu-item`,`margin: 0;`),V(`menu-item-content`,`
 padding: 0 20px;
 border-bottom: 2px solid #0000;
 `,[z(`&::before`,`display: none;`),G(`selected`,`border-bottom: 2px solid var(--n-border-color-horizontal)`)]),V(`menu-item-content`,[G(`selected`,[B(`icon`,`color: var(--n-item-icon-color-active-horizontal);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-horizontal);
 `,[z(`a`,`color: var(--n-item-text-color-active-horizontal);`),B(`extra`,`color: var(--n-item-text-color-active-horizontal);`)])]),G(`child-active`,`
 border-bottom: 2px solid var(--n-border-color-horizontal);
 `,[V(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `,[z(`a`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `),B(`extra`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `)]),B(`icon`,`
 color: var(--n-item-icon-color-child-active-horizontal);
 `)]),W(`disabled`,[W(`selected, child-active`,[z(`&:focus-within`,Rt)]),G(`selected`,[$(null,[B(`icon`,`color: var(--n-item-icon-color-active-hover-horizontal);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover-horizontal);
 `,[z(`a`,`color: var(--n-item-text-color-active-hover-horizontal);`),B(`extra`,`color: var(--n-item-text-color-active-hover-horizontal);`)])])]),G(`child-active`,[$(null,[B(`icon`,`color: var(--n-item-icon-color-child-active-hover-horizontal);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover-horizontal);
 `,[z(`a`,`color: var(--n-item-text-color-child-active-hover-horizontal);`),B(`extra`,`color: var(--n-item-text-color-child-active-hover-horizontal);`)])])]),$(`border-bottom: 2px solid var(--n-border-color-horizontal);`,Rt)]),V(`menu-item-content-header`,[z(`a`,`color: var(--n-item-text-color-horizontal);`)])])]),W(`responsive`,[V(`menu-item-content-header`,`
 overflow: hidden;
 text-overflow: ellipsis;
 `)]),G(`collapsed`,[V(`menu-item-content`,[G(`selected`,[z(`&::before`,`
 background-color: var(--n-item-color-active-collapsed) !important;
 `)]),V(`menu-item-content-header`,`opacity: 0;`),B(`arrow`,`opacity: 0;`),B(`icon`,`color: var(--n-item-icon-color-collapsed);`)])]),V(`menu-item`,`
 height: var(--n-item-height);
 margin-top: 6px;
 position: relative;
 `),V(`menu-item-content`,`
 box-sizing: border-box;
 line-height: 1.75;
 height: 100%;
 display: grid;
 grid-template-areas: "icon content arrow";
 grid-template-columns: auto 1fr auto;
 align-items: center;
 cursor: pointer;
 position: relative;
 padding-right: 18px;
 transition:
 background-color .3s var(--n-bezier),
 padding-left .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[z(`> *`,`z-index: 1;`),z(`&::before`,`
 z-index: auto;
 content: "";
 background-color: #0000;
 position: absolute;
 left: 8px;
 right: 8px;
 top: 0;
 bottom: 0;
 pointer-events: none;
 border-radius: var(--n-border-radius);
 transition: background-color .3s var(--n-bezier);
 `),G(`disabled`,`
 opacity: .45;
 cursor: not-allowed;
 `),G(`collapsed`,[B(`arrow`,`transform: rotate(0);`)]),G(`selected`,[z(`&::before`,`background-color: var(--n-item-color-active);`),B(`arrow`,`color: var(--n-arrow-color-active);`),B(`icon`,`color: var(--n-item-icon-color-active);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-active);
 `,[z(`a`,`color: var(--n-item-text-color-active);`),B(`extra`,`color: var(--n-item-text-color-active);`)])]),G(`child-active`,[V(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active);
 `,[z(`a`,`
 color: var(--n-item-text-color-child-active);
 `),B(`extra`,`
 color: var(--n-item-text-color-child-active);
 `)]),B(`arrow`,`
 color: var(--n-arrow-color-child-active);
 `),B(`icon`,`
 color: var(--n-item-icon-color-child-active);
 `)]),W(`disabled`,[W(`selected, child-active`,[z(`&:focus-within`,Lt)]),G(`selected`,[$(null,[B(`arrow`,`color: var(--n-arrow-color-active-hover);`),B(`icon`,`color: var(--n-item-icon-color-active-hover);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover);
 `,[z(`a`,`color: var(--n-item-text-color-active-hover);`),B(`extra`,`color: var(--n-item-text-color-active-hover);`)])])]),G(`child-active`,[$(null,[B(`arrow`,`color: var(--n-arrow-color-child-active-hover);`),B(`icon`,`color: var(--n-item-icon-color-child-active-hover);`),V(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover);
 `,[z(`a`,`color: var(--n-item-text-color-child-active-hover);`),B(`extra`,`color: var(--n-item-text-color-child-active-hover);`)])])]),G(`selected`,[$(null,[z(`&::before`,`background-color: var(--n-item-color-active-hover);`)])]),$(null,Lt)]),B(`icon`,`
 grid-area: icon;
 color: var(--n-item-icon-color);
 transition:
 color .3s var(--n-bezier),
 font-size .3s var(--n-bezier),
 margin-right .3s var(--n-bezier);
 box-sizing: content-box;
 display: inline-flex;
 align-items: center;
 justify-content: center;
 `),B(`arrow`,`
 grid-area: arrow;
 font-size: 16px;
 color: var(--n-arrow-color);
 transform: rotate(180deg);
 opacity: 1;
 transition:
 color .3s var(--n-bezier),
 transform 0.2s var(--n-bezier),
 opacity 0.2s var(--n-bezier);
 `),V(`menu-item-content-header`,`
 grid-area: content;
 transition:
 color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 opacity: 1;
 white-space: nowrap;
 color: var(--n-item-text-color);
 `,[z(`a`,`
 outline: none;
 text-decoration: none;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `,[z(`&::before`,`
 content: "";
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),B(`extra`,`
 font-size: .93em;
 color: var(--n-group-text-color);
 transition: color .3s var(--n-bezier);
 `)])]),V(`submenu`,`
 cursor: pointer;
 position: relative;
 margin-top: 6px;
 `,[V(`menu-item-content`,`
 height: var(--n-item-height);
 `),V(`submenu-children`,`
 overflow: hidden;
 padding: 0;
 `,[Le({duration:`.2s`})])]),V(`menu-item-group`,[V(`menu-item-group-title`,`
 margin-top: 6px;
 color: var(--n-group-text-color);
 cursor: default;
 font-size: .93em;
 height: 36px;
 display: flex;
 align-items: center;
 transition:
 padding-left .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)])]),V(`menu-tooltip`,[z(`a`,`
 color: inherit;
 text-decoration: none;
 `)]),V(`menu-divider`,`
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-divider-color);
 height: 1px;
 margin: 6px 18px;
 `)]);function $(e,t){return[G(`hover`,e,t),z(`&:hover`,e,t)]}var Bt=a({name:`MenuOptionContent`,props:{collapsed:Boolean,disabled:Boolean,title:[String,Function],icon:Function,extra:[String,Function],showArrow:Boolean,childActive:Boolean,hover:Boolean,paddingLeft:Number,selected:Boolean,maxIconSize:{type:Number,required:!0},activeIconSize:{type:Number,required:!0},iconMarginRight:{type:Number,required:!0},clsPrefix:{type:String,required:!0},onClick:Function,tmNode:{type:Object,required:!0},isEllipsisPlaceholder:Boolean},setup(e){let{props:t}=s(Q);return{menuProps:t,style:g(()=>{let{paddingLeft:t}=e;return{paddingLeft:t&&`${t}px`}}),iconStyle:g(()=>{let{maxIconSize:t,activeIconSize:n,iconMarginRight:r}=e;return{width:`${t}px`,height:`${t}px`,fontSize:`${n}px`,marginRight:`${r}px`}})}},render(){let{clsPrefix:e,tmNode:n,menuProps:{renderIcon:r,renderLabel:i,renderExtra:a,expandIcon:o}}=this,s=r?r(n.rawNode):Z(this.icon);return t(`div`,{onClick:e=>{var t;(t=this.onClick)==null||t.call(this,e)},role:`none`,class:[`${e}-menu-item-content`,{[`${e}-menu-item-content--selected`]:this.selected,[`${e}-menu-item-content--collapsed`]:this.collapsed,[`${e}-menu-item-content--child-active`]:this.childActive,[`${e}-menu-item-content--disabled`]:this.disabled,[`${e}-menu-item-content--hover`]:this.hover}],style:this.style},s&&t(`div`,{class:`${e}-menu-item-content__icon`,style:this.iconStyle,role:`none`},[s]),t(`div`,{class:`${e}-menu-item-content-header`,role:`none`},this.isEllipsisPlaceholder?this.title:i?i(n.rawNode):Z(this.title),this.extra||a?t(`span`,{class:`${e}-menu-item-content-header__extra`},` `,a?a(n.rawNode):Z(this.extra)):null),this.showArrow?t(ve,{ariaHidden:!0,class:`${e}-menu-item-content__arrow`,clsPrefix:e},{default:()=>o?o(n.rawNode):t(Ye,null)}):null)}}),Vt=8;function Ht(e){let t=s(Q),{props:n,mergedCollapsedRef:r}=t,i=s(Ft,null),a=s(It,null),o=g(()=>n.mode===`horizontal`),c=g(()=>o.value?n.dropdownPlacement:`tmNodes`in e?`right-start`:`right`),l=g(()=>Math.max(n.collapsedIconSize??n.iconSize,n.iconSize));return{dropdownPlacement:c,activeIconSize:g(()=>!o.value&&e.root&&r.value?n.collapsedIconSize??n.iconSize:n.iconSize),maxIconSize:l,paddingLeft:g(()=>{if(o.value)return;let{collapsedWidth:t,indent:s,rootIndent:c}=n,{root:u,isGroup:d}=e,f=c===void 0?s:c;return u?r.value?t/2-l.value/2:f:a&&typeof a.paddingLeftRef.value==`number`?s/2+a.paddingLeftRef.value:i&&typeof i.paddingLeftRef.value==`number`?(d?s/2:s)+i.paddingLeftRef.value:0}),iconMarginRight:g(()=>{let{collapsedWidth:t,indent:i,rootIndent:a}=n,{value:s}=l,{root:c}=e;return o.value||!c||!r.value?Vt:(a===void 0?i:a)+s+Vt-(t+s)/2}),NMenu:t,NSubmenu:i,NMenuOptionGroup:a}}var Ut={internalKey:{type:[String,Number],required:!0},root:Boolean,isGroup:Boolean,level:{type:Number,required:!0},title:[String,Function],extra:[String,Function]},Wt=a({name:`MenuDivider`,setup(){let{mergedClsPrefixRef:e,isHorizontalRef:n}=s(Q);return()=>n.value?null:t(`div`,{class:`${e.value}-menu-divider`})}}),Gt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function}),Kt=M(Gt),qt=a({name:`MenuOption`,props:Gt,setup(e){let t=Ht(e),{NSubmenu:n,NMenu:r,NMenuOptionGroup:i}=t,{props:a,mergedClsPrefixRef:o,mergedCollapsedRef:s}=r,c=n?n.mergedDisabledRef:i?i.mergedDisabledRef:{value:!1},l=g(()=>c.value||e.disabled);function u(t){let{onClick:n}=e;n&&n(t)}function d(t){l.value||(r.doSelect(e.internalKey,e.tmNode.rawNode),u(t))}return{mergedClsPrefix:o,dropdownPlacement:t.dropdownPlacement,paddingLeft:t.paddingLeft,iconMarginRight:t.iconMarginRight,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,mergedTheme:r.mergedThemeRef,menuProps:a,dropdownEnabled:ae(()=>e.root&&s.value&&a.mode!==`horizontal`&&!l.value),selected:ae(()=>r.mergedValueRef.value===e.internalKey),mergedDisabled:l,handleClick:d}},render(){let{mergedClsPrefix:e,mergedTheme:n,tmNode:r,menuProps:{renderLabel:i,nodeProps:a}}=this,o=a?.(r.rawNode);return t(`div`,Object.assign({},o,{role:`menuitem`,class:[`${e}-menu-item`,o?.class]}),t(Ce,{theme:n.peers.Tooltip,themeOverrides:n.peerOverrides.Tooltip,trigger:`hover`,placement:this.dropdownPlacement,disabled:!this.dropdownEnabled||this.title===void 0,internalExtraClass:[`menu-tooltip`]},{default:()=>i?i(r.rawNode):Z(this.title),trigger:()=>t(Bt,{tmNode:r,clsPrefix:e,paddingLeft:this.paddingLeft,iconMarginRight:this.iconMarginRight,maxIconSize:this.maxIconSize,activeIconSize:this.activeIconSize,selected:this.selected,title:this.title,extra:this.extra,disabled:this.mergedDisabled,icon:this.icon,onClick:this.handleClick})}))}}),Jt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},tmNodes:{type:Array,required:!0}}),Yt=M(Jt),Xt=a({name:`MenuOptionGroup`,props:Jt,setup(e){let n=Ht(e),{NSubmenu:r}=n,i=g(()=>r?.mergedDisabledRef.value?!0:e.tmNode.disabled);m(It,{paddingLeftRef:n.paddingLeft,mergedDisabledRef:i});let{mergedClsPrefixRef:a,props:o}=s(Q);return function(){let{value:r}=a,i=n.paddingLeft.value,{nodeProps:s}=o,c=s?.(e.tmNode.rawNode);return t(`div`,{class:`${r}-menu-item-group`,role:`group`},t(`div`,Object.assign({},c,{class:[`${r}-menu-item-group-title`,c?.class],style:[c?.style||``,i===void 0?``:`padding-left: ${i}px;`]}),Z(e.title),e.extra?t(y,null,` `,Z(e.extra)):null),t(`div`,null,e.tmNodes.map(e=>$t(e,o))))}}});function Zt(e){return e.type===`divider`||e.type===`render`}function Qt(e){return e.type===`divider`}function $t(e,n){let{rawNode:r}=e,{show:i}=r;if(i===!1)return null;if(Zt(r))return Qt(r)?t(Wt,Object.assign({key:e.key},r.props)):null;let{labelField:a}=n,{key:o,level:s,isGroup:c}=e,l=Object.assign(Object.assign({},r),{title:r.title||r[a],extra:r.titleExtra||r.extra,key:o,internalKey:o,level:s,root:s===0,isGroup:c});return e.children?e.isGroup?t(Xt,ke(l,Yt,{tmNode:e,tmNodes:e.children,key:o})):t(nn,ke(l,tn,{key:o,rawNodes:r[n.childrenField],tmNodes:e.children,tmNode:e})):t(qt,ke(l,Kt,{key:o,tmNode:e}))}var en=Object.assign(Object.assign({},Ut),{rawNodes:{type:Array,default:()=>[]},tmNodes:{type:Array,default:()=>[]},tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function,domId:String,virtualChildActive:{type:Boolean,default:void 0},isEllipsisPlaceholder:Boolean}),tn=M(en),nn=a({name:`Submenu`,props:en,setup(e){let t=Ht(e),{NMenu:n,NSubmenu:r}=t,{props:i,mergedCollapsedRef:a,mergedThemeRef:o}=n,s=g(()=>{let{disabled:t}=e;return r?.mergedDisabledRef.value||i.disabled?!0:t}),c=S(!1);m(Ft,{paddingLeftRef:t.paddingLeft,mergedDisabledRef:s}),m(It,null);function l(){let{onClick:t}=e;t&&t()}function u(){s.value||(a.value||n.toggleExpand(e.internalKey),l())}function d(e){c.value=e}return{menuProps:i,mergedTheme:o,doSelect:n.doSelect,inverted:n.invertedRef,isHorizontal:n.isHorizontalRef,mergedClsPrefix:n.mergedClsPrefixRef,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,iconMarginRight:t.iconMarginRight,dropdownPlacement:t.dropdownPlacement,dropdownShow:c,paddingLeft:t.paddingLeft,mergedDisabled:s,mergedValue:n.mergedValueRef,childActive:ae(()=>e.virtualChildActive??n.activePathRef.value.includes(e.internalKey)),collapsed:g(()=>i.mode===`horizontal`?!1:a.value?!0:!n.mergedExpandedKeysRef.value.includes(e.internalKey)),dropdownEnabled:g(()=>!s.value&&(i.mode===`horizontal`||a.value)),handlePopoverShowChange:d,handleClick:u}},render(){let{mergedClsPrefix:e,menuProps:{renderIcon:n,renderLabel:r}}=this,i=()=>{let{isHorizontal:e,paddingLeft:n,collapsed:r,mergedDisabled:i,maxIconSize:a,activeIconSize:o,title:s,childActive:c,icon:l,handleClick:u,menuProps:{nodeProps:d},dropdownShow:f,iconMarginRight:p,tmNode:m,mergedClsPrefix:h,isEllipsisPlaceholder:g,extra:_}=this,v=d?.(m.rawNode);return t(`div`,Object.assign({},v,{class:[`${h}-menu-item`,v?.class],role:`menuitem`}),t(Bt,{tmNode:m,paddingLeft:n,collapsed:r,disabled:i,iconMarginRight:p,maxIconSize:a,activeIconSize:o,title:s,extra:_,showArrow:!e,childActive:c,clsPrefix:h,icon:l,hover:f,onClick:u,isEllipsisPlaceholder:g}))},a=()=>t(R,null,{default:()=>{let{tmNodes:n,collapsed:r}=this;return r?null:t(`div`,{class:`${e}-submenu-children`,role:`menu`},n.map(e=>$t(e,this.menuProps)))}});return this.root?t(Te,Object.assign({size:`large`,trigger:`hover`},this.menuProps?.dropdownProps,{themeOverrides:this.mergedTheme.peerOverrides.Dropdown,theme:this.mergedTheme.peers.Dropdown,builtinThemeOverrides:{fontSizeLarge:`14px`,optionIconSizeLarge:`18px`},value:this.mergedValue,disabled:!this.dropdownEnabled,placement:this.dropdownPlacement,keyField:this.menuProps.keyField,labelField:this.menuProps.labelField,childrenField:this.menuProps.childrenField,onUpdateShow:this.handlePopoverShowChange,options:this.rawNodes,onSelect:this.doSelect,inverted:this.inverted,renderIcon:n,renderLabel:r}),{default:()=>t(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},i(),this.isHorizontal?null:a())}):t(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},i(),a())}}),rn=a({name:`Menu`,inheritAttrs:!1,props:Object.assign(Object.assign({},K.props),{options:{type:Array,default:()=>[]},collapsed:{type:Boolean,default:void 0},collapsedWidth:{type:Number,default:48},iconSize:{type:Number,default:20},collapsedIconSize:{type:Number,default:24},rootIndent:Number,indent:{type:Number,default:32},labelField:{type:String,default:`label`},keyField:{type:String,default:`key`},childrenField:{type:String,default:`children`},disabledField:{type:String,default:`disabled`},defaultExpandAll:Boolean,defaultExpandedKeys:Array,expandedKeys:Array,value:[String,Number],defaultValue:{type:[String,Number],default:null},mode:{type:String,default:`vertical`},watchProps:{type:Array,default:void 0},disabled:Boolean,show:{type:Boolean,default:!0},inverted:Boolean,"onUpdate:expandedKeys":[Function,Array],onUpdateExpandedKeys:[Function,Array],onUpdateValue:[Function,Array],"onUpdate:value":[Function,Array],expandIcon:Function,renderIcon:Function,renderLabel:Function,renderExtra:Function,dropdownProps:Object,accordion:Boolean,nodeProps:Function,dropdownPlacement:{type:String,default:`bottom`},responsive:Boolean,items:Array,onOpenNamesChange:[Function,Array],onSelect:[Function,Array],onExpandedNamesChange:[Function,Array],expandedNames:Array,defaultExpandedNames:Array}),setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=U(e),i=K(`Menu`,`-menu`,zt,Ae,e,n),a=s(yt,null),o=g(()=>{let{collapsed:t}=e;if(t!==void 0)return t;if(a){let{collapseModeRef:e,collapsedRef:t}=a;if(e.value===`width`)return t.value??!1}return!1}),c=g(()=>{let{keyField:t,childrenField:n,disabledField:r}=e;return Se(e.items||e.options,{getIgnored(e){return Zt(e)},getChildren(e){return e[n]},getDisabled(e){return e[r]},getKey(e){return e[t]??e.name}})}),u=g(()=>new Set(c.value.treeNodes.map(e=>e.key))),{watchProps:d}=e,f=S(null);d?.includes(`defaultValue`)?l(()=>{f.value=e.defaultValue}):f.value=e.defaultValue;let p=J(C(e,`value`),f),h=S([]),_=()=>{h.value=e.defaultExpandAll?c.value.getNonLeafKeys():e.defaultExpandedNames||e.defaultExpandedKeys||c.value.getPath(p.value,{includeSelf:!1}).keyPath};d?.includes(`defaultExpandedKeys`)?l(_):_();let v=De(e,[`expandedNames`,`expandedKeys`]),y=J(v,h),b=g(()=>c.value.treeNodes),x=g(()=>c.value.getPath(p.value).keyPath);m(Q,{props:e,mergedCollapsedRef:o,mergedThemeRef:i,mergedValueRef:p,mergedExpandedKeysRef:y,activePathRef:x,mergedClsPrefixRef:n,isHorizontalRef:g(()=>e.mode===`horizontal`),invertedRef:C(e,`inverted`),doSelect:w,toggleExpand:ee});function w(t,n){let{"onUpdate:value":r,onUpdateValue:i,onSelect:a}=e;i&&k(i,t,n),r&&k(r,t,n),a&&k(a,t,n),f.value=t}function T(t){let{"onUpdate:expandedKeys":n,onUpdateExpandedKeys:r,onExpandedNamesChange:i,onOpenNamesChange:a}=e;n&&k(n,t),r&&k(r,t),i&&k(i,t),a&&k(a,t),h.value=t}function ee(t){let n=Array.from(y.value),r=n.findIndex(e=>e===t);if(~r)n.splice(r,1);else{if(e.accordion&&u.value.has(t)){let e=n.findIndex(e=>u.value.has(e));e>-1&&n.splice(e,1)}n.push(t)}T(n)}let E=t=>{let n=c.value.getPath(t??p.value,{includeSelf:!1}).keyPath;if(!n.length)return;let r=Array.from(y.value),i=new Set([...r,...n]);e.accordion&&u.value.forEach(e=>{i.has(e)&&!n.includes(e)&&i.delete(e)}),T(Array.from(i))},D=g(()=>{let{inverted:t}=e,{common:{cubicBezierEaseInOut:n},self:r}=i.value,{borderRadius:a,borderColorHorizontal:o,fontSize:s,itemHeight:c,dividerColor:l}=r,u={"--n-divider-color":l,"--n-bezier":n,"--n-font-size":s,"--n-border-color-horizontal":o,"--n-border-radius":a,"--n-item-height":c};return t?(u[`--n-group-text-color`]=r.groupTextColorInverted,u[`--n-color`]=r.colorInverted,u[`--n-item-text-color`]=r.itemTextColorInverted,u[`--n-item-text-color-hover`]=r.itemTextColorHoverInverted,u[`--n-item-text-color-active`]=r.itemTextColorActiveInverted,u[`--n-item-text-color-child-active`]=r.itemTextColorChildActiveInverted,u[`--n-item-text-color-child-active-hover`]=r.itemTextColorChildActiveInverted,u[`--n-item-text-color-active-hover`]=r.itemTextColorActiveHoverInverted,u[`--n-item-icon-color`]=r.itemIconColorInverted,u[`--n-item-icon-color-hover`]=r.itemIconColorHoverInverted,u[`--n-item-icon-color-active`]=r.itemIconColorActiveInverted,u[`--n-item-icon-color-active-hover`]=r.itemIconColorActiveHoverInverted,u[`--n-item-icon-color-child-active`]=r.itemIconColorChildActiveInverted,u[`--n-item-icon-color-child-active-hover`]=r.itemIconColorChildActiveHoverInverted,u[`--n-item-icon-color-collapsed`]=r.itemIconColorCollapsedInverted,u[`--n-item-text-color-horizontal`]=r.itemTextColorHorizontalInverted,u[`--n-item-text-color-hover-horizontal`]=r.itemTextColorHoverHorizontalInverted,u[`--n-item-text-color-active-horizontal`]=r.itemTextColorActiveHorizontalInverted,u[`--n-item-text-color-child-active-horizontal`]=r.itemTextColorChildActiveHorizontalInverted,u[`--n-item-text-color-child-active-hover-horizontal`]=r.itemTextColorChildActiveHoverHorizontalInverted,u[`--n-item-text-color-active-hover-horizontal`]=r.itemTextColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-horizontal`]=r.itemIconColorHorizontalInverted,u[`--n-item-icon-color-hover-horizontal`]=r.itemIconColorHoverHorizontalInverted,u[`--n-item-icon-color-active-horizontal`]=r.itemIconColorActiveHorizontalInverted,u[`--n-item-icon-color-active-hover-horizontal`]=r.itemIconColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-child-active-horizontal`]=r.itemIconColorChildActiveHorizontalInverted,u[`--n-item-icon-color-child-active-hover-horizontal`]=r.itemIconColorChildActiveHoverHorizontalInverted,u[`--n-arrow-color`]=r.arrowColorInverted,u[`--n-arrow-color-hover`]=r.arrowColorHoverInverted,u[`--n-arrow-color-active`]=r.arrowColorActiveInverted,u[`--n-arrow-color-active-hover`]=r.arrowColorActiveHoverInverted,u[`--n-arrow-color-child-active`]=r.arrowColorChildActiveInverted,u[`--n-arrow-color-child-active-hover`]=r.arrowColorChildActiveHoverInverted,u[`--n-item-color-hover`]=r.itemColorHoverInverted,u[`--n-item-color-active`]=r.itemColorActiveInverted,u[`--n-item-color-active-hover`]=r.itemColorActiveHoverInverted,u[`--n-item-color-active-collapsed`]=r.itemColorActiveCollapsedInverted):(u[`--n-group-text-color`]=r.groupTextColor,u[`--n-color`]=r.color,u[`--n-item-text-color`]=r.itemTextColor,u[`--n-item-text-color-hover`]=r.itemTextColorHover,u[`--n-item-text-color-active`]=r.itemTextColorActive,u[`--n-item-text-color-child-active`]=r.itemTextColorChildActive,u[`--n-item-text-color-child-active-hover`]=r.itemTextColorChildActiveHover,u[`--n-item-text-color-active-hover`]=r.itemTextColorActiveHover,u[`--n-item-icon-color`]=r.itemIconColor,u[`--n-item-icon-color-hover`]=r.itemIconColorHover,u[`--n-item-icon-color-active`]=r.itemIconColorActive,u[`--n-item-icon-color-active-hover`]=r.itemIconColorActiveHover,u[`--n-item-icon-color-child-active`]=r.itemIconColorChildActive,u[`--n-item-icon-color-child-active-hover`]=r.itemIconColorChildActiveHover,u[`--n-item-icon-color-collapsed`]=r.itemIconColorCollapsed,u[`--n-item-text-color-horizontal`]=r.itemTextColorHorizontal,u[`--n-item-text-color-hover-horizontal`]=r.itemTextColorHoverHorizontal,u[`--n-item-text-color-active-horizontal`]=r.itemTextColorActiveHorizontal,u[`--n-item-text-color-child-active-horizontal`]=r.itemTextColorChildActiveHorizontal,u[`--n-item-text-color-child-active-hover-horizontal`]=r.itemTextColorChildActiveHoverHorizontal,u[`--n-item-text-color-active-hover-horizontal`]=r.itemTextColorActiveHoverHorizontal,u[`--n-item-icon-color-horizontal`]=r.itemIconColorHorizontal,u[`--n-item-icon-color-hover-horizontal`]=r.itemIconColorHoverHorizontal,u[`--n-item-icon-color-active-horizontal`]=r.itemIconColorActiveHorizontal,u[`--n-item-icon-color-active-hover-horizontal`]=r.itemIconColorActiveHoverHorizontal,u[`--n-item-icon-color-child-active-horizontal`]=r.itemIconColorChildActiveHorizontal,u[`--n-item-icon-color-child-active-hover-horizontal`]=r.itemIconColorChildActiveHoverHorizontal,u[`--n-arrow-color`]=r.arrowColor,u[`--n-arrow-color-hover`]=r.arrowColorHover,u[`--n-arrow-color-active`]=r.arrowColorActive,u[`--n-arrow-color-active-hover`]=r.arrowColorActiveHover,u[`--n-arrow-color-child-active`]=r.arrowColorChildActive,u[`--n-arrow-color-child-active-hover`]=r.arrowColorChildActiveHover,u[`--n-item-color-hover`]=r.itemColorHover,u[`--n-item-color-active`]=r.itemColorActive,u[`--n-item-color-active-hover`]=r.itemColorActiveHover,u[`--n-item-color-active-collapsed`]=r.itemColorActiveCollapsed),u}),O=r?H(`menu`,g(()=>e.inverted?`a`:`b`),D,e):void 0,A=oe(),j=S(null),te=S(null),M=!0,N=()=>{var e;M?M=!1:(e=j.value)==null||e.sync({showAllItemsBeforeCalculate:!0})};function P(){return document.getElementById(A)}let F=S(-1);function ne(t){F.value=e.options.length-t}function I(e){e||(F.value=-1)}let L=g(()=>{let t=F.value;return{children:t===-1?[]:e.options.slice(t)}}),R=g(()=>{let{childrenField:t,disabledField:n,keyField:r}=e;return Se([L.value],{getIgnored(e){return Zt(e)},getChildren(e){return e[t]},getDisabled(e){return e[n]},getKey(e){return e[r]??e.name}})}),re=g(()=>Se([{}]).treeNodes[0]);function ie(){if(F.value===-1)return t(nn,{root:!0,level:0,key:`__ellpisisGroupPlaceholder__`,internalKey:`__ellpisisGroupPlaceholder__`,title:`···`,tmNode:re.value,domId:A,isEllipsisPlaceholder:!0});let e=R.value.treeNodes[0],n=x.value;return t(nn,{level:0,root:!0,key:`__ellpisisGroup__`,internalKey:`__ellpisisGroup__`,title:`···`,virtualChildActive:!!e.children?.some(e=>n.includes(e.key)),tmNode:e,domId:A,rawNodes:e.rawNode.children||[],tmNodes:e.children||[],isEllipsisPlaceholder:!0})}return{mergedClsPrefix:n,controlledExpandedKeys:v,uncontrolledExpanededKeys:h,mergedExpandedKeys:y,uncontrolledValue:f,mergedValue:p,activePath:x,tmNodes:b,mergedTheme:i,mergedCollapsed:o,cssVars:r?void 0:D,themeClass:O?.themeClass,overflowRef:j,counterRef:te,updateCounter:()=>{},onResize:N,onUpdateOverflow:I,onUpdateCount:ne,renderCounter:ie,getCounter:P,onRender:O?.onRender,showOption:E,deriveResponsiveState:N}},render(){let{mergedClsPrefix:e,mode:n,themeClass:r,onRender:i}=this;i?.();let a=()=>this.tmNodes.map(e=>$t(e,this.$props)),o=n===`horizontal`&&this.responsive,s=()=>t(`div`,c(this.$attrs,{role:n===`horizontal`?`menubar`:`menu`,class:[`${e}-menu`,r,`${e}-menu--${n}`,o&&`${e}-menu--responsive`,this.mergedCollapsed&&`${e}-menu--collapsed`],style:this.cssVars}),o?t(se,{ref:`overflowRef`,onUpdateOverflow:this.onUpdateOverflow,getCounter:this.getCounter,onUpdateCount:this.onUpdateCount,updateCounter:this.updateCounter,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:a,counter:this.renderCounter}):a());return o?t(ce,{onResize:this.onResize},{default:s}):s()}}),an={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},on=a({name:`CloudDownloadOutline`,render:function(e,t){return f(),E(`svg`,an,t[0]||=[T(`path`,{d:`M320 336h76c55 0 100-21.21 100-75.6s-53-73.47-96-75.6C391.11 99.74 329 48 256 48c-69 0-113.44 45.79-128 91.2c-60 5.7-112 35.88-112 98.4S70 336 136 336h56`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 400.1l64 63.9l64-63.9`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M256 224v224.03`},null,-1)])}}),sn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},cn=a({name:`DocumentTextOutline`,render:function(e,t){return f(),E(`svg`,sn,t[0]||=[T(`path`,{d:`M416 221.25V416a48 48 0 0 1-48 48H144a48 48 0 0 1-48-48V96a48 48 0 0 1 48-48h98.75a32 32 0 0 1 22.62 9.37l141.26 141.26a32 32 0 0 1 9.37 22.62z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M256 56v120a32 32 0 0 0 32 32h120`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 288h160`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 368h160`},null,-1)])}}),ln={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},un=a({name:`HomeOutline`,render:function(e,t){return f(),E(`svg`,ln,t[0]||=[T(`path`,{d:`M80 212v236a16 16 0 0 0 16 16h96V328a24 24 0 0 1 24-24h80a24 24 0 0 1 24 24v136h96a16 16 0 0 0 16-16V212`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M480 256L266.89 52c-5-5.28-16.69-5.34-21.78 0L32 256`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M400 179V64h-48v69`},null,-1)])}}),dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},fn=a({name:`KeyOutline`,render:function(e,t){return f(),E(`svg`,dn,t[0]||=[T(`path`,{d:`M218.1 167.17c0 13 0 25.6 4.1 37.4c-43.1 50.6-156.9 184.3-167.5 194.5a20.17 20.17 0 0 0-6.7 15c0 8.5 5.2 16.7 9.6 21.3c6.6 6.9 34.8 33 40 28c15.4-15 18.5-19 24.8-25.2c9.5-9.3-1-28.3 2.3-36s6.8-9.2 12.5-10.4s15.8 2.9 23.7 3c8.3.1 12.8-3.4 19-9.2c5-4.6 8.6-8.9 8.7-15.6c.2-9-12.8-20.9-3.1-30.4s23.7 6.2 34 5s22.8-15.5 24.1-21.6s-11.7-21.8-9.7-30.7c.7-3 6.8-10 11.4-11s25 6.9 29.6 5.9c5.6-1.2 12.1-7.1 17.4-10.4c15.5 6.7 29.6 9.4 47.7 9.4c68.5 0 124-53.4 124-119.2S408.5 48 340 48s-121.9 53.37-121.9 119.17zM400 144a32 32 0 1 1-32-32a32 32 0 0 1 32 32z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),pn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},mn=a({name:`LanguageOutline`,render:function(e,t){return f(),E(`svg`,pn,t[0]||=[i(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M48 112h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M192 64v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M272 448l96-224l96 224"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M301.5 384h133"></path><path d="M281.3 112S257 206 199 277S80 384 80 384" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path><path d="M256 336s-35-27-72-75s-56-85-56-85" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path>`,6)])}}),hn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},gn=a({name:`LayersOutline`,render:function(e,t){return f(),E(`svg`,hn,t[0]||=[T(`path`,{d:`M434.8 137.65l-149.36-68.1c-16.19-7.4-42.69-7.4-58.88 0L77.3 137.65c-17.6 8-17.6 21.09 0 29.09l148 67.5c16.89 7.7 44.69 7.7 61.58 0l148-67.5c17.52-8 17.52-21.1-.08-29.09z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M160 308.52l-82.7 37.11c-17.6 8-17.6 21.1 0 29.1l148 67.5c16.89 7.69 44.69 7.69 61.58 0l148-67.5c17.6-8 17.6-21.1 0-29.1l-79.94-38.47`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M160 204.48l-82.8 37.16c-17.6 8-17.6 21.1 0 29.1l148 67.49c16.89 7.7 44.69 7.7 61.58 0l148-67.49c17.7-8 17.7-21.1.1-29.1L352 204.48`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),_n={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},vn=a({name:`ListOutline`,render:function(e,t){return f(),E(`svg`,_n,t[0]||=[i(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 144h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 256h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 368h288"></path><circle cx="80" cy="144" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="256" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="368" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle>`,6)])}}),yn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},bn=a({name:`LogOutOutline`,render:function(e,t){return f(),E(`svg`,yn,t[0]||=[T(`path`,{d:`M304 336v40a40 40 0 0 1-40 40H104a40 40 0 0 1-40-40V136a40 40 0 0 1 40-40h152c22.09 0 48 17.91 48 40v40`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M368 336l80-80l-80-80`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 256h256`},null,-1)])}}),xn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Sn=a({name:`MenuOutline`,render:function(e,t){return f(),E(`svg`,xn,t[0]||=[T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 160h352`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 256h352`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 352h352`},null,-1)])}}),Cn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},wn=a({name:`MoonOutline`,render:function(e,t){return f(),E(`svg`,Cn,t[0]||=[T(`path`,{d:`M160 136c0-30.62 4.51-61.61 16-88C99.57 81.27 48 159.32 48 248c0 119.29 96.71 216 216 216c88.68 0 166.73-51.57 200-128c-26.39 11.49-57.38 16-88 16c-119.29 0-216-96.71-216-216z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),Tn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},En=a({name:`PeopleOutline`,render:function(e,t){return f(),E(`svg`,Tn,t[0]||=[T(`path`,{d:`M402 168c-2.93 40.67-33.1 72-66 72s-63.12-31.32-66-72c-3-42.31 26.37-72 66-72s69 30.46 66 72z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M336 304c-65.17 0-127.84 32.37-143.54 95.41c-2.08 8.34 3.15 16.59 11.72 16.59h263.65c8.57 0 13.77-8.25 11.72-16.59C463.85 335.36 401.18 304 336 304z`,fill:`none`,stroke:`currentColor`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1),T(`path`,{d:`M200 185.94c-2.34 32.48-26.72 58.06-53 58.06s-50.7-25.57-53-58.06C91.61 152.15 115.34 128 147 128s55.39 24.77 53 57.94z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{d:`M206 306c-18.05-8.27-37.93-11.45-59-11.45c-52 0-102.1 25.85-114.65 76.2c-1.65 6.66 2.53 13.25 9.37 13.25H154`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1)])}}),Dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},On=a({name:`SettingsOutline`,render:function(e,t){return f(),E(`svg`,Dn,t[0]||=[T(`path`,{d:`M262.29 192.31a64 64 0 1 0 57.4 57.4a64.13 64.13 0 0 0-57.4-57.4zM416.39 256a154.34 154.34 0 0 1-1.53 20.79l45.21 35.46a10.81 10.81 0 0 1 2.45 13.75l-42.77 74a10.81 10.81 0 0 1-13.14 4.59l-44.9-18.08a16.11 16.11 0 0 0-15.17 1.75A164.48 164.48 0 0 1 325 400.8a15.94 15.94 0 0 0-8.82 12.14l-6.73 47.89a11.08 11.08 0 0 1-10.68 9.17h-85.54a11.11 11.11 0 0 1-10.69-8.87l-6.72-47.82a16.07 16.07 0 0 0-9-12.22a155.3 155.3 0 0 1-21.46-12.57a16 16 0 0 0-15.11-1.71l-44.89 18.07a10.81 10.81 0 0 1-13.14-4.58l-42.77-74a10.8 10.8 0 0 1 2.45-13.75l38.21-30a16.05 16.05 0 0 0 6-14.08c-.36-4.17-.58-8.33-.58-12.5s.21-8.27.58-12.35a16 16 0 0 0-6.07-13.94l-38.19-30A10.81 10.81 0 0 1 49.48 186l42.77-74a10.81 10.81 0 0 1 13.14-4.59l44.9 18.08a16.11 16.11 0 0 0 15.17-1.75A164.48 164.48 0 0 1 187 111.2a15.94 15.94 0 0 0 8.82-12.14l6.73-47.89A11.08 11.08 0 0 1 213.23 42h85.54a11.11 11.11 0 0 1 10.69 8.87l6.72 47.82a16.07 16.07 0 0 0 9 12.22a155.3 155.3 0 0 1 21.46 12.57a16 16 0 0 0 15.11 1.71l44.89-18.07a10.81 10.81 0 0 1 13.14 4.58l42.77 74a10.8 10.8 0 0 1-2.45 13.75l-38.21 30a16.05 16.05 0 0 0-6.05 14.08c.33 4.14.55 8.3.55 12.47z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),kn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},An=a({name:`ShieldCheckmarkOutline`,render:function(e,t){return f(),E(`svg`,kn,t[0]||=[T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M336 176L225.2 304L176 255.8`},null,-1),T(`path`,{d:`M463.1 112.37C373.68 96.33 336.71 84.45 256 48c-80.71 36.45-117.68 48.33-207.1 64.37C32.7 369.13 240.58 457.79 256 464c15.42-6.21 223.3-94.87 207.1-351.63z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),jn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Mn=a({name:`SunnyOutline`,render:function(e,t){return f(),E(`svg`,jn,t[0]||=[i(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 48v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 416v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 108.92l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 369.14l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M464 256h-48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M96 256H48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 403.08l-33.94-33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 142.86l-33.94-33.94"></path><circle cx="256" cy="256" r="80" fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32"></circle>`,9)])}}),Nn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Pn=a({name:`SwapHorizontalOutline`,render:function(e,t){return f(),E(`svg`,Nn,t[0]||=[T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M304 48l112 112l-112 112`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M398.87 160H96`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M208 464L96 352l112-112`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M114 352h302`},null,-1)])}}),Fn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},In=a({name:`TerminalOutline`,render:function(e,t){return f(),E(`svg`,Fn,t[0]||=[T(`rect`,{x:`32`,y:`48`,width:`448`,height:`416`,rx:`48`,ry:`48`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M96 112l80 64l-80 64`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 240h64`},null,-1)])}}),Ln={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Rn=a({name:`TimeOutline`,render:function(e,t){return f(),E(`svg`,Ln,t[0]||=[T(`path`,{d:`M256 64C150 64 64 150 64 256s86 192 192 192s192-86 192-192S362 64 256 64z`,fill:`none`,stroke:`currentColor`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1),T(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M256 128v144h96`},null,-1)])}}),zn={key:0,class:`logo-text`},Bn={class:`header-left`},Vn={class:`header-actions`},Hn={key:0},Un={class:`content-container`},Wn=Ee(a({__name:`AppLayout`,setup(i){let a=ue(),s=pe(),{t:c}=fe(),l=L(),p=Ve(),m=g({get:()=>p.sidebarCollapsed,set:e=>{p.sidebarCollapsed=e}}),h=S(!1),v=S(!1),C=g(()=>p.darkMode);function O(){h.value=window.innerWidth<900,h.value||(v.value=!1)}r(()=>{O(),window.addEventListener(`resize`,O)}),u(()=>window.removeEventListener(`resize`,O));function k(){p.toggleDarkMode()}let A=g(()=>l.user?.display_name||l.user?.username||``),j=g(()=>{let e=s.path,t=P.flatMap(e=>[e,...e.children??[]]).map(e=>String(e.key)).filter(e=>e.startsWith(`/`)).sort((e,t)=>t.length-e.length);for(let n of t)if(e.startsWith(n))return n;return`/`}),te=g(()=>{let e=[{label:`GoDDI`,path:`/`}],t=s.path;return t.startsWith(`/dns/client`)?(e.push({label:c(`nav.tools`),path:`/dns/client`}),e.push({label:c(`nav.dnsClient`),path:`/dns/client`})):t.startsWith(`/dns`)?(e.push({label:c(`nav.dns`),path:`/dns`}),t.includes(`/zones`)&&e.push({label:c(`nav.dnsZones`),path:`/dns/zones`}),t.includes(`/forwarders`)&&e.push({label:c(`nav.dnsForwarders`),path:`/dns/forwarders`}),t.includes(`/security`)&&e.push({label:c(`nav.dnsSecurity`),path:`/dns/security`}),t.includes(`/cache`)&&e.push({label:c(`nav.dnsCache`),path:`/dns/cache`}),t.includes(`/client`)&&e.push({label:c(`nav.dnsClient`),path:`/dns/client`})):t.startsWith(`/dhcp`)?(e.push({label:c(`nav.dhcp`),path:`/dhcp`}),t.includes(`/scopes`)&&e.push({label:c(`nav.dhcpScopes`),path:`/dhcp/scopes`}),t.includes(`/leases`)&&e.push({label:c(`nav.dhcpLeases`),path:`/dhcp/leases`}),t.includes(`/reservations`)&&e.push({label:c(`nav.dhcpReservations`),path:`/dhcp/reservations`}),t.includes(`/options`)&&e.push({label:c(`nav.dhcpOptions`),path:`/dhcp/options`})):t.startsWith(`/ipam`)?(e.push({label:c(`nav.ipam`),path:`/ipam`}),t.includes(`/spaces`)&&e.push({label:c(`nav.ipamSpaces`),path:`/ipam/spaces`}),t.includes(`/subnets`)&&e.push({label:c(`nav.ipamSubnets`),path:`/ipam/subnets`}),t.includes(`/addresses`)&&e.push({label:c(`nav.ipamAddresses`),path:`/ipam/addresses`})):t.startsWith(`/admin`)?(e.push({label:c(`nav.administration`),path:`/admin`}),t.includes(`/users`)&&e.push({label:c(`nav.adminUsers`),path:`/admin/users`}),t.includes(`/roles`)&&e.push({label:c(`nav.adminRoles`),path:`/admin/roles`}),t.includes(`/groups`)&&e.push({label:c(`nav.adminGroups`),path:`/admin/groups`}),t.includes(`/tokens`)&&e.push({label:c(`nav.adminTokens`),path:`/admin/tokens`}),t.includes(`/sessions`)&&e.push({label:c(`nav.adminSessions`),path:`/admin/sessions`})):t.startsWith(`/logs`)?(e.push({label:c(`nav.logs`),path:`/logs`}),t.includes(`/audit`)&&e.push({label:c(`nav.logsAudit`),path:`/logs/audit`}),t.includes(`/dns`)&&e.push({label:c(`nav.logsDns`),path:`/logs/dns`}),t.includes(`/dhcp`)&&e.push({label:c(`nav.logsDhcp`),path:`/logs/dhcp`})):t.startsWith(`/settings`)&&(e.push({label:c(`nav.administration`),path:`/admin`}),t.includes(`/backup`)?e.push({label:c(`nav.settingsBackup`),path:`/settings/backup`}):e.push({label:c(`nav.settings`),path:`/settings`})),e});function M(e){return()=>t(X,null,{default:()=>t(e)})}function N(e){let t=e.label;return typeof t==`function`?t():t||``}let P=[{key:`/`,label:()=>c(`nav.dashboard`),icon:M(un)},{key:`/dns`,label:()=>c(`nav.dns`),icon:M(Ge),children:[{key:`/dns/zones`,label:()=>c(`nav.dnsZones`),icon:M(gn)},{key:`/dns/forwarders`,label:()=>c(`nav.dnsForwarders`),icon:M(Pn)},{key:`/dns/security`,label:()=>c(`nav.dnsSecurity`),icon:M(An)},{key:`/dns/cache`,label:()=>c(`nav.dnsCache`),icon:M(Ke)}]},{key:`/dhcp`,label:()=>c(`nav.dhcp`),icon:M(Ue),children:[{key:`/dhcp/scopes`,label:()=>c(`nav.dhcpScopes`),icon:M(We)},{key:`/dhcp/leases`,label:()=>c(`nav.dhcpLeases`),icon:M(cn)},{key:`/dhcp/reservations`,label:()=>c(`nav.dhcpReservations`),icon:M(vn)},{key:`/dhcp/options`,label:()=>c(`nav.dhcpOptions`),icon:M(On)}]},{key:`/ipam`,label:()=>c(`nav.ipam`),icon:M(Ke),children:[{key:`/ipam/spaces`,label:()=>c(`nav.ipamSpaces`),icon:M(gn)},{key:`/ipam/subnets`,label:()=>c(`nav.ipamSubnets`),icon:M(We)},{key:`/ipam/addresses`,label:()=>c(`nav.ipamAddresses`),icon:M(vn)}]},{key:`/tools`,label:()=>c(`nav.tools`),icon:M(In),children:[{key:`/dns/client`,label:()=>c(`nav.dnsClient`),icon:M(In)}]},{key:`/logs`,label:()=>c(`nav.logs`),icon:M(Je),children:[{key:`/logs/audit`,label:()=>c(`nav.logsAudit`),icon:M(cn)},{key:`/logs/dns`,label:()=>c(`nav.logsDns`),icon:M(Ge)},{key:`/logs/dhcp`,label:()=>c(`nav.logsDhcp`),icon:M(Ue)}]},{key:`/admin`,label:()=>c(`nav.administration`),icon:M(En),children:[{key:`/admin/users`,label:()=>c(`nav.adminUsers`),icon:M(qe)},{key:`/admin/groups`,label:()=>c(`nav.adminGroups`),icon:M(En)},{key:`/admin/roles`,label:()=>c(`nav.adminRoles`),icon:M(fn)},{key:`/admin/tokens`,label:()=>c(`nav.adminTokens`),icon:M(fn)},{key:`/admin/sessions`,label:()=>c(`nav.adminSessions`),icon:M(Rn)},{key:`/settings`,label:()=>c(`nav.settings`),icon:M(On)},{key:`/settings/backup`,label:()=>c(`nav.settingsBackup`),icon:M(on)}]}],F={"/dns":{resource:`dns`,action:`read`},"/dhcp":{resource:`dhcp`,action:`read`},"/ipam":{resource:`ipam`,action:`read`},"/admin/users":{resource:`user`,action:`read`},"/admin/roles":{resource:`role`,action:`read`},"/admin/groups":{resource:`group`,action:`read`},"/admin/tokens":{resource:`token`,action:`read`},"/admin/sessions":{resource:`user`,action:`read`},"/logs/audit":{resource:`audit`,action:`read`},"/logs/dns":{resource:`dns`,action:`read`},"/logs/dhcp":{resource:`dhcp`,action:`read`},"/settings":{resource:`settings`,action:`read`},"/settings/backup":{resource:`backup`,action:`read`}};function I(e){let t=F[e];return!t||l.hasPermission(t.resource,t.action)}let R=g(()=>P.flatMap(e=>{let t=String(e.key);if(!I(t))return[];if(e.children){let t=e.children.filter(e=>I(String(e.key)));return t.length===0?[]:[{...e,children:t}]}return I(t)?[e]:[]}));function re(e){v.value=!1,a.push(e)}let ie=g(()=>[{label:`English`,key:`en-US`,disabled:p.locale===`en-US`},{label:`简体中文`,key:`zh-CN`,disabled:p.locale===`zh-CN`}]);function ae(e){p.setLocale(e)}let oe=[{label:()=>c(`auth.logout`),key:`logout`,icon:M(bn)}];async function se(e){e===`logout`&&(await l.logout(),a.push(`/login`))}return(t,r)=>{let i=rn,s=Pt,l=gt,u=ht,p=ne,g=et,S=Qe,O=Te,M=Oe,P=kt,F=w(`router-view`),I=Et,L=Tt;return f(),D(L,{"has-sider":``,class:`app-shell`},{default:e(()=>[h.value?_(``,!0):(f(),D(s,{key:0,bordered:``,"collapse-mode":`width`,"collapsed-width":64,width:232,collapsed:m.value,"show-trigger":``,onCollapse:r[0]||=e=>m.value=!0,onExpand:r[1]||=e=>m.value=!1,"native-scrollbar":!1,class:`desktop-sider`},{default:e(()=>[T(`div`,{class:x([`logo`,{"logo-collapsed":m.value}])},[r[4]||=T(`span`,{class:`logo-icon`},`G`,-1),m.value?_(``,!0):(f(),E(`span`,zn,`GoDDI`))],2),d(i,{collapsed:m.value,"collapsed-width":64,"collapsed-icon-size":22,options:R.value,value:j.value,"onUpdate:value":re,"render-label":N},null,8,[`collapsed`,`options`,`value`,`render-label`])]),_:1},8,[`collapsed`])),d(u,{show:v.value,"onUpdate:show":r[2]||=e=>v.value=e,placement:`left`,width:280},{default:e(()=>[d(l,{"body-content-style":`padding: 0;`,closable:``},{default:e(()=>[r[5]||=T(`div`,{class:`logo mobile-logo`},[T(`span`,{class:`logo-icon`},`G`),T(`span`,{class:`logo-text`},`GoDDI`)],-1),d(i,{options:R.value,value:j.value,"onUpdate:value":re,"render-label":N},null,8,[`options`,`value`,`render-label`])]),_:1})]),_:1},8,[`show`]),d(L,{class:`main-layout`},{default:e(()=>[d(P,{bordered:``,class:`app-header`},{default:e(()=>[T(`div`,Bn,[h.value?(f(),D(p,{key:0,quaternary:``,circle:``,"aria-label":`Open navigation`,onClick:r[3]||=e=>v.value=!0},{icon:e(()=>[d(b(X),null,{default:e(()=>[d(b(Sn))]),_:1})]),_:1})):_(``,!0),d(S,null,{default:e(()=>[(f(!0),E(y,null,o(te.value,t=>(f(),D(g,{key:t.path,class:`breadcrumb-item`,onClick:e=>b(a).resolve(t.path).matched.some(e=>e.path===t.path)&&b(a).push(t.path)},{default:e(()=>[ee(n(t.label),1)]),_:2},1032,[`onClick`]))),128))]),_:1})]),T(`div`,Vn,[d(O,{options:ie.value,onSelect:ae},{default:e(()=>[d(p,{quaternary:``,circle:``,"aria-label":b(c)(`common.language`)},{icon:e(()=>[d(b(X),null,{default:e(()=>[d(b(mn))]),_:1})]),_:1},8,[`aria-label`])]),_:1},8,[`options`]),d(M,{size:`small`,"aria-label":b(c)(`common.appearance`),value:C.value,"onUpdate:value":k},{checked:e(()=>[d(b(X),null,{default:e(()=>[d(b(Mn))]),_:1})]),unchecked:e(()=>[d(b(X),null,{default:e(()=>[d(b(wn))]),_:1})]),_:1},8,[`aria-label`,`value`]),d(O,{options:oe,onSelect:se},{default:e(()=>[d(p,{quaternary:``},{icon:e(()=>[d(b(X),null,{default:e(()=>[d(b(qe))]),_:1})]),default:e(()=>[h.value?_(``,!0):(f(),E(`span`,Hn,n(A.value),1))]),_:1})]),_:1})])]),_:1}),d(I,{"content-style":h.value?`padding: 24px 16px;`:`padding: 32px;`,"native-scrollbar":!1,class:x([`app-content`,{"app-content-dark":C.value}])},{default:e(()=>[T(`div`,Un,[d(F)])]),_:1},8,[`content-style`,`class`])]),_:1})]),_:1})}}}),[[`__scopeId`,`data-v-1951bf90`]]);export{Wn as default};