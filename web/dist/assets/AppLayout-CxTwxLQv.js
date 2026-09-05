import{$ as e,B as t,C as n,E as r,H as i,K as a,L as o,N as s,Ot as c,Q as l,S as u,U as d,W as f,X as p,Z as m,_ as h,b as g,ct as _,d as v,ft as y,g as b,j as x,k as S,kt as C,mt as w,v as T,w as E,y as D,z as O}from"./echarts-Cw2yHLaZ.js";import{A as k,At as A,C as j,Ct as M,Dt as ee,Et as N,Ft as P,Kt as F,Lt as I,M as L,N as R,O as z,Ot as B,Pt as V,Rt as H,St as U,Ut as W,_ as te,b as ne,dt as G,ft as K,h as re,k as ie,t as ae,u as oe,vt as se,w as ce,xt as le,y as ue,zt as q}from"./auth-CYMAsBzV.js";import{C as de,E as fe,c as pe,d as me,g as he,i as ge,m as _e,o as ve,r as ye,s as be,y as xe}from"./vue-core-BDTvi3xZ.js";import{a as Se,n as Ce,s as we,t as Te}from"./Dropdown-DJaIvDX1.js";import{o as J,s as Y}from"./get-CsENiSKu.js";import{t as Ee}from"./use-compitable-OuJoh8Ky.js";import{t as X}from"./Icon-BR-dxpzD.js";import{t as De}from"./Switch-BqlmE47V.js";import{at as Oe,c as ke,ct as Ae,d as je,dt as Me,ft as Ne,gt as Pe,ht as Fe,j as Ie,k as Le,mt as Re,pt as ze,rt as Z,t as Be,ut as Ve}from"./index-ChsYpqrV.js";import{i as He,n as Ue,r as We,t as Ge}from"./ServerOutline-DV6jFSlv.js";import{t as Ke}from"./PersonOutline-Xn-3hk7Y.js";import{t as qe}from"./SearchOutline-DJvOw2QO.js";import{t as Je}from"./_plugin-vue_export-helper-BDNMzG2s.js";var Ye=r({name:`ChevronDownFilled`,render(){return S(`svg`,{viewBox:`0 0 16 16`,fill:`none`,xmlns:`http://www.w3.org/2000/svg`},S(`path`,{d:`M3.20041 5.73966C3.48226 5.43613 3.95681 5.41856 4.26034 5.70041L8 9.22652L11.7397 5.70041C12.0432 5.41856 12.5177 5.43613 12.7996 5.73966C13.0815 6.0432 13.0639 6.51775 12.7603 6.7996L8.51034 10.7996C8.22258 11.0668 7.77743 11.0668 7.48967 10.7996L3.23966 6.7996C2.93613 6.51775 2.91856 6.0432 3.20041 5.73966Z`,fill:`currentColor`}))}}),Xe=P(`breadcrumb`,`
 white-space: nowrap;
 cursor: default;
 line-height: var(--n-item-line-height);
`,[V(`ul`,`
 list-style: none;
 padding: 0;
 margin: 0;
 `),V(`a`,`
 color: inherit;
 text-decoration: inherit;
 `),P(`breadcrumb-item`,`
 font-size: var(--n-font-size);
 transition: color .3s var(--n-bezier);
 display: inline-flex;
 align-items: center;
 `,[P(`icon`,`
 font-size: 18px;
 vertical-align: -.2em;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `),V(`&:not(:last-child)`,[H(`clickable`,[I(`link`,`
 cursor: pointer;
 `,[V(`&:hover`,`
 background-color: var(--n-item-color-hover);
 `),V(`&:active`,`
 background-color: var(--n-item-color-pressed); 
 `)])])]),I(`link`,`
 padding: 4px;
 border-radius: var(--n-item-border-radius);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 position: relative;
 `,[V(`&:hover`,`
 color: var(--n-item-text-color-hover);
 `,[P(`icon`,`
 color: var(--n-item-text-color-hover);
 `)]),V(`&:active`,`
 color: var(--n-item-text-color-pressed);
 `,[P(`icon`,`
 color: var(--n-item-text-color-pressed);
 `)])]),I(`separator`,`
 margin: 0 8px;
 color: var(--n-separator-color);
 transition: color .3s var(--n-bezier);
 user-select: none;
 -webkit-user-select: none;
 `),V(`&:last-child`,[I(`link`,`
 font-weight: var(--n-font-weight-active);
 cursor: unset;
 color: var(--n-item-text-color-active);
 `,[P(`icon`,`
 color: var(--n-item-text-color-active);
 `)]),I(`separator`,`
 display: none;
 `)])])]),Ze=B(`n-breadcrumb`),Qe=r({name:`Breadcrumb`,props:Object.assign(Object.assign({},k.props),{separator:{type:String,default:`/`}}),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=K(e),r=k(`Breadcrumb`,`-breadcrumb`,Xe,Le,e,t);d(Ze,{separatorRef:y(e,`separator`),mergedClsPrefixRef:t});let i=b(()=>{let{common:{cubicBezierEaseInOut:e},self:{separatorColor:t,itemTextColor:n,itemTextColorHover:i,itemTextColorPressed:a,itemTextColorActive:o,fontSize:s,fontWeightActive:c,itemBorderRadius:l,itemColorHover:u,itemColorPressed:d,itemLineHeight:f}}=r.value;return{"--n-font-size":s,"--n-bezier":e,"--n-item-text-color":n,"--n-item-text-color-hover":i,"--n-item-text-color-pressed":a,"--n-item-text-color-active":o,"--n-separator-color":t,"--n-item-color-hover":u,"--n-item-color-pressed":d,"--n-item-border-radius":l,"--n-font-weight-active":c,"--n-item-line-height":f}}),a=n?G(`breadcrumb`,void 0,i,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),S(`nav`,{class:[`${this.mergedClsPrefix}-breadcrumb`,this.themeClass],style:this.cssVars,"aria-label":`Breadcrumb`},S(`ul`,null,this.$slots))}});function $e(e=ee?window:null){let n=()=>{let{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}=e?.location||{};return{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}},r=_(n()),i=()=>{r.value=n()};return O(()=>{e&&(e.addEventListener(`popstate`,i),e.addEventListener(`hashchange`,i))}),t(()=>{e&&(e.removeEventListener(`popstate`,i),e.removeEventListener(`hashchange`,i))}),r}var et=r({name:`BreadcrumbItem`,props:{separator:String,href:String,clickable:{type:Boolean,default:!0},showSeparator:{type:Boolean,default:!0},onClick:Function},slots:Object,setup(e,{slots:t}){let n=x(Ze,null);if(!n)return()=>null;let{separatorRef:r,mergedClsPrefixRef:i}=n,a=$e(),o=b(()=>e.href?`a`:`span`),s=b(()=>a.value.href===e.href?`location`:null);return()=>{let{value:n}=i;return S(`li`,{class:[`${n}-breadcrumb-item`,e.clickable&&`${n}-breadcrumb-item--clickable`]},S(o.value,{class:`${n}-breadcrumb-item__link`,"aria-current":s.value,href:e.href,onClick:e.onClick},t),e.showSeparator&&S(`span`,{class:`${n}-breadcrumb-item__separator`,"aria-hidden":`true`},se(t.separator,()=>[e.separator??r.value])))}}}),tt=r({name:`NDrawerContent`,inheritAttrs:!1,props:{blockScroll:Boolean,show:{type:Boolean,default:void 0},displayDirective:{type:String,required:!0},placement:{type:String,required:!0},contentClass:String,contentStyle:[Object,String],nativeScrollbar:{type:Boolean,required:!0},scrollbarProps:Object,trapFocus:{type:Boolean,default:!0},autoFocus:{type:Boolean,default:!0},showMask:{type:[Boolean,String],required:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,onClickoutside:Function,onAfterLeave:Function,onAfterEnter:Function,onEsc:Function},setup(e){let t=_(!!e.show),n=_(null),r=x(Pe),i=0,a=``,s=null,c=_(!1),l=_(!1),u=b(()=>e.placement===`top`||e.placement===`bottom`),{mergedClsPrefixRef:f,mergedRtlRef:h}=K(e),g=R(`Drawer`,h,f),v=k,y=e=>{l.value=!0,i=u.value?e.clientY:e.clientX,a=document.body.style.cursor,document.body.style.cursor=u.value?`ns-resize`:`ew-resize`,document.body.addEventListener(`mousemove`,O),document.body.addEventListener(`mouseleave`,v),document.body.addEventListener(`mouseup`,k)},S=()=>{s!==null&&(window.clearTimeout(s),s=null),l.value?c.value=!0:s=window.setTimeout(()=>{c.value=!0},300)},C=()=>{s!==null&&(window.clearTimeout(s),s=null),c.value=!1},{doUpdateHeight:w,doUpdateWidth:T}=r,E=t=>{let{maxWidth:n}=e;if(n&&t>n)return n;let{minWidth:r}=e;return r&&t<r?r:t},D=t=>{let{maxHeight:n}=e;if(n&&t>n)return n;let{minHeight:r}=e;return r&&t<r?r:t};function O(t){if(l.value)if(u.value){let r=n.value?.offsetHeight||0,a=i-t.clientY;r+=e.placement===`bottom`?a:-a,r=D(r),w(r),i=t.clientY}else{let r=n.value?.offsetWidth||0,a=i-t.clientX;r+=e.placement===`right`?a:-a,r=E(r),T(r),i=t.clientX}}function k(){l.value&&(i=0,l.value=!1,document.body.style.cursor=a,document.body.removeEventListener(`mousemove`,O),document.body.removeEventListener(`mouseup`,k),document.body.removeEventListener(`mouseleave`,v))}m(()=>{e.show&&(t.value=!0)}),p(()=>e.show,e=>{e||k()}),o(()=>{k()});let A=b(()=>{let{show:t}=e,n=[[F,t]];return e.showMask||n.push([Ve,e.onClickoutside,void 0,{capture:!0}]),n});function j(){var n;t.value=!1,(n=e.onAfterLeave)==null||n.call(e)}return Me(b(()=>e.blockScroll&&t.value)),d(Fe,n),d(ze,null),d(Re,null),{bodyRef:n,rtlEnabled:g,mergedClsPrefix:r.mergedClsPrefixRef,isMounted:r.isMountedRef,mergedTheme:r.mergedThemeRef,displayed:t,transitionName:b(()=>({right:`slide-in-from-right-transition`,left:`slide-in-from-left-transition`,top:`slide-in-from-top-transition`,bottom:`slide-in-from-bottom-transition`})[e.placement]),handleAfterLeave:j,bodyDirectives:A,handleMousedownResizeTrigger:y,handleMouseenterResizeTrigger:S,handleMouseleaveResizeTrigger:C,isDragging:l,isHoverOnResizeTrigger:c}},render(){let{$slots:t,mergedClsPrefix:n}=this;return this.displayDirective===`show`||this.displayed||this.show?e(S(`div`,{role:`none`},S(be,{disabled:!this.showMask||!this.trapFocus,active:this.show,autoFocus:this.autoFocus,onEsc:this.onEsc},{default:()=>S(W,{name:this.transitionName,appear:this.isMounted,onAfterEnter:this.onAfterEnter,onAfterLeave:this.handleAfterLeave},{default:()=>e(S(`div`,s(this.$attrs,{role:`dialog`,ref:`bodyRef`,"aria-modal":`true`,class:[`${n}-drawer`,this.rtlEnabled&&`${n}-drawer--rtl`,`${n}-drawer--${this.placement}-placement`,this.isDragging&&`${n}-drawer--unselectable`,this.nativeScrollbar&&`${n}-drawer--native-scrollbar`]}),[this.resizable?S(`div`,{class:[`${n}-drawer__resize-trigger`,(this.isDragging||this.isHoverOnResizeTrigger)&&`${n}-drawer__resize-trigger--hover`],onMouseenter:this.handleMouseenterResizeTrigger,onMouseleave:this.handleMouseleaveResizeTrigger,onMousedown:this.handleMousedownResizeTrigger}):null,this.nativeScrollbar?S(`div`,{class:[`${n}-drawer-content-wrapper`,this.contentClass],style:this.contentStyle,role:`none`},t):S(re,Object.assign({},this.scrollbarProps,{contentStyle:this.contentStyle,contentClass:[`${n}-drawer-content-wrapper`,this.contentClass],theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar}),t)]),this.bodyDirectives)})})),[[F,this.displayDirective===`if`||this.displayed||this.show]]):null}}),{cubicBezierEaseIn:nt,cubicBezierEaseOut:rt}=L;function it({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-bottom`}={}){return[V(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${nt}`}),V(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${rt}`}),V(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),V(`&.${n}-transition-enter-from`,{transform:`translateY(100%)`}),V(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),V(`&.${n}-transition-leave-to`,{transform:`translateY(100%)`})]}var{cubicBezierEaseIn:at,cubicBezierEaseOut:ot}=L;function st({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-left`}={}){return[V(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${at}`}),V(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ot}`}),V(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),V(`&.${n}-transition-enter-from`,{transform:`translateX(-100%)`}),V(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),V(`&.${n}-transition-leave-to`,{transform:`translateX(-100%)`})]}var{cubicBezierEaseIn:ct,cubicBezierEaseOut:lt}=L;function ut({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-right`}={}){return[V(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ct}`}),V(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${lt}`}),V(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),V(`&.${n}-transition-enter-from`,{transform:`translateX(100%)`}),V(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),V(`&.${n}-transition-leave-to`,{transform:`translateX(100%)`})]}var{cubicBezierEaseIn:dt,cubicBezierEaseOut:ft}=L;function pt({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-top`}={}){return[V(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${dt}`}),V(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ft}`}),V(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),V(`&.${n}-transition-enter-from`,{transform:`translateY(-100%)`}),V(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),V(`&.${n}-transition-leave-to`,{transform:`translateY(-100%)`})]}var mt=V([P(`drawer`,`
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
 `,[ut(),st(),pt(),it(),H(`unselectable`,`
 user-select: none; 
 -webkit-user-select: none;
 `),H(`native-scrollbar`,[P(`drawer-content-wrapper`,`
 overflow: auto;
 height: 100%;
 `)]),I(`resize-trigger`,`
 position: absolute;
 background-color: #0000;
 transition: background-color .3s var(--n-bezier);
 `,[H(`hover`,`
 background-color: var(--n-resize-trigger-color-hover);
 `)]),P(`drawer-content-wrapper`,`
 box-sizing: border-box;
 `),P(`drawer-content`,`
 height: 100%;
 display: flex;
 flex-direction: column;
 `,[H(`native-scrollbar`,[P(`drawer-body-content-wrapper`,`
 height: 100%;
 overflow: auto;
 `)]),P(`drawer-body`,`
 flex: 1 0 0;
 overflow: hidden;
 `),P(`drawer-body-content-wrapper`,`
 box-sizing: border-box;
 padding: var(--n-body-padding);
 `),P(`drawer-header`,`
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
 `,[I(`main`,`
 flex: 1;
 `),I(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)]),P(`drawer-footer`,`
 display: flex;
 justify-content: flex-end;
 border-top: var(--n-footer-border-top);
 transition: border .3s var(--n-bezier);
 padding: var(--n-footer-padding);
 `)]),H(`right-placement`,`
 top: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-bottom-left-radius: var(--n-border-radius);
 `,[I(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 left: 0;
 transform: translateX(-1.5px);
 cursor: ew-resize;
 `)]),H(`left-placement`,`
 top: 0;
 bottom: 0;
 left: 0;
 border-top-right-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[I(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 right: 0;
 transform: translateX(1.5px);
 cursor: ew-resize;
 `)]),H(`top-placement`,`
 top: 0;
 left: 0;
 right: 0;
 border-bottom-left-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[I(`resize-trigger`,`
 width: 100%;
 height: 3px;
 bottom: 0;
 left: 0;
 transform: translateY(1.5px);
 cursor: ns-resize;
 `)]),H(`bottom-placement`,`
 left: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-top-right-radius: var(--n-border-radius);
 `,[I(`resize-trigger`,`
 width: 100%;
 height: 3px;
 top: 0;
 left: 0;
 transform: translateY(-1.5px);
 cursor: ns-resize;
 `)])]),V(`body`,[V(`>`,[P(`drawer-container`,`
 position: fixed;
 `)])]),P(`drawer-container`,`
 position: relative;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 `,[V(`> *`,`
 pointer-events: all;
 `)]),P(`drawer-mask`,`
 background-color: rgba(0, 0, 0, .3);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `,[H(`invisible`,`
 background-color: rgba(0, 0, 0, 0)
 `),ne({enterDuration:`0.2s`,leaveDuration:`0.2s`,enterCubicBezier:`var(--n-bezier-in)`,leaveCubicBezier:`var(--n-bezier-out)`})])]),ht=r({name:`Drawer`,inheritAttrs:!1,props:Object.assign(Object.assign({},k.props),{show:Boolean,width:[Number,String],height:[Number,String],placement:{type:String,default:`right`},maskClosable:{type:Boolean,default:!0},showMask:{type:[Boolean,String],default:!0},to:[String,Object],displayDirective:{type:String,default:`if`},nativeScrollbar:{type:Boolean,default:!0},zIndex:Number,onMaskClick:Function,scrollbarProps:Object,contentClass:String,contentStyle:[Object,String],trapFocus:{type:Boolean,default:!0},onEsc:Function,autoFocus:{type:Boolean,default:!0},closeOnEsc:{type:Boolean,default:!0},blockScroll:{type:Boolean,default:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,defaultWidth:{type:[Number,String],default:251},defaultHeight:{type:[Number,String],default:251},onUpdateWidth:[Function,Array],onUpdateHeight:[Function,Array],"onUpdate:width":[Function,Array],"onUpdate:height":[Function,Array],"onUpdate:show":[Function,Array],onUpdateShow:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,drawerStyle:[String,Object],drawerClass:String,target:null,onShow:Function,onHide:Function}),setup(e){let{mergedClsPrefixRef:t,namespaceRef:n,inlineThemeDisabled:r}=K(e),i=xe(),a=k(`Drawer`,`-drawer`,mt,je,e,t),o=_(e.defaultWidth),s=_(e.defaultHeight),c=Y(y(e,`width`),o),l=Y(y(e,`height`),s),u=b(()=>{let{placement:t}=e;return t===`top`||t===`bottom`?``:J(c.value)}),f=b(()=>{let{placement:t}=e;return t===`left`||t===`right`?``:J(l.value)}),p=t=>{let{onUpdateWidth:n,"onUpdate:width":r}=e;n&&U(n,t),r&&U(r,t),o.value=t},m=t=>{let{onUpdateHeight:n,"onUpdate:width":r}=e;n&&U(n,t),r&&U(r,t),s.value=t},h=b(()=>[{width:u.value,height:f.value},e.drawerStyle||``]);function g(t){let{onMaskClick:n,maskClosable:r}=e;r&&C(!1),n&&n(t)}function v(e){g(e)}let x=Ne();function S(t){var n;(n=e.onEsc)==null||n.call(e),e.show&&e.closeOnEsc&&Ae(t)&&(x.value||C(!1))}function C(t){let{onHide:n,onUpdateShow:r,"onUpdate:show":i}=e;r&&U(r,t),i&&U(i,t),n&&!t&&U(n,t)}d(Pe,{isMountedRef:i,mergedThemeRef:a,mergedClsPrefixRef:t,doUpdateShow:C,doUpdateHeight:m,doUpdateWidth:p});let w=b(()=>{let{common:{cubicBezierEaseInOut:e,cubicBezierEaseIn:t,cubicBezierEaseOut:n},self:{color:r,textColor:i,boxShadow:o,lineHeight:s,headerPadding:c,footerPadding:l,borderRadius:u,bodyPadding:d,titleFontSize:f,titleTextColor:p,titleFontWeight:m,headerBorderBottom:h,footerBorderTop:g,closeIconColor:_,closeIconColorHover:v,closeIconColorPressed:y,closeColorHover:b,closeColorPressed:x,closeIconSize:S,closeSize:C,closeBorderRadius:w,resizableTriggerColorHover:T}}=a.value;return{"--n-line-height":s,"--n-color":r,"--n-border-radius":u,"--n-text-color":i,"--n-box-shadow":o,"--n-bezier":e,"--n-bezier-out":n,"--n-bezier-in":t,"--n-header-padding":c,"--n-body-padding":d,"--n-footer-padding":l,"--n-title-text-color":p,"--n-title-font-size":f,"--n-title-font-weight":m,"--n-header-border-bottom":h,"--n-footer-border-top":g,"--n-close-icon-color":_,"--n-close-icon-color-hover":v,"--n-close-icon-color-pressed":y,"--n-close-size":C,"--n-close-color-hover":b,"--n-close-color-pressed":x,"--n-close-icon-size":S,"--n-close-border-radius":w,"--n-resize-trigger-color-hover":T}}),T=r?G(`drawer`,void 0,w,e):void 0;return{mergedClsPrefix:t,namespace:n,mergedBodyStyle:h,handleOutsideClick:v,handleMaskClick:g,handleEsc:S,mergedTheme:a,cssVars:r?void 0:w,themeClass:T?.themeClass,onRender:T?.onRender,isMounted:i}},render(){let{mergedClsPrefix:t}=this;return S(_e,{to:this.to,show:this.show},{default:()=>{var n;return(n=this.onRender)==null||n.call(this),e(S(`div`,{class:[`${t}-drawer-container`,this.namespace,this.themeClass],style:this.cssVars,role:`none`},this.showMask?S(W,{name:`fade-in-transition`,appear:this.isMounted},{default:()=>this.show?S(`div`,{"aria-hidden":!0,class:[`${t}-drawer-mask`,this.showMask===`transparent`&&`${t}-drawer-mask--invisible`],onClick:this.handleMaskClick}):null}):null,S(tt,Object.assign({},this.$attrs,{class:[this.drawerClass,this.$attrs.class],style:[this.mergedBodyStyle,this.$attrs.style],blockScroll:this.blockScroll,contentStyle:this.contentStyle,contentClass:this.contentClass,placement:this.placement,scrollbarProps:this.scrollbarProps,show:this.show,displayDirective:this.displayDirective,nativeScrollbar:this.nativeScrollbar,onAfterEnter:this.onAfterEnter,onAfterLeave:this.onAfterLeave,trapFocus:this.trapFocus,autoFocus:this.autoFocus,resizable:this.resizable,maxHeight:this.maxHeight,minHeight:this.minHeight,maxWidth:this.maxWidth,minWidth:this.minWidth,showMask:this.showMask,onEsc:this.handleEsc,onClickoutside:this.handleOutsideClick}),this.$slots)),[[he,{zIndex:this.zIndex,enabled:this.show}]])}})}}),gt=r({name:`DrawerContent`,props:{title:String,headerClass:String,headerStyle:[Object,String],footerClass:String,footerStyle:[Object,String],bodyClass:String,bodyStyle:[Object,String],bodyContentClass:String,bodyContentStyle:[Object,String],nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,closable:Boolean},slots:Object,setup(){let e=x(Pe,null);e||M(`drawer-content`,"`n-drawer-content` must be placed inside `n-drawer`.");let{doUpdateShow:t}=e;function n(){t(!1)}return{handleCloseClick:n,mergedTheme:e.mergedThemeRef,mergedClsPrefix:e.mergedClsPrefixRef}},render(){let{title:e,mergedClsPrefix:t,nativeScrollbar:n,mergedTheme:r,bodyClass:i,bodyStyle:a,bodyContentClass:o,bodyContentStyle:s,headerClass:c,headerStyle:l,footerClass:u,footerStyle:d,scrollbarProps:f,closable:p,$slots:m}=this;return S(`div`,{role:`none`,class:[`${t}-drawer-content`,n&&`${t}-drawer-content--native-scrollbar`]},m.header||e||p?S(`div`,{class:[`${t}-drawer-header`,c],style:l,role:`none`},S(`div`,{class:`${t}-drawer-header__main`,role:`heading`,"aria-level":`1`},m.header===void 0?e:m.header()),p&&S(ce,{onClick:this.handleCloseClick,clsPrefix:t,class:`${t}-drawer-header__close`,absolute:!0})):null,n?S(`div`,{class:[`${t}-drawer-body`,i],style:a,role:`none`},S(`div`,{class:[`${t}-drawer-body-content-wrapper`,o],style:s,role:`none`},m)):S(re,Object.assign({themeOverrides:r.peerOverrides.Scrollbar,theme:r.peers.Scrollbar},f,{class:`${t}-drawer-body`,contentClass:[`${t}-drawer-body-content-wrapper`,o],contentStyle:s}),m),m.footer?S(`div`,{class:[`${t}-drawer-footer`,u],style:d,role:`none`},m.footer()):null)}});function _t(e){let{baseColor:t,textColor2:n,bodyColor:r,cardColor:i,dividerColor:a,actionColor:o,scrollbarColor:s,scrollbarColorHover:c,invertedColor:l}=e;return{textColor:n,textColorInverted:`#FFF`,color:r,colorEmbedded:o,headerColor:i,headerColorInverted:l,footerColor:o,footerColorInverted:l,headerBorderColor:a,headerBorderColorInverted:l,footerBorderColor:a,footerBorderColorInverted:l,siderBorderColor:a,siderBorderColorInverted:l,siderColor:i,siderColorInverted:l,siderToggleButtonBorder:`1px solid ${a}`,siderToggleButtonColor:t,siderToggleButtonIconColor:n,siderToggleButtonIconColorInverted:n,siderToggleBarColor:A(r,s),siderToggleBarColorHover:A(r,c),__invertScrollbar:`true`}}var vt=ie({name:`Layout`,common:ue,peers:{Scrollbar:te},self:_t}),yt=B(`n-layout-sider`),bt={type:String,default:`static`},xt=P(`layout`,`
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
`,[P(`layout-scroll-container`,`
 overflow-x: hidden;
 box-sizing: border-box;
 height: 100%;
 `),H(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),St={embedded:Boolean,position:bt,nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,onScroll:Function,contentClass:String,contentStyle:{type:[String,Object],default:``},hasSider:Boolean,siderPlacement:{type:String,default:`left`}},Ct=B(`n-layout`);function wt(e){return r({name:e?`LayoutContent`:`Layout`,props:Object.assign(Object.assign({},k.props),St),setup(e){let t=_(null),n=_(null),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=K(e),a=k(`Layout`,`-layout`,xt,vt,e,r);function o(r,i){if(e.nativeScrollbar){let{value:e}=t;e&&(i===void 0?e.scrollTo(r):e.scrollTo(r,i))}else{let{value:e}=n;e&&e.scrollTo(r,i)}}d(Ct,e);let s=0,c=0,l=t=>{var n;let r=t.target;s=r.scrollLeft,c=r.scrollTop,(n=e.onScroll)==null||n.call(e,t)};N(()=>{if(e.nativeScrollbar){let e=t.value;e&&(e.scrollTop=c,e.scrollLeft=s)}});let u={display:`flex`,flexWrap:`nowrap`,width:`100%`,flexDirection:`row`},f={scrollTo:o},p=b(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=a.value;return{"--n-bezier":t,"--n-color":e.embedded?n.colorEmbedded:n.color,"--n-text-color":n.textColor}}),m=i?G(`layout`,b(()=>e.embedded?`e`:``),p,e):void 0;return Object.assign({mergedClsPrefix:r,scrollableElRef:t,scrollbarInstRef:n,hasSiderStyle:u,mergedTheme:a,handleNativeElScroll:l,cssVars:i?void 0:p,themeClass:m?.themeClass,onRender:m?.onRender},f)},render(){var t;let{mergedClsPrefix:n,hasSider:r}=this;(t=this.onRender)==null||t.call(this);let i=r?this.hasSiderStyle:void 0;return S(`div`,{class:[this.themeClass,e&&`${n}-layout-content`,`${n}-layout`,`${n}-layout--${this.position}-positioned`],style:this.cssVars},this.nativeScrollbar?S(`div`,{ref:`scrollableElRef`,class:[`${n}-layout-scroll-container`,this.contentClass],style:[this.contentStyle,i],onScroll:this.handleNativeElScroll},this.$slots):S(re,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,contentClass:this.contentClass,contentStyle:[this.contentStyle,i]}),this.$slots))}})}var Tt=wt(!1),Et=wt(!0),Dt=P(`layout-header`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 box-sizing: border-box;
 width: 100%;
 background-color: var(--n-color);
 color: var(--n-text-color);
`,[H(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 `),H(`bordered`,`
 border-bottom: solid 1px var(--n-border-color);
 `)]),Ot={position:bt,inverted:Boolean,bordered:{type:Boolean,default:!1}},kt=r({name:`LayoutHeader`,props:Object.assign(Object.assign({},k.props),Ot),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=K(e),r=k(`Layout`,`-layout-header`,Dt,vt,e,t),i=b(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=r.value,i={"--n-bezier":t};return e.inverted?(i[`--n-color`]=n.headerColorInverted,i[`--n-text-color`]=n.textColorInverted,i[`--n-border-color`]=n.headerBorderColorInverted):(i[`--n-color`]=n.headerColor,i[`--n-text-color`]=n.textColor,i[`--n-border-color`]=n.headerBorderColor),i}),a=n?G(`layout-header`,b(()=>e.inverted?`a`:`b`),i,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{mergedClsPrefix:t}=this;return(e=this.onRender)==null||e.call(this),S(`div`,{class:[`${t}-layout-header`,this.themeClass,this.position&&`${t}-layout-header--${this.position}-positioned`,this.bordered&&`${t}-layout-header--bordered`],style:this.cssVars},this.$slots)}}),At=P(`layout-sider`,`
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
`,[H(`bordered`,[I(`border`,`
 content: "";
 position: absolute;
 top: 0;
 bottom: 0;
 width: 1px;
 background-color: var(--n-border-color);
 transition: background-color .3s var(--n-bezier);
 `)]),I(`left-placement`,[H(`bordered`,[I(`border`,`
 right: 0;
 `)])]),H(`right-placement`,`
 justify-content: flex-start;
 `,[H(`bordered`,[I(`border`,`
 left: 0;
 `)]),H(`collapsed`,[P(`layout-toggle-button`,[P(`base-icon`,`
 transform: rotate(180deg);
 `)]),P(`layout-toggle-bar`,[V(`&:hover`,[I(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),I(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])])]),P(`layout-toggle-button`,`
 left: 0;
 transform: translateX(-50%) translateY(-50%);
 `,[P(`base-icon`,`
 transform: rotate(0);
 `)]),P(`layout-toggle-bar`,`
 left: -28px;
 transform: rotate(180deg);
 `,[V(`&:hover`,[I(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),I(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})])])]),H(`collapsed`,[P(`layout-toggle-bar`,[V(`&:hover`,[I(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),I(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])]),P(`layout-toggle-button`,[P(`base-icon`,`
 transform: rotate(0);
 `)])]),P(`layout-toggle-button`,`
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
 `,[P(`base-icon`,`
 transition: transform .3s var(--n-bezier);
 transform: rotate(180deg);
 `)]),P(`layout-toggle-bar`,`
 cursor: pointer;
 height: 72px;
 width: 32px;
 position: absolute;
 top: calc(50% - 36px);
 right: -28px;
 `,[I(`top, bottom`,`
 position: absolute;
 width: 4px;
 border-radius: 2px;
 height: 38px;
 left: 14px;
 transition: 
 background-color .3s var(--n-bezier),
 transform .3s var(--n-bezier);
 `),I(`bottom`,`
 position: absolute;
 top: 34px;
 `),V(`&:hover`,[I(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),I(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})]),I(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color)`}),V(`&:hover`,[I(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color-hover)`})])]),I(`border`,`
 position: absolute;
 top: 0;
 right: 0;
 bottom: 0;
 width: 1px;
 transition: background-color .3s var(--n-bezier);
 `),P(`layout-sider-scroll-container`,`
 flex-grow: 1;
 flex-shrink: 0;
 box-sizing: border-box;
 height: 100%;
 opacity: 0;
 transition: opacity .3s var(--n-bezier);
 max-width: 100%;
 `),H(`show-content`,[P(`layout-sider-scroll-container`,{opacity:1})]),H(`absolute-positioned`,`
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 `)]),jt=r({props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return S(`div`,{onClick:this.onClick,class:`${e}-layout-toggle-bar`},S(`div`,{class:`${e}-layout-toggle-bar__top`}),S(`div`,{class:`${e}-layout-toggle-bar__bottom`}))}}),Mt=r({name:`LayoutToggleButton`,props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return S(`div`,{class:`${e}-layout-toggle-button`,onClick:this.onClick},S(z,{clsPrefix:e},{default:()=>S(we,null)}))}}),Nt={position:bt,bordered:Boolean,collapsedWidth:{type:Number,default:48},width:{type:[Number,String],default:272},contentClass:String,contentStyle:{type:[String,Object],default:``},collapseMode:{type:String,default:`transform`},collapsed:{type:Boolean,default:void 0},defaultCollapsed:Boolean,showCollapsedContent:{type:Boolean,default:!0},showTrigger:{type:[Boolean,String],default:!1},nativeScrollbar:{type:Boolean,default:!0},inverted:Boolean,scrollbarProps:Object,triggerClass:String,triggerStyle:[String,Object],collapsedTriggerClass:String,collapsedTriggerStyle:[String,Object],"onUpdate:collapsed":[Function,Array],onUpdateCollapsed:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,onExpand:[Function,Array],onCollapse:[Function,Array],onScroll:Function},Pt=r({name:`LayoutSider`,props:Object.assign(Object.assign({},k.props),Nt),setup(e){let t=x(Ct),n=_(null),r=_(null),i=_(e.defaultCollapsed),a=Y(y(e,`collapsed`),i),o=b(()=>J(a.value?e.collapsedWidth:e.width)),s=b(()=>e.collapseMode===`transform`?{minWidth:J(e.width)}:{}),c=b(()=>t?t.siderPlacement:`left`);function l(t,i){if(e.nativeScrollbar){let{value:e}=n;e&&(i===void 0?e.scrollTo(t):e.scrollTo(t,i))}else{let{value:e}=r;e&&e.scrollTo(t,i)}}function u(){let{"onUpdate:collapsed":t,onUpdateCollapsed:n,onExpand:r,onCollapse:o}=e,{value:s}=a;n&&U(n,!s),t&&U(t,!s),i.value=!s,s?r&&U(r):o&&U(o)}let f=0,p=0,m=t=>{var n;let r=t.target;f=r.scrollLeft,p=r.scrollTop,(n=e.onScroll)==null||n.call(e,t)};N(()=>{if(e.nativeScrollbar){let e=n.value;e&&(e.scrollTop=p,e.scrollLeft=f)}}),d(yt,{collapsedRef:a,collapseModeRef:y(e,`collapseMode`)});let{mergedClsPrefixRef:h,inlineThemeDisabled:g}=K(e),v=k(`Layout`,`-layout-sider`,At,vt,e,h);function S(t){var n,r;t.propertyName===`max-width`&&(a.value?(n=e.onAfterLeave)==null||n.call(e):(r=e.onAfterEnter)==null||r.call(e))}let C={scrollTo:l},w=b(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=v.value,{siderToggleButtonColor:r,siderToggleButtonBorder:i,siderToggleBarColor:a,siderToggleBarColorHover:o}=n,s={"--n-bezier":t,"--n-toggle-button-color":r,"--n-toggle-button-border":i,"--n-toggle-bar-color":a,"--n-toggle-bar-color-hover":o};return e.inverted?(s[`--n-color`]=n.siderColorInverted,s[`--n-text-color`]=n.textColorInverted,s[`--n-border-color`]=n.siderBorderColorInverted,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColorInverted,s.__invertScrollbar=n.__invertScrollbar):(s[`--n-color`]=n.siderColor,s[`--n-text-color`]=n.textColor,s[`--n-border-color`]=n.siderBorderColor,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColor),s}),T=g?G(`layout-sider`,b(()=>e.inverted?`a`:`b`),w,e):void 0;return Object.assign({scrollableElRef:n,scrollbarInstRef:r,mergedClsPrefix:h,mergedTheme:v,styleMaxWidth:o,mergedCollapsed:a,scrollContainerStyle:s,siderPlacement:c,handleNativeElScroll:m,handleTransitionend:S,handleTriggerClick:u,inlineThemeDisabled:g,cssVars:w,themeClass:T?.themeClass,onRender:T?.onRender},C)},render(){var e;let{mergedClsPrefix:t,mergedCollapsed:n,showTrigger:r}=this;return(e=this.onRender)==null||e.call(this),S(`aside`,{class:[`${t}-layout-sider`,this.themeClass,`${t}-layout-sider--${this.position}-positioned`,`${t}-layout-sider--${this.siderPlacement}-placement`,this.bordered&&`${t}-layout-sider--bordered`,n&&`${t}-layout-sider--collapsed`,(!n||this.showCollapsedContent)&&`${t}-layout-sider--show-content`],onTransitionend:this.handleTransitionend,style:[this.inlineThemeDisabled?void 0:this.cssVars,{maxWidth:this.styleMaxWidth,width:J(this.width)}]},this.nativeScrollbar?S(`div`,{class:[`${t}-layout-sider-scroll-container`,this.contentClass],onScroll:this.handleNativeElScroll,style:[this.scrollContainerStyle,{overflow:`auto`},this.contentStyle],ref:`scrollableElRef`},this.$slots):S(re,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,style:this.scrollContainerStyle,contentStyle:this.contentStyle,contentClass:this.contentClass,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,builtinThemeOverrides:this.inverted&&this.cssVars.__invertScrollbar===`true`?{colorHover:`rgba(255, 255, 255, .4)`,color:`rgba(255, 255, 255, .3)`}:void 0}),this.$slots),r?S(r===`bar`?jt:Mt,{clsPrefix:t,class:n?this.collapsedTriggerClass:this.triggerClass,style:n?this.collapsedTriggerStyle:this.triggerStyle,onClick:this.handleTriggerClick}):null,this.bordered?S(`div`,{class:`${t}-layout-sider__border`}):null)}}),Q=B(`n-menu`),Ft=B(`n-submenu`),It=B(`n-menu-item-group`),Lt=[V(`&::before`,`background-color: var(--n-item-color-hover);`),I(`arrow`,`
 color: var(--n-arrow-color-hover);
 `),I(`icon`,`
 color: var(--n-item-icon-color-hover);
 `),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover);
 `,[V(`a`,`
 color: var(--n-item-text-color-hover);
 `),I(`extra`,`
 color: var(--n-item-text-color-hover);
 `)])],Rt=[I(`icon`,`
 color: var(--n-item-icon-color-hover-horizontal);
 `),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover-horizontal);
 `,[V(`a`,`
 color: var(--n-item-text-color-hover-horizontal);
 `),I(`extra`,`
 color: var(--n-item-text-color-hover-horizontal);
 `)])],zt=V([P(`menu`,`
 background-color: var(--n-color);
 color: var(--n-item-text-color);
 overflow: hidden;
 transition: background-color .3s var(--n-bezier);
 box-sizing: border-box;
 font-size: var(--n-font-size);
 padding-bottom: 6px;
 `,[H(`horizontal`,`
 max-width: 100%;
 width: 100%;
 display: flex;
 overflow: hidden;
 padding-bottom: 0;
 `,[P(`submenu`,`margin: 0;`),P(`menu-item`,`margin: 0;`),P(`menu-item-content`,`
 padding: 0 20px;
 border-bottom: 2px solid #0000;
 `,[V(`&::before`,`display: none;`),H(`selected`,`border-bottom: 2px solid var(--n-border-color-horizontal)`)]),P(`menu-item-content`,[H(`selected`,[I(`icon`,`color: var(--n-item-icon-color-active-horizontal);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-horizontal);
 `,[V(`a`,`color: var(--n-item-text-color-active-horizontal);`),I(`extra`,`color: var(--n-item-text-color-active-horizontal);`)])]),H(`child-active`,`
 border-bottom: 2px solid var(--n-border-color-horizontal);
 `,[P(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `,[V(`a`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `),I(`extra`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `)]),I(`icon`,`
 color: var(--n-item-icon-color-child-active-horizontal);
 `)]),q(`disabled`,[q(`selected, child-active`,[V(`&:focus-within`,Rt)]),H(`selected`,[$(null,[I(`icon`,`color: var(--n-item-icon-color-active-hover-horizontal);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover-horizontal);
 `,[V(`a`,`color: var(--n-item-text-color-active-hover-horizontal);`),I(`extra`,`color: var(--n-item-text-color-active-hover-horizontal);`)])])]),H(`child-active`,[$(null,[I(`icon`,`color: var(--n-item-icon-color-child-active-hover-horizontal);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover-horizontal);
 `,[V(`a`,`color: var(--n-item-text-color-child-active-hover-horizontal);`),I(`extra`,`color: var(--n-item-text-color-child-active-hover-horizontal);`)])])]),$(`border-bottom: 2px solid var(--n-border-color-horizontal);`,Rt)]),P(`menu-item-content-header`,[V(`a`,`color: var(--n-item-text-color-horizontal);`)])])]),q(`responsive`,[P(`menu-item-content-header`,`
 overflow: hidden;
 text-overflow: ellipsis;
 `)]),H(`collapsed`,[P(`menu-item-content`,[H(`selected`,[V(`&::before`,`
 background-color: var(--n-item-color-active-collapsed) !important;
 `)]),P(`menu-item-content-header`,`opacity: 0;`),I(`arrow`,`opacity: 0;`),I(`icon`,`color: var(--n-item-icon-color-collapsed);`)])]),P(`menu-item`,`
 height: var(--n-item-height);
 margin-top: 6px;
 position: relative;
 `),P(`menu-item-content`,`
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
 `,[V(`> *`,`z-index: 1;`),V(`&::before`,`
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
 `),H(`disabled`,`
 opacity: .45;
 cursor: not-allowed;
 `),H(`collapsed`,[I(`arrow`,`transform: rotate(0);`)]),H(`selected`,[V(`&::before`,`background-color: var(--n-item-color-active);`),I(`arrow`,`color: var(--n-arrow-color-active);`),I(`icon`,`color: var(--n-item-icon-color-active);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-active);
 `,[V(`a`,`color: var(--n-item-text-color-active);`),I(`extra`,`color: var(--n-item-text-color-active);`)])]),H(`child-active`,[P(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active);
 `,[V(`a`,`
 color: var(--n-item-text-color-child-active);
 `),I(`extra`,`
 color: var(--n-item-text-color-child-active);
 `)]),I(`arrow`,`
 color: var(--n-arrow-color-child-active);
 `),I(`icon`,`
 color: var(--n-item-icon-color-child-active);
 `)]),q(`disabled`,[q(`selected, child-active`,[V(`&:focus-within`,Lt)]),H(`selected`,[$(null,[I(`arrow`,`color: var(--n-arrow-color-active-hover);`),I(`icon`,`color: var(--n-item-icon-color-active-hover);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover);
 `,[V(`a`,`color: var(--n-item-text-color-active-hover);`),I(`extra`,`color: var(--n-item-text-color-active-hover);`)])])]),H(`child-active`,[$(null,[I(`arrow`,`color: var(--n-arrow-color-child-active-hover);`),I(`icon`,`color: var(--n-item-icon-color-child-active-hover);`),P(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover);
 `,[V(`a`,`color: var(--n-item-text-color-child-active-hover);`),I(`extra`,`color: var(--n-item-text-color-child-active-hover);`)])])]),H(`selected`,[$(null,[V(`&::before`,`background-color: var(--n-item-color-active-hover);`)])]),$(null,Lt)]),I(`icon`,`
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
 `),I(`arrow`,`
 grid-area: arrow;
 font-size: 16px;
 color: var(--n-arrow-color);
 transform: rotate(180deg);
 opacity: 1;
 transition:
 color .3s var(--n-bezier),
 transform 0.2s var(--n-bezier),
 opacity 0.2s var(--n-bezier);
 `),P(`menu-item-content-header`,`
 grid-area: content;
 transition:
 color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 opacity: 1;
 white-space: nowrap;
 color: var(--n-item-text-color);
 `,[V(`a`,`
 outline: none;
 text-decoration: none;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `,[V(`&::before`,`
 content: "";
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),I(`extra`,`
 font-size: .93em;
 color: var(--n-group-text-color);
 transition: color .3s var(--n-bezier);
 `)])]),P(`submenu`,`
 cursor: pointer;
 position: relative;
 margin-top: 6px;
 `,[P(`menu-item-content`,`
 height: var(--n-item-height);
 `),P(`submenu-children`,`
 overflow: hidden;
 padding: 0;
 `,[Ie({duration:`.2s`})])]),P(`menu-item-group`,[P(`menu-item-group-title`,`
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
 `)])]),P(`menu-tooltip`,[V(`a`,`
 color: inherit;
 text-decoration: none;
 `)]),P(`menu-divider`,`
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-divider-color);
 height: 1px;
 margin: 6px 18px;
 `)]);function $(e,t){return[H(`hover`,e,t),V(`&:hover`,e,t)]}var Bt=r({name:`MenuOptionContent`,props:{collapsed:Boolean,disabled:Boolean,title:[String,Function],icon:Function,extra:[String,Function],showArrow:Boolean,childActive:Boolean,hover:Boolean,paddingLeft:Number,selected:Boolean,maxIconSize:{type:Number,required:!0},activeIconSize:{type:Number,required:!0},iconMarginRight:{type:Number,required:!0},clsPrefix:{type:String,required:!0},onClick:Function,tmNode:{type:Object,required:!0},isEllipsisPlaceholder:Boolean},setup(e){let{props:t}=x(Q);return{menuProps:t,style:b(()=>{let{paddingLeft:t}=e;return{paddingLeft:t&&`${t}px`}}),iconStyle:b(()=>{let{maxIconSize:t,activeIconSize:n,iconMarginRight:r}=e;return{width:`${t}px`,height:`${t}px`,fontSize:`${n}px`,marginRight:`${r}px`}})}},render(){let{clsPrefix:e,tmNode:t,menuProps:{renderIcon:n,renderLabel:r,renderExtra:i,expandIcon:a}}=this,o=n?n(t.rawNode):Z(this.icon);return S(`div`,{onClick:e=>{var t;(t=this.onClick)==null||t.call(this,e)},role:`none`,class:[`${e}-menu-item-content`,{[`${e}-menu-item-content--selected`]:this.selected,[`${e}-menu-item-content--collapsed`]:this.collapsed,[`${e}-menu-item-content--child-active`]:this.childActive,[`${e}-menu-item-content--disabled`]:this.disabled,[`${e}-menu-item-content--hover`]:this.hover}],style:this.style},o&&S(`div`,{class:`${e}-menu-item-content__icon`,style:this.iconStyle,role:`none`},[o]),S(`div`,{class:`${e}-menu-item-content-header`,role:`none`},this.isEllipsisPlaceholder?this.title:r?r(t.rawNode):Z(this.title),this.extra||i?S(`span`,{class:`${e}-menu-item-content-header__extra`},` `,i?i(t.rawNode):Z(this.extra)):null),this.showArrow?S(z,{ariaHidden:!0,class:`${e}-menu-item-content__arrow`,clsPrefix:e},{default:()=>a?a(t.rawNode):S(Ye,null)}):null)}}),Vt=8;function Ht(e){let t=x(Q),{props:n,mergedCollapsedRef:r}=t,i=x(Ft,null),a=x(It,null),o=b(()=>n.mode===`horizontal`),s=b(()=>o.value?n.dropdownPlacement:`tmNodes`in e?`right-start`:`right`),c=b(()=>Math.max(n.collapsedIconSize??n.iconSize,n.iconSize));return{dropdownPlacement:s,activeIconSize:b(()=>!o.value&&e.root&&r.value?n.collapsedIconSize??n.iconSize:n.iconSize),maxIconSize:c,paddingLeft:b(()=>{if(o.value)return;let{collapsedWidth:t,indent:s,rootIndent:l}=n,{root:u,isGroup:d}=e,f=l===void 0?s:l;return u?r.value?t/2-c.value/2:f:a&&typeof a.paddingLeftRef.value==`number`?s/2+a.paddingLeftRef.value:i&&typeof i.paddingLeftRef.value==`number`?(d?s/2:s)+i.paddingLeftRef.value:0}),iconMarginRight:b(()=>{let{collapsedWidth:t,indent:i,rootIndent:a}=n,{value:s}=c,{root:l}=e;return o.value||!l||!r.value?Vt:(a===void 0?i:a)+s+Vt-(t+s)/2}),NMenu:t,NSubmenu:i,NMenuOptionGroup:a}}var Ut={internalKey:{type:[String,Number],required:!0},root:Boolean,isGroup:Boolean,level:{type:Number,required:!0},title:[String,Function],extra:[String,Function]},Wt=r({name:`MenuDivider`,setup(){let{mergedClsPrefixRef:e,isHorizontalRef:t}=x(Q);return()=>t.value?null:S(`div`,{class:`${e.value}-menu-divider`})}}),Gt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function}),Kt=le(Gt),qt=r({name:`MenuOption`,props:Gt,setup(e){let t=Ht(e),{NSubmenu:n,NMenu:r,NMenuOptionGroup:i}=t,{props:a,mergedClsPrefixRef:o,mergedCollapsedRef:s}=r,c=n?n.mergedDisabledRef:i?i.mergedDisabledRef:{value:!1},l=b(()=>c.value||e.disabled);function u(t){let{onClick:n}=e;n&&n(t)}function d(t){l.value||(r.doSelect(e.internalKey,e.tmNode.rawNode),u(t))}return{mergedClsPrefix:o,dropdownPlacement:t.dropdownPlacement,paddingLeft:t.paddingLeft,iconMarginRight:t.iconMarginRight,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,mergedTheme:r.mergedThemeRef,menuProps:a,dropdownEnabled:de(()=>e.root&&s.value&&a.mode!==`horizontal`&&!l.value),selected:de(()=>r.mergedValueRef.value===e.internalKey),mergedDisabled:l,handleClick:d}},render(){let{mergedClsPrefix:e,mergedTheme:t,tmNode:n,menuProps:{renderLabel:r,nodeProps:i}}=this,a=i?.(n.rawNode);return S(`div`,Object.assign({},a,{role:`menuitem`,class:[`${e}-menu-item`,a?.class]}),S(Ce,{theme:t.peers.Tooltip,themeOverrides:t.peerOverrides.Tooltip,trigger:`hover`,placement:this.dropdownPlacement,disabled:!this.dropdownEnabled||this.title===void 0,internalExtraClass:[`menu-tooltip`]},{default:()=>r?r(n.rawNode):Z(this.title),trigger:()=>S(Bt,{tmNode:n,clsPrefix:e,paddingLeft:this.paddingLeft,iconMarginRight:this.iconMarginRight,maxIconSize:this.maxIconSize,activeIconSize:this.activeIconSize,selected:this.selected,title:this.title,extra:this.extra,disabled:this.mergedDisabled,icon:this.icon,onClick:this.handleClick})}))}}),Jt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},tmNodes:{type:Array,required:!0}}),Yt=le(Jt),Xt=r({name:`MenuOptionGroup`,props:Jt,setup(e){let t=Ht(e),{NSubmenu:n}=t,r=b(()=>n?.mergedDisabledRef.value?!0:e.tmNode.disabled);d(It,{paddingLeftRef:t.paddingLeft,mergedDisabledRef:r});let{mergedClsPrefixRef:i,props:a}=x(Q);return function(){let{value:n}=i,r=t.paddingLeft.value,{nodeProps:o}=a,s=o?.(e.tmNode.rawNode);return S(`div`,{class:`${n}-menu-item-group`,role:`group`},S(`div`,Object.assign({},s,{class:[`${n}-menu-item-group-title`,s?.class],style:[s?.style||``,r===void 0?``:`padding-left: ${r}px;`]}),Z(e.title),e.extra?S(v,null,` `,Z(e.extra)):null),S(`div`,null,e.tmNodes.map(e=>$t(e,a))))}}});function Zt(e){return e.type===`divider`||e.type===`render`}function Qt(e){return e.type===`divider`}function $t(e,t){let{rawNode:n}=e,{show:r}=n;if(r===!1)return null;if(Zt(n))return Qt(n)?S(Wt,Object.assign({key:e.key},n.props)):null;let{labelField:i}=t,{key:a,level:o,isGroup:s}=e,c=Object.assign(Object.assign({},n),{title:n.title||n[i],extra:n.titleExtra||n.extra,key:a,internalKey:a,level:o,root:o===0,isGroup:s});return e.children?e.isGroup?S(Xt,Oe(c,Yt,{tmNode:e,tmNodes:e.children,key:a})):S(nn,Oe(c,tn,{key:a,rawNodes:n[t.childrenField],tmNodes:e.children,tmNode:e})):S(qt,Oe(c,Kt,{key:a,tmNode:e}))}var en=Object.assign(Object.assign({},Ut),{rawNodes:{type:Array,default:()=>[]},tmNodes:{type:Array,default:()=>[]},tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function,domId:String,virtualChildActive:{type:Boolean,default:void 0},isEllipsisPlaceholder:Boolean}),tn=le(en),nn=r({name:`Submenu`,props:en,setup(e){let t=Ht(e),{NMenu:n,NSubmenu:r}=t,{props:i,mergedCollapsedRef:a,mergedThemeRef:o}=n,s=b(()=>{let{disabled:t}=e;return r?.mergedDisabledRef.value||i.disabled?!0:t}),c=_(!1);d(Ft,{paddingLeftRef:t.paddingLeft,mergedDisabledRef:s}),d(It,null);function l(){let{onClick:t}=e;t&&t()}function u(){s.value||(a.value||n.toggleExpand(e.internalKey),l())}function f(e){c.value=e}return{menuProps:i,mergedTheme:o,doSelect:n.doSelect,inverted:n.invertedRef,isHorizontal:n.isHorizontalRef,mergedClsPrefix:n.mergedClsPrefixRef,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,iconMarginRight:t.iconMarginRight,dropdownPlacement:t.dropdownPlacement,dropdownShow:c,paddingLeft:t.paddingLeft,mergedDisabled:s,mergedValue:n.mergedValueRef,childActive:de(()=>e.virtualChildActive??n.activePathRef.value.includes(e.internalKey)),collapsed:b(()=>i.mode===`horizontal`?!1:a.value?!0:!n.mergedExpandedKeysRef.value.includes(e.internalKey)),dropdownEnabled:b(()=>!s.value&&(i.mode===`horizontal`||a.value)),handlePopoverShowChange:f,handleClick:u}},render(){let{mergedClsPrefix:e,menuProps:{renderIcon:t,renderLabel:n}}=this,r=()=>{let{isHorizontal:e,paddingLeft:t,collapsed:n,mergedDisabled:r,maxIconSize:i,activeIconSize:a,title:o,childActive:s,icon:c,handleClick:l,menuProps:{nodeProps:u},dropdownShow:d,iconMarginRight:f,tmNode:p,mergedClsPrefix:m,isEllipsisPlaceholder:h,extra:g}=this,_=u?.(p.rawNode);return S(`div`,Object.assign({},_,{class:[`${m}-menu-item`,_?.class],role:`menuitem`}),S(Bt,{tmNode:p,paddingLeft:t,collapsed:n,disabled:r,iconMarginRight:f,maxIconSize:i,activeIconSize:a,title:o,extra:g,showArrow:!e,childActive:s,clsPrefix:m,icon:c,hover:d,onClick:l,isEllipsisPlaceholder:h}))},i=()=>S(j,null,{default:()=>{let{tmNodes:t,collapsed:n}=this;return n?null:S(`div`,{class:`${e}-submenu-children`,role:`menu`},t.map(e=>$t(e,this.menuProps)))}});return this.root?S(Te,Object.assign({size:`large`,trigger:`hover`},this.menuProps?.dropdownProps,{themeOverrides:this.mergedTheme.peerOverrides.Dropdown,theme:this.mergedTheme.peers.Dropdown,builtinThemeOverrides:{fontSizeLarge:`14px`,optionIconSizeLarge:`18px`},value:this.mergedValue,disabled:!this.dropdownEnabled,placement:this.dropdownPlacement,keyField:this.menuProps.keyField,labelField:this.menuProps.labelField,childrenField:this.menuProps.childrenField,onUpdateShow:this.handlePopoverShowChange,options:this.rawNodes,onSelect:this.doSelect,inverted:this.inverted,renderIcon:t,renderLabel:n}),{default:()=>S(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},r(),this.isHorizontal?null:i())}):S(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},r(),i())}}),rn=r({name:`Menu`,inheritAttrs:!1,props:Object.assign(Object.assign({},k.props),{options:{type:Array,default:()=>[]},collapsed:{type:Boolean,default:void 0},collapsedWidth:{type:Number,default:48},iconSize:{type:Number,default:20},collapsedIconSize:{type:Number,default:24},rootIndent:Number,indent:{type:Number,default:32},labelField:{type:String,default:`label`},keyField:{type:String,default:`key`},childrenField:{type:String,default:`children`},disabledField:{type:String,default:`disabled`},defaultExpandAll:Boolean,defaultExpandedKeys:Array,expandedKeys:Array,value:[String,Number],defaultValue:{type:[String,Number],default:null},mode:{type:String,default:`vertical`},watchProps:{type:Array,default:void 0},disabled:Boolean,show:{type:Boolean,default:!0},inverted:Boolean,"onUpdate:expandedKeys":[Function,Array],onUpdateExpandedKeys:[Function,Array],onUpdateValue:[Function,Array],"onUpdate:value":[Function,Array],expandIcon:Function,renderIcon:Function,renderLabel:Function,renderExtra:Function,dropdownProps:Object,accordion:Boolean,nodeProps:Function,dropdownPlacement:{type:String,default:`bottom`},responsive:Boolean,items:Array,onOpenNamesChange:[Function,Array],onSelect:[Function,Array],onExpandedNamesChange:[Function,Array],expandedNames:Array,defaultExpandedNames:Array}),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=K(e),r=k(`Menu`,`-menu`,zt,ke,e,t),i=x(yt,null),a=b(()=>{let{collapsed:t}=e;if(t!==void 0)return t;if(i){let{collapseModeRef:e,collapsedRef:t}=i;if(e.value===`width`)return t.value??!1}return!1}),o=b(()=>{let{keyField:t,childrenField:n,disabledField:r}=e;return Se(e.items||e.options,{getIgnored(e){return Zt(e)},getChildren(e){return e[n]},getDisabled(e){return e[r]},getKey(e){return e[t]??e.name}})}),s=b(()=>new Set(o.value.treeNodes.map(e=>e.key))),{watchProps:c}=e,l=_(null);c?.includes(`defaultValue`)?m(()=>{l.value=e.defaultValue}):l.value=e.defaultValue;let u=Y(y(e,`value`),l),f=_([]),p=()=>{f.value=e.defaultExpandAll?o.value.getNonLeafKeys():e.defaultExpandedNames||e.defaultExpandedKeys||o.value.getPath(u.value,{includeSelf:!1}).keyPath};c?.includes(`defaultExpandedKeys`)?m(p):p();let h=Ee(e,[`expandedNames`,`expandedKeys`]),g=Y(h,f),v=b(()=>o.value.treeNodes),C=b(()=>o.value.getPath(u.value).keyPath);d(Q,{props:e,mergedCollapsedRef:a,mergedThemeRef:r,mergedValueRef:u,mergedExpandedKeysRef:g,activePathRef:C,mergedClsPrefixRef:t,isHorizontalRef:b(()=>e.mode===`horizontal`),invertedRef:y(e,`inverted`),doSelect:w,toggleExpand:E});function w(t,n){let{"onUpdate:value":r,onUpdateValue:i,onSelect:a}=e;i&&U(i,t,n),r&&U(r,t,n),a&&U(a,t,n),l.value=t}function T(t){let{"onUpdate:expandedKeys":n,onUpdateExpandedKeys:r,onExpandedNamesChange:i,onOpenNamesChange:a}=e;n&&U(n,t),r&&U(r,t),i&&U(i,t),a&&U(a,t),f.value=t}function E(t){let n=Array.from(g.value),r=n.findIndex(e=>e===t);if(~r)n.splice(r,1);else{if(e.accordion&&s.value.has(t)){let e=n.findIndex(e=>s.value.has(e));e>-1&&n.splice(e,1)}n.push(t)}T(n)}let D=t=>{let n=o.value.getPath(t??u.value,{includeSelf:!1}).keyPath;if(!n.length)return;let r=Array.from(g.value),i=new Set([...r,...n]);e.accordion&&s.value.forEach(e=>{i.has(e)&&!n.includes(e)&&i.delete(e)}),T(Array.from(i))},O=b(()=>{let{inverted:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value,{borderRadius:a,borderColorHorizontal:o,fontSize:s,itemHeight:c,dividerColor:l}=i,u={"--n-divider-color":l,"--n-bezier":n,"--n-font-size":s,"--n-border-color-horizontal":o,"--n-border-radius":a,"--n-item-height":c};return t?(u[`--n-group-text-color`]=i.groupTextColorInverted,u[`--n-color`]=i.colorInverted,u[`--n-item-text-color`]=i.itemTextColorInverted,u[`--n-item-text-color-hover`]=i.itemTextColorHoverInverted,u[`--n-item-text-color-active`]=i.itemTextColorActiveInverted,u[`--n-item-text-color-child-active`]=i.itemTextColorChildActiveInverted,u[`--n-item-text-color-child-active-hover`]=i.itemTextColorChildActiveInverted,u[`--n-item-text-color-active-hover`]=i.itemTextColorActiveHoverInverted,u[`--n-item-icon-color`]=i.itemIconColorInverted,u[`--n-item-icon-color-hover`]=i.itemIconColorHoverInverted,u[`--n-item-icon-color-active`]=i.itemIconColorActiveInverted,u[`--n-item-icon-color-active-hover`]=i.itemIconColorActiveHoverInverted,u[`--n-item-icon-color-child-active`]=i.itemIconColorChildActiveInverted,u[`--n-item-icon-color-child-active-hover`]=i.itemIconColorChildActiveHoverInverted,u[`--n-item-icon-color-collapsed`]=i.itemIconColorCollapsedInverted,u[`--n-item-text-color-horizontal`]=i.itemTextColorHorizontalInverted,u[`--n-item-text-color-hover-horizontal`]=i.itemTextColorHoverHorizontalInverted,u[`--n-item-text-color-active-horizontal`]=i.itemTextColorActiveHorizontalInverted,u[`--n-item-text-color-child-active-horizontal`]=i.itemTextColorChildActiveHorizontalInverted,u[`--n-item-text-color-child-active-hover-horizontal`]=i.itemTextColorChildActiveHoverHorizontalInverted,u[`--n-item-text-color-active-hover-horizontal`]=i.itemTextColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-horizontal`]=i.itemIconColorHorizontalInverted,u[`--n-item-icon-color-hover-horizontal`]=i.itemIconColorHoverHorizontalInverted,u[`--n-item-icon-color-active-horizontal`]=i.itemIconColorActiveHorizontalInverted,u[`--n-item-icon-color-active-hover-horizontal`]=i.itemIconColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-child-active-horizontal`]=i.itemIconColorChildActiveHorizontalInverted,u[`--n-item-icon-color-child-active-hover-horizontal`]=i.itemIconColorChildActiveHoverHorizontalInverted,u[`--n-arrow-color`]=i.arrowColorInverted,u[`--n-arrow-color-hover`]=i.arrowColorHoverInverted,u[`--n-arrow-color-active`]=i.arrowColorActiveInverted,u[`--n-arrow-color-active-hover`]=i.arrowColorActiveHoverInverted,u[`--n-arrow-color-child-active`]=i.arrowColorChildActiveInverted,u[`--n-arrow-color-child-active-hover`]=i.arrowColorChildActiveHoverInverted,u[`--n-item-color-hover`]=i.itemColorHoverInverted,u[`--n-item-color-active`]=i.itemColorActiveInverted,u[`--n-item-color-active-hover`]=i.itemColorActiveHoverInverted,u[`--n-item-color-active-collapsed`]=i.itemColorActiveCollapsedInverted):(u[`--n-group-text-color`]=i.groupTextColor,u[`--n-color`]=i.color,u[`--n-item-text-color`]=i.itemTextColor,u[`--n-item-text-color-hover`]=i.itemTextColorHover,u[`--n-item-text-color-active`]=i.itemTextColorActive,u[`--n-item-text-color-child-active`]=i.itemTextColorChildActive,u[`--n-item-text-color-child-active-hover`]=i.itemTextColorChildActiveHover,u[`--n-item-text-color-active-hover`]=i.itemTextColorActiveHover,u[`--n-item-icon-color`]=i.itemIconColor,u[`--n-item-icon-color-hover`]=i.itemIconColorHover,u[`--n-item-icon-color-active`]=i.itemIconColorActive,u[`--n-item-icon-color-active-hover`]=i.itemIconColorActiveHover,u[`--n-item-icon-color-child-active`]=i.itemIconColorChildActive,u[`--n-item-icon-color-child-active-hover`]=i.itemIconColorChildActiveHover,u[`--n-item-icon-color-collapsed`]=i.itemIconColorCollapsed,u[`--n-item-text-color-horizontal`]=i.itemTextColorHorizontal,u[`--n-item-text-color-hover-horizontal`]=i.itemTextColorHoverHorizontal,u[`--n-item-text-color-active-horizontal`]=i.itemTextColorActiveHorizontal,u[`--n-item-text-color-child-active-horizontal`]=i.itemTextColorChildActiveHorizontal,u[`--n-item-text-color-child-active-hover-horizontal`]=i.itemTextColorChildActiveHoverHorizontal,u[`--n-item-text-color-active-hover-horizontal`]=i.itemTextColorActiveHoverHorizontal,u[`--n-item-icon-color-horizontal`]=i.itemIconColorHorizontal,u[`--n-item-icon-color-hover-horizontal`]=i.itemIconColorHoverHorizontal,u[`--n-item-icon-color-active-horizontal`]=i.itemIconColorActiveHorizontal,u[`--n-item-icon-color-active-hover-horizontal`]=i.itemIconColorActiveHoverHorizontal,u[`--n-item-icon-color-child-active-horizontal`]=i.itemIconColorChildActiveHorizontal,u[`--n-item-icon-color-child-active-hover-horizontal`]=i.itemIconColorChildActiveHoverHorizontal,u[`--n-arrow-color`]=i.arrowColor,u[`--n-arrow-color-hover`]=i.arrowColorHover,u[`--n-arrow-color-active`]=i.arrowColorActive,u[`--n-arrow-color-active-hover`]=i.arrowColorActiveHover,u[`--n-arrow-color-child-active`]=i.arrowColorChildActive,u[`--n-arrow-color-child-active-hover`]=i.arrowColorChildActiveHover,u[`--n-item-color-hover`]=i.itemColorHover,u[`--n-item-color-active`]=i.itemColorActive,u[`--n-item-color-active-hover`]=i.itemColorActiveHover,u[`--n-item-color-active-collapsed`]=i.itemColorActiveCollapsed),u}),A=n?G(`menu`,b(()=>e.inverted?`a`:`b`),O,e):void 0,j=fe(),M=_(null),ee=_(null),N=!0,P=()=>{var e;N?N=!1:(e=M.value)==null||e.sync({showAllItemsBeforeCalculate:!0})};function F(){return document.getElementById(j)}let I=_(-1);function L(t){I.value=e.options.length-t}function R(e){e||(I.value=-1)}let z=b(()=>{let t=I.value;return{children:t===-1?[]:e.options.slice(t)}}),B=b(()=>{let{childrenField:t,disabledField:n,keyField:r}=e;return Se([z.value],{getIgnored(e){return Zt(e)},getChildren(e){return e[t]},getDisabled(e){return e[n]},getKey(e){return e[r]??e.name}})}),V=b(()=>Se([{}]).treeNodes[0]);function H(){if(I.value===-1)return S(nn,{root:!0,level:0,key:`__ellpisisGroupPlaceholder__`,internalKey:`__ellpisisGroupPlaceholder__`,title:`···`,tmNode:V.value,domId:j,isEllipsisPlaceholder:!0});let e=B.value.treeNodes[0],t=C.value;return S(nn,{level:0,root:!0,key:`__ellpisisGroup__`,internalKey:`__ellpisisGroup__`,title:`···`,virtualChildActive:!!e.children?.some(e=>t.includes(e.key)),tmNode:e,domId:j,rawNodes:e.rawNode.children||[],tmNodes:e.children||[],isEllipsisPlaceholder:!0})}return{mergedClsPrefix:t,controlledExpandedKeys:h,uncontrolledExpanededKeys:f,mergedExpandedKeys:g,uncontrolledValue:l,mergedValue:u,activePath:C,tmNodes:v,mergedTheme:r,mergedCollapsed:a,cssVars:n?void 0:O,themeClass:A?.themeClass,overflowRef:M,counterRef:ee,updateCounter:()=>{},onResize:P,onUpdateOverflow:R,onUpdateCount:L,renderCounter:H,getCounter:F,onRender:A?.onRender,showOption:D,deriveResponsiveState:P}},render(){let{mergedClsPrefix:e,mode:t,themeClass:n,onRender:r}=this;r?.();let i=()=>this.tmNodes.map(e=>$t(e,this.$props)),a=t===`horizontal`&&this.responsive,o=()=>S(`div`,s(this.$attrs,{role:t===`horizontal`?`menubar`:`menu`,class:[`${e}-menu`,n,`${e}-menu--${t}`,a&&`${e}-menu--responsive`,this.mergedCollapsed&&`${e}-menu--collapsed`],style:this.cssVars}),a?S(pe,{ref:`overflowRef`,onUpdateOverflow:this.onUpdateOverflow,getCounter:this.getCounter,onUpdateCount:this.onUpdateCount,updateCounter:this.updateCounter,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:i,counter:this.renderCounter}):i());return a?S(me,{onResize:this.onResize},{default:o}):o()}}),an={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},on=r({name:`CloudDownloadOutline`,render:function(e,t){return i(),g(`svg`,an,t[0]||=[h(`path`,{d:`M320 336h76c55 0 100-21.21 100-75.6s-53-73.47-96-75.6C391.11 99.74 329 48 256 48c-69 0-113.44 45.79-128 91.2c-60 5.7-112 35.88-112 98.4S70 336 136 336h56`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 400.1l64 63.9l64-63.9`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M256 224v224.03`},null,-1)])}}),sn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},cn=r({name:`DocumentTextOutline`,render:function(e,t){return i(),g(`svg`,sn,t[0]||=[h(`path`,{d:`M416 221.25V416a48 48 0 0 1-48 48H144a48 48 0 0 1-48-48V96a48 48 0 0 1 48-48h98.75a32 32 0 0 1 22.62 9.37l141.26 141.26a32 32 0 0 1 9.37 22.62z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M256 56v120a32 32 0 0 0 32 32h120`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 288h160`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 368h160`},null,-1)])}}),ln={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},un=r({name:`HomeOutline`,render:function(e,t){return i(),g(`svg`,ln,t[0]||=[h(`path`,{d:`M80 212v236a16 16 0 0 0 16 16h96V328a24 24 0 0 1 24-24h80a24 24 0 0 1 24 24v136h96a16 16 0 0 0 16-16V212`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M480 256L266.89 52c-5-5.28-16.69-5.34-21.78 0L32 256`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M400 179V64h-48v69`},null,-1)])}}),dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},fn=r({name:`KeyOutline`,render:function(e,t){return i(),g(`svg`,dn,t[0]||=[h(`path`,{d:`M218.1 167.17c0 13 0 25.6 4.1 37.4c-43.1 50.6-156.9 184.3-167.5 194.5a20.17 20.17 0 0 0-6.7 15c0 8.5 5.2 16.7 9.6 21.3c6.6 6.9 34.8 33 40 28c15.4-15 18.5-19 24.8-25.2c9.5-9.3-1-28.3 2.3-36s6.8-9.2 12.5-10.4s15.8 2.9 23.7 3c8.3.1 12.8-3.4 19-9.2c5-4.6 8.6-8.9 8.7-15.6c.2-9-12.8-20.9-3.1-30.4s23.7 6.2 34 5s22.8-15.5 24.1-21.6s-11.7-21.8-9.7-30.7c.7-3 6.8-10 11.4-11s25 6.9 29.6 5.9c5.6-1.2 12.1-7.1 17.4-10.4c15.5 6.7 29.6 9.4 47.7 9.4c68.5 0 124-53.4 124-119.2S408.5 48 340 48s-121.9 53.37-121.9 119.17zM400 144a32 32 0 1 1-32-32a32 32 0 0 1 32 32z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),pn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},mn=r({name:`LanguageOutline`,render:function(e,t){return i(),g(`svg`,pn,t[0]||=[u(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M48 112h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M192 64v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M272 448l96-224l96 224"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M301.5 384h133"></path><path d="M281.3 112S257 206 199 277S80 384 80 384" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path><path d="M256 336s-35-27-72-75s-56-85-56-85" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path>`,6)])}}),hn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},gn=r({name:`LayersOutline`,render:function(e,t){return i(),g(`svg`,hn,t[0]||=[h(`path`,{d:`M434.8 137.65l-149.36-68.1c-16.19-7.4-42.69-7.4-58.88 0L77.3 137.65c-17.6 8-17.6 21.09 0 29.09l148 67.5c16.89 7.7 44.69 7.7 61.58 0l148-67.5c17.52-8 17.52-21.1-.08-29.09z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M160 308.52l-82.7 37.11c-17.6 8-17.6 21.1 0 29.1l148 67.5c16.89 7.69 44.69 7.69 61.58 0l148-67.5c17.6-8 17.6-21.1 0-29.1l-79.94-38.47`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M160 204.48l-82.8 37.16c-17.6 8-17.6 21.1 0 29.1l148 67.49c16.89 7.7 44.69 7.7 61.58 0l148-67.49c17.7-8 17.7-21.1.1-29.1L352 204.48`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),_n={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},vn=r({name:`ListOutline`,render:function(e,t){return i(),g(`svg`,_n,t[0]||=[u(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 144h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 256h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 368h288"></path><circle cx="80" cy="144" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="256" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="368" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle>`,6)])}}),yn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},bn=r({name:`LogOutOutline`,render:function(e,t){return i(),g(`svg`,yn,t[0]||=[h(`path`,{d:`M304 336v40a40 40 0 0 1-40 40H104a40 40 0 0 1-40-40V136a40 40 0 0 1 40-40h152c22.09 0 48 17.91 48 40v40`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M368 336l80-80l-80-80`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 256h256`},null,-1)])}}),xn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Sn=r({name:`MenuOutline`,render:function(e,t){return i(),g(`svg`,xn,t[0]||=[h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 160h352`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 256h352`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 352h352`},null,-1)])}}),Cn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},wn=r({name:`MoonOutline`,render:function(e,t){return i(),g(`svg`,Cn,t[0]||=[h(`path`,{d:`M160 136c0-30.62 4.51-61.61 16-88C99.57 81.27 48 159.32 48 248c0 119.29 96.71 216 216 216c88.68 0 166.73-51.57 200-128c-26.39 11.49-57.38 16-88 16c-119.29 0-216-96.71-216-216z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),Tn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},En=r({name:`PeopleOutline`,render:function(e,t){return i(),g(`svg`,Tn,t[0]||=[h(`path`,{d:`M402 168c-2.93 40.67-33.1 72-66 72s-63.12-31.32-66-72c-3-42.31 26.37-72 66-72s69 30.46 66 72z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M336 304c-65.17 0-127.84 32.37-143.54 95.41c-2.08 8.34 3.15 16.59 11.72 16.59h263.65c8.57 0 13.77-8.25 11.72-16.59C463.85 335.36 401.18 304 336 304z`,fill:`none`,stroke:`currentColor`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1),h(`path`,{d:`M200 185.94c-2.34 32.48-26.72 58.06-53 58.06s-50.7-25.57-53-58.06C91.61 152.15 115.34 128 147 128s55.39 24.77 53 57.94z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{d:`M206 306c-18.05-8.27-37.93-11.45-59-11.45c-52 0-102.1 25.85-114.65 76.2c-1.65 6.66 2.53 13.25 9.37 13.25H154`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1)])}}),Dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},On=r({name:`SettingsOutline`,render:function(e,t){return i(),g(`svg`,Dn,t[0]||=[h(`path`,{d:`M262.29 192.31a64 64 0 1 0 57.4 57.4a64.13 64.13 0 0 0-57.4-57.4zM416.39 256a154.34 154.34 0 0 1-1.53 20.79l45.21 35.46a10.81 10.81 0 0 1 2.45 13.75l-42.77 74a10.81 10.81 0 0 1-13.14 4.59l-44.9-18.08a16.11 16.11 0 0 0-15.17 1.75A164.48 164.48 0 0 1 325 400.8a15.94 15.94 0 0 0-8.82 12.14l-6.73 47.89a11.08 11.08 0 0 1-10.68 9.17h-85.54a11.11 11.11 0 0 1-10.69-8.87l-6.72-47.82a16.07 16.07 0 0 0-9-12.22a155.3 155.3 0 0 1-21.46-12.57a16 16 0 0 0-15.11-1.71l-44.89 18.07a10.81 10.81 0 0 1-13.14-4.58l-42.77-74a10.8 10.8 0 0 1 2.45-13.75l38.21-30a16.05 16.05 0 0 0 6-14.08c-.36-4.17-.58-8.33-.58-12.5s.21-8.27.58-12.35a16 16 0 0 0-6.07-13.94l-38.19-30A10.81 10.81 0 0 1 49.48 186l42.77-74a10.81 10.81 0 0 1 13.14-4.59l44.9 18.08a16.11 16.11 0 0 0 15.17-1.75A164.48 164.48 0 0 1 187 111.2a15.94 15.94 0 0 0 8.82-12.14l6.73-47.89A11.08 11.08 0 0 1 213.23 42h85.54a11.11 11.11 0 0 1 10.69 8.87l6.72 47.82a16.07 16.07 0 0 0 9 12.22a155.3 155.3 0 0 1 21.46 12.57a16 16 0 0 0 15.11 1.71l44.89-18.07a10.81 10.81 0 0 1 13.14 4.58l42.77 74a10.8 10.8 0 0 1-2.45 13.75l-38.21 30a16.05 16.05 0 0 0-6.05 14.08c.33 4.14.55 8.3.55 12.47z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),kn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},An=r({name:`ShieldCheckmarkOutline`,render:function(e,t){return i(),g(`svg`,kn,t[0]||=[h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M336 176L225.2 304L176 255.8`},null,-1),h(`path`,{d:`M463.1 112.37C373.68 96.33 336.71 84.45 256 48c-80.71 36.45-117.68 48.33-207.1 64.37C32.7 369.13 240.58 457.79 256 464c15.42-6.21 223.3-94.87 207.1-351.63z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),jn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Mn=r({name:`SunnyOutline`,render:function(e,t){return i(),g(`svg`,jn,t[0]||=[u(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 48v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 416v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 108.92l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 369.14l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M464 256h-48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M96 256H48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 403.08l-33.94-33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 142.86l-33.94-33.94"></path><circle cx="256" cy="256" r="80" fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32"></circle>`,9)])}}),Nn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Pn=r({name:`SwapHorizontalOutline`,render:function(e,t){return i(),g(`svg`,Nn,t[0]||=[h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M304 48l112 112l-112 112`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M398.87 160H96`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M208 464L96 352l112-112`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M114 352h302`},null,-1)])}}),Fn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},In=r({name:`TerminalOutline`,render:function(e,t){return i(),g(`svg`,Fn,t[0]||=[h(`rect`,{x:`32`,y:`48`,width:`448`,height:`416`,rx:`48`,ry:`48`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M96 112l80 64l-80 64`},null,-1),h(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 240h64`},null,-1)])}}),Ln={key:0,class:`logo-text`},Rn={class:`header-left`},zn={class:`header-actions`},Bn={key:0},Vn={class:`content-container`},Hn=Je(r({__name:`AppLayout`,setup(e){let t=ge(),r=ye(),{t:s}=ve(),u=ae(),d=Be(),p=b({get:()=>d.sidebarCollapsed,set:e=>{d.sidebarCollapsed=e}}),m=_(!1),y=_(!1),x=b(()=>d.darkMode);function k(){m.value=window.innerWidth<900,m.value||(y.value=!1)}O(()=>{k(),window.addEventListener(`resize`,k)}),o(()=>window.removeEventListener(`resize`,k));function A(){d.toggleDarkMode()}let j=b(()=>u.user?.display_name||u.user?.username||``),M=b(()=>{let e=r.path,t=F.flatMap(e=>[e,...e.children??[]]).map(e=>String(e.key)).filter(e=>e.startsWith(`/`)).sort((e,t)=>t.length-e.length);for(let n of t)if(e.startsWith(n))return n;return`/`}),ee=b(()=>{let e=[{label:`GoDDI`,path:`/`}],t=r.path;return t.startsWith(`/dns`)?(e.push({label:s(`nav.dns`),path:`/dns`}),t.includes(`/zones`)&&e.push({label:s(`nav.dnsZones`),path:`/dns/zones`}),t.includes(`/forwarders`)&&e.push({label:s(`nav.dnsForwarders`),path:`/dns/forwarders`}),t.includes(`/security`)&&e.push({label:s(`nav.dnsSecurity`),path:`/dns/security`}),t.includes(`/cache`)&&e.push({label:s(`nav.dnsCache`),path:`/dns/cache`}),t.includes(`/client`)&&e.push({label:s(`nav.dnsClient`),path:`/dns/client`})):t.startsWith(`/dhcp`)?(e.push({label:s(`nav.dhcp`),path:`/dhcp`}),t.includes(`/scopes`)&&e.push({label:s(`nav.dhcpScopes`),path:`/dhcp/scopes`}),t.includes(`/leases`)&&e.push({label:s(`nav.dhcpLeases`),path:`/dhcp/leases`}),t.includes(`/reservations`)&&e.push({label:s(`nav.dhcpReservations`),path:`/dhcp/reservations`}),t.includes(`/options`)&&e.push({label:s(`nav.dhcpOptions`),path:`/dhcp/options`})):t.startsWith(`/ipam`)?(e.push({label:s(`nav.ipam`),path:`/ipam`}),t.includes(`/spaces`)&&e.push({label:s(`nav.ipamSpaces`),path:`/ipam/spaces`}),t.includes(`/subnets`)&&e.push({label:s(`nav.ipamSubnets`),path:`/ipam/subnets`}),t.includes(`/addresses`)&&e.push({label:s(`nav.ipamAddresses`),path:`/ipam/addresses`})):t.startsWith(`/admin`)?(e.push({label:s(`nav.admin`),path:`/admin`}),t.includes(`/users`)&&e.push({label:s(`nav.adminUsers`),path:`/admin/users`}),t.includes(`/roles`)&&e.push({label:s(`nav.adminRoles`),path:`/admin/roles`}),t.includes(`/groups`)&&e.push({label:s(`nav.adminGroups`),path:`/admin/groups`}),t.includes(`/tokens`)&&e.push({label:s(`nav.adminTokens`),path:`/admin/tokens`})):t.startsWith(`/logs`)?(e.push({label:s(`nav.logs`),path:`/logs`}),t.includes(`/audit`)&&e.push({label:s(`nav.logsAudit`),path:`/logs/audit`}),t.includes(`/dns`)&&e.push({label:s(`nav.logsDns`),path:`/logs/dns`}),t.includes(`/dhcp`)&&e.push({label:s(`nav.logsDhcp`),path:`/logs/dhcp`})):t.startsWith(`/settings`)&&(e.push({label:s(`nav.settings`),path:`/settings`}),t.includes(`/backup`)&&e.push({label:s(`nav.settingsBackup`),path:`/settings/backup`})),e});function N(e){return()=>S(X,null,{default:()=>S(e)})}function P(e){let t=e.label;return typeof t==`function`?t():t||``}let F=[{key:`/`,label:()=>s(`nav.dashboard`),icon:N(un)},{key:`/dns`,label:()=>s(`nav.dns`),icon:N(We),children:[{key:`/dns/zones`,label:()=>s(`nav.dnsZones`),icon:N(gn)},{key:`/dns/forwarders`,label:()=>s(`nav.dnsForwarders`),icon:N(Pn)},{key:`/dns/security`,label:()=>s(`nav.dnsSecurity`),icon:N(An)},{key:`/dns/cache`,label:()=>s(`nav.dnsCache`),icon:N(Ge)},{key:`/dns/client`,label:()=>s(`nav.dnsClient`),icon:N(In)}]},{key:`/dhcp`,label:()=>s(`nav.dhcp`),icon:N(He),children:[{key:`/dhcp/scopes`,label:()=>s(`nav.dhcpScopes`),icon:N(Ue)},{key:`/dhcp/leases`,label:()=>s(`nav.dhcpLeases`),icon:N(cn)},{key:`/dhcp/reservations`,label:()=>s(`nav.dhcpReservations`),icon:N(vn)},{key:`/dhcp/options`,label:()=>s(`nav.dhcpOptions`),icon:N(On)}]},{key:`/ipam`,label:()=>s(`nav.ipam`),icon:N(Ge),children:[{key:`/ipam/spaces`,label:()=>s(`nav.ipamSpaces`),icon:N(gn)},{key:`/ipam/subnets`,label:()=>s(`nav.ipamSubnets`),icon:N(Ue)},{key:`/ipam/addresses`,label:()=>s(`nav.ipamAddresses`),icon:N(vn)}]},{key:`/admin`,label:()=>s(`nav.admin`),icon:N(En),children:[{key:`/admin/users`,label:()=>s(`nav.adminUsers`),icon:N(Ke)},{key:`/admin/roles`,label:()=>s(`nav.adminRoles`),icon:N(fn)},{key:`/admin/groups`,label:()=>s(`nav.adminGroups`),icon:N(En)},{key:`/admin/tokens`,label:()=>s(`nav.adminTokens`),icon:N(fn)}]},{key:`/logs`,label:()=>s(`nav.logs`),icon:N(qe),children:[{key:`/logs/audit`,label:()=>s(`nav.logsAudit`),icon:N(cn)},{key:`/logs/dns`,label:()=>s(`nav.logsDns`),icon:N(We)},{key:`/logs/dhcp`,label:()=>s(`nav.logsDhcp`),icon:N(He)}]},{key:`settings-group`,label:()=>s(`nav.settings`),icon:N(On),children:[{key:`/settings`,label:()=>s(`settings.title`),icon:N(On)},{key:`/settings/backup`,label:()=>s(`nav.settingsBackup`),icon:N(on)}]}],I={"/dns":{resource:`dns`,action:`read`},"/dhcp":{resource:`dhcp`,action:`read`},"/ipam":{resource:`ipam`,action:`read`},"/admin/users":{resource:`user`,action:`read`},"/admin/roles":{resource:`role`,action:`read`},"/admin/groups":{resource:`group`,action:`read`},"/admin/tokens":{resource:`token`,action:`read`},"/logs/audit":{resource:`audit`,action:`read`},"/logs/dns":{resource:`dns`,action:`read`},"/logs/dhcp":{resource:`dhcp`,action:`read`},"/settings":{resource:`settings`,action:`read`},"/settings/backup":{resource:`backup`,action:`read`}};function L(e){let t=I[e];return!t||u.hasPermission(t.resource,t.action)}let R=b(()=>F.flatMap(e=>{let t=String(e.key);if(!L(t))return[];if(e.children){let t=e.children.filter(e=>L(String(e.key)));return t.length===0?[]:[{...e,children:t}]}return L(t)?[e]:[]}));function z(e){y.value=!1,t.push(e)}let B=b(()=>[{label:`English`,key:`en-US`,disabled:d.locale===`en-US`},{label:`简体中文`,key:`zh-CN`,disabled:d.locale===`zh-CN`}]);function V(e){d.setLocale(e)}let H=[{label:()=>s(`auth.logout`),key:`logout`,icon:N(bn)}];async function U(e){e===`logout`&&(await u.logout(),t.push(`/login`))}return(e,r)=>{let o=rn,u=Pt,d=gt,_=ht,b=oe,S=et,O=Qe,k=Te,N=De,F=kt,I=a(`router-view`),L=Et,W=Tt;return i(),T(W,{"has-sider":``,class:`app-shell`},{default:l(()=>[m.value?D(``,!0):(i(),T(u,{key:0,bordered:``,"collapse-mode":`width`,"collapsed-width":64,width:240,collapsed:p.value,"show-trigger":``,onCollapse:r[0]||=e=>p.value=!0,onExpand:r[1]||=e=>p.value=!1,"native-scrollbar":!1,class:`desktop-sider`},{default:l(()=>[h(`div`,{class:c([`logo`,{"logo-collapsed":p.value}])},[r[4]||=h(`span`,{class:`logo-icon`},`G`,-1),p.value?D(``,!0):(i(),g(`span`,Ln,`GoDDI`))],2),E(o,{collapsed:p.value,"collapsed-width":64,"collapsed-icon-size":22,options:R.value,value:M.value,"onUpdate:value":z,"render-label":P},null,8,[`collapsed`,`options`,`value`,`render-label`])]),_:1},8,[`collapsed`])),E(_,{show:y.value,"onUpdate:show":r[2]||=e=>y.value=e,placement:`left`,width:280},{default:l(()=>[E(d,{"body-content-style":`padding: 0;`,closable:``},{default:l(()=>[r[5]||=h(`div`,{class:`logo mobile-logo`},[h(`span`,{class:`logo-icon`},`G`),h(`span`,{class:`logo-text`},`GoDDI`)],-1),E(o,{options:R.value,value:M.value,"onUpdate:value":z,"render-label":P},null,8,[`options`,`value`,`render-label`])]),_:1})]),_:1},8,[`show`]),E(W,{class:`main-layout`},{default:l(()=>[E(F,{bordered:``,class:`app-header`},{default:l(()=>[h(`div`,Rn,[m.value?(i(),T(b,{key:0,quaternary:``,circle:``,"aria-label":`Open navigation`,onClick:r[3]||=e=>y.value=!0},{icon:l(()=>[E(w(X),null,{default:l(()=>[E(w(Sn))]),_:1})]),_:1})):D(``,!0),E(O,null,{default:l(()=>[(i(!0),g(v,null,f(ee.value,e=>(i(),T(S,{key:e.path,class:`breadcrumb-item`,onClick:n=>w(t).resolve(e.path).matched.some(t=>t.path===e.path)&&w(t).push(e.path)},{default:l(()=>[n(C(e.label),1)]),_:2},1032,[`onClick`]))),128))]),_:1})]),h(`div`,zn,[E(k,{options:B.value,onSelect:V},{default:l(()=>[E(b,{quaternary:``,circle:``,"aria-label":w(s)(`common.language`)},{icon:l(()=>[E(w(X),null,{default:l(()=>[E(w(mn))]),_:1})]),_:1},8,[`aria-label`])]),_:1},8,[`options`]),E(N,{size:`small`,value:x.value,"onUpdate:value":A},{checked:l(()=>[E(w(X),null,{default:l(()=>[E(w(Mn))]),_:1})]),unchecked:l(()=>[E(w(X),null,{default:l(()=>[E(w(wn))]),_:1})]),_:1},8,[`value`]),E(k,{options:H,onSelect:U},{default:l(()=>[E(b,{quaternary:``},{icon:l(()=>[E(w(X),null,{default:l(()=>[E(w(Ke))]),_:1})]),default:l(()=>[m.value?D(``,!0):(i(),g(`span`,Bn,C(j.value),1))]),_:1})]),_:1})])]),_:1}),E(L,{"content-style":m.value?`padding: 16px;`:`padding: 24px 28px;`,"native-scrollbar":!1,class:c([`app-content`,{"app-content-dark":x.value}])},{default:l(()=>[h(`div`,Vn,[E(I)])]),_:1},8,[`content-style`,`class`])]),_:1})]),_:1})}}}),[[`__scopeId`,`data-v-cc88d947`]]);export{Hn as default};