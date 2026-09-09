import{E as e,P as t,g as n,k as r,pt as i}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{$t as a,Ar as o,Dn as s,Gt as c,Nn as l,Nr as u,On as d,Or as f,Sr as p,Xt as m,Zt as h,_r as g,cn as _,en as v,gr as y,jn as b,jr as x,kr as S,nn as C,pn as w,tn as T,un as E}from"./router-mKxr9Z2z.js";import{_ as D,v as O}from"./index-Dcl5rJ-i.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:o,textColor1:s,textColor2:c,closeColorHover:l,closeColorPressed:u,closeIconColor:d,closeIconColorHover:f,closeIconColorPressed:p,infoColor:m,successColor:h,warningColor:_,errorColor:v,fontSize:b}=e;return Object.assign(Object.assign({},O),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:o,titleTextColor:s,iconColor:c,contentTextColor:c,closeBorderRadius:n,closeColorHover:l,closeColorPressed:u,closeIconColor:d,closeIconColorHover:f,closeIconColorPressed:p,borderInfo:`1px solid ${g(i,y(m,{alpha:.25}))}`,colorInfo:g(i,y(m,{alpha:.08})),titleTextColorInfo:s,iconColorInfo:m,contentTextColorInfo:c,closeColorHoverInfo:l,closeColorPressedInfo:u,closeIconColorInfo:d,closeIconColorHoverInfo:f,closeIconColorPressedInfo:p,borderSuccess:`1px solid ${g(i,y(h,{alpha:.25}))}`,colorSuccess:g(i,y(h,{alpha:.08})),titleTextColorSuccess:s,iconColorSuccess:h,contentTextColorSuccess:c,closeColorHoverSuccess:l,closeColorPressedSuccess:u,closeIconColorSuccess:d,closeIconColorHoverSuccess:f,closeIconColorPressedSuccess:p,borderWarning:`1px solid ${g(i,y(_,{alpha:.33}))}`,colorWarning:g(i,y(_,{alpha:.08})),titleTextColorWarning:s,iconColorWarning:_,contentTextColorWarning:c,closeColorHoverWarning:l,closeColorPressedWarning:u,closeIconColorWarning:d,closeIconColorHoverWarning:f,closeIconColorPressedWarning:p,borderError:`1px solid ${g(i,y(v,{alpha:.25}))}`,colorError:g(i,y(v,{alpha:.08})),titleTextColorError:s,iconColorError:v,contentTextColorError:c,closeColorHoverError:l,closeColorPressedError:u,closeIconColorError:d,closeIconColorHoverError:f,closeIconColorPressedError:p})}var A={name:`Alert`,common:c,self:k},j=S(`alert`,`
 line-height: var(--n-line-height);
 border-radius: var(--n-border-radius);
 position: relative;
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 text-align: start;
 word-break: break-word;
`,[o(`border`,`
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 transition: border-color .3s var(--n-bezier);
 border: var(--n-border);
 pointer-events: none;
 `),x(`closable`,[S(`alert-body`,[o(`title`,`
 padding-right: 24px;
 `)])]),o(`icon`,{color:`var(--n-icon-color)`}),S(`alert-body`,{padding:`var(--n-padding)`},[o(`title`,{color:`var(--n-title-text-color)`}),o(`content`,{color:`var(--n-content-text-color)`})]),D({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),o(`icon`,`
 position: absolute;
 left: 0;
 top: 0;
 align-items: center;
 justify-content: center;
 display: flex;
 width: var(--n-icon-size);
 height: var(--n-icon-size);
 font-size: var(--n-icon-size);
 margin: var(--n-icon-margin);
 `),o(`close`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 position: absolute;
 right: 0;
 top: 0;
 margin: var(--n-close-margin);
 `),x(`show-icon`,[S(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),x(`right-adjust`,[S(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),S(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[o(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[f(`& +`,[o(`content`,{marginTop:`9px`})])]),o(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),o(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=e({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},E.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:r,inlineThemeDisabled:a,mergedRtlRef:o}=d(e),c=E(`Alert`,`-alert`,j,A,e,t),l=w(`Alert`,o,t),f=n(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=c.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:s,iconMargin:l,iconMarginRtl:d,closeIconSize:f,closeBorderRadius:m,closeSize:h,closeMargin:g,closeMarginRtl:_,padding:v}=n,{type:y}=e,{left:b,right:x}=p(l);return{"--n-bezier":t,"--n-color":n[u(`color`,y)],"--n-close-icon-size":f,"--n-close-border-radius":m,"--n-close-color-hover":n[u(`closeColorHover`,y)],"--n-close-color-pressed":n[u(`closeColorPressed`,y)],"--n-close-icon-color":n[u(`closeIconColor`,y)],"--n-close-icon-color-hover":n[u(`closeIconColorHover`,y)],"--n-close-icon-color-pressed":n[u(`closeIconColorPressed`,y)],"--n-icon-color":n[u(`iconColor`,y)],"--n-border":n[u(`border`,y)],"--n-title-text-color":n[u(`titleTextColor`,y)],"--n-content-text-color":n[u(`contentTextColor`,y)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":s,"--n-icon-margin":l,"--n-icon-margin-rtl":d,"--n-close-size":h,"--n-close-margin":g,"--n-close-margin-rtl":_,"--n-padding":v,"--n-icon-margin-left":b,"--n-icon-margin-right":x}}),m=a?s(`alert`,n(()=>e.type[0]),f,e):void 0,h=i(!0),g=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:l,mergedClsPrefix:t,mergedBordered:r,visible:h,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(h.value=!1)})},handleAfterLeave:()=>{g()},mergedTheme:c,cssVars:a?void 0:f,themeClass:m?.themeClass,onRender:m?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),r(m,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:e,$slots:n}=this,i={class:[`${e}-alert`,this.themeClass,this.closable&&`${e}-alert--closable`,this.showIcon&&`${e}-alert--show-icon`,!this.title&&this.closable&&`${e}-alert--right-adjust`,this.rtlEnabled&&`${e}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?r(`div`,Object.assign({},t(this.$attrs,i)),this.closable&&r(h,{clsPrefix:e,class:`${e}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&r(`div`,{class:`${e}-alert__border`}),this.showIcon&&r(`div`,{class:`${e}-alert__icon`,"aria-hidden":`true`},b(n.icon,()=>[r(_,{clsPrefix:e},{default:()=>{switch(this.type){case`success`:return r(v,null);case`info`:return r(T,null);case`warning`:return r(a,null);case`error`:return r(C,null);default:return null}}})])),r(`div`,{class:[`${e}-alert-body`,this.mergedBordered&&`${e}-alert-body--bordered`]},l(n.header,t=>{let n=t||this.title;return n?r(`div`,{class:`${e}-alert-body__title`},n):null}),n.default&&r(`div`,{class:`${e}-alert-body__content`},n))):null}})}});export{M as t};