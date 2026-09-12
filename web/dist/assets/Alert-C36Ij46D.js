import{E as e,P as t,g as n,k as r,pt as i}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{$t as a,Bn as o,Br as s,Dr as c,Er as l,Fn as u,Jt as d,Mr as f,Pn as p,Rn as m,Rr as h,Ur as g,Vr as _,en as v,gn as y,in as b,mn as x,nn as S,on as C,rn as w,yn as T,zr as E}from"./router-BIZtGF2g.js";import{b as D,y as O}from"./index-DNIszxBR.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:o,textColor1:s,textColor2:u,closeColorHover:d,closeColorPressed:f,closeIconColor:p,closeIconColorHover:m,closeIconColorPressed:h,infoColor:g,successColor:_,warningColor:v,errorColor:y,fontSize:b}=e;return Object.assign(Object.assign({},D),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:o,titleTextColor:s,iconColor:u,contentTextColor:u,closeBorderRadius:n,closeColorHover:d,closeColorPressed:f,closeIconColor:p,closeIconColorHover:m,closeIconColorPressed:h,borderInfo:`1px solid ${c(i,l(g,{alpha:.25}))}`,colorInfo:c(i,l(g,{alpha:.08})),titleTextColorInfo:s,iconColorInfo:g,contentTextColorInfo:u,closeColorHoverInfo:d,closeColorPressedInfo:f,closeIconColorInfo:p,closeIconColorHoverInfo:m,closeIconColorPressedInfo:h,borderSuccess:`1px solid ${c(i,l(_,{alpha:.25}))}`,colorSuccess:c(i,l(_,{alpha:.08})),titleTextColorSuccess:s,iconColorSuccess:_,contentTextColorSuccess:u,closeColorHoverSuccess:d,closeColorPressedSuccess:f,closeIconColorSuccess:p,closeIconColorHoverSuccess:m,closeIconColorPressedSuccess:h,borderWarning:`1px solid ${c(i,l(v,{alpha:.33}))}`,colorWarning:c(i,l(v,{alpha:.08})),titleTextColorWarning:s,iconColorWarning:v,contentTextColorWarning:u,closeColorHoverWarning:d,closeColorPressedWarning:f,closeIconColorWarning:p,closeIconColorHoverWarning:m,closeIconColorPressedWarning:h,borderError:`1px solid ${c(i,l(y,{alpha:.25}))}`,colorError:c(i,l(y,{alpha:.08})),titleTextColorError:s,iconColorError:y,contentTextColorError:u,closeColorHoverError:d,closeColorPressedError:f,closeIconColorError:p,closeIconColorHoverError:m,closeIconColorPressedError:h})}var A={name:`Alert`,common:d,self:k},j=E(`alert`,`
 line-height: var(--n-line-height);
 border-radius: var(--n-border-radius);
 position: relative;
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 text-align: start;
 word-break: break-word;
`,[s(`border`,`
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 transition: border-color .3s var(--n-bezier);
 border: var(--n-border);
 pointer-events: none;
 `),_(`closable`,[E(`alert-body`,[s(`title`,`
 padding-right: 24px;
 `)])]),s(`icon`,{color:`var(--n-icon-color)`}),E(`alert-body`,{padding:`var(--n-padding)`},[s(`title`,{color:`var(--n-title-text-color)`}),s(`content`,{color:`var(--n-content-text-color)`})]),O({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),s(`icon`,`
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
 `),s(`close`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 position: absolute;
 right: 0;
 top: 0;
 margin: var(--n-close-margin);
 `),_(`show-icon`,[E(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),_(`right-adjust`,[E(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),E(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[s(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[h(`& +`,[s(`content`,{marginTop:`9px`})])]),s(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),s(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=e({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},y.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:r,inlineThemeDisabled:a,mergedRtlRef:o}=u(e),s=y(`Alert`,`-alert`,j,A,e,t),c=T(`Alert`,o,t),l=n(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=s.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:c,iconMargin:l,iconMarginRtl:u,closeIconSize:d,closeBorderRadius:p,closeSize:m,closeMargin:h,closeMarginRtl:_,padding:v}=n,{type:y}=e,{left:b,right:x}=f(l);return{"--n-bezier":t,"--n-color":n[g(`color`,y)],"--n-close-icon-size":d,"--n-close-border-radius":p,"--n-close-color-hover":n[g(`closeColorHover`,y)],"--n-close-color-pressed":n[g(`closeColorPressed`,y)],"--n-close-icon-color":n[g(`closeIconColor`,y)],"--n-close-icon-color-hover":n[g(`closeIconColorHover`,y)],"--n-close-icon-color-pressed":n[g(`closeIconColorPressed`,y)],"--n-icon-color":n[g(`iconColor`,y)],"--n-border":n[g(`border`,y)],"--n-title-text-color":n[g(`titleTextColor`,y)],"--n-content-text-color":n[g(`contentTextColor`,y)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":c,"--n-icon-margin":l,"--n-icon-margin-rtl":u,"--n-close-size":m,"--n-close-margin":h,"--n-close-margin-rtl":_,"--n-padding":v,"--n-icon-margin-left":b,"--n-icon-margin-right":x}}),d=a?p(`alert`,n(()=>e.type[0]),l,e):void 0,m=i(!0),h=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:c,mergedClsPrefix:t,mergedBordered:r,visible:m,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(m.value=!1)})},handleAfterLeave:()=>{h()},mergedTheme:s,cssVars:a?void 0:l,themeClass:d?.themeClass,onRender:d?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),r(a,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:e,$slots:n}=this,i={class:[`${e}-alert`,this.themeClass,this.closable&&`${e}-alert--closable`,this.showIcon&&`${e}-alert--show-icon`,!this.title&&this.closable&&`${e}-alert--right-adjust`,this.rtlEnabled&&`${e}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?r(`div`,Object.assign({},t(this.$attrs,i)),this.closable&&r(v,{clsPrefix:e,class:`${e}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&r(`div`,{class:`${e}-alert__border`}),this.showIcon&&r(`div`,{class:`${e}-alert__icon`,"aria-hidden":`true`},m(n.icon,()=>[r(x,{clsPrefix:e},{default:()=>{switch(this.type){case`success`:return r(w,null);case`info`:return r(b,null);case`warning`:return r(S,null);case`error`:return r(C,null);default:return null}}})])),r(`div`,{class:[`${e}-alert-body`,this.mergedBordered&&`${e}-alert-body--bordered`]},o(n.header,t=>{let n=t||this.title;return n?r(`div`,{class:`${e}-alert-body__title`},n):null}),n.default&&r(`div`,{class:`${e}-alert-body__content`},n))):null}})}});export{M as t};