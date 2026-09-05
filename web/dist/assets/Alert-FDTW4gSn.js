import{E as e,N as t,ct as n,g as r,k as i}from"./echarts-Cw2yHLaZ.js";import{A as a,At as o,Bt as s,C as c,Ft as l,Lt as u,N as d,O as f,Pt as p,Rt as m,bt as h,dt as g,ft as _,kt as v,vt as y,w as b,y as x}from"./auth-CYMAsBzV.js";import{A as S}from"./vue-core-BDTvi3xZ.js";import{B as C,H as w,M as T,V as E,j as D,z as O}from"./index-ChsYpqrV.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:s,textColor1:c,textColor2:l,closeColorHover:u,closeColorPressed:d,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,infoColor:h,successColor:g,warningColor:_,errorColor:y,fontSize:b}=e;return Object.assign(Object.assign({},T),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:s,titleTextColor:c,iconColor:l,contentTextColor:l,closeBorderRadius:n,closeColorHover:u,closeColorPressed:d,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,borderInfo:`1px solid ${o(i,v(h,{alpha:.25}))}`,colorInfo:o(i,v(h,{alpha:.08})),titleTextColorInfo:c,iconColorInfo:h,contentTextColorInfo:l,closeColorHoverInfo:u,closeColorPressedInfo:d,closeIconColorInfo:f,closeIconColorHoverInfo:p,closeIconColorPressedInfo:m,borderSuccess:`1px solid ${o(i,v(g,{alpha:.25}))}`,colorSuccess:o(i,v(g,{alpha:.08})),titleTextColorSuccess:c,iconColorSuccess:g,contentTextColorSuccess:l,closeColorHoverSuccess:u,closeColorPressedSuccess:d,closeIconColorSuccess:f,closeIconColorHoverSuccess:p,closeIconColorPressedSuccess:m,borderWarning:`1px solid ${o(i,v(_,{alpha:.33}))}`,colorWarning:o(i,v(_,{alpha:.08})),titleTextColorWarning:c,iconColorWarning:_,contentTextColorWarning:l,closeColorHoverWarning:u,closeColorPressedWarning:d,closeIconColorWarning:f,closeIconColorHoverWarning:p,closeIconColorPressedWarning:m,borderError:`1px solid ${o(i,v(y,{alpha:.25}))}`,colorError:o(i,v(y,{alpha:.08})),titleTextColorError:c,iconColorError:y,contentTextColorError:l,closeColorHoverError:u,closeColorPressedError:d,closeIconColorError:f,closeIconColorHoverError:p,closeIconColorPressedError:m})}var A={name:`Alert`,common:x,self:k},j=l(`alert`,`
 line-height: var(--n-line-height);
 border-radius: var(--n-border-radius);
 position: relative;
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 text-align: start;
 word-break: break-word;
`,[u(`border`,`
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 transition: border-color .3s var(--n-bezier);
 border: var(--n-border);
 pointer-events: none;
 `),m(`closable`,[l(`alert-body`,[u(`title`,`
 padding-right: 24px;
 `)])]),u(`icon`,{color:`var(--n-icon-color)`}),l(`alert-body`,{padding:`var(--n-padding)`},[u(`title`,{color:`var(--n-title-text-color)`}),u(`content`,{color:`var(--n-content-text-color)`})]),D({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),u(`icon`,`
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
 `),u(`close`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 position: absolute;
 right: 0;
 top: 0;
 margin: var(--n-close-margin);
 `),m(`show-icon`,[l(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),m(`right-adjust`,[l(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),l(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[u(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[p(`& +`,[u(`content`,{marginTop:`9px`})])]),u(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),u(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=e({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},a.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:i,inlineThemeDisabled:o,mergedRtlRef:c}=_(e),l=a(`Alert`,`-alert`,j,A,e,t),u=d(`Alert`,c,t),f=r(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=l.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:c,iconMargin:u,iconMarginRtl:d,closeIconSize:f,closeBorderRadius:p,closeSize:m,closeMargin:h,closeMarginRtl:g,padding:_}=n,{type:v}=e,{left:y,right:b}=S(u);return{"--n-bezier":t,"--n-color":n[s(`color`,v)],"--n-close-icon-size":f,"--n-close-border-radius":p,"--n-close-color-hover":n[s(`closeColorHover`,v)],"--n-close-color-pressed":n[s(`closeColorPressed`,v)],"--n-close-icon-color":n[s(`closeIconColor`,v)],"--n-close-icon-color-hover":n[s(`closeIconColorHover`,v)],"--n-close-icon-color-pressed":n[s(`closeIconColorPressed`,v)],"--n-icon-color":n[s(`iconColor`,v)],"--n-border":n[s(`border`,v)],"--n-title-text-color":n[s(`titleTextColor`,v)],"--n-content-text-color":n[s(`contentTextColor`,v)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":c,"--n-icon-margin":u,"--n-icon-margin-rtl":d,"--n-close-size":m,"--n-close-margin":h,"--n-close-margin-rtl":g,"--n-padding":_,"--n-icon-margin-left":y,"--n-icon-margin-right":b}}),p=o?g(`alert`,r(()=>e.type[0]),f,e):void 0,m=n(!0),h=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:u,mergedClsPrefix:t,mergedBordered:i,visible:m,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(m.value=!1)})},handleAfterLeave:()=>{h()},mergedTheme:l,cssVars:o?void 0:f,themeClass:p?.themeClass,onRender:p?.onRender}},render(){var e;return(e=this.onRender)==null||e.call(this),i(c,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:e,$slots:n}=this,r={class:[`${e}-alert`,this.themeClass,this.closable&&`${e}-alert--closable`,this.showIcon&&`${e}-alert--show-icon`,!this.title&&this.closable&&`${e}-alert--right-adjust`,this.rtlEnabled&&`${e}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?i(`div`,Object.assign({},t(this.$attrs,r)),this.closable&&i(b,{clsPrefix:e,class:`${e}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&i(`div`,{class:`${e}-alert__border`}),this.showIcon&&i(`div`,{class:`${e}-alert__icon`,"aria-hidden":`true`},y(n.icon,()=>[i(f,{clsPrefix:e},{default:()=>{switch(this.type){case`success`:return i(C,null);case`info`:return i(E,null);case`warning`:return i(O,null);case`error`:return i(w,null);default:return null}}})])),i(`div`,{class:[`${e}-alert-body`,this.mergedBordered&&`${e}-alert-body--bordered`]},h(n.header,t=>{let n=t||this.title;return n?i(`div`,{class:`${e}-alert-body__title`},n):null}),n.default&&i(`div`,{class:`${e}-alert-body__content`},n))):null}})}});export{M as t};