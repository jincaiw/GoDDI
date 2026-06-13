import{A as e,C as t,Dt as n,G as r,H as i,I as a,M as o,O as s,Ot as c,Q as l,R as u,S as d,T as f,U as p,V as m,X as h,Y as g,Z as _,_ as v,b as y,d as b,dt as x,g as S,pt as C,st as w,v as T,w as E,y as D,z as O}from"./echarts-DxBJA66o.js";import{C as k,D as A,Dt as j,Et as M,Gt as N,Ht as P,It as F,Lt as I,M as L,Nt as R,O as z,Pt as B,Rt as V,S as ee,St as te,Tt as ne,_t as re,bt as ie,dt as H,g as ae,j as U,k as W,kt as oe,l as se,m as G,t as ce,ut as K,v as le,xt as q,y as ue}from"./auth-DEMSBBAb.js";import{C as de,E as fe,c as pe,d as me,g as he,i as ge,m as _e,o as ve,r as ye,s as be,y as xe}from"./vue-core-RsSxNVS3.js";import{a as Se,n as Ce,s as we,t as Te}from"./Dropdown-uFX6gVBD.js";import{o as J,s as Y}from"./get-D5hjkymV.js";import{t as Ee}from"./use-compitable-mYvAsDS6.js";import{t as X}from"./Icon-BGId8hAq.js";import{t as De}from"./Switch-CFCaDdRr.js";import{at as Oe,c as ke,ct as Ae,d as je,dt as Me,ft as Ne,gt as Pe,ht as Fe,j as Ie,k as Le,mt as Re,pt as ze,rt as Z,t as Be,ut as Ve}from"./index-lJkVLtdC.js";import{i as He,n as Ue,r as We,t as Ge}from"./ServerOutline-Dk24Q-wH.js";import{t as Ke}from"./PersonOutline-DR9foYhP.js";import{t as qe}from"./SearchOutline-Bh_PuaGk.js";import{t as Je}from"./_plugin-vue_export-helper-BDNMzG2s.js";var Ye=f({name:`ChevronDownFilled`,render(){return s(`svg`,{viewBox:`0 0 16 16`,fill:`none`,xmlns:`http://www.w3.org/2000/svg`},s(`path`,{d:`M3.20041 5.73966C3.48226 5.43613 3.95681 5.41856 4.26034 5.70041L8 9.22652L11.7397 5.70041C12.0432 5.41856 12.5177 5.43613 12.7996 5.73966C13.0815 6.0432 13.0639 6.51775 12.7603 6.7996L8.51034 10.7996C8.22258 11.0668 7.77743 11.0668 7.48967 10.7996L3.23966 6.7996C2.93613 6.51775 2.91856 6.0432 3.20041 5.73966Z`,fill:`currentColor`}))}}),Xe=B(`breadcrumb`,`
 white-space: nowrap;
 cursor: default;
 line-height: var(--n-item-line-height);
`,[R(`ul`,`
 list-style: none;
 padding: 0;
 margin: 0;
 `),R(`a`,`
 color: inherit;
 text-decoration: inherit;
 `),B(`breadcrumb-item`,`
 font-size: var(--n-font-size);
 transition: color .3s var(--n-bezier);
 display: inline-flex;
 align-items: center;
 `,[B(`icon`,`
 font-size: 18px;
 vertical-align: -.2em;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `),R(`&:not(:last-child)`,[I(`clickable`,[F(`link`,`
 cursor: pointer;
 `,[R(`&:hover`,`
 background-color: var(--n-item-color-hover);
 `),R(`&:active`,`
 background-color: var(--n-item-color-pressed); 
 `)])])]),F(`link`,`
 padding: 4px;
 border-radius: var(--n-item-border-radius);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 position: relative;
 `,[R(`&:hover`,`
 color: var(--n-item-text-color-hover);
 `,[B(`icon`,`
 color: var(--n-item-text-color-hover);
 `)]),R(`&:active`,`
 color: var(--n-item-text-color-pressed);
 `,[B(`icon`,`
 color: var(--n-item-text-color-pressed);
 `)])]),F(`separator`,`
 margin: 0 8px;
 color: var(--n-separator-color);
 transition: color .3s var(--n-bezier);
 user-select: none;
 -webkit-user-select: none;
 `),R(`&:last-child`,[F(`link`,`
 font-weight: var(--n-font-weight-active);
 cursor: unset;
 color: var(--n-item-text-color-active);
 `,[B(`icon`,`
 color: var(--n-item-text-color-active);
 `)]),F(`separator`,`
 display: none;
 `)])])]),Ze=j(`n-breadcrumb`),Qe=f({name:`Breadcrumb`,props:Object.assign(Object.assign({},W.props),{separator:{type:String,default:`/`}}),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=H(e),r=W(`Breadcrumb`,`-breadcrumb`,Xe,Le,e,t);i(Ze,{separatorRef:x(e,`separator`),mergedClsPrefixRef:t});let a=S(()=>{let{common:{cubicBezierEaseInOut:e},self:{separatorColor:t,itemTextColor:n,itemTextColorHover:i,itemTextColorPressed:a,itemTextColorActive:o,fontSize:s,fontWeightActive:c,itemBorderRadius:l,itemColorHover:u,itemColorPressed:d,itemLineHeight:f}}=r.value;return{"--n-font-size":s,"--n-bezier":e,"--n-item-text-color":n,"--n-item-text-color-hover":i,"--n-item-text-color-pressed":a,"--n-item-text-color-active":o,"--n-separator-color":t,"--n-item-color-hover":u,"--n-item-color-pressed":d,"--n-item-border-radius":l,"--n-font-weight-active":c,"--n-item-line-height":f}}),o=n?K(`breadcrumb`,void 0,a,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:a,themeClass:o?.themeClass,onRender:o?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),s(`nav`,{class:[`${this.mergedClsPrefix}-breadcrumb`,this.themeClass],style:this.cssVars,"aria-label":`Breadcrumb`},s(`ul`,null,this.$slots))}});function $e(e=M?window:null){let t=()=>{let{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}=e?.location||{};return{hash:t,host:n,hostname:r,href:i,origin:a,pathname:o,port:s,protocol:c,search:l}},n=w(t()),r=()=>{n.value=t()};return u(()=>{e&&(e.addEventListener(`popstate`,r),e.addEventListener(`hashchange`,r))}),O(()=>{e&&(e.removeEventListener(`popstate`,r),e.removeEventListener(`hashchange`,r))}),n}var et=f({name:`BreadcrumbItem`,props:{separator:String,href:String,clickable:{type:Boolean,default:!0},showSeparator:{type:Boolean,default:!0},onClick:Function},slots:Object,setup(t,{slots:n}){let r=e(Ze,null);if(!r)return()=>null;let{separatorRef:i,mergedClsPrefixRef:a}=r,o=$e(),c=S(()=>t.href?`a`:`span`),l=S(()=>o.value.href===t.href?`location`:null);return()=>{let{value:e}=a;return s(`li`,{class:[`${e}-breadcrumb-item`,t.clickable&&`${e}-breadcrumb-item--clickable`]},s(c.value,{class:`${e}-breadcrumb-item__link`,"aria-current":l.value,href:t.href,onClick:t.onClick},n),t.showSeparator&&s(`span`,{class:`${e}-breadcrumb-item__separator`,"aria-hidden":`true`},re(n.separator,()=>[t.separator??i.value])))}}}),tt=f({name:`NDrawerContent`,inheritAttrs:!1,props:{blockScroll:Boolean,show:{type:Boolean,default:void 0},displayDirective:{type:String,required:!0},placement:{type:String,required:!0},contentClass:String,contentStyle:[Object,String],nativeScrollbar:{type:Boolean,required:!0},scrollbarProps:Object,trapFocus:{type:Boolean,default:!0},autoFocus:{type:Boolean,default:!0},showMask:{type:[Boolean,String],required:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,onClickoutside:Function,onAfterLeave:Function,onAfterEnter:Function,onEsc:Function},setup(t){let n=w(!!t.show),r=w(null),o=e(Pe),s=0,c=``,l=null,u=w(!1),d=w(!1),f=S(()=>t.placement===`top`||t.placement===`bottom`),{mergedClsPrefixRef:p,mergedRtlRef:m}=H(t),_=L(`Drawer`,m,p),v=k,y=e=>{d.value=!0,s=f.value?e.clientY:e.clientX,c=document.body.style.cursor,document.body.style.cursor=f.value?`ns-resize`:`ew-resize`,document.body.addEventListener(`mousemove`,O),document.body.addEventListener(`mouseleave`,v),document.body.addEventListener(`mouseup`,k)},b=()=>{l!==null&&(window.clearTimeout(l),l=null),d.value?u.value=!0:l=window.setTimeout(()=>{u.value=!0},300)},x=()=>{l!==null&&(window.clearTimeout(l),l=null),u.value=!1},{doUpdateHeight:C,doUpdateWidth:T}=o,E=e=>{let{maxWidth:n}=t;if(n&&e>n)return n;let{minWidth:r}=t;return r&&e<r?r:e},D=e=>{let{maxHeight:n}=t;if(n&&e>n)return n;let{minHeight:r}=t;return r&&e<r?r:e};function O(e){if(d.value)if(f.value){let n=r.value?.offsetHeight||0,i=s-e.clientY;n+=t.placement===`bottom`?i:-i,n=D(n),C(n),s=e.clientY}else{let n=r.value?.offsetWidth||0,i=s-e.clientX;n+=t.placement===`right`?i:-i,n=E(n),T(n),s=e.clientX}}function k(){d.value&&(s=0,d.value=!1,document.body.style.cursor=c,document.body.removeEventListener(`mousemove`,O),document.body.removeEventListener(`mouseup`,k),document.body.removeEventListener(`mouseleave`,v))}h(()=>{t.show&&(n.value=!0)}),g(()=>t.show,e=>{e||k()}),a(()=>{k()});let A=S(()=>{let{show:e}=t,n=[[N,e]];return t.showMask||n.push([Ve,t.onClickoutside,void 0,{capture:!0}]),n});function j(){var e;n.value=!1,(e=t.onAfterLeave)==null||e.call(t)}return Me(S(()=>t.blockScroll&&n.value)),i(Fe,r),i(ze,null),i(Re,null),{bodyRef:r,rtlEnabled:_,mergedClsPrefix:o.mergedClsPrefixRef,isMounted:o.isMountedRef,mergedTheme:o.mergedThemeRef,displayed:n,transitionName:S(()=>({right:`slide-in-from-right-transition`,left:`slide-in-from-left-transition`,top:`slide-in-from-top-transition`,bottom:`slide-in-from-bottom-transition`})[t.placement]),handleAfterLeave:j,bodyDirectives:A,handleMousedownResizeTrigger:y,handleMouseenterResizeTrigger:b,handleMouseleaveResizeTrigger:x,isDragging:d,isHoverOnResizeTrigger:u}},render(){let{$slots:e,mergedClsPrefix:t}=this;return this.displayDirective===`show`||this.displayed||this.show?l(s(`div`,{role:`none`},s(be,{disabled:!this.showMask||!this.trapFocus,active:this.show,autoFocus:this.autoFocus,onEsc:this.onEsc},{default:()=>s(P,{name:this.transitionName,appear:this.isMounted,onAfterEnter:this.onAfterEnter,onAfterLeave:this.handleAfterLeave},{default:()=>l(s(`div`,o(this.$attrs,{role:`dialog`,ref:`bodyRef`,"aria-modal":`true`,class:[`${t}-drawer`,this.rtlEnabled&&`${t}-drawer--rtl`,`${t}-drawer--${this.placement}-placement`,this.isDragging&&`${t}-drawer--unselectable`,this.nativeScrollbar&&`${t}-drawer--native-scrollbar`]}),[this.resizable?s(`div`,{class:[`${t}-drawer__resize-trigger`,(this.isDragging||this.isHoverOnResizeTrigger)&&`${t}-drawer__resize-trigger--hover`],onMouseenter:this.handleMouseenterResizeTrigger,onMouseleave:this.handleMouseleaveResizeTrigger,onMousedown:this.handleMousedownResizeTrigger}):null,this.nativeScrollbar?s(`div`,{class:[`${t}-drawer-content-wrapper`,this.contentClass],style:this.contentStyle,role:`none`},e):s(G,Object.assign({},this.scrollbarProps,{contentStyle:this.contentStyle,contentClass:[`${t}-drawer-content-wrapper`,this.contentClass],theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar}),e)]),this.bodyDirectives)})})),[[N,this.displayDirective===`if`||this.displayed||this.show]]):null}}),{cubicBezierEaseIn:nt,cubicBezierEaseOut:rt}=U;function it({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-bottom`}={}){return[R(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${nt}`}),R(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${rt}`}),R(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),R(`&.${n}-transition-enter-from`,{transform:`translateY(100%)`}),R(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),R(`&.${n}-transition-leave-to`,{transform:`translateY(100%)`})]}var{cubicBezierEaseIn:at,cubicBezierEaseOut:ot}=U;function st({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-left`}={}){return[R(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${at}`}),R(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ot}`}),R(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),R(`&.${n}-transition-enter-from`,{transform:`translateX(-100%)`}),R(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),R(`&.${n}-transition-leave-to`,{transform:`translateX(-100%)`})]}var{cubicBezierEaseIn:ct,cubicBezierEaseOut:lt}=U;function ut({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-right`}={}){return[R(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ct}`}),R(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${lt}`}),R(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),R(`&.${n}-transition-enter-from`,{transform:`translateX(100%)`}),R(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),R(`&.${n}-transition-leave-to`,{transform:`translateX(100%)`})]}var{cubicBezierEaseIn:dt,cubicBezierEaseOut:ft}=U;function pt({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-top`}={}){return[R(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${dt}`}),R(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${ft}`}),R(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),R(`&.${n}-transition-enter-from`,{transform:`translateY(-100%)`}),R(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),R(`&.${n}-transition-leave-to`,{transform:`translateY(-100%)`})]}var mt=R([B(`drawer`,`
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
 `,[ut(),st(),pt(),it(),I(`unselectable`,`
 user-select: none; 
 -webkit-user-select: none;
 `),I(`native-scrollbar`,[B(`drawer-content-wrapper`,`
 overflow: auto;
 height: 100%;
 `)]),F(`resize-trigger`,`
 position: absolute;
 background-color: #0000;
 transition: background-color .3s var(--n-bezier);
 `,[I(`hover`,`
 background-color: var(--n-resize-trigger-color-hover);
 `)]),B(`drawer-content-wrapper`,`
 box-sizing: border-box;
 `),B(`drawer-content`,`
 height: 100%;
 display: flex;
 flex-direction: column;
 `,[I(`native-scrollbar`,[B(`drawer-body-content-wrapper`,`
 height: 100%;
 overflow: auto;
 `)]),B(`drawer-body`,`
 flex: 1 0 0;
 overflow: hidden;
 `),B(`drawer-body-content-wrapper`,`
 box-sizing: border-box;
 padding: var(--n-body-padding);
 `),B(`drawer-header`,`
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
 `,[F(`main`,`
 flex: 1;
 `),F(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)]),B(`drawer-footer`,`
 display: flex;
 justify-content: flex-end;
 border-top: var(--n-footer-border-top);
 transition: border .3s var(--n-bezier);
 padding: var(--n-footer-padding);
 `)]),I(`right-placement`,`
 top: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-bottom-left-radius: var(--n-border-radius);
 `,[F(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 left: 0;
 transform: translateX(-1.5px);
 cursor: ew-resize;
 `)]),I(`left-placement`,`
 top: 0;
 bottom: 0;
 left: 0;
 border-top-right-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[F(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 right: 0;
 transform: translateX(1.5px);
 cursor: ew-resize;
 `)]),I(`top-placement`,`
 top: 0;
 left: 0;
 right: 0;
 border-bottom-left-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[F(`resize-trigger`,`
 width: 100%;
 height: 3px;
 bottom: 0;
 left: 0;
 transform: translateY(1.5px);
 cursor: ns-resize;
 `)]),I(`bottom-placement`,`
 left: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-top-right-radius: var(--n-border-radius);
 `,[F(`resize-trigger`,`
 width: 100%;
 height: 3px;
 top: 0;
 left: 0;
 transform: translateY(-1.5px);
 cursor: ns-resize;
 `)])]),R(`body`,[R(`>`,[B(`drawer-container`,`
 position: fixed;
 `)])]),B(`drawer-container`,`
 position: relative;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 `,[R(`> *`,`
 pointer-events: all;
 `)]),B(`drawer-mask`,`
 background-color: rgba(0, 0, 0, .3);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `,[I(`invisible`,`
 background-color: rgba(0, 0, 0, 0)
 `),ue({enterDuration:`0.2s`,leaveDuration:`0.2s`,enterCubicBezier:`var(--n-bezier-in)`,leaveCubicBezier:`var(--n-bezier-out)`})])]),ht=f({name:`Drawer`,inheritAttrs:!1,props:Object.assign(Object.assign({},W.props),{show:Boolean,width:[Number,String],height:[Number,String],placement:{type:String,default:`right`},maskClosable:{type:Boolean,default:!0},showMask:{type:[Boolean,String],default:!0},to:[String,Object],displayDirective:{type:String,default:`if`},nativeScrollbar:{type:Boolean,default:!0},zIndex:Number,onMaskClick:Function,scrollbarProps:Object,contentClass:String,contentStyle:[Object,String],trapFocus:{type:Boolean,default:!0},onEsc:Function,autoFocus:{type:Boolean,default:!0},closeOnEsc:{type:Boolean,default:!0},blockScroll:{type:Boolean,default:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,defaultWidth:{type:[Number,String],default:251},defaultHeight:{type:[Number,String],default:251},onUpdateWidth:[Function,Array],onUpdateHeight:[Function,Array],"onUpdate:width":[Function,Array],"onUpdate:height":[Function,Array],"onUpdate:show":[Function,Array],onUpdateShow:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,drawerStyle:[String,Object],drawerClass:String,target:null,onShow:Function,onHide:Function}),setup(e){let{mergedClsPrefixRef:t,namespaceRef:n,inlineThemeDisabled:r}=H(e),a=xe(),o=W(`Drawer`,`-drawer`,mt,je,e,t),s=w(e.defaultWidth),c=w(e.defaultHeight),l=Y(x(e,`width`),s),u=Y(x(e,`height`),c),d=S(()=>{let{placement:t}=e;return t===`top`||t===`bottom`?``:J(l.value)}),f=S(()=>{let{placement:t}=e;return t===`left`||t===`right`?``:J(u.value)}),p=t=>{let{onUpdateWidth:n,"onUpdate:width":r}=e;n&&q(n,t),r&&q(r,t),s.value=t},m=t=>{let{onUpdateHeight:n,"onUpdate:width":r}=e;n&&q(n,t),r&&q(r,t),c.value=t},h=S(()=>[{width:d.value,height:f.value},e.drawerStyle||``]);function g(t){let{onMaskClick:n,maskClosable:r}=e;r&&b(!1),n&&n(t)}function _(e){g(e)}let v=Ne();function y(t){var n;(n=e.onEsc)==null||n.call(e),e.show&&e.closeOnEsc&&Ae(t)&&(v.value||b(!1))}function b(t){let{onHide:n,onUpdateShow:r,"onUpdate:show":i}=e;r&&q(r,t),i&&q(i,t),n&&!t&&q(n,t)}i(Pe,{isMountedRef:a,mergedThemeRef:o,mergedClsPrefixRef:t,doUpdateShow:b,doUpdateHeight:m,doUpdateWidth:p});let C=S(()=>{let{common:{cubicBezierEaseInOut:e,cubicBezierEaseIn:t,cubicBezierEaseOut:n},self:{color:r,textColor:i,boxShadow:a,lineHeight:s,headerPadding:c,footerPadding:l,borderRadius:u,bodyPadding:d,titleFontSize:f,titleTextColor:p,titleFontWeight:m,headerBorderBottom:h,footerBorderTop:g,closeIconColor:_,closeIconColorHover:v,closeIconColorPressed:y,closeColorHover:b,closeColorPressed:x,closeIconSize:S,closeSize:C,closeBorderRadius:w,resizableTriggerColorHover:T}}=o.value;return{"--n-line-height":s,"--n-color":r,"--n-border-radius":u,"--n-text-color":i,"--n-box-shadow":a,"--n-bezier":e,"--n-bezier-out":n,"--n-bezier-in":t,"--n-header-padding":c,"--n-body-padding":d,"--n-footer-padding":l,"--n-title-text-color":p,"--n-title-font-size":f,"--n-title-font-weight":m,"--n-header-border-bottom":h,"--n-footer-border-top":g,"--n-close-icon-color":_,"--n-close-icon-color-hover":v,"--n-close-icon-color-pressed":y,"--n-close-size":C,"--n-close-color-hover":b,"--n-close-color-pressed":x,"--n-close-icon-size":S,"--n-close-border-radius":w,"--n-resize-trigger-color-hover":T}}),T=r?K(`drawer`,void 0,C,e):void 0;return{mergedClsPrefix:t,namespace:n,mergedBodyStyle:h,handleOutsideClick:_,handleMaskClick:g,handleEsc:y,mergedTheme:o,cssVars:r?void 0:C,themeClass:T?.themeClass,onRender:T?.onRender,isMounted:a}},render(){let{mergedClsPrefix:e}=this;return s(_e,{to:this.to,show:this.show},{default:()=>{var t;return(t=this.onRender)==null||t.call(this),l(s(`div`,{class:[`${e}-drawer-container`,this.namespace,this.themeClass],style:this.cssVars,role:`none`},this.showMask?s(P,{name:`fade-in-transition`,appear:this.isMounted},{default:()=>this.show?s(`div`,{"aria-hidden":!0,class:[`${e}-drawer-mask`,this.showMask===`transparent`&&`${e}-drawer-mask--invisible`],onClick:this.handleMaskClick}):null}):null,s(tt,Object.assign({},this.$attrs,{class:[this.drawerClass,this.$attrs.class],style:[this.mergedBodyStyle,this.$attrs.style],blockScroll:this.blockScroll,contentStyle:this.contentStyle,contentClass:this.contentClass,placement:this.placement,scrollbarProps:this.scrollbarProps,show:this.show,displayDirective:this.displayDirective,nativeScrollbar:this.nativeScrollbar,onAfterEnter:this.onAfterEnter,onAfterLeave:this.onAfterLeave,trapFocus:this.trapFocus,autoFocus:this.autoFocus,resizable:this.resizable,maxHeight:this.maxHeight,minHeight:this.minHeight,maxWidth:this.maxWidth,minWidth:this.minWidth,showMask:this.showMask,onEsc:this.handleEsc,onClickoutside:this.handleOutsideClick}),this.$slots)),[[he,{zIndex:this.zIndex,enabled:this.show}]])}})}}),gt=f({name:`DrawerContent`,props:{title:String,headerClass:String,headerStyle:[Object,String],footerClass:String,footerStyle:[Object,String],bodyClass:String,bodyStyle:[Object,String],bodyContentClass:String,bodyContentStyle:[Object,String],nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,closable:Boolean},slots:Object,setup(){let t=e(Pe,null);t||te(`drawer-content`,"`n-drawer-content` must be placed inside `n-drawer`.");let{doUpdateShow:n}=t;function r(){n(!1)}return{handleCloseClick:r,mergedTheme:t.mergedThemeRef,mergedClsPrefix:t.mergedClsPrefixRef}},render(){let{title:e,mergedClsPrefix:t,nativeScrollbar:n,mergedTheme:r,bodyClass:i,bodyStyle:a,bodyContentClass:o,bodyContentStyle:c,headerClass:l,headerStyle:u,footerClass:d,footerStyle:f,scrollbarProps:p,closable:m,$slots:h}=this;return s(`div`,{role:`none`,class:[`${t}-drawer-content`,n&&`${t}-drawer-content--native-scrollbar`]},h.header||e||m?s(`div`,{class:[`${t}-drawer-header`,l],style:u,role:`none`},s(`div`,{class:`${t}-drawer-header__main`,role:`heading`,"aria-level":`1`},h.header===void 0?e:h.header()),m&&s(k,{onClick:this.handleCloseClick,clsPrefix:t,class:`${t}-drawer-header__close`,absolute:!0})):null,n?s(`div`,{class:[`${t}-drawer-body`,i],style:a,role:`none`},s(`div`,{class:[`${t}-drawer-body-content-wrapper`,o],style:c,role:`none`},h)):s(G,Object.assign({themeOverrides:r.peerOverrides.Scrollbar,theme:r.peers.Scrollbar},p,{class:`${t}-drawer-body`,contentClass:[`${t}-drawer-body-content-wrapper`,o],contentStyle:c}),h),h.footer?s(`div`,{class:[`${t}-drawer-footer`,d],style:f,role:`none`},h.footer()):null)}});function _t(e){let{baseColor:t,textColor2:n,bodyColor:r,cardColor:i,dividerColor:a,actionColor:o,scrollbarColor:s,scrollbarColorHover:c,invertedColor:l}=e;return{textColor:n,textColorInverted:`#FFF`,color:r,colorEmbedded:o,headerColor:i,headerColorInverted:l,footerColor:o,footerColorInverted:l,headerBorderColor:a,headerBorderColorInverted:l,footerBorderColor:a,footerBorderColorInverted:l,siderBorderColor:a,siderBorderColorInverted:l,siderColor:i,siderColorInverted:l,siderToggleButtonBorder:`1px solid ${a}`,siderToggleButtonColor:t,siderToggleButtonIconColor:n,siderToggleButtonIconColorInverted:n,siderToggleBarColor:oe(r,s),siderToggleBarColorHover:oe(r,c),__invertScrollbar:`true`}}var vt=z({name:`Layout`,common:le,peers:{Scrollbar:ae},self:_t}),yt=j(`n-layout-sider`),bt={type:String,default:`static`},xt=B(`layout`,`
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
`,[B(`layout-scroll-container`,`
 overflow-x: hidden;
 box-sizing: border-box;
 height: 100%;
 `),I(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),St={embedded:Boolean,position:bt,nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,onScroll:Function,contentClass:String,contentStyle:{type:[String,Object],default:``},hasSider:Boolean,siderPlacement:{type:String,default:`left`}},Ct=j(`n-layout`);function wt(e){return f({name:e?`LayoutContent`:`Layout`,props:Object.assign(Object.assign({},W.props),St),setup(e){let t=w(null),n=w(null),{mergedClsPrefixRef:r,inlineThemeDisabled:a}=H(e),o=W(`Layout`,`-layout`,xt,vt,e,r);function s(r,i){if(e.nativeScrollbar){let{value:e}=t;e&&(i===void 0?e.scrollTo(r):e.scrollTo(r,i))}else{let{value:e}=n;e&&e.scrollTo(r,i)}}i(Ct,e);let c=0,l=0,u=t=>{var n;let r=t.target;c=r.scrollLeft,l=r.scrollTop,(n=e.onScroll)==null||n.call(e,t)};ne(()=>{if(e.nativeScrollbar){let e=t.value;e&&(e.scrollTop=l,e.scrollLeft=c)}});let d={display:`flex`,flexWrap:`nowrap`,width:`100%`,flexDirection:`row`},f={scrollTo:s},p=S(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=o.value;return{"--n-bezier":t,"--n-color":e.embedded?n.colorEmbedded:n.color,"--n-text-color":n.textColor}}),m=a?K(`layout`,S(()=>e.embedded?`e`:``),p,e):void 0;return Object.assign({mergedClsPrefix:r,scrollableElRef:t,scrollbarInstRef:n,hasSiderStyle:d,mergedTheme:o,handleNativeElScroll:u,cssVars:a?void 0:p,themeClass:m?.themeClass,onRender:m?.onRender},f)},render(){var t;let{mergedClsPrefix:n,hasSider:r}=this;(t=this.onRender)==null||t.call(this);let i=r?this.hasSiderStyle:void 0;return s(`div`,{class:[this.themeClass,e&&`${n}-layout-content`,`${n}-layout`,`${n}-layout--${this.position}-positioned`],style:this.cssVars},this.nativeScrollbar?s(`div`,{ref:`scrollableElRef`,class:[`${n}-layout-scroll-container`,this.contentClass],style:[this.contentStyle,i],onScroll:this.handleNativeElScroll},this.$slots):s(G,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,contentClass:this.contentClass,contentStyle:[this.contentStyle,i]}),this.$slots))}})}var Tt=wt(!1),Et=wt(!0),Dt=B(`layout-header`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 box-sizing: border-box;
 width: 100%;
 background-color: var(--n-color);
 color: var(--n-text-color);
`,[I(`absolute-positioned`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 `),I(`bordered`,`
 border-bottom: solid 1px var(--n-border-color);
 `)]),Ot={position:bt,inverted:Boolean,bordered:{type:Boolean,default:!1}},kt=f({name:`LayoutHeader`,props:Object.assign(Object.assign({},W.props),Ot),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=H(e),r=W(`Layout`,`-layout-header`,Dt,vt,e,t),i=S(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=r.value,i={"--n-bezier":t};return e.inverted?(i[`--n-color`]=n.headerColorInverted,i[`--n-text-color`]=n.textColorInverted,i[`--n-border-color`]=n.headerBorderColorInverted):(i[`--n-color`]=n.headerColor,i[`--n-text-color`]=n.textColor,i[`--n-border-color`]=n.headerBorderColor),i}),a=n?K(`layout-header`,S(()=>e.inverted?`a`:`b`),i,e):void 0;return{mergedClsPrefix:t,cssVars:n?void 0:i,themeClass:a?.themeClass,onRender:a?.onRender}},render(){var e;let{mergedClsPrefix:t}=this;return(e=this.onRender)==null||e.call(this),s(`div`,{class:[`${t}-layout-header`,this.themeClass,this.position&&`${t}-layout-header--${this.position}-positioned`,this.bordered&&`${t}-layout-header--bordered`],style:this.cssVars},this.$slots)}}),At=B(`layout-sider`,`
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
`,[I(`bordered`,[F(`border`,`
 content: "";
 position: absolute;
 top: 0;
 bottom: 0;
 width: 1px;
 background-color: var(--n-border-color);
 transition: background-color .3s var(--n-bezier);
 `)]),F(`left-placement`,[I(`bordered`,[F(`border`,`
 right: 0;
 `)])]),I(`right-placement`,`
 justify-content: flex-start;
 `,[I(`bordered`,[F(`border`,`
 left: 0;
 `)]),I(`collapsed`,[B(`layout-toggle-button`,[B(`base-icon`,`
 transform: rotate(180deg);
 `)]),B(`layout-toggle-bar`,[R(`&:hover`,[F(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),F(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])])]),B(`layout-toggle-button`,`
 left: 0;
 transform: translateX(-50%) translateY(-50%);
 `,[B(`base-icon`,`
 transform: rotate(0);
 `)]),B(`layout-toggle-bar`,`
 left: -28px;
 transform: rotate(180deg);
 `,[R(`&:hover`,[F(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),F(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})])])]),I(`collapsed`,[B(`layout-toggle-bar`,[R(`&:hover`,[F(`top`,{transform:`rotate(-12deg) scale(1.15) translateY(-2px)`}),F(`bottom`,{transform:`rotate(12deg) scale(1.15) translateY(2px)`})])]),B(`layout-toggle-button`,[B(`base-icon`,`
 transform: rotate(0);
 `)])]),B(`layout-toggle-button`,`
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
 `,[B(`base-icon`,`
 transition: transform .3s var(--n-bezier);
 transform: rotate(180deg);
 `)]),B(`layout-toggle-bar`,`
 cursor: pointer;
 height: 72px;
 width: 32px;
 position: absolute;
 top: calc(50% - 36px);
 right: -28px;
 `,[F(`top, bottom`,`
 position: absolute;
 width: 4px;
 border-radius: 2px;
 height: 38px;
 left: 14px;
 transition: 
 background-color .3s var(--n-bezier),
 transform .3s var(--n-bezier);
 `),F(`bottom`,`
 position: absolute;
 top: 34px;
 `),R(`&:hover`,[F(`top`,{transform:`rotate(12deg) scale(1.15) translateY(-2px)`}),F(`bottom`,{transform:`rotate(-12deg) scale(1.15) translateY(2px)`})]),F(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color)`}),R(`&:hover`,[F(`top, bottom`,{backgroundColor:`var(--n-toggle-bar-color-hover)`})])]),F(`border`,`
 position: absolute;
 top: 0;
 right: 0;
 bottom: 0;
 width: 1px;
 transition: background-color .3s var(--n-bezier);
 `),B(`layout-sider-scroll-container`,`
 flex-grow: 1;
 flex-shrink: 0;
 box-sizing: border-box;
 height: 100%;
 opacity: 0;
 transition: opacity .3s var(--n-bezier);
 max-width: 100%;
 `),I(`show-content`,[B(`layout-sider-scroll-container`,{opacity:1})]),I(`absolute-positioned`,`
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 `)]),jt=f({props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return s(`div`,{onClick:this.onClick,class:`${e}-layout-toggle-bar`},s(`div`,{class:`${e}-layout-toggle-bar__top`}),s(`div`,{class:`${e}-layout-toggle-bar__bottom`}))}}),Mt=f({name:`LayoutToggleButton`,props:{clsPrefix:{type:String,required:!0},onClick:Function},render(){let{clsPrefix:e}=this;return s(`div`,{class:`${e}-layout-toggle-button`,onClick:this.onClick},s(A,{clsPrefix:e},{default:()=>s(we,null)}))}}),Nt={position:bt,bordered:Boolean,collapsedWidth:{type:Number,default:48},width:{type:[Number,String],default:272},contentClass:String,contentStyle:{type:[String,Object],default:``},collapseMode:{type:String,default:`transform`},collapsed:{type:Boolean,default:void 0},defaultCollapsed:Boolean,showCollapsedContent:{type:Boolean,default:!0},showTrigger:{type:[Boolean,String],default:!1},nativeScrollbar:{type:Boolean,default:!0},inverted:Boolean,scrollbarProps:Object,triggerClass:String,triggerStyle:[String,Object],collapsedTriggerClass:String,collapsedTriggerStyle:[String,Object],"onUpdate:collapsed":[Function,Array],onUpdateCollapsed:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,onExpand:[Function,Array],onCollapse:[Function,Array],onScroll:Function},Pt=f({name:`LayoutSider`,props:Object.assign(Object.assign({},W.props),Nt),setup(t){let n=e(Ct),r=w(null),a=w(null),o=w(t.defaultCollapsed),s=Y(x(t,`collapsed`),o),c=S(()=>J(s.value?t.collapsedWidth:t.width)),l=S(()=>t.collapseMode===`transform`?{minWidth:J(t.width)}:{}),u=S(()=>n?n.siderPlacement:`left`);function d(e,n){if(t.nativeScrollbar){let{value:t}=r;t&&(n===void 0?t.scrollTo(e):t.scrollTo(e,n))}else{let{value:t}=a;t&&t.scrollTo(e,n)}}function f(){let{"onUpdate:collapsed":e,onUpdateCollapsed:n,onExpand:r,onCollapse:i}=t,{value:a}=s;n&&q(n,!a),e&&q(e,!a),o.value=!a,a?r&&q(r):i&&q(i)}let p=0,m=0,h=e=>{var n;let r=e.target;p=r.scrollLeft,m=r.scrollTop,(n=t.onScroll)==null||n.call(t,e)};ne(()=>{if(t.nativeScrollbar){let e=r.value;e&&(e.scrollTop=m,e.scrollLeft=p)}}),i(yt,{collapsedRef:s,collapseModeRef:x(t,`collapseMode`)});let{mergedClsPrefixRef:g,inlineThemeDisabled:_}=H(t),v=W(`Layout`,`-layout-sider`,At,vt,t,g);function y(e){var n,r;e.propertyName===`max-width`&&(s.value?(n=t.onAfterLeave)==null||n.call(t):(r=t.onAfterEnter)==null||r.call(t))}let b={scrollTo:d},C=S(()=>{let{common:{cubicBezierEaseInOut:e},self:n}=v.value,{siderToggleButtonColor:r,siderToggleButtonBorder:i,siderToggleBarColor:a,siderToggleBarColorHover:o}=n,s={"--n-bezier":e,"--n-toggle-button-color":r,"--n-toggle-button-border":i,"--n-toggle-bar-color":a,"--n-toggle-bar-color-hover":o};return t.inverted?(s[`--n-color`]=n.siderColorInverted,s[`--n-text-color`]=n.textColorInverted,s[`--n-border-color`]=n.siderBorderColorInverted,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColorInverted,s.__invertScrollbar=n.__invertScrollbar):(s[`--n-color`]=n.siderColor,s[`--n-text-color`]=n.textColor,s[`--n-border-color`]=n.siderBorderColor,s[`--n-toggle-button-icon-color`]=n.siderToggleButtonIconColor),s}),T=_?K(`layout-sider`,S(()=>t.inverted?`a`:`b`),C,t):void 0;return Object.assign({scrollableElRef:r,scrollbarInstRef:a,mergedClsPrefix:g,mergedTheme:v,styleMaxWidth:c,mergedCollapsed:s,scrollContainerStyle:l,siderPlacement:u,handleNativeElScroll:h,handleTransitionend:y,handleTriggerClick:f,inlineThemeDisabled:_,cssVars:C,themeClass:T?.themeClass,onRender:T?.onRender},b)},render(){var e;let{mergedClsPrefix:t,mergedCollapsed:n,showTrigger:r}=this;return(e=this.onRender)==null||e.call(this),s(`aside`,{class:[`${t}-layout-sider`,this.themeClass,`${t}-layout-sider--${this.position}-positioned`,`${t}-layout-sider--${this.siderPlacement}-placement`,this.bordered&&`${t}-layout-sider--bordered`,n&&`${t}-layout-sider--collapsed`,(!n||this.showCollapsedContent)&&`${t}-layout-sider--show-content`],onTransitionend:this.handleTransitionend,style:[this.inlineThemeDisabled?void 0:this.cssVars,{maxWidth:this.styleMaxWidth,width:J(this.width)}]},this.nativeScrollbar?s(`div`,{class:[`${t}-layout-sider-scroll-container`,this.contentClass],onScroll:this.handleNativeElScroll,style:[this.scrollContainerStyle,{overflow:`auto`},this.contentStyle],ref:`scrollableElRef`},this.$slots):s(G,Object.assign({},this.scrollbarProps,{onScroll:this.onScroll,ref:`scrollbarInstRef`,style:this.scrollContainerStyle,contentStyle:this.contentStyle,contentClass:this.contentClass,theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar,builtinThemeOverrides:this.inverted&&this.cssVars.__invertScrollbar===`true`?{colorHover:`rgba(255, 255, 255, .4)`,color:`rgba(255, 255, 255, .3)`}:void 0}),this.$slots),r?s(r===`bar`?jt:Mt,{clsPrefix:t,class:n?this.collapsedTriggerClass:this.triggerClass,style:n?this.collapsedTriggerStyle:this.triggerStyle,onClick:this.handleTriggerClick}):null,this.bordered?s(`div`,{class:`${t}-layout-sider__border`}):null)}}),Q=j(`n-menu`),Ft=j(`n-submenu`),It=j(`n-menu-item-group`),Lt=[R(`&::before`,`background-color: var(--n-item-color-hover);`),F(`arrow`,`
 color: var(--n-arrow-color-hover);
 `),F(`icon`,`
 color: var(--n-item-icon-color-hover);
 `),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover);
 `,[R(`a`,`
 color: var(--n-item-text-color-hover);
 `),F(`extra`,`
 color: var(--n-item-text-color-hover);
 `)])],Rt=[F(`icon`,`
 color: var(--n-item-icon-color-hover-horizontal);
 `),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-hover-horizontal);
 `,[R(`a`,`
 color: var(--n-item-text-color-hover-horizontal);
 `),F(`extra`,`
 color: var(--n-item-text-color-hover-horizontal);
 `)])],zt=R([B(`menu`,`
 background-color: var(--n-color);
 color: var(--n-item-text-color);
 overflow: hidden;
 transition: background-color .3s var(--n-bezier);
 box-sizing: border-box;
 font-size: var(--n-font-size);
 padding-bottom: 6px;
 `,[I(`horizontal`,`
 max-width: 100%;
 width: 100%;
 display: flex;
 overflow: hidden;
 padding-bottom: 0;
 `,[B(`submenu`,`margin: 0;`),B(`menu-item`,`margin: 0;`),B(`menu-item-content`,`
 padding: 0 20px;
 border-bottom: 2px solid #0000;
 `,[R(`&::before`,`display: none;`),I(`selected`,`border-bottom: 2px solid var(--n-border-color-horizontal)`)]),B(`menu-item-content`,[I(`selected`,[F(`icon`,`color: var(--n-item-icon-color-active-horizontal);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-horizontal);
 `,[R(`a`,`color: var(--n-item-text-color-active-horizontal);`),F(`extra`,`color: var(--n-item-text-color-active-horizontal);`)])]),I(`child-active`,`
 border-bottom: 2px solid var(--n-border-color-horizontal);
 `,[B(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `,[R(`a`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `),F(`extra`,`
 color: var(--n-item-text-color-child-active-horizontal);
 `)]),F(`icon`,`
 color: var(--n-item-icon-color-child-active-horizontal);
 `)]),V(`disabled`,[V(`selected, child-active`,[R(`&:focus-within`,Rt)]),I(`selected`,[$(null,[F(`icon`,`color: var(--n-item-icon-color-active-hover-horizontal);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover-horizontal);
 `,[R(`a`,`color: var(--n-item-text-color-active-hover-horizontal);`),F(`extra`,`color: var(--n-item-text-color-active-hover-horizontal);`)])])]),I(`child-active`,[$(null,[F(`icon`,`color: var(--n-item-icon-color-child-active-hover-horizontal);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover-horizontal);
 `,[R(`a`,`color: var(--n-item-text-color-child-active-hover-horizontal);`),F(`extra`,`color: var(--n-item-text-color-child-active-hover-horizontal);`)])])]),$(`border-bottom: 2px solid var(--n-border-color-horizontal);`,Rt)]),B(`menu-item-content-header`,[R(`a`,`color: var(--n-item-text-color-horizontal);`)])])]),V(`responsive`,[B(`menu-item-content-header`,`
 overflow: hidden;
 text-overflow: ellipsis;
 `)]),I(`collapsed`,[B(`menu-item-content`,[I(`selected`,[R(`&::before`,`
 background-color: var(--n-item-color-active-collapsed) !important;
 `)]),B(`menu-item-content-header`,`opacity: 0;`),F(`arrow`,`opacity: 0;`),F(`icon`,`color: var(--n-item-icon-color-collapsed);`)])]),B(`menu-item`,`
 height: var(--n-item-height);
 margin-top: 6px;
 position: relative;
 `),B(`menu-item-content`,`
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
 `,[R(`> *`,`z-index: 1;`),R(`&::before`,`
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
 `),I(`disabled`,`
 opacity: .45;
 cursor: not-allowed;
 `),I(`collapsed`,[F(`arrow`,`transform: rotate(0);`)]),I(`selected`,[R(`&::before`,`background-color: var(--n-item-color-active);`),F(`arrow`,`color: var(--n-arrow-color-active);`),F(`icon`,`color: var(--n-item-icon-color-active);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-active);
 `,[R(`a`,`color: var(--n-item-text-color-active);`),F(`extra`,`color: var(--n-item-text-color-active);`)])]),I(`child-active`,[B(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active);
 `,[R(`a`,`
 color: var(--n-item-text-color-child-active);
 `),F(`extra`,`
 color: var(--n-item-text-color-child-active);
 `)]),F(`arrow`,`
 color: var(--n-arrow-color-child-active);
 `),F(`icon`,`
 color: var(--n-item-icon-color-child-active);
 `)]),V(`disabled`,[V(`selected, child-active`,[R(`&:focus-within`,Lt)]),I(`selected`,[$(null,[F(`arrow`,`color: var(--n-arrow-color-active-hover);`),F(`icon`,`color: var(--n-item-icon-color-active-hover);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-active-hover);
 `,[R(`a`,`color: var(--n-item-text-color-active-hover);`),F(`extra`,`color: var(--n-item-text-color-active-hover);`)])])]),I(`child-active`,[$(null,[F(`arrow`,`color: var(--n-arrow-color-child-active-hover);`),F(`icon`,`color: var(--n-item-icon-color-child-active-hover);`),B(`menu-item-content-header`,`
 color: var(--n-item-text-color-child-active-hover);
 `,[R(`a`,`color: var(--n-item-text-color-child-active-hover);`),F(`extra`,`color: var(--n-item-text-color-child-active-hover);`)])])]),I(`selected`,[$(null,[R(`&::before`,`background-color: var(--n-item-color-active-hover);`)])]),$(null,Lt)]),F(`icon`,`
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
 `),F(`arrow`,`
 grid-area: arrow;
 font-size: 16px;
 color: var(--n-arrow-color);
 transform: rotate(180deg);
 opacity: 1;
 transition:
 color .3s var(--n-bezier),
 transform 0.2s var(--n-bezier),
 opacity 0.2s var(--n-bezier);
 `),B(`menu-item-content-header`,`
 grid-area: content;
 transition:
 color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 opacity: 1;
 white-space: nowrap;
 color: var(--n-item-text-color);
 `,[R(`a`,`
 outline: none;
 text-decoration: none;
 transition: color .3s var(--n-bezier);
 color: var(--n-item-text-color);
 `,[R(`&::before`,`
 content: "";
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),F(`extra`,`
 font-size: .93em;
 color: var(--n-group-text-color);
 transition: color .3s var(--n-bezier);
 `)])]),B(`submenu`,`
 cursor: pointer;
 position: relative;
 margin-top: 6px;
 `,[B(`menu-item-content`,`
 height: var(--n-item-height);
 `),B(`submenu-children`,`
 overflow: hidden;
 padding: 0;
 `,[Ie({duration:`.2s`})])]),B(`menu-item-group`,[B(`menu-item-group-title`,`
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
 `)])]),B(`menu-tooltip`,[R(`a`,`
 color: inherit;
 text-decoration: none;
 `)]),B(`menu-divider`,`
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-divider-color);
 height: 1px;
 margin: 6px 18px;
 `)]);function $(e,t){return[I(`hover`,e,t),R(`&:hover`,e,t)]}var Bt=f({name:`MenuOptionContent`,props:{collapsed:Boolean,disabled:Boolean,title:[String,Function],icon:Function,extra:[String,Function],showArrow:Boolean,childActive:Boolean,hover:Boolean,paddingLeft:Number,selected:Boolean,maxIconSize:{type:Number,required:!0},activeIconSize:{type:Number,required:!0},iconMarginRight:{type:Number,required:!0},clsPrefix:{type:String,required:!0},onClick:Function,tmNode:{type:Object,required:!0},isEllipsisPlaceholder:Boolean},setup(t){let{props:n}=e(Q);return{menuProps:n,style:S(()=>{let{paddingLeft:e}=t;return{paddingLeft:e&&`${e}px`}}),iconStyle:S(()=>{let{maxIconSize:e,activeIconSize:n,iconMarginRight:r}=t;return{width:`${e}px`,height:`${e}px`,fontSize:`${n}px`,marginRight:`${r}px`}})}},render(){let{clsPrefix:e,tmNode:t,menuProps:{renderIcon:n,renderLabel:r,renderExtra:i,expandIcon:a}}=this,o=n?n(t.rawNode):Z(this.icon);return s(`div`,{onClick:e=>{var t;(t=this.onClick)==null||t.call(this,e)},role:`none`,class:[`${e}-menu-item-content`,{[`${e}-menu-item-content--selected`]:this.selected,[`${e}-menu-item-content--collapsed`]:this.collapsed,[`${e}-menu-item-content--child-active`]:this.childActive,[`${e}-menu-item-content--disabled`]:this.disabled,[`${e}-menu-item-content--hover`]:this.hover}],style:this.style},o&&s(`div`,{class:`${e}-menu-item-content__icon`,style:this.iconStyle,role:`none`},[o]),s(`div`,{class:`${e}-menu-item-content-header`,role:`none`},this.isEllipsisPlaceholder?this.title:r?r(t.rawNode):Z(this.title),this.extra||i?s(`span`,{class:`${e}-menu-item-content-header__extra`},` `,i?i(t.rawNode):Z(this.extra)):null),this.showArrow?s(A,{ariaHidden:!0,class:`${e}-menu-item-content__arrow`,clsPrefix:e},{default:()=>a?a(t.rawNode):s(Ye,null)}):null)}}),Vt=8;function Ht(t){let n=e(Q),{props:r,mergedCollapsedRef:i}=n,a=e(Ft,null),o=e(It,null),s=S(()=>r.mode===`horizontal`),c=S(()=>s.value?r.dropdownPlacement:`tmNodes`in t?`right-start`:`right`),l=S(()=>Math.max(r.collapsedIconSize??r.iconSize,r.iconSize));return{dropdownPlacement:c,activeIconSize:S(()=>!s.value&&t.root&&i.value?r.collapsedIconSize??r.iconSize:r.iconSize),maxIconSize:l,paddingLeft:S(()=>{if(s.value)return;let{collapsedWidth:e,indent:n,rootIndent:c}=r,{root:u,isGroup:d}=t,f=c===void 0?n:c;return u?i.value?e/2-l.value/2:f:o&&typeof o.paddingLeftRef.value==`number`?n/2+o.paddingLeftRef.value:a&&typeof a.paddingLeftRef.value==`number`?(d?n/2:n)+a.paddingLeftRef.value:0}),iconMarginRight:S(()=>{let{collapsedWidth:e,indent:n,rootIndent:a}=r,{value:o}=l,{root:c}=t;return s.value||!c||!i.value?Vt:(a===void 0?n:a)+o+Vt-(e+o)/2}),NMenu:n,NSubmenu:a,NMenuOptionGroup:o}}var Ut={internalKey:{type:[String,Number],required:!0},root:Boolean,isGroup:Boolean,level:{type:Number,required:!0},title:[String,Function],extra:[String,Function]},Wt=f({name:`MenuDivider`,setup(){let{mergedClsPrefixRef:t,isHorizontalRef:n}=e(Q);return()=>n.value?null:s(`div`,{class:`${t.value}-menu-divider`})}}),Gt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function}),Kt=ie(Gt),qt=f({name:`MenuOption`,props:Gt,setup(e){let t=Ht(e),{NSubmenu:n,NMenu:r,NMenuOptionGroup:i}=t,{props:a,mergedClsPrefixRef:o,mergedCollapsedRef:s}=r,c=n?n.mergedDisabledRef:i?i.mergedDisabledRef:{value:!1},l=S(()=>c.value||e.disabled);function u(t){let{onClick:n}=e;n&&n(t)}function d(t){l.value||(r.doSelect(e.internalKey,e.tmNode.rawNode),u(t))}return{mergedClsPrefix:o,dropdownPlacement:t.dropdownPlacement,paddingLeft:t.paddingLeft,iconMarginRight:t.iconMarginRight,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,mergedTheme:r.mergedThemeRef,menuProps:a,dropdownEnabled:de(()=>e.root&&s.value&&a.mode!==`horizontal`&&!l.value),selected:de(()=>r.mergedValueRef.value===e.internalKey),mergedDisabled:l,handleClick:d}},render(){let{mergedClsPrefix:e,mergedTheme:t,tmNode:n,menuProps:{renderLabel:r,nodeProps:i}}=this,a=i?.(n.rawNode);return s(`div`,Object.assign({},a,{role:`menuitem`,class:[`${e}-menu-item`,a?.class]}),s(Ce,{theme:t.peers.Tooltip,themeOverrides:t.peerOverrides.Tooltip,trigger:`hover`,placement:this.dropdownPlacement,disabled:!this.dropdownEnabled||this.title===void 0,internalExtraClass:[`menu-tooltip`]},{default:()=>r?r(n.rawNode):Z(this.title),trigger:()=>s(Bt,{tmNode:n,clsPrefix:e,paddingLeft:this.paddingLeft,iconMarginRight:this.iconMarginRight,maxIconSize:this.maxIconSize,activeIconSize:this.activeIconSize,selected:this.selected,title:this.title,extra:this.extra,disabled:this.mergedDisabled,icon:this.icon,onClick:this.handleClick})}))}}),Jt=Object.assign(Object.assign({},Ut),{tmNode:{type:Object,required:!0},tmNodes:{type:Array,required:!0}}),Yt=ie(Jt),Xt=f({name:`MenuOptionGroup`,props:Jt,setup(t){let n=Ht(t),{NSubmenu:r}=n,a=S(()=>r?.mergedDisabledRef.value?!0:t.tmNode.disabled);i(It,{paddingLeftRef:n.paddingLeft,mergedDisabledRef:a});let{mergedClsPrefixRef:o,props:c}=e(Q);return function(){let{value:e}=o,r=n.paddingLeft.value,{nodeProps:i}=c,a=i?.(t.tmNode.rawNode);return s(`div`,{class:`${e}-menu-item-group`,role:`group`},s(`div`,Object.assign({},a,{class:[`${e}-menu-item-group-title`,a?.class],style:[a?.style||``,r===void 0?``:`padding-left: ${r}px;`]}),Z(t.title),t.extra?s(b,null,` `,Z(t.extra)):null),s(`div`,null,t.tmNodes.map(e=>$t(e,c))))}}});function Zt(e){return e.type===`divider`||e.type===`render`}function Qt(e){return e.type===`divider`}function $t(e,t){let{rawNode:n}=e,{show:r}=n;if(r===!1)return null;if(Zt(n))return Qt(n)?s(Wt,Object.assign({key:e.key},n.props)):null;let{labelField:i}=t,{key:a,level:o,isGroup:c}=e,l=Object.assign(Object.assign({},n),{title:n.title||n[i],extra:n.titleExtra||n.extra,key:a,internalKey:a,level:o,root:o===0,isGroup:c});return e.children?e.isGroup?s(Xt,Oe(l,Yt,{tmNode:e,tmNodes:e.children,key:a})):s(nn,Oe(l,tn,{key:a,rawNodes:n[t.childrenField],tmNodes:e.children,tmNode:e})):s(qt,Oe(l,Kt,{key:a,tmNode:e}))}var en=Object.assign(Object.assign({},Ut),{rawNodes:{type:Array,default:()=>[]},tmNodes:{type:Array,default:()=>[]},tmNode:{type:Object,required:!0},disabled:Boolean,icon:Function,onClick:Function,domId:String,virtualChildActive:{type:Boolean,default:void 0},isEllipsisPlaceholder:Boolean}),tn=ie(en),nn=f({name:`Submenu`,props:en,setup(e){let t=Ht(e),{NMenu:n,NSubmenu:r}=t,{props:a,mergedCollapsedRef:o,mergedThemeRef:s}=n,c=S(()=>{let{disabled:t}=e;return r?.mergedDisabledRef.value||a.disabled?!0:t}),l=w(!1);i(Ft,{paddingLeftRef:t.paddingLeft,mergedDisabledRef:c}),i(It,null);function u(){let{onClick:t}=e;t&&t()}function d(){c.value||(o.value||n.toggleExpand(e.internalKey),u())}function f(e){l.value=e}return{menuProps:a,mergedTheme:s,doSelect:n.doSelect,inverted:n.invertedRef,isHorizontal:n.isHorizontalRef,mergedClsPrefix:n.mergedClsPrefixRef,maxIconSize:t.maxIconSize,activeIconSize:t.activeIconSize,iconMarginRight:t.iconMarginRight,dropdownPlacement:t.dropdownPlacement,dropdownShow:l,paddingLeft:t.paddingLeft,mergedDisabled:c,mergedValue:n.mergedValueRef,childActive:de(()=>e.virtualChildActive??n.activePathRef.value.includes(e.internalKey)),collapsed:S(()=>a.mode===`horizontal`?!1:o.value?!0:!n.mergedExpandedKeysRef.value.includes(e.internalKey)),dropdownEnabled:S(()=>!c.value&&(a.mode===`horizontal`||o.value)),handlePopoverShowChange:f,handleClick:d}},render(){let{mergedClsPrefix:e,menuProps:{renderIcon:t,renderLabel:n}}=this,r=()=>{let{isHorizontal:e,paddingLeft:t,collapsed:n,mergedDisabled:r,maxIconSize:i,activeIconSize:a,title:o,childActive:c,icon:l,handleClick:u,menuProps:{nodeProps:d},dropdownShow:f,iconMarginRight:p,tmNode:m,mergedClsPrefix:h,isEllipsisPlaceholder:g,extra:_}=this,v=d?.(m.rawNode);return s(`div`,Object.assign({},v,{class:[`${h}-menu-item`,v?.class],role:`menuitem`}),s(Bt,{tmNode:m,paddingLeft:t,collapsed:n,disabled:r,iconMarginRight:p,maxIconSize:i,activeIconSize:a,title:o,extra:_,showArrow:!e,childActive:c,clsPrefix:h,icon:l,hover:f,onClick:u,isEllipsisPlaceholder:g}))},i=()=>s(ee,null,{default:()=>{let{tmNodes:t,collapsed:n}=this;return n?null:s(`div`,{class:`${e}-submenu-children`,role:`menu`},t.map(e=>$t(e,this.menuProps)))}});return this.root?s(Te,Object.assign({size:`large`,trigger:`hover`},this.menuProps?.dropdownProps,{themeOverrides:this.mergedTheme.peerOverrides.Dropdown,theme:this.mergedTheme.peers.Dropdown,builtinThemeOverrides:{fontSizeLarge:`14px`,optionIconSizeLarge:`18px`},value:this.mergedValue,disabled:!this.dropdownEnabled,placement:this.dropdownPlacement,keyField:this.menuProps.keyField,labelField:this.menuProps.labelField,childrenField:this.menuProps.childrenField,onUpdateShow:this.handlePopoverShowChange,options:this.rawNodes,onSelect:this.doSelect,inverted:this.inverted,renderIcon:t,renderLabel:n}),{default:()=>s(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},r(),this.isHorizontal?null:i())}):s(`div`,{class:`${e}-submenu`,role:`menu`,"aria-expanded":!this.collapsed,id:this.domId},r(),i())}}),rn=f({name:`Menu`,inheritAttrs:!1,props:Object.assign(Object.assign({},W.props),{options:{type:Array,default:()=>[]},collapsed:{type:Boolean,default:void 0},collapsedWidth:{type:Number,default:48},iconSize:{type:Number,default:20},collapsedIconSize:{type:Number,default:24},rootIndent:Number,indent:{type:Number,default:32},labelField:{type:String,default:`label`},keyField:{type:String,default:`key`},childrenField:{type:String,default:`children`},disabledField:{type:String,default:`disabled`},defaultExpandAll:Boolean,defaultExpandedKeys:Array,expandedKeys:Array,value:[String,Number],defaultValue:{type:[String,Number],default:null},mode:{type:String,default:`vertical`},watchProps:{type:Array,default:void 0},disabled:Boolean,show:{type:Boolean,default:!0},inverted:Boolean,"onUpdate:expandedKeys":[Function,Array],onUpdateExpandedKeys:[Function,Array],onUpdateValue:[Function,Array],"onUpdate:value":[Function,Array],expandIcon:Function,renderIcon:Function,renderLabel:Function,renderExtra:Function,dropdownProps:Object,accordion:Boolean,nodeProps:Function,dropdownPlacement:{type:String,default:`bottom`},responsive:Boolean,items:Array,onOpenNamesChange:[Function,Array],onSelect:[Function,Array],onExpandedNamesChange:[Function,Array],expandedNames:Array,defaultExpandedNames:Array}),setup(t){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=H(t),a=W(`Menu`,`-menu`,zt,ke,t,n),o=e(yt,null),c=S(()=>{let{collapsed:e}=t;if(e!==void 0)return e;if(o){let{collapseModeRef:e,collapsedRef:t}=o;if(e.value===`width`)return t.value??!1}return!1}),l=S(()=>{let{keyField:e,childrenField:n,disabledField:r}=t;return Se(t.items||t.options,{getIgnored(e){return Zt(e)},getChildren(e){return e[n]},getDisabled(e){return e[r]},getKey(t){return t[e]??t.name}})}),u=S(()=>new Set(l.value.treeNodes.map(e=>e.key))),{watchProps:d}=t,f=w(null);d?.includes(`defaultValue`)?h(()=>{f.value=t.defaultValue}):f.value=t.defaultValue;let p=Y(x(t,`value`),f),m=w([]),g=()=>{m.value=t.defaultExpandAll?l.value.getNonLeafKeys():t.defaultExpandedNames||t.defaultExpandedKeys||l.value.getPath(p.value,{includeSelf:!1}).keyPath};d?.includes(`defaultExpandedKeys`)?h(g):g();let _=Ee(t,[`expandedNames`,`expandedKeys`]),v=Y(_,m),y=S(()=>l.value.treeNodes),b=S(()=>l.value.getPath(p.value).keyPath);i(Q,{props:t,mergedCollapsedRef:c,mergedThemeRef:a,mergedValueRef:p,mergedExpandedKeysRef:v,activePathRef:b,mergedClsPrefixRef:n,isHorizontalRef:S(()=>t.mode===`horizontal`),invertedRef:x(t,`inverted`),doSelect:C,toggleExpand:E});function C(e,n){let{"onUpdate:value":r,onUpdateValue:i,onSelect:a}=t;i&&q(i,e,n),r&&q(r,e,n),a&&q(a,e,n),f.value=e}function T(e){let{"onUpdate:expandedKeys":n,onUpdateExpandedKeys:r,onExpandedNamesChange:i,onOpenNamesChange:a}=t;n&&q(n,e),r&&q(r,e),i&&q(i,e),a&&q(a,e),m.value=e}function E(e){let n=Array.from(v.value),r=n.findIndex(t=>t===e);if(~r)n.splice(r,1);else{if(t.accordion&&u.value.has(e)){let e=n.findIndex(e=>u.value.has(e));e>-1&&n.splice(e,1)}n.push(e)}T(n)}let D=e=>{let n=l.value.getPath(e??p.value,{includeSelf:!1}).keyPath;if(!n.length)return;let r=Array.from(v.value),i=new Set([...r,...n]);t.accordion&&u.value.forEach(e=>{i.has(e)&&!n.includes(e)&&i.delete(e)}),T(Array.from(i))},O=S(()=>{let{inverted:e}=t,{common:{cubicBezierEaseInOut:n},self:r}=a.value,{borderRadius:i,borderColorHorizontal:o,fontSize:s,itemHeight:c,dividerColor:l}=r,u={"--n-divider-color":l,"--n-bezier":n,"--n-font-size":s,"--n-border-color-horizontal":o,"--n-border-radius":i,"--n-item-height":c};return e?(u[`--n-group-text-color`]=r.groupTextColorInverted,u[`--n-color`]=r.colorInverted,u[`--n-item-text-color`]=r.itemTextColorInverted,u[`--n-item-text-color-hover`]=r.itemTextColorHoverInverted,u[`--n-item-text-color-active`]=r.itemTextColorActiveInverted,u[`--n-item-text-color-child-active`]=r.itemTextColorChildActiveInverted,u[`--n-item-text-color-child-active-hover`]=r.itemTextColorChildActiveInverted,u[`--n-item-text-color-active-hover`]=r.itemTextColorActiveHoverInverted,u[`--n-item-icon-color`]=r.itemIconColorInverted,u[`--n-item-icon-color-hover`]=r.itemIconColorHoverInverted,u[`--n-item-icon-color-active`]=r.itemIconColorActiveInverted,u[`--n-item-icon-color-active-hover`]=r.itemIconColorActiveHoverInverted,u[`--n-item-icon-color-child-active`]=r.itemIconColorChildActiveInverted,u[`--n-item-icon-color-child-active-hover`]=r.itemIconColorChildActiveHoverInverted,u[`--n-item-icon-color-collapsed`]=r.itemIconColorCollapsedInverted,u[`--n-item-text-color-horizontal`]=r.itemTextColorHorizontalInverted,u[`--n-item-text-color-hover-horizontal`]=r.itemTextColorHoverHorizontalInverted,u[`--n-item-text-color-active-horizontal`]=r.itemTextColorActiveHorizontalInverted,u[`--n-item-text-color-child-active-horizontal`]=r.itemTextColorChildActiveHorizontalInverted,u[`--n-item-text-color-child-active-hover-horizontal`]=r.itemTextColorChildActiveHoverHorizontalInverted,u[`--n-item-text-color-active-hover-horizontal`]=r.itemTextColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-horizontal`]=r.itemIconColorHorizontalInverted,u[`--n-item-icon-color-hover-horizontal`]=r.itemIconColorHoverHorizontalInverted,u[`--n-item-icon-color-active-horizontal`]=r.itemIconColorActiveHorizontalInverted,u[`--n-item-icon-color-active-hover-horizontal`]=r.itemIconColorActiveHoverHorizontalInverted,u[`--n-item-icon-color-child-active-horizontal`]=r.itemIconColorChildActiveHorizontalInverted,u[`--n-item-icon-color-child-active-hover-horizontal`]=r.itemIconColorChildActiveHoverHorizontalInverted,u[`--n-arrow-color`]=r.arrowColorInverted,u[`--n-arrow-color-hover`]=r.arrowColorHoverInverted,u[`--n-arrow-color-active`]=r.arrowColorActiveInverted,u[`--n-arrow-color-active-hover`]=r.arrowColorActiveHoverInverted,u[`--n-arrow-color-child-active`]=r.arrowColorChildActiveInverted,u[`--n-arrow-color-child-active-hover`]=r.arrowColorChildActiveHoverInverted,u[`--n-item-color-hover`]=r.itemColorHoverInverted,u[`--n-item-color-active`]=r.itemColorActiveInverted,u[`--n-item-color-active-hover`]=r.itemColorActiveHoverInverted,u[`--n-item-color-active-collapsed`]=r.itemColorActiveCollapsedInverted):(u[`--n-group-text-color`]=r.groupTextColor,u[`--n-color`]=r.color,u[`--n-item-text-color`]=r.itemTextColor,u[`--n-item-text-color-hover`]=r.itemTextColorHover,u[`--n-item-text-color-active`]=r.itemTextColorActive,u[`--n-item-text-color-child-active`]=r.itemTextColorChildActive,u[`--n-item-text-color-child-active-hover`]=r.itemTextColorChildActiveHover,u[`--n-item-text-color-active-hover`]=r.itemTextColorActiveHover,u[`--n-item-icon-color`]=r.itemIconColor,u[`--n-item-icon-color-hover`]=r.itemIconColorHover,u[`--n-item-icon-color-active`]=r.itemIconColorActive,u[`--n-item-icon-color-active-hover`]=r.itemIconColorActiveHover,u[`--n-item-icon-color-child-active`]=r.itemIconColorChildActive,u[`--n-item-icon-color-child-active-hover`]=r.itemIconColorChildActiveHover,u[`--n-item-icon-color-collapsed`]=r.itemIconColorCollapsed,u[`--n-item-text-color-horizontal`]=r.itemTextColorHorizontal,u[`--n-item-text-color-hover-horizontal`]=r.itemTextColorHoverHorizontal,u[`--n-item-text-color-active-horizontal`]=r.itemTextColorActiveHorizontal,u[`--n-item-text-color-child-active-horizontal`]=r.itemTextColorChildActiveHorizontal,u[`--n-item-text-color-child-active-hover-horizontal`]=r.itemTextColorChildActiveHoverHorizontal,u[`--n-item-text-color-active-hover-horizontal`]=r.itemTextColorActiveHoverHorizontal,u[`--n-item-icon-color-horizontal`]=r.itemIconColorHorizontal,u[`--n-item-icon-color-hover-horizontal`]=r.itemIconColorHoverHorizontal,u[`--n-item-icon-color-active-horizontal`]=r.itemIconColorActiveHorizontal,u[`--n-item-icon-color-active-hover-horizontal`]=r.itemIconColorActiveHoverHorizontal,u[`--n-item-icon-color-child-active-horizontal`]=r.itemIconColorChildActiveHorizontal,u[`--n-item-icon-color-child-active-hover-horizontal`]=r.itemIconColorChildActiveHoverHorizontal,u[`--n-arrow-color`]=r.arrowColor,u[`--n-arrow-color-hover`]=r.arrowColorHover,u[`--n-arrow-color-active`]=r.arrowColorActive,u[`--n-arrow-color-active-hover`]=r.arrowColorActiveHover,u[`--n-arrow-color-child-active`]=r.arrowColorChildActive,u[`--n-arrow-color-child-active-hover`]=r.arrowColorChildActiveHover,u[`--n-item-color-hover`]=r.itemColorHover,u[`--n-item-color-active`]=r.itemColorActive,u[`--n-item-color-active-hover`]=r.itemColorActiveHover,u[`--n-item-color-active-collapsed`]=r.itemColorActiveCollapsed),u}),k=r?K(`menu`,S(()=>t.inverted?`a`:`b`),O,t):void 0,A=fe(),j=w(null),M=w(null),N=!0,P=()=>{var e;N?N=!1:(e=j.value)==null||e.sync({showAllItemsBeforeCalculate:!0})};function F(){return document.getElementById(A)}let I=w(-1);function L(e){I.value=t.options.length-e}function R(e){e||(I.value=-1)}let z=S(()=>{let e=I.value;return{children:e===-1?[]:t.options.slice(e)}}),B=S(()=>{let{childrenField:e,disabledField:n,keyField:r}=t;return Se([z.value],{getIgnored(e){return Zt(e)},getChildren(t){return t[e]},getDisabled(e){return e[n]},getKey(e){return e[r]??e.name}})}),V=S(()=>Se([{}]).treeNodes[0]);function ee(){if(I.value===-1)return s(nn,{root:!0,level:0,key:`__ellpisisGroupPlaceholder__`,internalKey:`__ellpisisGroupPlaceholder__`,title:`···`,tmNode:V.value,domId:A,isEllipsisPlaceholder:!0});let e=B.value.treeNodes[0],t=b.value;return s(nn,{level:0,root:!0,key:`__ellpisisGroup__`,internalKey:`__ellpisisGroup__`,title:`···`,virtualChildActive:!!e.children?.some(e=>t.includes(e.key)),tmNode:e,domId:A,rawNodes:e.rawNode.children||[],tmNodes:e.children||[],isEllipsisPlaceholder:!0})}return{mergedClsPrefix:n,controlledExpandedKeys:_,uncontrolledExpanededKeys:m,mergedExpandedKeys:v,uncontrolledValue:f,mergedValue:p,activePath:b,tmNodes:y,mergedTheme:a,mergedCollapsed:c,cssVars:r?void 0:O,themeClass:k?.themeClass,overflowRef:j,counterRef:M,updateCounter:()=>{},onResize:P,onUpdateOverflow:R,onUpdateCount:L,renderCounter:ee,getCounter:F,onRender:k?.onRender,showOption:D,deriveResponsiveState:P}},render(){let{mergedClsPrefix:e,mode:t,themeClass:n,onRender:r}=this;r?.();let i=()=>this.tmNodes.map(e=>$t(e,this.$props)),a=t===`horizontal`&&this.responsive,c=()=>s(`div`,o(this.$attrs,{role:t===`horizontal`?`menubar`:`menu`,class:[`${e}-menu`,n,`${e}-menu--${t}`,a&&`${e}-menu--responsive`,this.mergedCollapsed&&`${e}-menu--collapsed`],style:this.cssVars}),a?s(pe,{ref:`overflowRef`,onUpdateOverflow:this.onUpdateOverflow,getCounter:this.getCounter,onUpdateCount:this.onUpdateCount,updateCounter:this.updateCounter,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:i,counter:this.renderCounter}):i());return a?s(me,{onResize:this.onResize},{default:c}):c()}}),an={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},on=f({name:`CloudDownloadOutline`,render:function(e,t){return m(),y(`svg`,an,t[0]||=[v(`path`,{d:`M320 336h76c55 0 100-21.21 100-75.6s-53-73.47-96-75.6C391.11 99.74 329 48 256 48c-69 0-113.44 45.79-128 91.2c-60 5.7-112 35.88-112 98.4S70 336 136 336h56`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 400.1l64 63.9l64-63.9`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M256 224v224.03`},null,-1)])}}),sn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},cn=f({name:`DocumentTextOutline`,render:function(e,t){return m(),y(`svg`,sn,t[0]||=[v(`path`,{d:`M416 221.25V416a48 48 0 0 1-48 48H144a48 48 0 0 1-48-48V96a48 48 0 0 1 48-48h98.75a32 32 0 0 1 22.62 9.37l141.26 141.26a32 32 0 0 1 9.37 22.62z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M256 56v120a32 32 0 0 0 32 32h120`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 288h160`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 368h160`},null,-1)])}}),ln={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},un=f({name:`HomeOutline`,render:function(e,t){return m(),y(`svg`,ln,t[0]||=[v(`path`,{d:`M80 212v236a16 16 0 0 0 16 16h96V328a24 24 0 0 1 24-24h80a24 24 0 0 1 24 24v136h96a16 16 0 0 0 16-16V212`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M480 256L266.89 52c-5-5.28-16.69-5.34-21.78 0L32 256`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M400 179V64h-48v69`},null,-1)])}}),dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},fn=f({name:`KeyOutline`,render:function(e,t){return m(),y(`svg`,dn,t[0]||=[v(`path`,{d:`M218.1 167.17c0 13 0 25.6 4.1 37.4c-43.1 50.6-156.9 184.3-167.5 194.5a20.17 20.17 0 0 0-6.7 15c0 8.5 5.2 16.7 9.6 21.3c6.6 6.9 34.8 33 40 28c15.4-15 18.5-19 24.8-25.2c9.5-9.3-1-28.3 2.3-36s6.8-9.2 12.5-10.4s15.8 2.9 23.7 3c8.3.1 12.8-3.4 19-9.2c5-4.6 8.6-8.9 8.7-15.6c.2-9-12.8-20.9-3.1-30.4s23.7 6.2 34 5s22.8-15.5 24.1-21.6s-11.7-21.8-9.7-30.7c.7-3 6.8-10 11.4-11s25 6.9 29.6 5.9c5.6-1.2 12.1-7.1 17.4-10.4c15.5 6.7 29.6 9.4 47.7 9.4c68.5 0 124-53.4 124-119.2S408.5 48 340 48s-121.9 53.37-121.9 119.17zM400 144a32 32 0 1 1-32-32a32 32 0 0 1 32 32z`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),pn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},mn=f({name:`LanguageOutline`,render:function(e,t){return m(),y(`svg`,pn,t[0]||=[d(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M48 112h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M192 64v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M272 448l96-224l96 224"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M301.5 384h133"></path><path d="M281.3 112S257 206 199 277S80 384 80 384" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path><path d="M256 336s-35-27-72-75s-56-85-56-85" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></path>`,6)])}}),hn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},gn=f({name:`LayersOutline`,render:function(e,t){return m(),y(`svg`,hn,t[0]||=[v(`path`,{d:`M434.8 137.65l-149.36-68.1c-16.19-7.4-42.69-7.4-58.88 0L77.3 137.65c-17.6 8-17.6 21.09 0 29.09l148 67.5c16.89 7.7 44.69 7.7 61.58 0l148-67.5c17.52-8 17.52-21.1-.08-29.09z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M160 308.52l-82.7 37.11c-17.6 8-17.6 21.1 0 29.1l148 67.5c16.89 7.69 44.69 7.69 61.58 0l148-67.5c17.6-8 17.6-21.1 0-29.1l-79.94-38.47`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M160 204.48l-82.8 37.16c-17.6 8-17.6 21.1 0 29.1l148 67.49c16.89 7.7 44.69 7.7 61.58 0l148-67.49c17.7-8 17.7-21.1.1-29.1L352 204.48`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),_n={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},vn=f({name:`ListOutline`,render:function(e,t){return m(),y(`svg`,_n,t[0]||=[d(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 144h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 256h288"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32" d="M160 368h288"></path><circle cx="80" cy="144" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="256" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle><circle cx="80" cy="368" r="16" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="32"></circle>`,6)])}}),yn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},bn=f({name:`LogOutOutline`,render:function(e,t){return m(),y(`svg`,yn,t[0]||=[v(`path`,{d:`M304 336v40a40 40 0 0 1-40 40H104a40 40 0 0 1-40-40V136a40 40 0 0 1 40-40h152c22.09 0 48 17.91 48 40v40`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M368 336l80-80l-80-80`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M176 256h256`},null,-1)])}}),xn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Sn=f({name:`MenuOutline`,render:function(e,t){return m(),y(`svg`,xn,t[0]||=[v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 160h352`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 256h352`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`,d:`M80 352h352`},null,-1)])}}),Cn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},wn=f({name:`MoonOutline`,render:function(e,t){return m(),y(`svg`,Cn,t[0]||=[v(`path`,{d:`M160 136c0-30.62 4.51-61.61 16-88C99.57 81.27 48 159.32 48 248c0 119.29 96.71 216 216 216c88.68 0 166.73-51.57 200-128c-26.39 11.49-57.38 16-88 16c-119.29 0-216-96.71-216-216z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),Tn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},En=f({name:`PeopleOutline`,render:function(e,t){return m(),y(`svg`,Tn,t[0]||=[v(`path`,{d:`M402 168c-2.93 40.67-33.1 72-66 72s-63.12-31.32-66-72c-3-42.31 26.37-72 66-72s69 30.46 66 72z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M336 304c-65.17 0-127.84 32.37-143.54 95.41c-2.08 8.34 3.15 16.59 11.72 16.59h263.65c8.57 0 13.77-8.25 11.72-16.59C463.85 335.36 401.18 304 336 304z`,fill:`none`,stroke:`currentColor`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1),v(`path`,{d:`M200 185.94c-2.34 32.48-26.72 58.06-53 58.06s-50.7-25.57-53-58.06C91.61 152.15 115.34 128 147 128s55.39 24.77 53 57.94z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{d:`M206 306c-18.05-8.27-37.93-11.45-59-11.45c-52 0-102.1 25.85-114.65 76.2c-1.65 6.66 2.53 13.25 9.37 13.25H154`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-miterlimit":`10`,"stroke-width":`32`},null,-1)])}}),Dn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},On=f({name:`SettingsOutline`,render:function(e,t){return m(),y(`svg`,Dn,t[0]||=[v(`path`,{d:`M262.29 192.31a64 64 0 1 0 57.4 57.4a64.13 64.13 0 0 0-57.4-57.4zM416.39 256a154.34 154.34 0 0 1-1.53 20.79l45.21 35.46a10.81 10.81 0 0 1 2.45 13.75l-42.77 74a10.81 10.81 0 0 1-13.14 4.59l-44.9-18.08a16.11 16.11 0 0 0-15.17 1.75A164.48 164.48 0 0 1 325 400.8a15.94 15.94 0 0 0-8.82 12.14l-6.73 47.89a11.08 11.08 0 0 1-10.68 9.17h-85.54a11.11 11.11 0 0 1-10.69-8.87l-6.72-47.82a16.07 16.07 0 0 0-9-12.22a155.3 155.3 0 0 1-21.46-12.57a16 16 0 0 0-15.11-1.71l-44.89 18.07a10.81 10.81 0 0 1-13.14-4.58l-42.77-74a10.8 10.8 0 0 1 2.45-13.75l38.21-30a16.05 16.05 0 0 0 6-14.08c-.36-4.17-.58-8.33-.58-12.5s.21-8.27.58-12.35a16 16 0 0 0-6.07-13.94l-38.19-30A10.81 10.81 0 0 1 49.48 186l42.77-74a10.81 10.81 0 0 1 13.14-4.59l44.9 18.08a16.11 16.11 0 0 0 15.17-1.75A164.48 164.48 0 0 1 187 111.2a15.94 15.94 0 0 0 8.82-12.14l6.73-47.89A11.08 11.08 0 0 1 213.23 42h85.54a11.11 11.11 0 0 1 10.69 8.87l6.72 47.82a16.07 16.07 0 0 0 9 12.22a155.3 155.3 0 0 1 21.46 12.57a16 16 0 0 0 15.11 1.71l44.89-18.07a10.81 10.81 0 0 1 13.14 4.58l42.77 74a10.8 10.8 0 0 1-2.45 13.75l-38.21 30a16.05 16.05 0 0 0-6.05 14.08c.33 4.14.55 8.3.55 12.47z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),kn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},An=f({name:`ShieldCheckmarkOutline`,render:function(e,t){return m(),y(`svg`,kn,t[0]||=[v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M336 176L225.2 304L176 255.8`},null,-1),v(`path`,{d:`M463.1 112.37C373.68 96.33 336.71 84.45 256 48c-80.71 36.45-117.68 48.33-207.1 64.37C32.7 369.13 240.58 457.79 256 464c15.42-6.21 223.3-94.87 207.1-351.63z`,fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1)])}}),jn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Mn=f({name:`SunnyOutline`,render:function(e,t){return m(),y(`svg`,jn,t[0]||=[d(`<path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 48v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M256 416v48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 108.92l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 369.14l-33.94 33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M464 256h-48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M96 256H48"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M403.08 403.08l-33.94-33.94"></path><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32" d="M142.86 142.86l-33.94-33.94"></path><circle cx="256" cy="256" r="80" fill="none" stroke="currentColor" stroke-linecap="round" stroke-miterlimit="10" stroke-width="32"></circle>`,9)])}}),Nn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},Pn=f({name:`SwapHorizontalOutline`,render:function(e,t){return m(),y(`svg`,Nn,t[0]||=[v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M304 48l112 112l-112 112`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M398.87 160H96`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M208 464L96 352l112-112`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M114 352h302`},null,-1)])}}),Fn={xmlns:`http://www.w3.org/2000/svg`,"xmlns:xlink":`http://www.w3.org/1999/xlink`,viewBox:`0 0 512 512`},In=f({name:`TerminalOutline`,render:function(e,t){return m(),y(`svg`,Fn,t[0]||=[v(`rect`,{x:`32`,y:`48`,width:`448`,height:`416`,rx:`48`,ry:`48`,fill:`none`,stroke:`currentColor`,"stroke-linejoin":`round`,"stroke-width":`32`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M96 112l80 64l-80 64`},null,-1),v(`path`,{fill:`none`,stroke:`currentColor`,"stroke-linecap":`round`,"stroke-linejoin":`round`,"stroke-width":`32`,d:`M192 240h64`},null,-1)])}}),Ln={key:0,class:`logo-text`},Rn={class:`header-left`},zn={class:`header-actions`},Bn={key:0},Vn={class:`content-container`},Hn=Je(f({__name:`AppLayout`,setup(e){let i=ge(),o=ye(),{t:l}=ve(),d=ce(),f=Be(),h=S({get:()=>f.sidebarCollapsed,set:e=>{f.sidebarCollapsed=e}}),g=w(!1),x=w(!1),O=S(()=>f.darkMode);function k(){g.value=window.innerWidth<900,g.value||(x.value=!1)}u(()=>{k(),window.addEventListener(`resize`,k)}),a(()=>window.removeEventListener(`resize`,k));function A(){f.toggleDarkMode()}let j=S(()=>d.user?.display_name||d.user?.username||``),M=S(()=>{let e=o.path,t=I.flatMap(e=>[e,...e.children??[]]).map(e=>String(e.key)).filter(e=>e.startsWith(`/`)).sort((e,t)=>t.length-e.length);for(let n of t)if(e.startsWith(n))return n;return`/`}),N=S(()=>{let e=[{label:`GoDDI`,path:`/`}],t=o.path;return t.startsWith(`/dns`)?(e.push({label:l(`nav.dns`),path:`/dns`}),t.includes(`/zones`)&&e.push({label:l(`nav.dnsZones`),path:`/dns/zones`}),t.includes(`/forwarders`)&&e.push({label:l(`nav.dnsForwarders`),path:`/dns/forwarders`}),t.includes(`/security`)&&e.push({label:l(`nav.dnsSecurity`),path:`/dns/security`}),t.includes(`/cache`)&&e.push({label:l(`nav.dnsCache`),path:`/dns/cache`}),t.includes(`/client`)&&e.push({label:l(`nav.dnsClient`),path:`/dns/client`})):t.startsWith(`/dhcp`)?(e.push({label:l(`nav.dhcp`),path:`/dhcp`}),t.includes(`/scopes`)&&e.push({label:l(`nav.dhcpScopes`),path:`/dhcp/scopes`}),t.includes(`/leases`)&&e.push({label:l(`nav.dhcpLeases`),path:`/dhcp/leases`}),t.includes(`/reservations`)&&e.push({label:l(`nav.dhcpReservations`),path:`/dhcp/reservations`}),t.includes(`/options`)&&e.push({label:l(`nav.dhcpOptions`),path:`/dhcp/options`})):t.startsWith(`/ipam`)?(e.push({label:l(`nav.ipam`),path:`/ipam`}),t.includes(`/spaces`)&&e.push({label:l(`nav.ipamSpaces`),path:`/ipam/spaces`}),t.includes(`/subnets`)&&e.push({label:l(`nav.ipamSubnets`),path:`/ipam/subnets`}),t.includes(`/addresses`)&&e.push({label:l(`nav.ipamAddresses`),path:`/ipam/addresses`})):t.startsWith(`/admin`)?(e.push({label:l(`nav.admin`),path:`/admin`}),t.includes(`/users`)&&e.push({label:l(`nav.adminUsers`),path:`/admin/users`}),t.includes(`/roles`)&&e.push({label:l(`nav.adminRoles`),path:`/admin/roles`}),t.includes(`/groups`)&&e.push({label:l(`nav.adminGroups`),path:`/admin/groups`}),t.includes(`/tokens`)&&e.push({label:l(`nav.adminTokens`),path:`/admin/tokens`})):t.startsWith(`/logs`)?(e.push({label:l(`nav.logs`),path:`/logs`}),t.includes(`/audit`)&&e.push({label:l(`nav.logsAudit`),path:`/logs/audit`}),t.includes(`/dns`)&&e.push({label:l(`nav.logsDns`),path:`/logs/dns`}),t.includes(`/dhcp`)&&e.push({label:l(`nav.logsDhcp`),path:`/logs/dhcp`})):t.startsWith(`/settings`)&&(e.push({label:l(`nav.settings`),path:`/settings`}),t.includes(`/backup`)&&e.push({label:l(`nav.settingsBackup`),path:`/settings/backup`})),e});function P(e){return()=>s(X,null,{default:()=>s(e)})}function F(e){let t=e.label;return typeof t==`function`?t():t||``}let I=[{key:`/`,label:()=>l(`nav.dashboard`),icon:P(un)},{key:`/dns`,label:()=>l(`nav.dns`),icon:P(We),children:[{key:`/dns/zones`,label:()=>l(`nav.dnsZones`),icon:P(gn)},{key:`/dns/forwarders`,label:()=>l(`nav.dnsForwarders`),icon:P(Pn)},{key:`/dns/security`,label:()=>l(`nav.dnsSecurity`),icon:P(An)},{key:`/dns/cache`,label:()=>l(`nav.dnsCache`),icon:P(Ge)},{key:`/dns/client`,label:()=>l(`nav.dnsClient`),icon:P(In)}]},{key:`/dhcp`,label:()=>l(`nav.dhcp`),icon:P(He),children:[{key:`/dhcp/scopes`,label:()=>l(`nav.dhcpScopes`),icon:P(Ue)},{key:`/dhcp/leases`,label:()=>l(`nav.dhcpLeases`),icon:P(cn)},{key:`/dhcp/reservations`,label:()=>l(`nav.dhcpReservations`),icon:P(vn)},{key:`/dhcp/options`,label:()=>l(`nav.dhcpOptions`),icon:P(On)}]},{key:`/ipam`,label:()=>l(`nav.ipam`),icon:P(Ge),children:[{key:`/ipam/spaces`,label:()=>l(`nav.ipamSpaces`),icon:P(gn)},{key:`/ipam/subnets`,label:()=>l(`nav.ipamSubnets`),icon:P(Ue)},{key:`/ipam/addresses`,label:()=>l(`nav.ipamAddresses`),icon:P(vn)}]},{key:`/admin`,label:()=>l(`nav.admin`),icon:P(En),children:[{key:`/admin/users`,label:()=>l(`nav.adminUsers`),icon:P(Ke)},{key:`/admin/roles`,label:()=>l(`nav.adminRoles`),icon:P(fn)},{key:`/admin/groups`,label:()=>l(`nav.adminGroups`),icon:P(En)},{key:`/admin/tokens`,label:()=>l(`nav.adminTokens`),icon:P(fn)}]},{key:`/logs`,label:()=>l(`nav.logs`),icon:P(qe),children:[{key:`/logs/audit`,label:()=>l(`nav.logsAudit`),icon:P(cn)},{key:`/logs/dns`,label:()=>l(`nav.logsDns`),icon:P(We)},{key:`/logs/dhcp`,label:()=>l(`nav.logsDhcp`),icon:P(He)}]},{key:`settings-group`,label:()=>l(`nav.settings`),icon:P(On),children:[{key:`/settings`,label:()=>l(`settings.title`),icon:P(On)},{key:`/settings/backup`,label:()=>l(`nav.settingsBackup`),icon:P(on)}]}],L={"/dns":{resource:`dns`,action:`read`},"/dhcp":{resource:`dhcp`,action:`read`},"/ipam":{resource:`ipam`,action:`read`},"/admin/users":{resource:`user`,action:`read`},"/admin/roles":{resource:`role`,action:`read`},"/admin/groups":{resource:`group`,action:`read`},"/admin/tokens":{resource:`token`,action:`read`},"/logs/audit":{resource:`audit`,action:`read`},"/logs/dns":{resource:`dns`,action:`read`},"/logs/dhcp":{resource:`dhcp`,action:`read`},"/settings":{resource:`settings`,action:`read`},"/settings/backup":{resource:`backup`,action:`read`}};function R(e){let t=L[e];return!t||d.hasPermission(t.resource,t.action)}let z=S(()=>I.flatMap(e=>{let t=String(e.key);if(e.children){let t=e.children.filter(e=>R(String(e.key)));return t.length===0?[]:[{...e,children:t}]}return R(t)?[e]:[]}));function B(e){x.value=!1,i.push(e)}let V=S(()=>[{label:`English`,key:`en-US`,disabled:f.locale===`en-US`},{label:`简体中文`,key:`zh-CN`,disabled:f.locale===`zh-CN`}]);function ee(e){f.setLocale(e)}let te=[{label:()=>l(`auth.logout`),key:`logout`,icon:P(bn)}];async function ne(e){e===`logout`&&(await d.logout(),i.push(`/login`))}return(e,a)=>{let o=rn,s=Pt,u=gt,d=ht,f=se,S=et,w=Qe,k=Te,P=De,I=kt,L=r(`router-view`),R=Et,re=Tt;return m(),T(re,{"has-sider":``,class:`app-shell`},{default:_(()=>[g.value?D(``,!0):(m(),T(s,{key:0,bordered:``,"collapse-mode":`width`,"collapsed-width":64,width:240,collapsed:h.value,"show-trigger":``,onCollapse:a[0]||=e=>h.value=!0,onExpand:a[1]||=e=>h.value=!1,"native-scrollbar":!1,class:`desktop-sider`},{default:_(()=>[v(`div`,{class:n([`logo`,{"logo-collapsed":h.value}])},[a[4]||=v(`span`,{class:`logo-icon`},`G`,-1),h.value?D(``,!0):(m(),y(`span`,Ln,`GoDDI`))],2),E(o,{collapsed:h.value,"collapsed-width":64,"collapsed-icon-size":22,options:z.value,value:M.value,"onUpdate:value":B,"render-label":F},null,8,[`collapsed`,`options`,`value`,`render-label`])]),_:1},8,[`collapsed`])),E(d,{show:x.value,"onUpdate:show":a[2]||=e=>x.value=e,placement:`left`,width:280},{default:_(()=>[E(u,{"body-content-style":`padding: 0;`,closable:``},{default:_(()=>[a[5]||=v(`div`,{class:`logo mobile-logo`},[v(`span`,{class:`logo-icon`},`G`),v(`span`,{class:`logo-text`},`GoDDI`)],-1),E(o,{options:z.value,value:M.value,"onUpdate:value":B,"render-label":F},null,8,[`options`,`value`,`render-label`])]),_:1})]),_:1},8,[`show`]),E(re,{class:`main-layout`},{default:_(()=>[E(I,{bordered:``,class:`app-header`},{default:_(()=>[v(`div`,Rn,[g.value?(m(),T(f,{key:0,quaternary:``,circle:``,"aria-label":`Open navigation`,onClick:a[3]||=e=>x.value=!0},{icon:_(()=>[E(C(X),null,{default:_(()=>[E(C(Sn))]),_:1})]),_:1})):D(``,!0),E(w,null,{default:_(()=>[(m(!0),y(b,null,p(N.value,e=>(m(),T(S,{key:e.path,class:`breadcrumb-item`,onClick:t=>C(i).push(e.path)},{default:_(()=>[t(c(e.label),1)]),_:2},1032,[`onClick`]))),128))]),_:1})]),v(`div`,zn,[E(k,{options:V.value,onSelect:ee},{default:_(()=>[E(f,{quaternary:``,circle:``,"aria-label":C(l)(`common.language`)},{icon:_(()=>[E(C(X),null,{default:_(()=>[E(C(mn))]),_:1})]),_:1},8,[`aria-label`])]),_:1},8,[`options`]),E(P,{size:`small`,value:O.value,"onUpdate:value":A},{checked:_(()=>[E(C(X),null,{default:_(()=>[E(C(Mn))]),_:1})]),unchecked:_(()=>[E(C(X),null,{default:_(()=>[E(C(wn))]),_:1})]),_:1},8,[`value`]),E(k,{options:te,onSelect:ne},{default:_(()=>[E(f,{quaternary:``},{icon:_(()=>[E(C(X),null,{default:_(()=>[E(C(Ke))]),_:1})]),default:_(()=>[g.value?D(``,!0):(m(),y(`span`,Bn,c(j.value),1))]),_:1})]),_:1})])]),_:1}),E(R,{"content-style":g.value?`padding: 16px;`:`padding: 24px 28px;`,"native-scrollbar":!1,class:n([`app-content`,{"app-content-dark":O.value}])},{default:_(()=>[v(`div`,Vn,[E(L)])]),_:1},8,[`content-style`,`class`])]),_:1})]),_:1})}}}),[[`__scopeId`,`data-v-e56222aa`]]);export{Hn as default};