import{E as e,N as t,ct as n,g as r,k as i}from"./echarts-Cw2yHLaZ.js";import{C as a,D as o,I as s,R as c,S as l}from"./auth-DWcZ46z-.js";import{A as u}from"./vue-core-BDTvi3xZ.js";import{B as d,G as f,J as p,K as m,N as h,P as g,V as _,Y as v,Z as y,i as b,n as x,t as S}from"./light-BluJl8SD.js";import{B as C,H as w,M as T,V as E,j as D,z as O}from"./index-lNh7vZn9.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:o,textColor1:s,textColor2:c,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,infoColor:h,successColor:g,warningColor:v,errorColor:y,fontSize:b}=e;return Object.assign(Object.assign({},T),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:o,titleTextColor:s,iconColor:c,contentTextColor:c,closeBorderRadius:n,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,borderInfo:`1px solid ${_(i,d(h,{alpha:.25}))}`,colorInfo:_(i,d(h,{alpha:.08})),titleTextColorInfo:s,iconColorInfo:h,contentTextColorInfo:c,closeColorHoverInfo:l,closeColorPressedInfo:u,closeIconColorInfo:f,closeIconColorHoverInfo:p,closeIconColorPressedInfo:m,borderSuccess:`1px solid ${_(i,d(g,{alpha:.25}))}`,colorSuccess:_(i,d(g,{alpha:.08})),titleTextColorSuccess:s,iconColorSuccess:g,contentTextColorSuccess:c,closeColorHoverSuccess:l,closeColorPressedSuccess:u,closeIconColorSuccess:f,closeIconColorHoverSuccess:p,closeIconColorPressedSuccess:m,borderWarning:`1px solid ${_(i,d(v,{alpha:.33}))}`,colorWarning:_(i,d(v,{alpha:.08})),titleTextColorWarning:s,iconColorWarning:v,contentTextColorWarning:c,closeColorHoverWarning:l,closeColorPressedWarning:u,closeIconColorWarning:f,closeIconColorHoverWarning:p,closeIconColorPressedWarning:m,borderError:`1px solid ${_(i,d(y,{alpha:.25}))}`,colorError:_(i,d(y,{alpha:.08})),titleTextColorError:s,iconColorError:y,contentTextColorError:c,closeColorHoverError:l,closeColorPressedError:u,closeIconColorError:f,closeIconColorHoverError:p,closeIconColorPressedError:m})}var A={name:`Alert`,common:S,self:k},j=m(`alert`,`
 line-height: var(--n-line-height);
 border-radius: var(--n-border-radius);
 position: relative;
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 text-align: start;
 word-break: break-word;
`,[p(`border`,`
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 transition: border-color .3s var(--n-bezier);
 border: var(--n-border);
 pointer-events: none;
 `),v(`closable`,[m(`alert-body`,[p(`title`,`
 padding-right: 24px;
 `)])]),p(`icon`,{color:`var(--n-icon-color)`}),m(`alert-body`,{padding:`var(--n-padding)`},[p(`title`,{color:`var(--n-title-text-color)`}),p(`content`,{color:`var(--n-content-text-color)`})]),D({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),p(`icon`,`
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
 `),p(`close`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 position: absolute;
 right: 0;
 top: 0;
 margin: var(--n-close-margin);
 `),v(`show-icon`,[m(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),v(`right-adjust`,[m(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),m(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[p(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[f(`& +`,[p(`content`,{marginTop:`9px`})])]),p(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),p(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=e({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},b.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:i,inlineThemeDisabled:a,mergedRtlRef:s}=g(e),c=b(`Alert`,`-alert`,j,A,e,t),l=o(`Alert`,s,t),d=r(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=c.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:s,iconMargin:l,iconMarginRtl:d,closeIconSize:f,closeBorderRadius:p,closeSize:m,closeMargin:h,closeMarginRtl:g,padding:_}=n,{type:v}=e,{left:b,right:x}=u(l);return{"--n-bezier":t,"--n-color":n[y(`color`,v)],"--n-close-icon-size":f,"--n-close-border-radius":p,"--n-close-color-hover":n[y(`closeColorHover`,v)],"--n-close-color-pressed":n[y(`closeColorPressed`,v)],"--n-close-icon-color":n[y(`closeIconColor`,v)],"--n-close-icon-color-hover":n[y(`closeIconColorHover`,v)],"--n-close-icon-color-pressed":n[y(`closeIconColorPressed`,v)],"--n-icon-color":n[y(`iconColor`,v)],"--n-border":n[y(`border`,v)],"--n-title-text-color":n[y(`titleTextColor`,v)],"--n-content-text-color":n[y(`contentTextColor`,v)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":s,"--n-icon-margin":l,"--n-icon-margin-rtl":d,"--n-close-size":m,"--n-close-margin":h,"--n-close-margin-rtl":g,"--n-padding":_,"--n-icon-margin-left":b,"--n-icon-margin-right":x}}),f=a?h(`alert`,r(()=>e.type[0]),d,e):void 0,p=n(!0),m=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:l,mergedClsPrefix:t,mergedBordered:i,visible:p,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(p.value=!1)})},handleAfterLeave:()=>{m()},mergedTheme:c,cssVars:a?void 0:d,themeClass:f?.themeClass,onRender:f?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),i(l,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:e,$slots:n}=this,r={class:[`${e}-alert`,this.themeClass,this.closable&&`${e}-alert--closable`,this.showIcon&&`${e}-alert--show-icon`,!this.title&&this.closable&&`${e}-alert--right-adjust`,this.rtlEnabled&&`${e}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?i(`div`,Object.assign({},t(this.$attrs,r)),this.closable&&i(a,{clsPrefix:e,class:`${e}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&i(`div`,{class:`${e}-alert__border`}),this.showIcon&&i(`div`,{class:`${e}-alert__icon`,"aria-hidden":`true`},s(n.icon,()=>[i(x,{clsPrefix:e},{default:()=>{switch(this.type){case`success`:return i(C,null);case`info`:return i(E,null);case`warning`:return i(O,null);case`error`:return i(w,null);default:return null}}})])),i(`div`,{class:[`${e}-alert-body`,this.mergedBordered&&`${e}-alert-body--bordered`]},c(n.header,t=>{let n=t||this.title;return n?i(`div`,{class:`${e}-alert-body__title`},n):null}),n.default&&i(`div`,{class:`${e}-alert-body__content`},n))):null}})}});export{M as t};