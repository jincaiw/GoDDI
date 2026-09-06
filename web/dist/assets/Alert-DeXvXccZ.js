import{F as e,O as t,j as n,ut as r,v as i}from"./echarts-eUEtiXc8.js";import{B as a,R as o,T as s,k as c,w as l}from"./auth-B7OufbA5.js";import{A as u}from"./vue-core-CcEAoDdX.js";import{B as d,G as f,J as p,K as m,N as h,P as g,V as _,Y as v,Z as y,i as b,n as x,t as S}from"./light-BKCENzy2.js";import{B as C,H as w,M as T,V as E,j as D,z as O}from"./index-CGmIOu7l.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:o,textColor1:s,textColor2:c,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,infoColor:h,successColor:g,warningColor:v,errorColor:y,fontSize:b}=e;return Object.assign(Object.assign({},T),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:o,titleTextColor:s,iconColor:c,contentTextColor:c,closeBorderRadius:n,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,borderInfo:`1px solid ${_(i,d(h,{alpha:.25}))}`,colorInfo:_(i,d(h,{alpha:.08})),titleTextColorInfo:s,iconColorInfo:h,contentTextColorInfo:c,closeColorHoverInfo:l,closeColorPressedInfo:u,closeIconColorInfo:f,closeIconColorHoverInfo:p,closeIconColorPressedInfo:m,borderSuccess:`1px solid ${_(i,d(g,{alpha:.25}))}`,colorSuccess:_(i,d(g,{alpha:.08})),titleTextColorSuccess:s,iconColorSuccess:g,contentTextColorSuccess:c,closeColorHoverSuccess:l,closeColorPressedSuccess:u,closeIconColorSuccess:f,closeIconColorHoverSuccess:p,closeIconColorPressedSuccess:m,borderWarning:`1px solid ${_(i,d(v,{alpha:.33}))}`,colorWarning:_(i,d(v,{alpha:.08})),titleTextColorWarning:s,iconColorWarning:v,contentTextColorWarning:c,closeColorHoverWarning:l,closeColorPressedWarning:u,closeIconColorWarning:f,closeIconColorHoverWarning:p,closeIconColorPressedWarning:m,borderError:`1px solid ${_(i,d(y,{alpha:.25}))}`,colorError:_(i,d(y,{alpha:.08})),titleTextColorError:s,iconColorError:y,contentTextColorError:c,closeColorHoverError:l,closeColorPressedError:u,closeIconColorError:f,closeIconColorHoverError:p,closeIconColorPressedError:m})}var A={name:`Alert`,common:S,self:k},j=m(`alert`,`
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
 `,[f(`& +`,[p(`content`,{marginTop:`9px`})])]),p(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),p(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=t({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},b.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:n,inlineThemeDisabled:a,mergedRtlRef:o}=g(e),s=b(`Alert`,`-alert`,j,A,e,t),l=c(`Alert`,o,t),d=i(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=s.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:c,iconMargin:l,iconMarginRtl:d,closeIconSize:f,closeBorderRadius:p,closeSize:m,closeMargin:h,closeMarginRtl:g,padding:_}=n,{type:v}=e,{left:b,right:x}=u(l);return{"--n-bezier":t,"--n-color":n[y(`color`,v)],"--n-close-icon-size":f,"--n-close-border-radius":p,"--n-close-color-hover":n[y(`closeColorHover`,v)],"--n-close-color-pressed":n[y(`closeColorPressed`,v)],"--n-close-icon-color":n[y(`closeIconColor`,v)],"--n-close-icon-color-hover":n[y(`closeIconColorHover`,v)],"--n-close-icon-color-pressed":n[y(`closeIconColorPressed`,v)],"--n-icon-color":n[y(`iconColor`,v)],"--n-border":n[y(`border`,v)],"--n-title-text-color":n[y(`titleTextColor`,v)],"--n-content-text-color":n[y(`contentTextColor`,v)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":c,"--n-icon-margin":l,"--n-icon-margin-rtl":d,"--n-close-size":m,"--n-close-margin":h,"--n-close-margin-rtl":g,"--n-padding":_,"--n-icon-margin-left":b,"--n-icon-margin-right":x}}),f=a?h(`alert`,i(()=>e.type[0]),d,e):void 0,p=r(!0),m=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:l,mergedClsPrefix:t,mergedBordered:n,visible:p,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(p.value=!1)})},handleAfterLeave:()=>{m()},mergedTheme:s,cssVars:a?void 0:d,themeClass:f?.themeClass,onRender:f?.onRender}},render(){var t;return(t=this.onRender)==null||t.call(this),n(l,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:t,$slots:r}=this,i={class:[`${t}-alert`,this.themeClass,this.closable&&`${t}-alert--closable`,this.showIcon&&`${t}-alert--show-icon`,!this.title&&this.closable&&`${t}-alert--right-adjust`,this.rtlEnabled&&`${t}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?n(`div`,Object.assign({},e(this.$attrs,i)),this.closable&&n(s,{clsPrefix:t,class:`${t}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&n(`div`,{class:`${t}-alert__border`}),this.showIcon&&n(`div`,{class:`${t}-alert__icon`,"aria-hidden":`true`},o(r.icon,()=>[n(x,{clsPrefix:t},{default:()=>{switch(this.type){case`success`:return n(C,null);case`info`:return n(E,null);case`warning`:return n(O,null);case`error`:return n(w,null);default:return null}}})])),n(`div`,{class:[`${t}-alert-body`,this.mergedBordered&&`${t}-alert-body--bordered`]},a(r.header,e=>{let r=e||this.title;return r?n(`div`,{class:`${t}-alert-body__title`},r):null}),r.default&&n(`div`,{class:`${t}-alert-body__content`},r))):null}})}});export{M as t};