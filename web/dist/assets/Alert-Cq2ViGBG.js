import{M as e,O as t,T as n,g as r,st as i}from"./echarts-DxBJA66o.js";import{C as a,D as o,It as s,Lt as c,M as l,Nt as u,Ot as d,Pt as f,S as p,_t as m,dt as h,k as g,kt as _,ut as v,v as y,yt as b,zt as x}from"./auth-DEMSBBAb.js";import{A as S}from"./vue-core-RsSxNVS3.js";import{B as C,H as w,M as T,V as E,j as D,z as O}from"./index-lJkVLtdC.js";function k(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:i,dividerColor:a,actionColor:o,textColor1:s,textColor2:c,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,infoColor:h,successColor:g,warningColor:v,errorColor:y,fontSize:b}=e;return Object.assign(Object.assign({},T),{fontSize:b,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${a}`,color:o,titleTextColor:s,iconColor:c,contentTextColor:c,closeBorderRadius:n,closeColorHover:l,closeColorPressed:u,closeIconColor:f,closeIconColorHover:p,closeIconColorPressed:m,borderInfo:`1px solid ${_(i,d(h,{alpha:.25}))}`,colorInfo:_(i,d(h,{alpha:.08})),titleTextColorInfo:s,iconColorInfo:h,contentTextColorInfo:c,closeColorHoverInfo:l,closeColorPressedInfo:u,closeIconColorInfo:f,closeIconColorHoverInfo:p,closeIconColorPressedInfo:m,borderSuccess:`1px solid ${_(i,d(g,{alpha:.25}))}`,colorSuccess:_(i,d(g,{alpha:.08})),titleTextColorSuccess:s,iconColorSuccess:g,contentTextColorSuccess:c,closeColorHoverSuccess:l,closeColorPressedSuccess:u,closeIconColorSuccess:f,closeIconColorHoverSuccess:p,closeIconColorPressedSuccess:m,borderWarning:`1px solid ${_(i,d(v,{alpha:.33}))}`,colorWarning:_(i,d(v,{alpha:.08})),titleTextColorWarning:s,iconColorWarning:v,contentTextColorWarning:c,closeColorHoverWarning:l,closeColorPressedWarning:u,closeIconColorWarning:f,closeIconColorHoverWarning:p,closeIconColorPressedWarning:m,borderError:`1px solid ${_(i,d(y,{alpha:.25}))}`,colorError:_(i,d(y,{alpha:.08})),titleTextColorError:s,iconColorError:y,contentTextColorError:c,closeColorHoverError:l,closeColorPressedError:u,closeIconColorError:f,closeIconColorHoverError:p,closeIconColorPressedError:m})}var A={name:`Alert`,common:y,self:k},j=f(`alert`,`
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
 `),c(`closable`,[f(`alert-body`,[s(`title`,`
 padding-right: 24px;
 `)])]),s(`icon`,{color:`var(--n-icon-color)`}),f(`alert-body`,{padding:`var(--n-padding)`},[s(`title`,{color:`var(--n-title-text-color)`}),s(`content`,{color:`var(--n-content-text-color)`})]),D({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),s(`icon`,`
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
 `),c(`show-icon`,[f(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),c(`right-adjust`,[f(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),f(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[s(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[u(`& +`,[s(`content`,{marginTop:`9px`})])]),s(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),s(`icon`,{transition:`color .3s var(--n-bezier)`})]),M=n({name:`Alert`,inheritAttrs:!1,props:Object.assign(Object.assign({},g.props),{title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function}),slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:n,inlineThemeDisabled:a,mergedRtlRef:o}=h(e),s=g(`Alert`,`-alert`,j,A,e,t),c=l(`Alert`,o,t),u=r(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=s.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:o,iconSize:c,iconMargin:l,iconMarginRtl:u,closeIconSize:d,closeBorderRadius:f,closeSize:p,closeMargin:m,closeMarginRtl:h,padding:g}=n,{type:_}=e,{left:v,right:y}=S(l);return{"--n-bezier":t,"--n-color":n[x(`color`,_)],"--n-close-icon-size":d,"--n-close-border-radius":f,"--n-close-color-hover":n[x(`closeColorHover`,_)],"--n-close-color-pressed":n[x(`closeColorPressed`,_)],"--n-close-icon-color":n[x(`closeIconColor`,_)],"--n-close-icon-color-hover":n[x(`closeIconColorHover`,_)],"--n-close-icon-color-pressed":n[x(`closeIconColorPressed`,_)],"--n-icon-color":n[x(`iconColor`,_)],"--n-border":n[x(`border`,_)],"--n-title-text-color":n[x(`titleTextColor`,_)],"--n-content-text-color":n[x(`contentTextColor`,_)],"--n-line-height":o,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":c,"--n-icon-margin":l,"--n-icon-margin-rtl":u,"--n-close-size":p,"--n-close-margin":m,"--n-close-margin-rtl":h,"--n-padding":g,"--n-icon-margin-left":v,"--n-icon-margin-right":y}}),d=a?v(`alert`,r(()=>e.type[0]),u,e):void 0,f=i(!0),p=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:c,mergedClsPrefix:t,mergedBordered:n,visible:f,handleCloseClick:()=>{Promise.resolve(e.onClose?.call(e)).then(e=>{e!==!1&&(f.value=!1)})},handleAfterLeave:()=>{p()},mergedTheme:s,cssVars:a?void 0:u,themeClass:d?.themeClass,onRender:d?.onRender}},render(){var n;return(n=this.onRender)==null||n.call(this),t(p,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:n,$slots:r}=this,i={class:[`${n}-alert`,this.themeClass,this.closable&&`${n}-alert--closable`,this.showIcon&&`${n}-alert--show-icon`,!this.title&&this.closable&&`${n}-alert--right-adjust`,this.rtlEnabled&&`${n}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?t(`div`,Object.assign({},e(this.$attrs,i)),this.closable&&t(a,{clsPrefix:n,class:`${n}-alert__close`,onClick:this.handleCloseClick}),this.bordered&&t(`div`,{class:`${n}-alert__border`}),this.showIcon&&t(`div`,{class:`${n}-alert__icon`,"aria-hidden":`true`},m(r.icon,()=>[t(o,{clsPrefix:n},{default:()=>{switch(this.type){case`success`:return t(C,null);case`info`:return t(E,null);case`warning`:return t(O,null);case`error`:return t(w,null);default:return null}}})])),t(`div`,{class:[`${n}-alert-body`,this.mergedBordered&&`${n}-alert-body--bordered`]},b(r.header,e=>{let r=e||this.title;return r?t(`div`,{class:`${n}-alert-body__title`},r):null}),r.default&&t(`div`,{class:`${n}-alert-body__content`},r))):null}})}});export{M as t};